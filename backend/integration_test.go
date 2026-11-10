package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestSecurityAndRateLimit(t *testing.T) {
	app := &App{origins: map[string]bool{"http://localhost:5173": true}}
	handler := app.security(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, test := range []struct {
		origin, header string
		status         int
	}{
		{"http://evil.example", "1", 403}, {"http://localhost:5173", "", 403}, {"http://localhost:5173", "1", 204}, {"", "1", 204},
	} {
		req := httptest.NewRequest("POST", "/api/auth/login", nil)
		req.Header.Set("Origin", test.origin)
		req.Header.Set("X-MUCTR-Request", test.header)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != test.status {
			t.Fatalf("origin %s: %d", test.origin, w.Code)
		}
	}
	limiter := newAttemptLimiter()
	for i := 0; i < 30; i++ {
		if !limiter.allow("client") {
			t.Fatal("limited too early")
		}
	}
	if limiter.allow("client") {
		t.Fatal("31st attempt allowed")
	}
	if !limiter.allow("another-client") {
		t.Fatal("unrelated client blocked")
	}
}
func TestTrustedProxy(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Real-IP", "203.0.113.10")
	if app.clientIP(req) != "127.0.0.1" {
		t.Fatal("trusted unconfigured forwarding header")
	}
	app.trustedProxyHost = "127.0.0.1"
	if app.clientIP(req) != "203.0.113.10" {
		t.Fatal("proxy client IP lost")
	}
	req.RemoteAddr = "192.0.2.1:1234"
	if app.clientIP(req) != "192.0.2.1" {
		t.Fatal("trusted header from other peer")
	}
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Real-IP", "invalid")
	if app.clientIP(req) != "127.0.0.1" {
		t.Fatal("invalid forwarding header accepted")
	}
}
func TestAuthAndPortfolioIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("run with docker compose -f compose.test.yml run --rm tools")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "portfolio_test" {
		t.Fatal("integration tests require dedicated portfolio_test database")
	}
	base, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	if _, err = base.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS "uuid-ossp"; CREATE EXTENSION IF NOT EXISTS pgcrypto;`); err != nil {
		t.Fatal(err)
	}
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer base.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, file := range []string{"init-db.sql", "02-auth.sql", "04-portfolio-upload.sql", "03-dev-auth-seed.sql", "02-auth.sql", "04-portfolio-upload.sql", "03-dev-auth-seed.sql"} {
		data, e := os.ReadFile("../database/" + file)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(ctx, string(data)); e != nil {
			t.Fatalf("%s: %v", file, e)
		}
	}
	s3, err := minio.New(os.Getenv("TEST_S3_ENDPOINT"), &minio.Options{Creds: credentials.NewStaticV4("test-minio", "test-minio-password", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	bucket := "test-" + uuid.NewString()
	for i := 0; i < 30; i++ {
		err = s3.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for object := range s3.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			_ = s3.RemoveObject(ctx, bucket, object.Key, minio.RemoveObjectOptions{})
		}
		_ = s3.RemoveBucket(ctx, bucket)
	}()
	if _, err = db.Exec(ctx, "UPDATE portfolio_entries SET s3_bucket_name=$1", bucket); err != nil {
		t.Fatal(err)
	}
	app := &App{db: db, storage: s3, bucket: bucket, origins: map[string]bool{"http://localhost:5173": true}, limiter: newAttemptLimiter()}
	app.reconcileStorage(ctx)
	server := httptest.NewServer(app.routes())
	defer server.Close()
	client := func() *http.Client { jar, _ := cookiejar.New(nil); return &http.Client{Jar: jar} }
	anonymous, a, b, admin, seeded := client(), client(), client(), client(), client()
	request := func(c *http.Client, method, path string, body any, want int) []byte {
		t.Helper()
		var reader io.Reader
		if body != nil {
			data, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			reader = bytes.NewReader(data)
		}
		req, e := http.NewRequest(method, server.URL+path, reader)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-MUCTR-Request", "1")
		req.Header.Set("Origin", "http://localhost:5173")
		response, e := c.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		if response.StatusCode != want {
			t.Fatalf("%s %s: expected %d got %d: %s", method, path, want, response.StatusCode, data)
		}
		return data
	}
	register := func(c *http.Client, email string) User {
		t.Helper()
		data := request(c, "POST", "/api/auth/register", map[string]any{"email": email, "password": "Password123!dev", "firstName": "Тест", "lastName": "Студент", "enrollmentYear": 2024}, 201)
		var user User
		if err := json.Unmarshal(data, &user); err != nil {
			t.Fatal(err)
		}
		return user
	}
	request(anonymous, "GET", "/api/health", nil, 200)
	request(anonymous, "GET", "/api/drive", nil, 401)
	request(anonymous, "POST", "/api/auth/register", map[string]any{"role": "admin"}, 400)
	ua := register(a, "alice@example.test")
	ub := register(b, "bob@example.test")
	if ua.Role != "student" || ua.StudentID == nil {
		t.Fatal("registration did not create student")
	}
	request(anonymous, "POST", "/api/auth/register", map[string]any{"email": "ALICE@example.test", "password": "Password123!dev", "firstName": "A", "lastName": "B", "enrollmentYear": 2024}, 409)
	var hash string
	if err = db.QueryRow(ctx, "SELECT password_hash FROM auth_users WHERE id=$1", ua.ID).Scan(&hash); err != nil || hash == "Password123!dev" || !strings.HasPrefix(hash, "$2") {
		t.Fatalf("password storage: %v", err)
	}
	apiURL, _ := url.Parse(server.URL)
	savedCookie := a.Jar.Cookies(apiURL)[0]
	sum := sha256.Sum256([]byte(savedCookie.Value))
	var n int
	if err = db.QueryRow(ctx, "SELECT COUNT(*) FROM auth_sessions WHERE token_hash=$1", sum[:]).Scan(&n); err != nil || n != 1 {
		t.Fatal("session token not hashed in DB")
	}
	request(anonymous, "POST", "/api/auth/login", map[string]string{"email": "alice@example.test", "password": "wrong"}, 401)
	request(admin, "POST", "/api/auth/login", map[string]string{"email": "admin@muctr.local", "password": "Admin123!dev"}, 200)
	request(seeded, "POST", "/api/auth/login", map[string]string{"email": "i.petrov@university.local", "password": "Student123!dev"}, 200)
	request(a, "GET", "/api/students", nil, 403)
	request(admin, "GET", "/api/students", nil, 200)
	request(a, "POST", "/api/disciplines", map[string]string{"name": "Химия"}, 403)
	var subject Discipline
	if err = json.Unmarshal(request(admin, "POST", "/api/disciplines", map[string]string{"name": "Тестовый предмет", "description": "Для теста"}, 201), &subject); err != nil {
		t.Fatal(err)
	}
	request(admin, "POST", "/api/disciplines", map[string]string{"name": "тестовый предмет"}, 409)
	list := func(c *http.Client, path string) ([]DriveFile, int) {
		t.Helper()
		var result struct {
			Files []DriveFile `json:"files"`
			Total int         `json:"total"`
		}
		if err := json.Unmarshal(request(c, "GET", path, nil, 200), &result); err != nil {
			t.Fatal(err)
		}
		return result.Files, result.Total
	}
	if _, total := list(admin, "/api/drive"); total != 0 {
		t.Fatal("missing demo objects leaked into listing")
	}
	pdf := []byte("%PDF-1.4\n1 0 obj<</Type /Catalog>>endobj\n%%EOF")
	upload := func(c *http.Client, filename string, content []byte, extra map[string]string, want int) string {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		fields := map[string]string{"disciplineId": subject.ID, "workTypeId": "3", "semester": "2", "academicYearStart": "2024", "title": "Лабораторная проверка"}
		for k, v := range extra {
			fields[k] = v
		}
		for k, v := range fields {
			_ = writer.WriteField(k, v)
		}
		part, e := writer.CreateFormFile("file", filename)
		if e != nil {
			t.Fatal(e)
		}
		_, _ = part.Write(content)
		_ = writer.Close()
		req, e := http.NewRequest("POST", server.URL+"/api/files", &body)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("X-MUCTR-Request", "1")
		response, e := c.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		if response.StatusCode != want {
			t.Fatalf("upload expected %d got %d: %s", want, response.StatusCode, data)
		}
		var result map[string]string
		_ = json.Unmarshal(data, &result)
		return result["id"]
	}
	upload(a, "report.pdf", pdf, map[string]string{"studentId": *ub.StudentID}, 400)
	upload(a, "report.pdf", pdf, map[string]string{"semester": "3"}, 400)
	upload(a, "report.pdf", pdf, map[string]string{"disciplineId": uuid.NewString()}, 400)
	upload(a, "report.pdf", []byte("<html>not a PDF</html>"), nil, 415)
	upload(a, "script.exe", []byte("MZbad"), nil, 415)
	upload(a, "large.pdf", bytes.Repeat([]byte("a"), int(maxUploadBytes)+1), nil, 413)
	textContent := []byte(strings.Repeat("a", 511) + "Я — конспект")
	textID := upload(a, "Конспект.txt", textContent, nil, 201)
	if got := request(a, "GET", "/api/files/"+textID+"/download", nil, 200); !bytes.Equal(got, textContent) {
		t.Fatal("UTF-8 text download changed bytes")
	}
	request(a, "DELETE", "/api/files/"+textID, nil, 204)
	binaryText := append([]byte(strings.Repeat("a", 600)), byte(0))
	upload(a, "bad.txt", binaryText, nil, 415)
	id := upload(a, "Лабораторная.pdf", pdf, nil, 201)
	if files, total := list(a, "/api/drive"); total != 1 || files[0].Semester != 2 || files[0].WorkTypeID != 3 || files[0].AcademicYearStart != 2024 {
		t.Fatal("own list/metadata incorrect")
	}
	if _, total := list(b, "/api/drive"); total != 0 {
		t.Fatal("other student's work leaked")
	}
	if _, total := list(admin, "/api/drive?student="+*ua.StudentID); total != 1 {
		t.Fatal("admin filter failed")
	}
	if _, total := list(a, "/api/drive?semester=1"); total != 0 {
		t.Fatal("semester filter failed")
	}
	request(b, "GET", "/api/drive?student="+*ua.StudentID, nil, 403)
	request(b, "GET", "/api/files/"+id+"/download", nil, 404)
	request(b, "DELETE", "/api/files/"+id, nil, 404)
	if got := request(a, "GET", "/api/files/"+id+"/download", nil, 200); !bytes.Equal(got, pdf) {
		t.Fatal("download changed file bytes")
	}
	request(admin, "GET", "/api/files/"+id+"/download", nil, 200)
	request(admin, "DELETE", "/api/disciplines/"+subject.ID, nil, 409)
	upload(b, "other.pdf", pdf, nil, 201)
	if _, total := list(admin, "/api/drive"); total != 2 {
		t.Fatal("admin cannot see all works")
	}
	// Refresh/persistence: a new HTTP handler backed by the same DB and storage
	// still recognizes the cookie and serves the object.
	restart := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/files/"+id+"/download", nil)
	req.AddCookie(savedCookie)
	app.routes().ServeHTTP(restart, req)
	if restart.Code != 200 || !bytes.Equal(restart.Body.Bytes(), pdf) {
		t.Fatal("session/file did not survive handler restart")
	}
	var objectKey string
	if err = db.QueryRow(ctx, "SELECT s3_object_key FROM portfolio_entries WHERE id=$1", id).Scan(&objectKey); err != nil {
		t.Fatal(err)
	}
	request(a, "DELETE", "/api/files/"+id, nil, 204)
	if _, err = s3.StatObject(ctx, bucket, objectKey, minio.StatObjectOptions{}); minio.ToErrorResponse(err).Code != "NoSuchKey" {
		t.Fatal("deleted object remains in MinIO")
	}
	if err = db.QueryRow(ctx, "SELECT COUNT(*) FROM portfolio_entries WHERE id=$1", id).Scan(&n); err != nil || n != 0 {
		t.Fatal("deleted row remains")
	}
	request(a, "POST", "/api/auth/logout", nil, 204)
	req = httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(savedCookie)
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatal("logged out cookie still valid")
	}
	if _, err = db.Exec(ctx, "UPDATE auth_sessions SET created_at=NOW()-INTERVAL '2 days',expires_at=NOW()-INTERVAL '1 day' WHERE user_id=$1", ub.ID); err != nil {
		t.Fatal(err)
	}
	request(b, "GET", "/api/me", nil, 401)
	if _, err = db.Exec(ctx, "UPDATE auth_users SET is_active=false WHERE email='i.petrov@university.local'"); err != nil {
		t.Fatal(err)
	}
	request(seeded, "GET", "/api/me", nil, 401)
	request(seeded, "POST", "/api/auth/login", map[string]string{"email": "i.petrov@university.local", "password": "Student123!dev"}, 401)
	request(anonymous, "POST", "/api/auth/login", map[string]string{"email": "ALICE@EXAMPLE.TEST", "password": "Password123!dev"}, 200)
	// Cookie attributes are checked on the real HTTP response.
	rawReq, _ := http.NewRequest("POST", server.URL+"/api/auth/login", strings.NewReader(`{"email":"alice@example.test","password":"Password123!dev"}`))
	rawReq.Header.Set("Content-Type", "application/json")
	rawReq.Header.Set("X-MUCTR-Request", "1")
	response, err := anonymous.Do(rawReq)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/" {
		t.Fatal("unsafe session cookie")
	}
	t.Log("registration, seeded login, admin role, sessions, CSRF, uploads, filters, file isolation, byte-identical download and deletion passed")
	// Ensure the migrations did not require a fabricated history record.
	if err = db.QueryRow(ctx, "SELECT COUNT(*) FROM student_group_history WHERE student_id=$1", *ua.StudentID).Scan(&n); err != nil || n != 0 {
		t.Fatal("registration fabricated group history")
	}
}
