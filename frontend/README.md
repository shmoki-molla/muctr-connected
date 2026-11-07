# МУЦТР Диск — React интерфейс

Интерфейс теперь подключается к Go API по адресу `/api/drive`.

## Запуск

```bash
npm install
npm run dev
```

Vite proxy уже настроен: запросы `/api/*` уходят на `http://localhost:8080`.

## Где подключение

- `src/api/driveApi.ts` — загрузка данных из Go API.
- `src/App.tsx` — состояние папок/файлов теперь приходит из API; локальные данные используются только как fallback.
- `vite.config.ts` — proxy для локальной разработки.

## Переменная для внешнего API

Если API не рядом с фронтом, можно задать:

```bash
VITE_API_URL=http://localhost:8080 npm run dev
```
