package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

type DriveFile struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Name              string    `json:"name"`
	Owner             string    `json:"owner"`
	StudentID         string    `json:"studentId"`
	DisciplineID      string    `json:"disciplineId"`
	DisciplineName    string    `json:"disciplineName"`
	WorkTypeID        int       `json:"workTypeId"`
	WorkTypeName      string    `json:"workTypeName"`
	Semester          int       `json:"semester"`
	AcademicYearStart int       `json:"academicYearStart"`
	FileSize          int64     `json:"fileSize"`
	MimeType          string    `json:"mimeType"`
	UploadedAt        time.Time `json:"uploadedAt"`
}
type DriveStats struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
}

func (a *App) drive(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		page = 100000
	}
	const pageSize = 20
	search := strings.TrimSpace(q.Get("q"))
	if len(search) > 400 {
		fail(w, 400, "Слишком длинный запрос")
		return
	}
	student := q.Get("student")
	if student != "" {
		if _, err := uuid.Parse(student); err != nil {
			fail(w, 400, "Некорректный ID студента")
			return
		}
		if u.Role != "admin" && (u.StudentID == nil || student != *u.StudentID) {
			fail(w, 403, "Недостаточно прав")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var stats DriveStats
	err := a.db.QueryRow(ctx, `SELECT COUNT(*),COALESCE(SUM(file_size),0) FROM portfolio_entries
 WHERE storage_status='ready' AND ($1::boolean OR student_id=$2::uuid)`, u.Role == "admin", u.StudentID).Scan(&stats.Count, &stats.Bytes)
	if err != nil {
		serverError(w, err)
		return
	}
	filter := ` FROM portfolio_entries pe JOIN students s ON s.id=pe.student_id
 JOIN disciplines d ON d.id=pe.discipline_id JOIN work_types wt ON wt.id=pe.work_type_id
 WHERE pe.storage_status='ready' AND ($1::boolean OR pe.student_id=$2::uuid)
 AND ($3='' OR pe.discipline_id::text=$3) AND ($4='' OR pe.work_type_id::text=$4)
 AND ($5='' OR pe.semester::text=$5) AND ($6='' OR pe.student_id::text=$6)
 AND ($7='' OR pe.original_filename ILIKE '%'||$7||'%' OR pe.title ILIKE '%'||$7||'%'
 OR d.name ILIKE '%'||$7||'%' OR s.last_name||' '||s.first_name ILIKE '%'||$7||'%')`
	args := []any{u.Role == "admin", u.StudentID, q.Get("subject"), q.Get("type"), q.Get("semester"), student, search}
	var total int
	if err = a.db.QueryRow(ctx, "SELECT COUNT(*)"+filter, args...).Scan(&total); err != nil {
		serverError(w, err)
		return
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := a.db.Query(ctx, `SELECT pe.id::text,COALESCE(pe.title,''),pe.original_filename,
 s.last_name||' '||s.first_name||COALESCE(' '||s.middle_name,''),s.id::text,d.id::text,d.name,
 wt.id,wt.name,COALESCE(pe.semester,1),COALESCE(pe.academic_year_start,EXTRACT(YEAR FROM pe.upload_date)::int),
 COALESCE(pe.file_size,0),COALESCE(pe.mime_type,'application/octet-stream'),pe.upload_date`+filter+
		" ORDER BY pe.upload_date DESC,pe.id LIMIT $8 OFFSET $9", args...)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	files := []DriveFile{}
	for rows.Next() {
		var f DriveFile
		if err = rows.Scan(&f.ID, &f.Title, &f.Name, &f.Owner, &f.StudentID, &f.DisciplineID, &f.DisciplineName, &f.WorkTypeID, &f.WorkTypeName, &f.Semester, &f.AcademicYearStart, &f.FileSize, &f.MimeType, &f.UploadedAt); err != nil {
			serverError(w, err)
			return
		}
		files = append(files, f)
	}
	if err = rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"files": files, "total": total, "page": page, "pageSize": pageSize, "stats": stats})
}

func validateFile(file multipart.File, size int64, name string) (string, error) {
	ext := strings.ToLower(filepath.Ext(name))
	head := make([]byte, 512)
	n, err := file.Read(head)
	if err != nil && err != io.EOF {
		return "", err
	}
	head = head[:n]
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	detected := http.DetectContentType(head)
	switch ext {
	case ".pdf":
		if bytes.HasPrefix(head, []byte("%PDF-")) {
			return "application/pdf", nil
		}
	case ".png", ".jpg", ".jpeg":
		if (ext == ".png" && detected == "image/png") || ((ext == ".jpg" || ext == ".jpeg") && detected == "image/jpeg") {
			return detected, nil
		}
	case ".txt", ".csv":
		if strings.HasPrefix(detected, "text/plain") {
			// A 512-byte prefix may split a Cyrillic character or conceal
			// invalid bytes later in the file. Validate the entire bounded file.
			content, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
			if err != nil {
				return "", err
			}
			if _, err = file.Seek(0, io.SeekStart); err != nil {
				return "", err
			}
			if int64(len(content)) <= maxUploadBytes && utf8.Valid(content) && !bytes.ContainsRune(content, 0) {
				return "text/plain; charset=utf-8", nil
			}
		}
	case ".doc", ".xls", ".ppt":
		if bytes.HasPrefix(head, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}) {
			return "application/octet-stream", nil
		}
	case ".docx", ".xlsx", ".pptx", ".zip":
		archive, err := zip.NewReader(file, size)
		if err != nil {
			break
		}
		if ext == ".zip" {
			return "application/zip", nil
		}
		needed := map[string]string{".docx": "word/document.xml", ".xlsx": "xl/workbook.xml", ".pptx": "ppt/presentation.xml"}
		types := map[string]string{".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation"}
		contentTypes, document := false, false
		for _, entry := range archive.File {
			if entry.Name == "[Content_Types].xml" {
				contentTypes = true
			}
			if entry.Name == needed[ext] {
				document = true
			}
		}
		if contentTypes && document {
			return types[ext], nil
		}
	}
	return "", errors.New("Допустимы PDF, DOC/DOCX, XLS/XLSX, PPT/PPTX, TXT, CSV, JPG, PNG и ZIP. Содержимое должно соответствовать формату")
}
func safeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name))
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(w, 413, "Файл слишком большой. Максимум 25 МБ")
		} else {
			fail(w, 400, "Не удалось прочитать файл и поля формы")
		}
		return
	}
	defer r.MultipartForm.RemoveAll()
	allowed := map[string]bool{"title": true, "disciplineId": true, "workTypeId": true, "semester": true, "academicYearStart": true}
	for key, values := range r.MultipartForm.Value {
		if !allowed[key] || len(values) != 1 {
			fail(w, 400, "Неизвестные или повторяющиеся поля формы")
			return
		}
	}
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		fail(w, 400, "Выберите один файл")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "Выберите файл")
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxUploadBytes {
		fail(w, 413, "Размер файла должен быть от 1 байта до 25 МБ")
		return
	}
	name := safeFilename(header.Filename)
	if name == "" || name == "." || utf8.RuneCountInString(name) > 255 {
		fail(w, 400, "Некорректное имя файла")
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = name
	}
	if utf8.RuneCountInString(title) > 255 {
		fail(w, 400, "Название работы: не более 255 символов")
		return
	}
	subject, err := uuid.Parse(r.FormValue("disciplineId"))
	if err != nil {
		fail(w, 400, "Выберите предмет")
		return
	}
	workType, err := strconv.Atoi(r.FormValue("workTypeId"))
	if err != nil || workType < 1 || workType > 32767 {
		fail(w, 400, "Выберите тип работы")
		return
	}
	semester, _ := strconv.Atoi(r.FormValue("semester"))
	if semester != 1 && semester != 2 {
		fail(w, 400, "Семестр должен быть 1 или 2")
		return
	}
	year, _ := strconv.Atoi(r.FormValue("academicYearStart"))
	if year < 2000 || year > time.Now().Year()+1 {
		fail(w, 400, "Проверьте учебный год")
		return
	}
	mimeType, err := validateFile(file, header.Size, name)
	if err != nil {
		fail(w, 415, err.Error())
		return
	}
	u := currentUser(r)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	var valid bool
	err = a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM disciplines WHERE id=$1)
 AND EXISTS(SELECT 1 FROM work_types WHERE id=$2)`, subject.String(), workType).Scan(&valid)
	if err != nil {
		serverError(w, err)
		return
	}
	if !valid {
		fail(w, 400, "Предмет или тип работы не найден")
		return
	}
	id := uuid.NewString()
	key := "students/" + *u.StudentID + "/" + id + strings.ToLower(filepath.Ext(name))
	_, err = a.db.Exec(ctx, `INSERT INTO portfolio_entries(id,student_id,student_group_history_id,discipline_id,work_type_id,semester,
 title,academic_year_start,original_filename,s3_bucket_name,s3_object_key,file_size,mime_type,storage_status)
 VALUES($1,$2,(SELECT id FROM student_group_history WHERE student_id=$2 AND academic_year_start=$3 LIMIT 1),
 $4,$5,$6,$7,$3,$8,$9,$10,$11,$12,'pending')`, id, *u.StudentID, year, subject.String(), workType, semester, title, name, a.bucket, key, header.Size, mimeType)
	if err != nil {
		serverError(w, err)
		return
	}
	_, err = a.storage.PutObject(ctx, a.bucket, key, file, header.Size, minio.PutObjectOptions{ContentType: mimeType})
	if err != nil {
		a.cleanupUpload(id, a.bucket, key)
		serverError(w, err)
		return
	}
	_, err = a.db.Exec(ctx, "UPDATE portfolio_entries SET storage_status='ready' WHERE id=$1", id)
	if err != nil {
		// Commit acknowledgement can be lost: leave a possibly-ready object intact.
		log.Printf("finalizing upload %s: %v", id, err)
		fail(w, 503, "Не удалось подтвердить загрузку. Обновите список перед повторной попыткой.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "name": name})
}
func (a *App) cleanupUpload(id, bucket, key string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.storage.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		log.Printf("cleanup object %s: %v", id, err)
		return
	}
	if _, err := a.db.Exec(ctx, "DELETE FROM portfolio_entries WHERE id=$1 AND storage_status='pending'", id); err != nil {
		log.Printf("cleanup row %s: %v", id, err)
	}
}
func (a *App) download(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		fail(w, 404, "Работа не найдена")
		return
	}
	u := currentUser(r)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	var bucket, key, name, contentType string
	err = a.db.QueryRow(ctx, `SELECT s3_bucket_name,s3_object_key,original_filename,COALESCE(mime_type,'application/octet-stream')
 FROM portfolio_entries WHERE id=$1 AND storage_status='ready' AND ($2::boolean OR student_id=$3::uuid)`, id.String(), u.Role == "admin", u.StudentID).
		Scan(&bucket, &key, &name, &contentType)
	if missing(err) {
		fail(w, 404, "Работа не найдена")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	object, err := a.storage.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		serverError(w, err)
		return
	}
	defer object.Close()
	info, err := object.Stat()
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			fail(w, 404, "Файл отсутствует в хранилище")
		} else {
			serverError(w, err)
		}
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	if _, err = io.Copy(w, object); err != nil {
		log.Printf("download %s: %v", id, err)
	}
}
func (a *App) deleteFile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		fail(w, 404, "Работа не найдена")
		return
	}
	u := currentUser(r)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var bucket, key string
	err = a.db.QueryRow(ctx, `UPDATE portfolio_entries SET storage_status='deleting' WHERE id=$1
 AND storage_status='ready' AND ($2::boolean OR student_id=$3::uuid) RETURNING s3_bucket_name,s3_object_key`,
		id.String(), u.Role == "admin", u.StudentID).Scan(&bucket, &key)
	if missing(err) {
		fail(w, 404, "Работа не найдена")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if err = a.storage.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		restoreCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, e := a.db.Exec(restoreCtx, "UPDATE portfolio_entries SET storage_status='ready' WHERE id=$1 AND storage_status='deleting'", id.String()); e != nil {
			log.Printf("restore delete %s: %v", id, e)
		}
		serverError(w, err)
		return
	}
	if _, err = a.db.Exec(ctx, "DELETE FROM portfolio_entries WHERE id=$1", id.String()); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (a *App) reconcileStorage(ctx context.Context) {
	rows, err := a.db.Query(ctx, `SELECT id::text,s3_bucket_name,s3_object_key,storage_status FROM portfolio_entries
 WHERE storage_status='legacy' OR (storage_status IN ('pending','deleting') AND updated_at < NOW()-INTERVAL '15 minutes')`)
	if err != nil {
		log.Printf("storage reconciliation: %v", err)
		return
	}
	type item struct{ id, bucket, key, status string }
	items := []item{}
	for rows.Next() {
		var i item
		if err = rows.Scan(&i.id, &i.bucket, &i.key, &i.status); err != nil {
			break
		}
		items = append(items, i)
	}
	rows.Close()
	for _, i := range items {
		if i.status == "legacy" {
			_, err := a.storage.StatObject(ctx, i.bucket, i.key, minio.StatObjectOptions{})
			status := "ready"
			if err != nil {
				if minio.ToErrorResponse(err).Code != "NoSuchKey" {
					log.Printf("legacy object %s: %v", i.id, err)
					continue
				}
				status = "missing"
			}
			if _, err = a.db.Exec(ctx, "UPDATE portfolio_entries SET storage_status=$2 WHERE id=$1 AND storage_status='legacy'", i.id, status); err != nil {
				log.Printf("reconcile %s: %v", i.id, err)
			}
		} else {
			if err := a.storage.RemoveObject(ctx, i.bucket, i.key, minio.RemoveObjectOptions{}); err != nil {
				log.Printf("cleanup %s: %v", i.id, err)
				continue
			}
			if _, err = a.db.Exec(ctx, "DELETE FROM portfolio_entries WHERE id=$1 AND storage_status=$2", i.id, i.status); err != nil {
				log.Printf("cleanup %s: %v", i.id, err)
			}
		}
	}
}
func (a *App) maintenance(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			work, cancel := context.WithTimeout(ctx, time.Minute)
			a.reconcileStorage(work)
			if _, err := a.db.Exec(work, "DELETE FROM auth_sessions WHERE expires_at<NOW() OR revoked_at IS NOT NULL"); err != nil {
				log.Print(fmt.Errorf("session cleanup: %w", err))
			}
			cancel()
		}
	}
}
