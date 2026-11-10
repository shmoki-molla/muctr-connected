package main

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type Discipline struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (a *App) disciplines(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	rows, err := a.db.Query(ctx, "SELECT id::text,name,COALESCE(description,'') FROM disciplines ORDER BY name")
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	items := []Discipline{}
	for rows.Next() {
		var d Discipline
		if err = rows.Scan(&d.ID, &d.Name, &d.Description); err != nil {
			serverError(w, err)
			return
		}
		items = append(items, d)
	}
	if err = rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, items)
}
func (a *App) workTypes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	rows, err := a.db.Query(ctx, "SELECT id,name FROM work_types ORDER BY id")
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	type workType struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	items := []workType{}
	for rows.Next() {
		var t workType
		if err = rows.Scan(&t.ID, &t.Name); err != nil {
			serverError(w, err)
			return
		}
		items = append(items, t)
	}
	if err = rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, items)
}
func (a *App) addDiscipline(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.Join(strings.Fields(req.Name), " ")
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 255 || utf8.RuneCountInString(req.Description) > 2000 {
		fail(w, 400, "Название: до 255 символов, описание: до 2000")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	// Serialize normalized-name checks without changing existing legacy subject names.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(746632)"); err != nil {
		serverError(w, err)
		return
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM disciplines WHERE lower(trim(name))=lower($1))", req.Name).Scan(&exists); err != nil {
		serverError(w, err)
		return
	}
	if exists {
		fail(w, 409, "Такой предмет уже существует")
		return
	}
	d := Discipline{Name: req.Name, Description: req.Description}
	if err = tx.QueryRow(ctx, "INSERT INTO disciplines(name,description) VALUES($1,$2) RETURNING id::text", d.Name, d.Description).Scan(&d.ID); err != nil {
		serverError(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 201, d)
}
func (a *App) deleteDiscipline(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		fail(w, 400, "Некорректный ID")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result, err := a.db.Exec(ctx, "DELETE FROM disciplines WHERE id=$1", id.String())
	if constraintError(err, "23503") {
		fail(w, 409, "Нельзя удалить предмет, к которому привязаны работы")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		fail(w, 404, "Предмет не найден")
		return
	}
	w.WriteHeader(204)
}
func (a *App) students(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	rows, err := a.db.Query(ctx, `SELECT s.id::text,s.last_name||' '||s.first_name||COALESCE(' '||s.middle_name,''),COALESCE(s.email,''),
 s.enrollment_year,COUNT(pe.id) FROM students s LEFT JOIN portfolio_entries pe ON pe.student_id=s.id AND pe.storage_status='ready'
 GROUP BY s.id ORDER BY s.last_name,s.first_name`)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	type student struct {
		ID             string `json:"id"`
		FullName       string `json:"fullName"`
		Email          string `json:"email"`
		EnrollmentYear int    `json:"enrollmentYear"`
		FileCount      int    `json:"fileCount"`
	}
	items := []student{}
	for rows.Next() {
		var s student
		if err = rows.Scan(&s.ID, &s.FullName, &s.Email, &s.EnrollmentYear, &s.FileCount); err != nil {
			serverError(w, err)
			return
		}
		items = append(items, s)
	}
	if err = rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, items)
}
func missing(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
