-- =============================================
-- 1. РАСШИРЕНИЯ
-- =============================================
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- =============================================
-- 2. СПРАВОЧНИКИ
-- =============================================

-- Факультеты
CREATE TABLE faculties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    short_name VARCHAR(50),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Дисциплины (Предметы)
CREATE TABLE disciplines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    faculty_id UUID REFERENCES faculties(id) ON DELETE SET NULL,
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Типы работ
CREATE TABLE work_types (
    id SMALLINT PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    description TEXT
);

INSERT INTO work_types (id, name, description) VALUES 
(1, 'Конспект лекции', 'Записи с лекционных занятий'), 
(2, 'Семинар', 'Работы с семинарских занятий'), 
(3, 'Лабораторная работа', 'Практические лабораторные задания'), 
(4, 'Контрольная работа', 'Промежуточные контрольные работы'), 
(5, 'Зачетная работа', 'Итоговые работы для зачета'), 
(6, 'Экзаменационная работа', 'Экзаменационные билеты и ответы')
ON CONFLICT (id) DO NOTHING;

-- =============================================
-- 3. СТУДЕНТЫ И ИСТОРИЯ ГРУПП
-- =============================================

-- Основная информация о студенте
CREATE TABLE students (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    middle_name VARCHAR(100),
    email VARCHAR(255) UNIQUE,
    enrollment_year SMALLINT NOT NULL,  -- Год поступления (важно для расчета курса)
    created_at TIMESTAMPTZ DEFAULT NOW(),
    
    CONSTRAINT valid_enrollment_year CHECK (enrollment_year >= 2000 AND enrollment_year <= EXTRACT(YEAR FROM NOW()) + 1)
);

-- История зачисления в группы (SCD Type 2)
CREATE TABLE student_group_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    group_name VARCHAR(50) NOT NULL,
    faculty_id UUID REFERENCES faculties(id) ON DELETE SET NULL,
    study_year SMALLINT NOT NULL,  -- Курс (1-6)
    academic_year_start SMALLINT NOT NULL,
    academic_year_end SMALLINT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    
    CONSTRAINT valid_academic_year CHECK (academic_year_end = academic_year_start + 1),
    CONSTRAINT valid_study_year CHECK (study_year BETWEEN 1 AND 6),
    UNIQUE (student_id, academic_year_start)
);

CREATE INDEX idx_student_history_lookup ON student_group_history(student_id, academic_year_start);
CREATE INDEX idx_student_history_group ON student_group_history(group_name, academic_year_start);

-- =============================================
-- 4. ПОРТФОЛИО (ФАЙЛЫ)
-- =============================================

CREATE TABLE portfolio_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Кому принадлежит и когда создано
    student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    student_group_history_id UUID NOT NULL REFERENCES student_group_history(id),
    
    -- Что за работа
    discipline_id UUID NOT NULL REFERENCES disciplines(id),
    work_type_id SMALLINT NOT NULL REFERENCES work_types(id),
    semester SMALLINT CHECK (semester IN (1, 2)),
    
    -- Метаданные файла
    original_filename VARCHAR(512) NOT NULL,
    s3_bucket_name VARCHAR(100) NOT NULL DEFAULT 'uni-portfolio-bucket',
    s3_object_key TEXT NOT NULL,
    file_size BIGINT CHECK (file_size >= 0),
    mime_type VARCHAR(100),
    
    -- Оценка и комментарии
    grade VARCHAR(10),
    teacher_comment TEXT,
    
    -- Системные поля
    upload_date TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    
    UNIQUE(s3_bucket_name, s3_object_key)
);

CREATE INDEX idx_portfolio_student ON portfolio_entries(student_id);
CREATE INDEX idx_portfolio_student_discipline ON portfolio_entries(student_id, discipline_id);
CREATE INDEX idx_portfolio_student_year ON portfolio_entries(student_id, upload_date);
CREATE INDEX idx_portfolio_discipline_worktype ON portfolio_entries(discipline_id, work_type_id);

-- =============================================
-- 5. ТРИГГЕР ДЛЯ АВТООБНОВЛЕНИЯ updated_at
-- =============================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_portfolio_entries_updated_at 
    BEFORE UPDATE ON portfolio_entries 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- =============================================
-- 6. ПРЕДСТАВЛЕНИЕ ДЛЯ УДОБНОГО ПОИСКА
-- =============================================

CREATE OR REPLACE VIEW portfolio_view AS
SELECT 
    pe.id,
    pe.student_id,
    s.last_name || ' ' || s.first_name || COALESCE(' ' || s.middle_name, '') AS student_full_name,
    s.enrollment_year,
    sgh.group_name,
    sgh.study_year AS course,
    sgh.academic_year_start,
    sgh.academic_year_end,
    d.name AS discipline_name,
    wt.name AS work_type_name,
    pe.semester,
    pe.original_filename,
    pe.s3_bucket_name,
    pe.s3_object_key,
    pe.file_size,
    pe.mime_type,
    pe.grade,
    pe.teacher_comment,
    pe.upload_date,
    f.short_name AS faculty_short_name
FROM portfolio_entries pe
JOIN students s ON pe.student_id = s.id
JOIN student_group_history sgh ON pe.student_group_history_id = sgh.id
JOIN disciplines d ON pe.discipline_id = d.id
JOIN work_types wt ON pe.work_type_id = wt.id
LEFT JOIN faculties f ON sgh.faculty_id = f.id;

-- =============================================
-- 7. ТЕСТОВЫЕ ДАННЫЕ (без ON CONFLICT)
-- =============================================

-- Факультеты (проверяем существование)
INSERT INTO faculties (id, name, short_name)
SELECT 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'Факультет вычислительной техники', 'ФВТ'
WHERE NOT EXISTS (SELECT 1 FROM faculties WHERE name = 'Факультет вычислительной техники');

INSERT INTO faculties (id, name, short_name)
SELECT 'b1f2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6', 'Факультет прикладной математики', 'ФПМ'
WHERE NOT EXISTS (SELECT 1 FROM faculties WHERE name = 'Факультет прикладной математики');

-- Дисциплины (проверяем существование)
INSERT INTO disciplines (id, name, faculty_id, description)
SELECT 'c1a2b3c4-d5e6-f7a8-b9c0-d1e2f3a4b5c6', 'Базы данных', 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'Проектирование и оптимизация реляционных БД'
WHERE NOT EXISTS (SELECT 1 FROM disciplines WHERE name = 'Базы данных');

INSERT INTO disciplines (id, name, faculty_id, description)
SELECT 'd2e3f4a5-b6c7-d8e9-f0a1-b2c3d4e5f6a7', 'Алгоритмы и структуры данных', 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'Основные алгоритмы и оценка сложности'
WHERE NOT EXISTS (SELECT 1 FROM disciplines WHERE name = 'Алгоритмы и структуры данных');

INSERT INTO disciplines (id, name, faculty_id, description)
SELECT 'e3f4a5b6-c7d8-e9f0-a1b2-c3d4e5f6a7b8', 'Математический анализ', 'b1f2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6', 'Пределы, производные, интегралы'
WHERE NOT EXISTS (SELECT 1 FROM disciplines WHERE name = 'Математический анализ');

-- Студенты (проверяем существование)
INSERT INTO students (id, first_name, last_name, middle_name, email, enrollment_year)
SELECT 'f47ac10b-58cc-4372-a567-0e02b2c3d479', 'Иван', 'Петров', 'Сергеевич', 'i.petrov@university.local', 2023
WHERE NOT EXISTS (SELECT 1 FROM students WHERE email = 'i.petrov@university.local');

INSERT INTO students (id, first_name, last_name, middle_name, email, enrollment_year)
SELECT 'a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6', 'Мария', 'Сидорова', 'Александровна', 'm.sidorova@university.local', 2024
WHERE NOT EXISTS (SELECT 1 FROM students WHERE email = 'm.sidorova@university.local');

-- История групп (проверяем существование)
INSERT INTO student_group_history (student_id, group_name, faculty_id, study_year, academic_year_start, academic_year_end)
SELECT 'f47ac10b-58cc-4372-a567-0e02b2c3d479', 'КС-13', 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 1, 2023, 2024
WHERE NOT EXISTS (SELECT 1 FROM student_group_history WHERE student_id = 'f47ac10b-58cc-4372-a567-0e02b2c3d479' AND academic_year_start = 2023);

INSERT INTO student_group_history (student_id, group_name, faculty_id, study_year, academic_year_start, academic_year_end)
SELECT 'f47ac10b-58cc-4372-a567-0e02b2c3d479', 'КС-24', 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 2, 2024, 2025
WHERE NOT EXISTS (SELECT 1 FROM student_group_history WHERE student_id = 'f47ac10b-58cc-4372-a567-0e02b2c3d479' AND academic_year_start = 2024);

INSERT INTO student_group_history (student_id, group_name, faculty_id, study_year, academic_year_start, academic_year_end)
SELECT 'a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6', 'КС-14', 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 1, 2024, 2025
WHERE NOT EXISTS (SELECT 1 FROM student_group_history WHERE student_id = 'a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6' AND academic_year_start = 2024);


-- Тестовые файлы портфолио, чтобы Go API и фронтенд сразу показывали реальные записи из БД
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
    teacher_comment,
    upload_date
)
SELECT
    'f47ac10b-58cc-4372-a567-0e02b2c3d479'::uuid,
    sgh.id,
    'c1a2b3c4-d5e6-f7a8-b9c0-d1e2f3a4b5c6'::uuid,
    3,
    2,
    'Лабораторная работа №1 Базы данных.pdf',
    'uni-portfolio-bucket',
    'portfolio/petrov/database-lab-1.pdf',
    3092653,
    'application/pdf',
    '5',
    'Отлично выполненная работа, сервер стабилен',
    NOW() - INTERVAL '3 days'
FROM student_group_history sgh
WHERE sgh.student_id = 'f47ac10b-58cc-4372-a567-0e02b2c3d479'::uuid
ORDER BY sgh.academic_year_start DESC
LIMIT 1
ON CONFLICT (s3_bucket_name, s3_object_key) DO NOTHING;

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
    teacher_comment,
    upload_date
)
SELECT
    'a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6'::uuid,
    sgh.id,
    'c1a2b3c4-d5e6-f7a8-b9c0-d1e2f3a4b5c6'::uuid,
    4,
    2,
    'SQL запросы и индексы.xlsx',
    'uni-portfolio-bucket',
    'portfolio/sidorova/sql-indexes.xlsx',
    430080,
    'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    '5',
    'Добавлены примеры индексов и планы запросов',
    NOW() - INTERVAL '2 days'
FROM student_group_history sgh
WHERE sgh.student_id = 'a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6'::uuid
ORDER BY sgh.academic_year_start DESC
LIMIT 1
ON CONFLICT (s3_bucket_name, s3_object_key) DO NOTHING;

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
    teacher_comment,
    upload_date
)
SELECT
    'f47ac10b-58cc-4372-a567-0e02b2c3d479'::uuid,
    sgh.id,
    'd2e3f4a5-b6c7-d8e9-f0a1-b2c3d4e5f6a7'::uuid,
    2,
    1,
    'Отчет по сортировкам.docx',
    'uni-portfolio-bucket',
    'portfolio/petrov/sorting-report.docx',
    1258291,
    'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    '4',
    'Нужно добавить сравнение сложности',
    NOW() - INTERVAL '1 day'
FROM student_group_history sgh
WHERE sgh.student_id = 'f47ac10b-58cc-4372-a567-0e02b2c3d479'::uuid
ORDER BY sgh.academic_year_start DESC
LIMIT 1
ON CONFLICT (s3_bucket_name, s3_object_key) DO NOTHING;

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
    teacher_comment,
    upload_date
)
SELECT
    'a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6'::uuid,
    sgh.id,
    'e3f4a5b6-c7d8-e9f0-a1b2-c3d4e5f6a7b8'::uuid,
    1,
    1,
    'Графики интегралов.pptx',
    'uni-portfolio-bucket',
    'portfolio/sidorova/integrals-presentation.pptx',
    3250585,
    'application/vnd.openxmlformats-officedocument.presentationml.presentation',
    '5',
    'Хорошая визуализация',
    NOW() - INTERVAL '4 hours'
FROM student_group_history sgh
WHERE sgh.student_id = 'a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6'::uuid
ORDER BY sgh.academic_year_start DESC
LIMIT 1
ON CONFLICT (s3_bucket_name, s3_object_key) DO NOTHING;

-- =============================================
-- 8. ПОЛЕЗНЫЕ ФУНКЦИИ ДЛЯ РАБОТЫ
-- =============================================

-- Функция для получения текущей группы студента
CREATE OR REPLACE FUNCTION get_current_group(p_student_id UUID)
RETURNS TABLE (
    group_name VARCHAR(50),
    study_year SMALLINT,
    academic_year_start SMALLINT,
    academic_year_end SMALLINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        sgh.group_name,
        sgh.study_year,
        sgh.academic_year_start,
        sgh.academic_year_end
    FROM student_group_history sgh
    WHERE sgh.student_id = p_student_id
    ORDER BY sgh.academic_year_start DESC
    LIMIT 1;
END;
$$ LANGUAGE plpgsql;

-- Функция для расчета курса студента на основе года поступления
CREATE OR REPLACE FUNCTION calculate_study_year(p_student_id UUID, p_academic_year_start SMALLINT)
RETURNS SMALLINT AS $$
DECLARE
    v_enrollment_year SMALLINT;
BEGIN
    SELECT enrollment_year INTO v_enrollment_year
    FROM students
    WHERE id = p_student_id;
    
    RETURN (p_academic_year_start - v_enrollment_year) + 1;
END;
$$ LANGUAGE plpgsql;

-- Выведем сообщение об успешной инициализации с проверкой
DO $$
DECLARE
    student_count INTEGER;
    discipline_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO student_count FROM students;
    SELECT COUNT(*) INTO discipline_count FROM disciplines;
    
    RAISE NOTICE '========================================';
    RAISE NOTICE 'База данных успешно инициализирована!';
    RAISE NOTICE 'Создано таблиц: 6';
    RAISE NOTICE 'Добавлено студентов: %', student_count;
    RAISE NOTICE 'Добавлено дисциплин: %', discipline_count;
    RAISE NOTICE '========================================';
END $$;