BEGIN;
-- A self-registered student can upload without a university group record.
ALTER TABLE portfolio_entries ALTER COLUMN student_group_history_id DROP NOT NULL;
ALTER TABLE portfolio_entries ADD COLUMN IF NOT EXISTS title VARCHAR(255);
ALTER TABLE portfolio_entries ADD COLUMN IF NOT EXISTS academic_year_start SMALLINT
 CHECK (academic_year_start BETWEEN 2000 AND 2100);
-- Existing records are checked against MinIO on API startup. Missing demo objects
-- stay in the database for inspection but are not presented as uploaded works.
ALTER TABLE portfolio_entries ADD COLUMN IF NOT EXISTS storage_status VARCHAR(20)
 NOT NULL DEFAULT 'legacy' CHECK (storage_status IN ('legacy','pending','ready','missing','deleting'));
ALTER TABLE portfolio_entries ALTER COLUMN storage_status SET DEFAULT 'pending';
UPDATE portfolio_entries pe SET academic_year_start = COALESCE(
 (SELECT h.academic_year_start FROM student_group_history h WHERE h.id=pe.student_group_history_id),
 EXTRACT(YEAR FROM pe.upload_date)::smallint)
 WHERE academic_year_start IS NULL;
UPDATE portfolio_entries SET title=original_filename WHERE title IS NULL;
CREATE INDEX IF NOT EXISTS idx_portfolio_owner_ready
 ON portfolio_entries(student_id,upload_date DESC) WHERE storage_status='ready';
INSERT INTO work_types(id,name,description) VALUES
 (7,'Лекция','Материалы лекционного занятия'),
 (8,'Другая работа','Другие учебные материалы'),
 (9,'Курсовая работа','Курсовой проект или работа'),
 (10,'Дипломная работа','Выпускная квалификационная работа')
 ON CONFLICT(id) DO NOTHING;

CREATE OR REPLACE VIEW portfolio_view AS
SELECT pe.id,pe.student_id,
 s.last_name||' '||s.first_name||COALESCE(' '||s.middle_name,'') AS student_full_name,
 s.enrollment_year,sgh.group_name,sgh.study_year AS course,
 COALESCE(sgh.academic_year_start,pe.academic_year_start) AS academic_year_start,
 COALESCE(sgh.academic_year_end,(pe.academic_year_start+1)::smallint) AS academic_year_end,
 d.name AS discipline_name,wt.name AS work_type_name,pe.semester,
 pe.original_filename,pe.s3_bucket_name,pe.s3_object_key,pe.file_size,pe.mime_type,
 pe.grade,pe.teacher_comment,pe.upload_date,f.short_name AS faculty_short_name
FROM portfolio_entries pe JOIN students s ON pe.student_id=s.id
LEFT JOIN student_group_history sgh ON pe.student_group_history_id=sgh.id
JOIN disciplines d ON pe.discipline_id=d.id JOIN work_types wt ON pe.work_type_id=wt.id
LEFT JOIN faculties f ON sgh.faculty_id=f.id;
COMMIT;
