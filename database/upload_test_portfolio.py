import os
import uuid
import json
from datetime import datetime
from pathlib import Path

import boto3
import psycopg2
from botocore.exceptions import ClientError
from psycopg2.extras import RealDictCursor

# ============================================
# КОНФИГУРАЦИЯ ПОДКЛЮЧЕНИЙ (ЛОКАЛЬНАЯ)
# ============================================
S3_CONFIG = {
    "endpoint_url": "http://localhost:9000",
    "aws_access_key_id": "minioadmin",
    "aws_secret_access_key": "minioadmin123",
    "region_name": "us-east-1",
    "use_ssl": False
}

BUCKET_NAME = "uni-portfolio-bucket"

DB_CONFIG = {
    "host": "localhost",
    "port": 5432,  # или 5439, если менял порт
    "database": "university_portfolio",
    "user": "portfolio_admin",
    "password": "local_secret_pass"
}

# ============================================
# ТЕСТОВЫЕ ДАННЫЕ (Имитация того, что пришлет фронтенд)
# ============================================
# Эти UUID уже есть в твоей базе (из init-db.sql)
TEST_STUDENT_ID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
TEST_DISCIPLINE_ID = "c1a2b3c4-d5e6-f7a8-b9c0-d1e2f3a4b5c6"
TEST_FACULTY_ID = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

# Данные о работе
METADATA = {
    "work_type_id": 3,        # 3 = Лабораторная работа
    "semester": 2,            # Весенний семестр
    "grade": "5",
    "teacher_comment": "Отлично выполненная работа, сервер стабилен",
    "academic_year": 2024     # Год СТАРТА учебного года (2024-2025)
}

def get_or_create_group_history(conn, student_id: str, academic_year_start: int):
    """
    Находит или создает запись в student_group_history для указанного года.
    Использует enrollment_year из таблицы students.
    """
    academic_year_end = academic_year_start + 1
    
    with conn.cursor() as cur:
        # Получаем год поступления студента
        cur.execute("SELECT enrollment_year FROM students WHERE id = %s", (student_id,))
        result = cur.fetchone()
        
        if not result:
            raise ValueError(f"Студент с ID {student_id} не найден")
        
        enrollment_year = result[0]
        
        # Вычисляем курс
        study_year = (academic_year_start - enrollment_year) + 1
        
        # Пробуем найти существующую запись
        cur.execute("""
            SELECT id FROM student_group_history 
            WHERE student_id = %s AND academic_year_start = %s
        """, (student_id, academic_year_start))
        
        row = cur.fetchone()
        if row:
            print(f"  ✓ Найдена существующая запись истории группы (курс {study_year})")
            return row[0]
        
        # Генерируем имя группы
        group_suffix = str(academic_year_start)[-1:] + str(study_year)
        group_name = f"КС-{group_suffix}"
        
        # Создаем новую запись
        cur.execute("""
            INSERT INTO student_group_history 
                (student_id, group_name, faculty_id, study_year, academic_year_start, academic_year_end)
            VALUES (%s, %s, %s, %s, %s, %s)
            RETURNING id
        """, (
            student_id, 
            group_name, 
            TEST_FACULTY_ID, 
            study_year, 
            academic_year_start, 
            academic_year_end
        ))
        
        history_id = cur.fetchone()[0]
        conn.commit()
        print(f"  + Создана новая запись: {group_name}, курс {study_year} ({academic_year_start}-{academic_year_end})")
        return history_id

def generate_s3_key(student_id: str, academic_year_start: int, semester: int, 
                    discipline_id: str, work_type_id: int, original_filename: str) -> str:
    """
    Генерирует путь к файлу в S3 по нашему шаблону.
    """
    academic_year_end = academic_year_start + 1
    filename = Path(original_filename).name
    safe_filename = "".join(c for c in filename if c.isalnum() or c in ('.', '_', '-')).strip()
    
    s3_key = (
        f"students/{student_id}/"
        f"{academic_year_start}_{academic_year_end}/"
        f"{semester}/"
        f"{discipline_id}/"
        f"{work_type_id}/"
        f"{safe_filename}"
    )
    return s3_key

def upload_to_s3(s3_client, bucket: str, key: str, file_path: str) -> bool:
    """
    Загружает файл в S3 (MinIO).
    """
    try:
        s3_client.upload_file(file_path, bucket, key)
        print(f"  ✓ Файл загружен в S3: {bucket}/{key}")
        return True
    except ClientError as e:
        print(f"  ✗ Ошибка загрузки в S3: {e}")
        return False

def create_portfolio_entry(conn, student_id: str, history_id: str, discipline_id: str,
                           work_type_id: int, semester: int, s3_key: str, 
                           original_filename: str, file_size: int, mime_type: str = None,
                           grade: str = None, comment: str = None) -> uuid.UUID:
    """
    Создает запись в таблице portfolio_entries.
    """
    with conn.cursor() as cur:
        cur.execute("""
            INSERT INTO portfolio_entries 
                (student_id, student_group_history_id, discipline_id, work_type_id, 
                 semester, s3_bucket_name, s3_object_key, original_filename, 
                 file_size, mime_type, grade, teacher_comment)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
            RETURNING id
        """, (
            student_id, history_id, discipline_id, work_type_id,
            semester, BUCKET_NAME, s3_key, original_filename,
            file_size, mime_type, grade, comment
        ))
        
        entry_id = cur.fetchone()[0]
        conn.commit()
        return entry_id

def verify_entry(conn, entry_id: uuid.UUID):
    """
    Проверяет, что запись создалась корректно.
    """
    with conn.cursor(cursor_factory=RealDictCursor) as cur:
        cur.execute("""
            SELECT 
                pe.id,
                s.last_name || ' ' || s.first_name as student_name,
                sgh.group_name,
                sgh.academic_year_start,
                sgh.academic_year_end,
                d.name as discipline,
                wt.name as work_type,
                pe.s3_object_key,
                pe.upload_date
            FROM portfolio_entries pe
            JOIN students s ON pe.student_id = s.id
            JOIN student_group_history sgh ON pe.student_group_history_id = sgh.id
            JOIN disciplines d ON pe.discipline_id = d.id
            JOIN work_types wt ON pe.work_type_id = wt.id
            WHERE pe.id = %s
        """, (entry_id,))
        
        return cur.fetchone()

def main():
    print("=" * 60)
    print("ТЕСТОВАЯ ЗАГРУЗКА ФАЙЛА В ПОРТФОЛИО")
    print("=" * 60)
    
    # 1. Создаем тестовый файл (или используем существующий)
    test_file = Path("test_lab_report.pdf")
    if not test_file.exists():
        print(f"\n Создаю тестовый файл: {test_file}")
        test_file.write_text("Это тестовый отчет по лабораторной работе.\nСтудент: Иванов И.И.\nДата: 2024-05-15")
    else:
        print(f"\n Использую существующий файл: {test_file}")
    
    file_size = test_file.stat().st_size
    
    # 2. Подключаемся к БД
    print("\n  Подключение к PostgreSQL...")
    try:
        conn = psycopg2.connect(**DB_CONFIG)
        print(f"  ✓ Подключен к {DB_CONFIG['database']} на порту {DB_CONFIG['port']}")
    except Exception as e:
        print(f"  ✗ Ошибка подключения к БД: {e}")
        print("  Подсказка: проверь, что контейнер uni_portfolio_db запущен (docker ps)")
        return
    
    # 3. Подключаемся к S3 (MinIO)
    print("\n Подключение к S3 (MinIO)...")
    try:
        s3_client = boto3.client('s3', **S3_CONFIG)
        s3_client.list_buckets()
        print(f"  ✓ Подключен к MinIO на {S3_CONFIG['endpoint_url']}")
    except Exception as e:
        print(f"  ✗ Ошибка подключения к S3: {e}")
        conn.close()
        return
    
    # 4. Получаем/создаем запись истории группы
    print(f"\n Студент: {TEST_STUDENT_ID}")
    print(f"Учебный год: {METADATA['academic_year']}-{METADATA['academic_year']+1}")
    history_id = get_or_create_group_history(conn, TEST_STUDENT_ID, METADATA['academic_year'])
    
    # 5. Генерируем S3 ключ
    s3_key = generate_s3_key(
        student_id=TEST_STUDENT_ID,
        academic_year_start=METADATA['academic_year'],
        semester=METADATA['semester'],
        discipline_id=TEST_DISCIPLINE_ID,
        work_type_id=METADATA['work_type_id'],
        original_filename=test_file.name
    )
    print(f"\n S3 Key: {s3_key}")
    
    # 6. Загружаем файл в S3
    print("\n⬆  Загрузка файла в S3...")
    if not upload_to_s3(s3_client, BUCKET_NAME, s3_key, str(test_file)):
        conn.close()
        return
    
    # 7. Создаем запись в БД
    print("\n📝 Создание записи в portfolio_entries...")
    try:
        entry_id = create_portfolio_entry(
            conn=conn,
            student_id=TEST_STUDENT_ID,
            history_id=history_id,
            discipline_id=TEST_DISCIPLINE_ID,
            work_type_id=METADATA['work_type_id'],
            semester=METADATA['semester'],
            s3_key=s3_key,
            original_filename=test_file.name,
            file_size=file_size,
            mime_type="application/pdf",
            grade=METADATA['grade'],
            comment=METADATA['teacher_comment']
        )
        print(f"  ✓ Запись создана, ID: {entry_id}")
    except Exception as e:
        print(f"  ✗ Ошибка создания записи: {e}")
        conn.close()
        return
    
    # 8. Проверяем результат
    print("\n🔍 Проверка созданной записи:")
    result = verify_entry(conn, entry_id)
    if result:
        print(f"  Студент: {result['student_name']}")
        print(f"  Группа: {result['group_name']} ({result['academic_year_start']}-{result['academic_year_end']})")
        print(f"  Дисциплина: {result['discipline']}")
        print(f"  Тип работы: {result['work_type']}")
        print(f"  S3 путь: {result['s3_object_key']}")
        print(f"  Дата загрузки: {result['upload_date']}")
    
    conn.close()
    
    # 9. Проверяем через MinIO консоль
    print("\n" + "=" * 60)
    print("ТЕСТ УСПЕШНО ЗАВЕРШЕН!")
    print("=" * 60)
    print(f"\nПроверь файл в MinIO консоли:")
    print(f"  1. Открой: http://localhost:9001")
    print(f"  2. Войди (minioadmin / minioadmin123)")
    print(f"  3. Зайди в бакет: {BUCKET_NAME}")
    print(f"  4. Пройди по пути: {s3_key}")
    print(f"\nИли посмотри в БД запросом:")
    print(f"  SELECT * FROM portfolio_entries WHERE id = '{entry_id}';")

if __name__ == "__main__":
    main()