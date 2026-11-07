package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	db *pgxpool.Pool
}

type DriveFolder struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type DriveFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Owner    string `json:"owner"`
	Modified string `json:"modified"`
	Size     string `json:"size"`
	Kind     string `json:"kind"`
	FolderID string `json:"folderId"`
}

type DriveResponse struct {
	Folders           []DriveFolder `json:"folders"`
	Files             []DriveFile   `json:"files"`
	Source            string        `json:"source"`
	DatabaseConnected bool          `json:"databaseConnected"`
	Message           string        `json:"message,omitempty"`
}

type Student struct {
	ID             string `json:"id"`
	FullName       string `json:"fullName"`
	Email          string `json:"email"`
	EnrollmentYear int    `json:"enrollmentYear"`
}

type Discipline struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Faculty     string `json:"faculty"`
	Description string `json:"description"`
}

type CreateFileRequest struct {
	StudentID    string `json:"studentId"`
	DisciplineID string `json:"disciplineId"`
	WorkTypeID   int    `json:"workTypeId"`
	Semester     int    `json:"semester"`
	Filename     string `json:"filename"`
	FileSize     int64  `json:"fileSize"`
	MimeType     string `json:"mimeType"`
	Grade        string `json:"grade"`
	Comment      string `json:"comment"`
}

func main() {
	ctx := context.Background()
	app := &App{}

	if dsn := strings.TrimSpace(os.Getenv("DATABASE_URL")); dsn != "" {
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			log.Printf("database pool init failed: %v", err)
		} else {
			app.db = pool
			defer pool.Close()
		}
	} else {
		log.Printf("DATABASE_URL is empty; API will return demo fallback data")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", app.health)
	mux.HandleFunc("GET /api/drive", app.drive)
	mux.HandleFunc("GET /api/students", app.students)
	mux.HandleFunc("GET /api/disciplines", app.disciplines)
	mux.HandleFunc("POST /api/files", app.createFile)

	staticDir := strings.TrimSpace(os.Getenv("STATIC_DIR"))
	if staticDir != "" {
		mux.HandleFunc("/", spaHandler(staticDir))
	}

	handler := cors(mux)
	port := env("PORT", "8080")
	log.Printf("MUCTR Drive API started on :%s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if a.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok":       false,
			"database": "not_configured",
			"message":  "DATABASE_URL не задан. API работает, но БД не подключена.",
		})
		return
	}

	if err := a.db.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok":       false,
			"database": "error",
			"message":  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"database": "connected",
	})
}

func (a *App) drive(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	if a.db == nil {
		writeJSON(w, http.StatusOK, demoDrive("База не настроена, показаны демо-данные из Go API."))
		return
	}

	if err := a.db.Ping(ctx); err != nil {
		resp := demoDrive("PostgreSQL пока недоступен, показаны демо-данные. Ошибка: " + err.Error())
		writeJSON(w, http.StatusOK, resp)
		return
	}

	folders, err := a.loadFolders(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, demoDrive("Не удалось прочитать папки из БД: "+err.Error()))
		return
	}

	files, err := a.loadFiles(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, demoDrive("Не удалось прочитать файлы из БД: "+err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, DriveResponse{
		Folders:           folders,
		Files:             files,
		Source:            "postgres",
		DatabaseConnected: true,
	})
}

func (a *App) loadFolders(ctx context.Context) ([]DriveFolder, error) {
	rows, err := a.db.Query(ctx, `
        SELECT d.id::text, d.name, COUNT(pe.id)::int AS file_count
        FROM disciplines d
        LEFT JOIN portfolio_entries pe ON pe.discipline_id = d.id
        GROUP BY d.id, d.name
        ORDER BY d.name
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []DriveFolder
	for rows.Next() {
		var f DriveFolder
		if err := rows.Scan(&f.ID, &f.Name, &f.Count); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

func (a *App) loadFiles(ctx context.Context) ([]DriveFile, error) {
	rows, err := a.db.Query(ctx, `
        SELECT
            pe.id::text,
            d.id::text AS folder_id,
            pe.original_filename,
            s.last_name || ' ' || s.first_name || COALESCE(' ' || s.middle_name, '') AS owner,
            pe.upload_date,
            COALESCE(pe.file_size, 0),
            COALESCE(pe.mime_type, '')
        FROM portfolio_entries pe
        JOIN students s ON s.id = pe.student_id
        JOIN disciplines d ON d.id = pe.discipline_id
        ORDER BY pe.upload_date DESC, pe.original_filename
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []DriveFile
	for rows.Next() {
		var id, folderID, name, owner, mimeType string
		var uploaded time.Time
		var size int64
		if err := rows.Scan(&id, &folderID, &name, &owner, &uploaded, &size, &mimeType); err != nil {
			return nil, err
		}
		files = append(files, DriveFile{
			ID:       id,
			FolderID: folderID,
			Name:     name,
			Owner:    owner,
			Modified: formatDateRU(uploaded),
			Size:     formatBytes(size),
			Kind:     fileKind(name, mimeType),
		})
	}
	return files, rows.Err()
}

func (a *App) students(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	if a.db == nil {
		writeJSON(w, http.StatusOK, []Student{})
		return
	}

	rows, err := a.db.Query(ctx, `
        SELECT id::text, last_name || ' ' || first_name || COALESCE(' ' || middle_name, ''), COALESCE(email, ''), enrollment_year::int
        FROM students
        ORDER BY last_name, first_name
    `)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	var students []Student
	for rows.Next() {
		var s Student
		if err := rows.Scan(&s.ID, &s.FullName, &s.Email, &s.EnrollmentYear); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		students = append(students, s)
	}
	writeJSON(w, http.StatusOK, students)
}

func (a *App) disciplines(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	if a.db == nil {
		writeJSON(w, http.StatusOK, []Discipline{})
		return
	}

	rows, err := a.db.Query(ctx, `
        SELECT d.id::text, d.name, COALESCE(f.short_name, ''), COALESCE(d.description, '')
        FROM disciplines d
        LEFT JOIN faculties f ON f.id = d.faculty_id
        ORDER BY d.name
    `)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	var disciplines []Discipline
	for rows.Next() {
		var d Discipline
		if err := rows.Scan(&d.ID, &d.Name, &d.Faculty, &d.Description); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		disciplines = append(disciplines, d)
	}
	writeJSON(w, http.StatusOK, disciplines)
}

func (a *App) createFile(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("DATABASE_URL не задан"))
		return
	}

	var req CreateFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	req.Filename = strings.TrimSpace(req.Filename)
	if req.StudentID == "" || req.DisciplineID == "" || req.WorkTypeID == 0 || req.Filename == "" {
		writeError(w, http.StatusBadRequest, errors.New("нужны studentId, disciplineId, workTypeId и filename"))
		return
	}
	if req.Semester == 0 {
		req.Semester = 1
	}
	if req.MimeType == "" {
		req.MimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(req.Filename)))
	}

	objectKey := fmt.Sprintf("uploads/%s/%s-%s", req.StudentID, randomHex(8), safeFileName(req.Filename))

	var id string
	err := a.db.QueryRow(ctx, `
        INSERT INTO portfolio_entries (
            student_id,
            student_group_history_id,
            discipline_id,
            work_type_id,
            semester,
            original_filename,
            s3_bucket_name,
            s3_object_key,
            file_size,
            mime_type,
            grade,
            teacher_comment
        )
        SELECT
            $1::uuid,
            sgh.id,
            $2::uuid,
            $3,
            $4,
            $5,
            'uni-portfolio-bucket',
            $6,
            $7,
            $8,
            NULLIF($9, ''),
            NULLIF($10, '')
        FROM student_group_history sgh
        WHERE sgh.student_id = $1::uuid
        ORDER BY sgh.academic_year_start DESC
        LIMIT 1
        RETURNING id::text
    `, req.StudentID, req.DisciplineID, req.WorkTypeID, req.Semester, req.Filename, objectKey, req.FileSize, req.MimeType, req.Grade, req.Comment).Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusBadRequest, errors.New("для студента не найдена история группы"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "s3ObjectKey": objectKey})
}

func demoDrive(message string) DriveResponse {
	return DriveResponse{
		Folders: []DriveFolder{
			{ID: "demo-db", Name: "Базы данных", Count: 2},
			{ID: "demo-algo", Name: "Алгоритмы", Count: 1},
			{ID: "demo-math", Name: "Мат. анализ", Count: 1},
		},
		Files: []DriveFile{
			{ID: "demo-1", FolderID: "demo-db", Name: "Лабораторная работа №1 Базы данных.pdf", Owner: "Петров Иван Сергеевич", Modified: "26 июня 2026", Size: "2,9 МБ", Kind: "pdf"},
			{ID: "demo-2", FolderID: "demo-db", Name: "SQL запросы и индексы.xlsx", Owner: "Сидорова Мария Александровна", Modified: "25 июня 2026", Size: "420 КБ", Kind: "xlsx"},
			{ID: "demo-3", FolderID: "demo-algo", Name: "Отчет по сортировкам.docx", Owner: "Петров Иван Сергеевич", Modified: "24 июня 2026", Size: "1,2 МБ", Kind: "docx"},
			{ID: "demo-4", FolderID: "demo-math", Name: "Графики интегралов.pptx", Owner: "Сидорова Мария Александровна", Modified: "23 июня 2026", Size: "3,1 МБ", Kind: "pptx"},
		},
		Source:            "go-fallback",
		DatabaseConnected: false,
		Message:           message,
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func spaHandler(dir string) http.HandlerFunc {
	fs := http.Dir(dir)
	fileServer := http.FileServer(fs)
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean(r.URL.Path)
		if strings.HasPrefix(path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}
		if _, err := os.Stat(filepath.Join(dir, path)); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func formatBytes(size int64) string {
	if size <= 0 {
		return "—"
	}
	units := []string{"Б", "КБ", "МБ", "ГБ"}
	value := float64(size)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return strconv.FormatInt(size, 10) + " Б"
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func formatDateRU(t time.Time) string {
	months := []string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
	month := months[int(t.Month())-1]
	return fmt.Sprintf("%d %s %d", t.Day(), month, t.Year())
}

func fileKind(name, mimeType string) string {
	ext := strings.ToLower(filepath.Ext(name))
	mt := strings.ToLower(mimeType)
	switch {
	case ext == ".pdf" || strings.Contains(mt, "pdf"):
		return "pdf"
	case ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || strings.HasPrefix(mt, "image/"):
		return "image"
	case ext == ".doc" || ext == ".docx" || strings.Contains(mt, "word"):
		return "docx"
	case ext == ".ppt" || ext == ".pptx" || strings.Contains(mt, "presentation"):
		return "pptx"
	case ext == ".xls" || ext == ".xlsx" || strings.Contains(mt, "spreadsheet"):
		return "xlsx"
	case ext == ".zip" || ext == ".rar" || strings.Contains(mt, "zip"):
		return "zip"
	default:
		return "pdf"
	}
}

func safeFileName(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "file"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", "\x00", "")
	return replacer.Replace(name)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}
