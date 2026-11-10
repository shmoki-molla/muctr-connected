package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const maxUploadBytes int64 = 25 << 20
const sessionCookie = "muctr_session"

type App struct {
	db               *pgxpool.Pool
	storage          *minio.Client
	bucket           string
	trustedProxyHost string
	secureCookie     bool
	origins          map[string]bool
	limiter          *attemptLimiter
}
type userKey struct{}
type User struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	Role      string  `json:"role"`
	StudentID *string `json:"studentId"`
	FullName  string  `json:"fullName"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	s3, err := minio.New(env("S3_ENDPOINT", "localhost:9000"), &minio.Options{
		Creds:  credentials.NewStaticV4(env("S3_ACCESS_KEY", "minioadmin"), env("S3_SECRET_KEY", "minioadmin123"), ""),
		Secure: env("S3_SECURE", "false") == "true",
	})
	if err != nil {
		log.Fatal(err)
	}
	origins := map[string]bool{}
	for _, origin := range strings.Split(env("APP_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173"), ",") {
		origins[strings.TrimSpace(origin)] = true
	}
	app := &App{db: pool, storage: s3, bucket: env("S3_BUCKET", "uni-portfolio-bucket"),
		trustedProxyHost: os.Getenv("TRUSTED_PROXY_HOST"),
		secureCookie:     env("COOKIE_SECURE", "false") == "true", origins: origins, limiter: newAttemptLimiter()}
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	if err := pool.Ping(startup); err != nil {
		log.Fatal(err)
	}
	if exists, err := s3.BucketExists(startup, app.bucket); err != nil || !exists {
		log.Fatalf("S3 bucket unavailable: %v", err)
	}
	app.reconcileStorage(startup)
	cancel()
	go app.maintenance(ctx)
	server := &http.Server{Addr: ":" + env("PORT", "8080"), Handler: app.routes(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 3 * time.Minute, WriteTimeout: 3 * time.Minute, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("MUCTR API listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.Handle("GET /api/me", a.requireUser("", http.HandlerFunc(a.me)))
	mux.Handle("GET /api/drive", a.requireUser("", http.HandlerFunc(a.drive)))
	mux.Handle("GET /api/disciplines", a.requireUser("", http.HandlerFunc(a.disciplines)))
	mux.Handle("GET /api/work-types", a.requireUser("", http.HandlerFunc(a.workTypes)))
	mux.Handle("POST /api/disciplines", a.requireUser("admin", http.HandlerFunc(a.addDiscipline)))
	mux.Handle("DELETE /api/disciplines/{id}", a.requireUser("admin", http.HandlerFunc(a.deleteDiscipline)))
	mux.Handle("GET /api/students", a.requireUser("admin", http.HandlerFunc(a.students)))
	mux.Handle("POST /api/files", a.requireUser("student", http.HandlerFunc(a.upload)))
	mux.Handle("GET /api/files/{id}/download", a.requireUser("", http.HandlerFunc(a.download)))
	mux.Handle("DELETE /api/files/{id}", a.requireUser("", http.HandlerFunc(a.deleteFile)))
	return a.security(mux)
}

func (a *App) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !a.origins[origin] {
				fail(w, 403, "Источник запроса не разрешён")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-MUCTR-Request")
			w.WriteHeader(204)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			// Non-simple header + exact Origin allowlist blocks cross-site form/fetch requests.
			if r.Header.Get("X-MUCTR-Request") != "1" {
				fail(w, 403, "Отсутствует защитный заголовок запроса")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := a.db.Ping(ctx); err != nil {
		fail(w, 503, "База данных недоступна")
		return
	}
	exists, err := a.storage.BucketExists(ctx, a.bucket)
	if err != nil || !exists {
		fail(w, 503, "Хранилище файлов недоступно")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func currentUser(r *http.Request) User { return r.Context().Value(userKey{}).(User) }
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("response: %v", err)
	}
}
func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func serverError(w http.ResponseWriter, err error) {
	log.Printf("request error: %v", err)
	fail(w, 503, "Не удалось выполнить запрос. Попробуйте ещё раз.")
}
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		fail(w, 415, "Ожидается JSON")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		fail(w, 400, "Некорректные поля запроса")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		fail(w, 400, "Ожидается один JSON-объект")
		return false
	}
	return true
}
func constraintError(err error, code string) bool {
	var e *pgconn.PgError
	return errors.As(err, &e) && e.Code == code
}
func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

type attemptWindow struct {
	count int
	until time.Time
}
type attemptLimiter struct {
	mu      sync.Mutex
	entries map[string]attemptWindow
}

func newAttemptLimiter() *attemptLimiter {
	return &attemptLimiter{entries: make(map[string]attemptWindow)}
}
func (l *attemptLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for k, v := range l.entries {
		if now.After(v.until) {
			delete(l.entries, k)
		}
	}
	v := l.entries[key]
	if v.count == 0 {
		v.until = now.Add(15 * time.Minute)
	}
	if v.count >= 30 || len(l.entries) >= 10000 {
		return false
	}
	v.count++
	l.entries[key] = v
	return true
}
func (a *App) clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	// Trust X-Real-IP only from the configured reverse proxy.
	// Compose may recreate frontend with a different IP.
	if a.trustedProxyHost != "" {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, a.trustedProxyHost)
		if err == nil {
			for _, address := range addresses {
				if address.IP.Equal(net.ParseIP(ip)) {
					if client := net.ParseIP(r.Header.Get("X-Real-IP")); client != nil {
						return client.String()
					}
				}
			}
		}
	}
	return ip
}
func (a *App) allowAuth(w http.ResponseWriter, r *http.Request) bool {
	if !a.limiter.allow(a.clientIP(r)) {
		w.Header().Set("Retry-After", "900")
		fail(w, 429, "Слишком много попыток. Повторите через 15 минут.")
		return false
	}
	return true
}
