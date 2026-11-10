package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type registerRequest struct {
	FirstName      string `json:"firstName"`
	LastName       string `json:"lastName"`
	MiddleName     string `json:"middleName"`
	Email          string `json:"email"`
	EnrollmentYear int    `json:"enrollmentYear"`
	Password       string `json:"password"`
}

func validEmail(email string) bool {
	parsed, err := mail.ParseAddress(email)
	return err == nil && parsed.Address == email && len(email) <= 255
}
func (a *App) register(w http.ResponseWriter, r *http.Request) {
	if !a.allowAuth(w, r) {
		return
	}
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.MiddleName = strings.TrimSpace(req.MiddleName)
	if !validEmail(req.Email) || req.FirstName == "" || req.LastName == "" || utf8.RuneCountInString(req.FirstName) > 100 || utf8.RuneCountInString(req.LastName) > 100 || utf8.RuneCountInString(req.MiddleName) > 100 {
		fail(w, 400, "Укажите имя, фамилию и корректный email")
		return
	}
	if req.EnrollmentYear < 2000 || req.EnrollmentYear > time.Now().Year()+1 {
		fail(w, 400, "Проверьте год поступления")
		return
	}
	if utf8.RuneCountInString(req.Password) < 10 || len(req.Password) > 72 || strings.ContainsRune(req.Password, 0) {
		fail(w, 400, "Пароль: от 10 символов и не более 72 байт")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		serverError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	var sid string
	err = tx.QueryRow(ctx, `INSERT INTO students(first_name,last_name,middle_name,email,enrollment_year) VALUES($1,$2,NULLIF($3,''),$4,$5) RETURNING id::text`,
		req.FirstName, req.LastName, req.MiddleName, req.Email, req.EnrollmentYear).Scan(&sid)
	if err != nil {
		authInsertError(w, err)
		return
	}
	user := User{Email: req.Email, Role: "student", StudentID: &sid, FullName: strings.TrimSpace(req.LastName + " " + req.FirstName + " " + req.MiddleName)}
	err = tx.QueryRow(ctx, `INSERT INTO auth_users(email,password_hash,role,student_id) VALUES($1,$2,'student',$3) RETURNING id::text`, req.Email, string(hash), sid).Scan(&user.ID)
	if err != nil {
		authInsertError(w, err)
		return
	}
	token, expires, err := newSession(ctx, tx, user.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		serverError(w, err)
		return
	}
	a.setSessionCookie(w, token, expires)
	writeJSON(w, 201, user)
}
func authInsertError(w http.ResponseWriter, err error) {
	if constraintError(err, "23505") {
		fail(w, 409, "Этот email уже зарегистрирован")
		return
	}
	serverError(w, err)
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if !a.allowAuth(w, r) {
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(req.Email) || len(req.Password) > 72 {
		fail(w, 401, "Неверный email или пароль")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var user User
	var hash string
	var active bool
	err := a.db.QueryRow(ctx, `SELECT u.id::text,u.email,u.role,u.student_id::text,
 COALESCE(s.last_name||' '||s.first_name||COALESCE(' '||s.middle_name,''),'Администратор'),
 u.password_hash,u.is_active FROM auth_users u LEFT JOIN students s ON s.id=u.student_id WHERE lower(u.email)=$1`, req.Email).
		Scan(&user.ID, &user.Email, &user.Role, &user.StudentID, &user.FullName, &hash, &active)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		serverError(w, err)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// Valid cost-12 hash to avoid skipping expensive comparison for unknown users.
		hash = "$2a$12$R9h/cIPz0gi.URNNX3kh2OPST9/PgBkqquzi.Ss7KIUgO2t0jWMUW"
	}
	valid := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) == nil
	if err != nil || !active || !valid {
		fail(w, 401, "Неверный email или пароль")
		return
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	// Rotate any session already present in this browser on a fresh login.
	if cookie, e := r.Cookie(sessionCookie); e == nil {
		sum := sha256.Sum256([]byte(cookie.Value))
		if _, e = tx.Exec(ctx, "DELETE FROM auth_sessions WHERE token_hash=$1", sum[:]); e != nil {
			serverError(w, e)
			return
		}
	}
	token, expires, err := newSession(ctx, tx, user.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		serverError(w, err)
		return
	}
	a.setSessionCookie(w, token, expires)
	writeJSON(w, 200, user)
}
func newSession(ctx context.Context, tx pgx.Tx, userID string) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	expires := time.Now().Add(7 * 24 * time.Hour)
	_, err := tx.Exec(ctx, "INSERT INTO auth_sessions(user_id,token_hash,expires_at) VALUES($1,$2,$3)", userID, hash[:], expires)
	return token, expires, err
}
func (a *App) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: a.secureCookie,
		SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
}
func (a *App) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: a.secureCookie,
		SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(0, 0)})
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		sum := sha256.Sum256([]byte(cookie.Value))
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if _, err = a.db.Exec(ctx, "DELETE FROM auth_sessions WHERE token_hash=$1", sum[:]); err != nil {
			serverError(w, err)
			return
		}
	}
	a.clearSessionCookie(w)
	w.WriteHeader(204)
}
func (a *App) requireUser(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || len(cookie.Value) != 64 {
			fail(w, 401, "Войдите в аккаунт")
			return
		}
		sum := sha256.Sum256([]byte(cookie.Value))
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		var user User
		err = a.db.QueryRow(ctx, `SELECT u.id::text,u.email,u.role,u.student_id::text,
 COALESCE(s.last_name||' '||s.first_name||COALESCE(' '||s.middle_name,''),'Администратор')
 FROM auth_sessions t JOIN auth_users u ON u.id=t.user_id LEFT JOIN students s ON s.id=u.student_id
 WHERE t.token_hash=$1 AND t.expires_at>NOW() AND t.revoked_at IS NULL AND u.is_active`, sum[:]).
			Scan(&user.ID, &user.Email, &user.Role, &user.StudentID, &user.FullName)
		cancel()
		if errors.Is(err, pgx.ErrNoRows) {
			a.clearSessionCookie(w)
			fail(w, 401, "Сессия завершена. Войдите снова.")
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		if role != "" && user.Role != role {
			fail(w, 403, "Недостаточно прав")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
	})
}
func (a *App) me(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, currentUser(r)) }
