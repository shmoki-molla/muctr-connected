# Интерфейс РХТУ Диска

React + TypeScript + Vite. Регистрация, вход, кабинет студента, загрузка файлов, поиск и фильтры, скачивание/удаление, панель администратора.

```powershell
npm ci
npm run dev
```

Node.js 22.12+. API должен работать на localhost:8080; Vite проксирует `/api`.
Сборка: `npm run build`. Полный стек: `docker compose up -d --build` из корня.

Основной интерфейс: `src/App.tsx`, стили: `src/styles.css`, API и типы: `src/api/driveApi.ts`.
Cookie отправляется с запросами автоматически. Токены в localStorage не хранятся. При ошибках API показывается ошибка, демонстрационные данные не используются.

Контракт: [AUTH_TASKS.md](../AUTH_TASKS.md). Аккаунты и запуск: [README.md](../README.md).
