# MUCTR Drive: React + Go + PostgreSQL

## Авторизация: схема и тестовые аккаунты

Таблицы auth_users и auth_sessions описаны в database/02-auth.sql.
Тестовые записи для локальной разработки — в database/03-dev-auth-seed.sql.
При создании нового тома PostgreSQL оба файла выполняются автоматически после init-db.sql.

| Роль | Email | Пароль |
| --- | --- | --- |
| Администратор | admin@muctr.local | Admin123!dev |
| Студент Иван Петров | i.petrov@university.local | Student123!dev |
| Студент Мария Сидорова | m.sidorova@university.local | Student123!dev |

На существующем томе init-скрипты повторно не запускаются. Применить схему и тестовые записи можно без удаления данных:

```bash
docker compose up -d postgres
docker compose exec postgres psql -U portfolio_admin -d university_portfolio -f /docker-entrypoint-initdb.d/02-auth.sql
docker compose exec postgres psql -U portfolio_admin -d university_portfolio -f /docker-entrypoint-initdb.d/03-dev-auth-seed.sql
```

Это только тестовые учётные данные; перед публикацией стенда замените их. Подробное задание команде — в AUTH_TASKS.md.

Минимальная связка для демонстрации:

- `frontend/` — React/Vite интерфейс диска.
- `backend/` — Go API, которое читает данные из PostgreSQL.
- `database/` — исходная БД: PostgreSQL + MinIO + SQL инициализация.
- `docker-compose.yml` — запуск всего стека одной командой.

## Быстрый запуск через Docker

```bash
docker compose up --build
```

После запуска:

- фронтенд: http://localhost:5173
- Go API: http://localhost:8080/api/health
- данные диска: http://localhost:8080/api/drive
- MinIO console: http://localhost:9001

Логин MinIO: `minioadmin`, пароль: `minioadmin123`.

Если база уже запускалась раньше и тестовые файлы не появились, пересоздай volume:

```bash
docker compose down -v
docker compose up --build
```

## Локальный запуск без Docker для Go/React

1. Поднять только базу:

```bash
docker compose up postgres minio minio-init
```

2. Запустить Go API:

```bash
cd backend
export DATABASE_URL='postgres://portfolio_admin:local_secret_pass@localhost:5432/university_portfolio?sslmode=disable'
go mod download
go run .
```

3. Запустить фронтенд:

```bash
cd frontend
npm install
npm run dev
```

Vite проксирует `/api` на `http://localhost:8080`, поэтому в коде фронта можно делать `fetch('/api/drive')`.

## Что уже связано

Go API:

- `GET /api/health` — проверка подключения к PostgreSQL.
- `GET /api/drive` — папки и файлы для интерфейса.
- `GET /api/students` — студенты из БД.
- `GET /api/disciplines` — дисциплины из БД.
- `POST /api/files` — создание записи файла в `portfolio_entries` без реальной загрузки в MinIO.

Фронтенд теперь сначала пробует загрузить `/api/drive`. Если Go API или PostgreSQL недоступны, интерфейс не падает и показывает демо-данные.

## Пример создания файла через Go API

```bash
curl -X POST http://localhost:8080/api/files \
  -H 'Content-Type: application/json' \
  -d '{
    "studentId":"f47ac10b-58cc-4372-a567-0e02b2c3d479",
    "disciplineId":"c1a2b3c4-d5e6-f7a8-b9c0-d1e2f3a4b5c6",
    "workTypeId":3,
    "semester":2,
    "filename":"Новый отчет.pdf",
    "fileSize":204800,
    "mimeType":"application/pdf",
    "grade":"5",
    "comment":"Добавлено через Go API"
  }'
```
