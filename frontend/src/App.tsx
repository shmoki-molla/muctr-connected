import { useEffect, useMemo, useState } from 'react';
import { loadDrive, type DriveApiResponse } from './api/driveApi';
import { files as mockFiles, folders as mockFolders, type DriveFile, type DriveFolder, type FileKind } from './data/mockDrive';

type ViewMode = 'grid' | 'list';

type IconName =
  | 'upload'
  | 'folder'
  | 'share'
  | 'send'
  | 'trash'
  | 'settings'
  | 'collapse'
  | 'search'
  | 'help'
  | 'bell'
  | 'chevronDown'
  | 'chevronRight'
  | 'grid'
  | 'list'
  | 'more'
  | 'menu'
  | 'plus';

const navItems: Array<{ icon: IconName; label: string; active?: boolean }> = [
  { icon: 'folder', label: 'Мой диск', active: true },
  { icon: 'share', label: 'Доступные мне' },
  { icon: 'send', label: 'Отправленные' },
  { icon: 'trash', label: 'Корзина' },
];

const recentItems = ['Проекты', 'Курсовая работа', 'Достижения'];

function Icon({ name, className = '' }: { name: IconName; className?: string }) {
  const common = {
    width: 22,
    height: 22,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 1.9,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    className,
  };

  switch (name) {
    case 'upload':
      return (
        <svg {...common}>
          <path d="M12 16V6" />
          <path d="m8 10 4-4 4 4" />
          <path d="M20 16.5A3.5 3.5 0 0 1 16.5 20h-9a3.5 3.5 0 0 1-.8-6.9A5.2 5.2 0 0 1 17 12.1" />
        </svg>
      );
    case 'folder':
      return (
        <svg {...common}>
          <path d="M3.5 6.8h6l1.7 2h9.3v8.9a2 2 0 0 1-2 2h-15z" />
          <path d="M3.5 8.8V6.7a1.7 1.7 0 0 1 1.7-1.7h4.2l1.8 2h7.3a2 2 0 0 1 2 2v.3" />
        </svg>
      );
    case 'share':
      return (
        <svg {...common}>
          <circle cx="8" cy="8" r="3" />
          <path d="M2.8 20a5.3 5.3 0 0 1 10.4 0" />
          <circle cx="17" cy="10" r="2" />
          <path d="M20.8 19a3.9 3.9 0 0 0-5.1-3.7" />
        </svg>
      );
    case 'send':
      return (
        <svg {...common}>
          <path d="m21 3-8.2 18-2.4-8.4L2 10.2z" />
          <path d="M21 3 10.4 12.6" />
        </svg>
      );
    case 'trash':
      return (
        <svg {...common}>
          <path d="M4 7h16" />
          <path d="M10 11v6" />
          <path d="M14 11v6" />
          <path d="M6 7l1 14h10l1-14" />
          <path d="M9 7V4h6v3" />
        </svg>
      );
    case 'settings':
      return (
        <svg {...common}>
          <circle cx="12" cy="12" r="3" />
          <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1-2 2-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5v.2h-2.9V20a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1-2-2 .1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H5.8v-2.9H6a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1 2-2 .1.1a1.7 1.7 0 0 0 1.8.3 1.7 1.7 0 0 0 1-1.5V3.8h2.9V4a1.7 1.7 0 0 0 1.1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1 2 2-.1.1a1.7 1.7 0 0 0-.3 1.8 1.7 1.7 0 0 0 1.5 1h.2v2.9H21a1.7 1.7 0 0 0-1.6 1.1Z" />
        </svg>
      );
    case 'collapse':
      return (
        <svg {...common}>
          <path d="m11 17-5-5 5-5" />
          <path d="m18 17-5-5 5-5" />
        </svg>
      );
    case 'search':
      return (
        <svg {...common}>
          <circle cx="11" cy="11" r="7" />
          <path d="m20 20-3.5-3.5" />
        </svg>
      );
    case 'help':
      return (
        <svg {...common}>
          <circle cx="12" cy="12" r="9" />
          <path d="M9.8 9.4a2.3 2.3 0 0 1 4.4 1c0 1.8-2.2 1.8-2.2 3.4" />
          <path d="M12 17.2h.01" />
        </svg>
      );
    case 'bell':
      return (
        <svg {...common}>
          <path d="M18 8a6 6 0 1 0-12 0c0 7-3 7-3 7h18s-3 0-3-7" />
          <path d="M10 19a2 2 0 0 0 4 0" />
        </svg>
      );
    case 'chevronDown':
      return (
        <svg {...common}>
          <path d="m7 10 5 5 5-5" />
        </svg>
      );
    case 'chevronRight':
      return (
        <svg {...common}>
          <path d="m9 18 6-6-6-6" />
        </svg>
      );
    case 'grid':
      return (
        <svg {...common}>
          <rect x="4" y="4" width="6" height="6" rx="1" />
          <rect x="14" y="4" width="6" height="6" rx="1" />
          <rect x="4" y="14" width="6" height="6" rx="1" />
          <rect x="14" y="14" width="6" height="6" rx="1" />
        </svg>
      );
    case 'list':
      return (
        <svg {...common}>
          <path d="M8 6h12" />
          <path d="M8 12h12" />
          <path d="M8 18h12" />
          <path d="M4 6h.01" />
          <path d="M4 12h.01" />
          <path d="M4 18h.01" />
        </svg>
      );
    case 'more':
      return (
        <svg {...common}>
          <circle cx="5" cy="12" r="1.2" fill="currentColor" stroke="none" />
          <circle cx="12" cy="12" r="1.2" fill="currentColor" stroke="none" />
          <circle cx="19" cy="12" r="1.2" fill="currentColor" stroke="none" />
        </svg>
      );
    case 'menu':
      return (
        <svg {...common}>
          <path d="M4 7h16" />
          <path d="M4 12h16" />
          <path d="M4 17h16" />
        </svg>
      );
    case 'plus':
      return (
        <svg {...common}>
          <path d="M12 5v14" />
          <path d="M5 12h14" />
        </svg>
      );
    default:
      return null;
  }
}

function Logo() {
  return (
    <div className="logo" aria-label="РХТУ">
      <span className="logo-text">РХТУ Диск</span>
    </div>
  );
}

function Sidebar() {
  return (
    <aside className="sidebar">
      <div className="sidebar-top">
        <Logo />
        <button className="upload-btn" type="button">
          <Icon name="upload" />
          <span>Загрузить</span>
        </button>

        <nav className="main-nav" aria-label="Основная навигация">
          {navItems.map((item) => (
            <button className={`nav-item ${item.active ? 'active' : ''}`} type="button" key={item.label}>
              <Icon name={item.icon} />
              <span>{item.label}</span>
            </button>
          ))}
        </nav>

        <div className="recent-block">
          <p className="section-label">Недавние</p>
          {recentItems.map((item) => (
            <button className="recent-item" type="button" key={item}>
              <Icon name="folder" />
              <span>{item}</span>
            </button>
          ))}
        </div>
      </div>

      <div className="sidebar-bottom">
        <div className="storage">
          <div className="storage-text">Использовано 4,2 ГБ из 15 ГБ</div>
          <div className="storage-line">
            <span />
          </div>
        </div>
        <button className="side-action" type="button">
          <Icon name="settings" />
          <span>Настройки</span>
        </button>
        <button className="side-action" type="button">
          <Icon name="collapse" />
          <span>Свернуть меню</span>
        </button>
      </div>
    </aside>
  );
}

function Topbar() {
  return (
    <header className="topbar">
      <button className="mobile-menu" type="button" aria-label="Открыть меню">
        <Icon name="menu" />
      </button>
      <div className="mobile-logo-wrap">
        <Logo />
      </div>
      <div className="topbar-spacer" />
      <button className="icon-btn" type="button" aria-label="Помощь">
        <Icon name="help" />
      </button>
      <button className="icon-btn" type="button" aria-label="Уведомления">
        <Icon name="bell" />
      </button>
      <button className="profile" type="button">
        <span className="avatar">ИИ</span>
        <span className="profile-text">
          <b>Иванов И.И.</b>
          <span>Студент</span>
        </span>
        <Icon name="chevronDown" />
      </button>
    </header>
  );
}

function FolderCard({ folder, selected, onOpen }: { folder: DriveFolder; selected: boolean; onOpen: () => void }) {
  return (
    <button className={`folder-card ${selected ? 'selected' : ''}`} type="button" onClick={onOpen}>
      <Icon name="folder" className="folder-icon" />
      <span className="folder-info">
        <b>{folder.name}</b>
        <small>{folder.count} файлов</small>
      </span>
      <Icon name="more" className="more-icon" />
    </button>
  );
}

function FileBadge({ kind }: { kind: FileKind }) {
  const labels: Record<FileKind, string> = {
    pdf: 'PDF',
    image: 'IMG',
    docx: 'W',
    pptx: 'P',
    xlsx: 'X',
    zip: 'ZIP',
  };

  return <span className={`file-badge ${kind}`}>{labels[kind]}</span>;
}

function FileRow({ file }: { file: DriveFile }) {
  return (
    <div className="file-row" role="row">
      <div className="file-cell name-cell" role="cell">
        <FileBadge kind={file.kind} />
        <span>{file.name}</span>
      </div>
      <div className="file-cell owner-cell" role="cell">
        {file.owner}
      </div>
      <div className="file-cell date-cell" role="cell">
        {file.modified}
      </div>
      <div className="file-cell size-cell" role="cell">
        {file.size}
      </div>
      <button className="row-more" type="button" aria-label={`Действия с файлом ${file.name}`}>
        <Icon name="more" />
      </button>
    </div>
  );
}

function DriveContent() {
  const [folders, setFolders] = useState<DriveFolder[]>(mockFolders);
  const [files, setFiles] = useState<DriveFile[]>(mockFiles);
  const [apiState, setApiState] = useState<Pick<DriveApiResponse, 'source' | 'databaseConnected' | 'message'>>({
    source: 'mock',
    databaseConnected: false,
    message: 'Пока показаны локальные заглушки из React.',
  });
  const [currentFolderId, setCurrentFolderId] = useState(mockFolders[0]?.id ?? '');
  const [query, setQuery] = useState('');
  const [viewMode, setViewMode] = useState<ViewMode>('grid');

  useEffect(() => {
    let cancelled = false;

    loadDrive()
      .then((data) => {
        if (cancelled) return;

        const nextFolders = data.folders.length > 0 ? data.folders : mockFolders;
        const nextFiles = data.files.length > 0 ? data.files : mockFiles;

        setFolders(nextFolders);
        setFiles(nextFiles);
        setApiState({
          source: data.source,
          databaseConnected: data.databaseConnected,
          message: data.message,
        });
        setCurrentFolderId((current) =>
          nextFolders.some((folder) => folder.id === current) ? current : nextFolders[0]?.id ?? current,
        );
      })
      .catch((error: Error) => {
        if (cancelled) return;
        setApiState({
          source: 'react-mock',
          databaseConnected: false,
          message: `Go API не ответил: ${error.message}. Показаны локальные заглушки.`,
        });
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const currentFolder = folders.find((folder) => folder.id === currentFolderId) ?? folders[0];
  const filteredFiles = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();

    return files
      .filter((file) => file.folderId === currentFolder?.id)
      .filter((file) => (normalizedQuery ? file.name.toLowerCase().includes(normalizedQuery) : true));
  }, [currentFolder?.id, files, query]);

  return (
    <main className="content">
      <div className="breadcrumbs" aria-label="Навигационная цепочка">
        <button type="button" onClick={() => setCurrentFolderId(folders[0]?.id ?? '')}>
          Мой диск
        </button>
        <Icon name="chevronRight" />
        <span>{currentFolder?.name ?? 'Файлы'}</span>
      </div>

      <div className={`api-status ${apiState.databaseConnected ? 'connected' : 'fallback'}`}>
        <b>{apiState.databaseConnected ? 'Подключено к PostgreSQL' : 'Демо-режим'}</b>
        <span>Источник: {apiState.source}</span>
        {apiState.message ? <small>{apiState.message}</small> : null}
      </div>

      <div className="toolbar">
        <label className="search-box">
          <Icon name="search" />
          <input
            type="search"
            value={query}
            placeholder="Поиск по файлам и папкам"
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>

        <button className="sort-btn" type="button">
          По названию
          <Icon name="chevronDown" />
        </button>

        <div className="view-switch" aria-label="Смена вида">
          <button className={viewMode === 'grid' ? 'active' : ''} type="button" onClick={() => setViewMode('grid')}>
            <Icon name="grid" />
          </button>
          <button className={viewMode === 'list' ? 'active' : ''} type="button" onClick={() => setViewMode('list')}>
            <Icon name="list" />
          </button>
        </div>
      </div>

      <section className="section-block">
        <div className="mobile-section-title">
          <h2>Папки</h2>
          <button type="button" aria-label="Показать списком">
            <Icon name="list" />
          </button>
        </div>
        <h2 className="desktop-section-title">Папки</h2>
        <div className={`folders ${viewMode === 'list' ? 'folders-list' : ''}`}>
          {folders.map((folder) => (
            <FolderCard
              key={folder.id}
              folder={folder}
              selected={folder.id === currentFolder.id}
              onOpen={() => setCurrentFolderId(folder.id)}
            />
          ))}
        </div>
      </section>

      <section className="section-block files-section">
        <h2>Файлы</h2>
        <div className="file-list" role="table" aria-label="Список файлов">
          <div className="file-head" role="row">
            <div>Название</div>
            <div>Владелец</div>
            <div className="head-sort">
              Изменено
              <Icon name="chevronDown" />
            </div>
            <div>Размер</div>
            <div />
          </div>

          <div className="file-scroll">
            {filteredFiles.length > 0 ? (
              filteredFiles.map((file) => <FileRow file={file} key={file.id} />)
            ) : (
              <div className="empty-state">Файлы не найдены</div>
            )}
          </div>
        </div>
      </section>
    </main>
  );
}

function BottomNav() {
  const items: Array<{ icon: IconName; label: string; active?: boolean }> = [
    { icon: 'folder', label: 'Мой диск', active: true },
    { icon: 'share', label: 'Доступные' },
    { icon: 'upload', label: 'Загрузить' },
    { icon: 'send', label: 'Отправленные' },
    { icon: 'more', label: 'Ещё' },
  ];

  return (
    <nav className="bottom-nav" aria-label="Мобильная навигация">
      {items.map((item) => (
        <button className={item.active ? 'active' : ''} type="button" key={item.label}>
          <Icon name={item.icon} />
          <span>{item.label}</span>
        </button>
      ))}
    </nav>
  );
}

export default function App() {
  return (
    <div className="app-shell">
      <Sidebar />
      <div className="workspace">
        <Topbar />
        <DriveContent />
      </div>
      <button className="floating-add" type="button" aria-label="Добавить файл">
        <Icon name="plus" />
      </button>
      <BottomNav />
    </div>
  );
}
