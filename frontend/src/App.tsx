import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { api, ApiError, downloadFile, uploadFile, type User, type Discipline, type WorkType, type Student, type DriveFile, type DriveResponse } from './api/driveApi';

const currentYear = new Date().getFullYear();
const academicYear = new Date().getMonth() >= 8 ? currentYear : currentYear - 1;
const emptyDrive: DriveResponse = { files: [], total: 0, page: 1, pageSize: 20, stats: { count: 0, bytes: 0 } };
function message(error: unknown) { return error instanceof Error ? error.message : 'Не удалось выполнить действие'; }
function bytes(value: number) { return value >= 1024 * 1024 ? (value / 1024 / 1024).toLocaleString('ru', { maximumFractionDigits: 1 }) + ' МБ' : Math.ceil(value / 1024) + ' КБ'; }
function Logo() { return <div className="brand"><span className="brand-mark">РХ</span><span>РХТУ Диск<small>Студенческое портфолио</small></span></div>; }
function Spinner({ children = 'Загрузка…' }: { children?: ReactNode }) { return <div className="loading" role="status"><span className="spinner" />{children}</div>; }
function Alert({ children }: { children: ReactNode }) { return <div className="alert-error" role="alert">{children}</div>; }

function Auth({ onUser }: { onUser: (user: User) => void }) {
 const [register, setRegister] = useState(false);
 const [busy, setBusy] = useState(false);
 const [error, setError] = useState('');
 const [showPassword, setShowPassword] = useState(false);
 async function submit(event: FormEvent<HTMLFormElement>) {
  event.preventDefault(); setError(''); setBusy(true);
  const form = new FormData(event.currentTarget);
  const data: Record<string, string | number> = { email: String(form.get('email')).trim(), password: String(form.get('password')) };
  if (register) {
   if (form.get('password') !== form.get('confirmPassword')) { setError('Пароли не совпадают'); setBusy(false); return; }
   if (new TextEncoder().encode(String(form.get('password'))).length > 72) { setError('Пароль должен занимать не более 72 байт'); setBusy(false); return; }
   Object.assign(data, { firstName: form.get('firstName'), lastName: form.get('lastName'), middleName: form.get('middleName') || '', enrollmentYear: Number(form.get('enrollmentYear')) });
  }
  try { onUser(await api<User>('/api/auth/' + (register ? 'register' : 'login'), { method: 'POST', body: JSON.stringify(data) })); }
  catch (err) { setError(message(err)); } finally { setBusy(false); }
 }
 return <div className="auth-layout">
  <aside className="auth-story"><Logo /><div><span className="eyebrow">ВАШ УЧЕБНЫЙ АРХИВ</span><h1>Все работы.<br />Одно портфолио.</h1><p>Сохраняйте лабораторные, конспекты и проекты. Возвращайтесь к своим материалам в любой момент.</p><div className="story-tags"><span>По предметам</span><span>По семестрам</span><span>Только ваши файлы</span></div></div><small>РХТУ · Цифровое пространство студента</small></aside>
  <main className="auth-main"><div className="auth-card"><div className="auth-mobile-brand"><Logo /></div><span className="eyebrow">ДОБРО ПОЖАЛОВАТЬ</span>
   <h2>{register ? 'Создать аккаунт' : 'Войти в портфолио'}</h2><p className="muted">{register ? 'Зарегистрируйтесь, чтобы сохранить первую работу.' : 'Введите email и пароль вашего аккаунта.'}</p>
   <div className="auth-tabs"><button type="button" className={!register ? 'selected' : ''} disabled={busy} onClick={() => { setRegister(false); setError(''); }}>Вход</button><button type="button" className={register ? 'selected' : ''} disabled={busy} onClick={() => { setRegister(true); setError(''); }}>Регистрация</button></div>
   <form onSubmit={submit} key={String(register)}>
    {register && <><div className="form-row"><label>Фамилия<input name="lastName" autoComplete="family-name" maxLength={100} required /></label><label>Имя<input name="firstName" autoComplete="given-name" maxLength={100} required /></label></div>
     <div className="form-row"><label>Отчество <small>необязательно</small><input name="middleName" autoComplete="additional-name" maxLength={100} /></label><label>Год поступления<input name="enrollmentYear" type="number" min={2000} max={currentYear + 1} defaultValue={currentYear} required /></label></div></>}
    <label>Email<input name="email" type="email" autoComplete="email" maxLength={255} placeholder="student@example.ru" required /></label>
    <label>Пароль<div className="password-field"><input name="password" type={showPassword ? 'text' : 'password'} autoComplete={register ? 'new-password' : 'current-password'} minLength={register ? 10 : undefined} required /><button type="button" onClick={() => setShowPassword(!showPassword)} aria-label={showPassword ? 'Скрыть пароль' : 'Показать пароль'}>{showPassword ? 'Скрыть' : 'Показать'}</button></div></label>
    {register && <><small className="field-hint">От 10 символов. Для кириллицы лимит в 72 байта достигается быстрее.</small><label>Повторите пароль<input name="confirmPassword" type={showPassword ? 'text' : 'password'} autoComplete="new-password" required /></label></>}
    {error && <Alert>{error}</Alert>}
    <button className="primary full" disabled={busy}>{busy ? 'Подождите…' : register ? 'Зарегистрироваться' : 'Войти'}</button>
   </form>
   <p className="auth-footnote">Для администратора используется та же форма входа.</p>
  </div></main>
 </div>;
}

function Modal({ title, children, onClose, busy = false }: { title: string; children: ReactNode; onClose: () => void; busy?: boolean }) {
 const ref = useRef<HTMLDialogElement>(null);
 useEffect(() => { const dialog = ref.current!; dialog.showModal(); return () => dialog.close(); }, []);
 return <dialog ref={ref} className="modal-card" aria-labelledby="modal-title" onCancel={(e) => { e.preventDefault(); if (!busy) onClose(); }}>
  <div className="modal-heading"><h2 id="modal-title">{title}</h2><button className="close-button" type="button" onClick={onClose} disabled={busy} aria-label="Закрыть">×</button></div>{children}
 </dialog>;
}
function Upload({ subjects, types, onClose, onDone }: { subjects: Discipline[]; types: WorkType[]; onClose: () => void; onDone: () => void }) {
 const [busy, setBusy] = useState(false); const [progress, setProgress] = useState(0); const [error, setError] = useState(''); const [file, setFile] = useState<File | null>(null);
 async function submit(event: FormEvent<HTMLFormElement>) {
  event.preventDefault(); setError('');
  if (!file) { setError('Выберите файл'); return; }
  if (file.size === 0 || file.size > 25 * 1024 * 1024) { setError('Размер файла должен быть от 1 байта до 25 МБ'); return; }
  const data = new FormData(event.currentTarget);
  setBusy(true); setProgress(0);
  try { await uploadFile(data, setProgress); onDone(); } catch (err) { setError(message(err)); } finally { setBusy(false); }
 }
 return <Modal title="Загрузить работу" busy={busy} onClose={onClose}>
  <p className="muted">Файл будет доступен вам и администратору.</p>
  <form onSubmit={submit}><fieldset disabled={busy}>
   <label>Название работы <small>необязательно</small><input name="title" maxLength={255} placeholder="Например, лабораторная № 3" /></label>
   <label>Предмет<select name="disciplineId" defaultValue="" required><option value="" disabled>Выберите предмет</option>{subjects.map(s => <option value={s.id} key={s.id}>{s.name}</option>)}</select></label>
   <label>Тип работы<select name="workTypeId" defaultValue="3" required>{types.map(t => <option value={t.id} key={t.id}>{t.name}</option>)}</select></label>
   <div className="form-row"><label>Семестр<select name="semester" defaultValue={new Date().getMonth() >= 8 || new Date().getMonth() === 0 ? '1' : '2'}><option value="1">1 · Осенний</option><option value="2">2 · Весенний</option></select></label><label>Начало учебного года<input name="academicYearStart" type="number" min={2000} max={currentYear + 1} defaultValue={academicYear} required /></label></div>
   <label className="file-picker">Выберите файл<input type="file" name="file" accept=".pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.csv,.png,.jpg,.jpeg,.zip" required onChange={e => setFile(e.target.files?.[0] || null)} /><span>{file ? file.name + ' · ' + bytes(file.size) : 'PDF, документы, таблицы, изображения или ZIP · до 25 МБ'}</span></label>
   {subjects.length === 0 && <Alert>Предметов пока нет. Попросите администратора добавить предмет.</Alert>}
  </fieldset>
  {busy && <div className="upload-progress" role="status"><progress max={100} value={progress} /><small>{progress === 100 ? 'Сохраняем работу…' : 'Передача файла: ' + progress + '%'}</small></div>}
  {error && <Alert>{error}</Alert>}
  <div className="modal-actions"><button type="button" className="secondary" disabled={busy} onClick={onClose}>Отмена</button><button className="primary" disabled={busy || !subjects.length || !types.length}>{busy ? 'Загружаем…' : 'Сохранить работу'}</button></div>
  </form>
 </Modal>;
}
type View = 'files' | 'subjects' | 'students';
function Workspace({ user, onLogout }: { user: User; onLogout: () => void }) {
 const admin = user.role === 'admin';
 const [view, setView] = useState<View>('files');
 const [subjects, setSubjects] = useState<Discipline[]>([]); const [types, setTypes] = useState<WorkType[]>([]); const [students, setStudents] = useState<Student[]>([]);
 const [catalogReady, setCatalogReady] = useState(false);
 const [drive, setDrive] = useState<DriveResponse>(emptyDrive); const [loading, setLoading] = useState(true); const [driveError, setDriveError] = useState('');
 const [error, setError] = useState(''); const [notice, setNotice] = useState('');
 const [revision, setRevision] = useState(0); const [upload, setUpload] = useState(false);
 const [query, setQuery] = useState(''); const [search, setSearch] = useState('');
 const [subject, setSubject] = useState(''); const [workType, setWorkType] = useState(''); const [semester, setSemester] = useState(''); const [student, setStudent] = useState(''); const [page, setPage] = useState(1);
 const [pendingDelete, setPendingDelete] = useState<{ id: string; name: string; kind: 'file' | 'subject' } | null>(null);
 const [busy, setBusy] = useState(false); const [downloading, setDownloading] = useState('');
 useEffect(() => { const timer = window.setTimeout(() => { setSearch(query); setPage(1); }, 300); return () => clearTimeout(timer); }, [query]);
 useEffect(() => {
  const controller = new AbortController(); setError(''); setCatalogReady(false);
  Promise.all([api<Discipline[]>('/api/disciplines', { signal: controller.signal }), api<WorkType[]>('/api/work-types', { signal: controller.signal }), admin ? api<Student[]>('/api/students', { signal: controller.signal }) : Promise.resolve([])])
   .then(([s, t, people]) => { setSubjects(s); setTypes(t); setStudents(people); setCatalogReady(true); })
   .catch(err => { if (!controller.signal.aborted) setError(message(err)); });
  return () => controller.abort();
 }, [revision, admin]);
 useEffect(() => {
  const controller = new AbortController(); setLoading(true); setDriveError('');
  const params = new URLSearchParams({ q: search, subject, type: workType, semester, student, page: String(page) });
  api<DriveResponse>('/api/drive?' + params, { signal: controller.signal }).then(data => {
   if (controller.signal.aborted) return;
   if (page > 1 && data.files.length === 0) { setPage(page - 1); return; }
   setDrive(data); setLoading(false);
  }).catch(err => { if (!controller.signal.aborted) { setDriveError(message(err)); setLoading(false); } });
  return () => controller.abort();
 }, [search, subject, workType, semester, student, page, revision]);
 function refresh() { setRevision(v => v + 1); }
 function filter(setter: (value: string) => void, value: string) { setter(value); setPage(1); }
 async function logout() { setBusy(true); setError(''); try { await api('/api/auth/logout', { method: 'POST' }); onLogout(); } catch (err) { setError(message(err)); } finally { setBusy(false); } }
 async function download(file: DriveFile) { setDownloading(file.id); setError(''); try { await downloadFile(file); } catch (err) { setError(message(err)); } finally { setDownloading(''); } }
 async function remove() {
  if (!pendingDelete) return; setBusy(true); setError('');
  try { await api('/api/' + (pendingDelete.kind === 'file' ? 'files/' : 'disciplines/') + pendingDelete.id, { method: 'DELETE' }); setPendingDelete(null); setNotice('Удалено'); refresh(); }
  catch (err) { setPendingDelete(null); setError(message(err)); } finally { setBusy(false); }
 }
 async function addSubject(event: FormEvent<HTMLFormElement>) {
  event.preventDefault(); const form = event.currentTarget; const data = new FormData(form); setBusy(true); setError('');
  try { await api('/api/disciplines', { method: 'POST', body: JSON.stringify({ name: data.get('name'), description: data.get('description') }) }); form.reset(); setNotice('Предмет добавлен'); refresh(); }
  catch (err) { setError(message(err)); } finally { setBusy(false); }
 }
 const initials = user.fullName.split(' ').slice(0, 2).map(s => s[0]).join('');
 return <div className="app-shell">
  <aside className="sidebar"><Logo /><div className="sidebar-label">{admin ? 'АДМИНИСТРАТОР' : 'ЛИЧНОЕ ПРОСТРАНСТВО'}</div>
   {!admin && <button className="primary" onClick={() => setUpload(true)} disabled={!catalogReady}>＋ Загрузить работу</button>}
   <nav aria-label="Разделы"><button className={view === 'files' ? 'active' : ''} onClick={() => setView('files')}>▤ <span>{admin ? 'Все работы' : 'Мои работы'}</span><b>{drive.stats.count}</b></button>
    {admin && <><button className={view === 'subjects' ? 'active' : ''} onClick={() => setView('subjects')}>▦ <span>Предметы</span></button><button className={view === 'students' ? 'active' : ''} onClick={() => setView('students')}>♙ <span>Студенты</span></button></>}
   </nav>
   <div className="sidebar-note"><span className="status-dot" />{admin ? 'Обзор портфолио всех студентов' : 'Ваши учебные материалы в одном месте'}<p>{admin ? 'Управляйте предметами и просматривайте загруженные работы.' : 'Работы видны вам и администратору проекта.'}</p></div>
  </aside>
  <div className="workspace"><header className="topbar"><span className="topbar-title">{admin ? 'Панель администратора' : 'Моё портфолио'}</span><div className="profile"><span className="avatar">{initials}</span><span><strong>{user.fullName}</strong><small>{user.email}</small></span><span className="role-badge">{admin ? 'Админ' : 'Студент'}</span></div><button className="secondary logout" onClick={logout} disabled={busy}>Выйти</button></header>
  <main className="content">
   <div className="page-heading"><div><span className="eyebrow">{admin ? 'УПРАВЛЕНИЕ' : 'ПОРТФОЛИО'}</span><h1>{view === 'subjects' ? 'Учебные предметы' : view === 'students' ? 'Студенты' : admin ? 'Все загруженные работы' : 'Мои работы'}</h1><p className="muted">{view === 'files' ? 'Находите материалы по предмету, типу работы и семестру.' : view === 'subjects' ? 'Предметы из этого списка доступны студентам при загрузке.' : 'Откройте портфолио любого студента.'}</p></div>
    <div className="heading-actions"><button className="secondary" onClick={refresh} disabled={loading}>Обновить</button>{!admin && <button className="primary" onClick={() => setUpload(true)} disabled={!catalogReady}>＋ Загрузить работу</button>}</div>
   </div>
   {error && <Alert>{error}</Alert>}
   {notice && <div className="notice" role="status">{notice}<button aria-label="Скрыть сообщение" onClick={() => setNotice('')}>×</button></div>}
   {view === 'files' && <>
    <div className="stats"><div><small>{admin ? 'Работ в хранилище' : 'Загружено работ'}</small><strong>{drive.stats.count}</strong></div><div><small>Объём файлов</small><strong>{bytes(drive.stats.bytes)}</strong></div><div><small>Доступно предметов</small><strong>{subjects.length}</strong></div></div>
    <section className="panel"><div className="filters"><label className="search-filter"><span>Поиск</span><input type="search" value={query} onChange={e => setQuery(e.target.value)} placeholder={admin ? 'Работа, предмет или студент' : 'Название работы или предмет'} /></label>
     <label><span>Предмет</span><select value={subject} onChange={e => filter(setSubject, e.target.value)}><option value="">Все предметы</option>{subjects.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
     <label><span>Тип работы</span><select value={workType} onChange={e => filter(setWorkType, e.target.value)}><option value="">Все типы</option>{types.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}</select></label>
     <label><span>Семестр</span><select value={semester} onChange={e => filter(setSemester, e.target.value)}><option value="">Все</option><option value="1">1</option><option value="2">2</option></select></label>
     {admin && <label><span>Студент</span><select value={student} onChange={e => filter(setStudent, e.target.value)}><option value="">Все студенты</option>{students.map(s => <option key={s.id} value={s.id}>{s.fullName}</option>)}</select></label>}
    </div>
    {loading ? <Spinner /> : driveError ? <div className="empty-state"><Alert>{driveError}</Alert><button className="secondary" onClick={refresh}>Повторить</button></div> : drive.files.length === 0 ? <div className="empty-state"><div className="empty-symbol">▤</div><h2>{search || subject || workType || semester || student ? 'Работы не найдены' : 'Здесь будут ваши работы'}</h2><p>{search || subject || workType || semester || student ? 'Попробуйте изменить условия поиска.' : admin ? 'Студенты ещё не загрузили ни одного файла.' : 'Загрузите первую лабораторную, конспект или проект.'}</p>{!admin && <button className="primary" onClick={() => setUpload(true)} disabled={!catalogReady}>Загрузить работу</button>}</div> :
    <div className="table-scroll"><table><thead><tr><th>Работа</th>{admin && <th>Студент</th>}<th>Предмет и тип</th><th>Семестр</th><th>Загружено</th><th>Действия</th></tr></thead><tbody>{drive.files.map(file => <tr key={file.id}>
     <td><div className="file-title"><span className={'file-badge ' + (file.mimeType.includes('pdf') ? 'pdf' : '')}>{file.name.split('.').pop()?.toUpperCase().slice(0, 4)}</span><span><strong>{file.title || file.name}</strong><small>{file.name} · {bytes(file.fileSize)}</small></span></div></td>
     {admin && <td>{file.owner}</td>}<td><span>{file.disciplineName}</span><small>{file.workTypeName}</small></td><td>{file.semester}<small>{file.academicYearStart}–{file.academicYearStart + 1}</small></td><td className="nowrap">{new Date(file.uploadedAt).toLocaleDateString('ru-RU')}</td>
     <td><div className="row-actions"><button className="text-button" onClick={() => download(file)} disabled={!!downloading} aria-label={'Скачать ' + file.name}>{downloading === file.id ? 'Скачиваем…' : 'Скачать'}</button><button className="text-button danger" onClick={() => setPendingDelete({ id: file.id, name: file.title || file.name, kind: 'file' })} aria-label={'Удалить ' + file.name}>Удалить</button></div></td>
    </tr>)}</tbody></table></div>}
    {!loading && !driveError && drive.total > 0 && <div className="pagination"><span>Найдено: {drive.total}</span><div><button className="secondary" disabled={page <= 1} onClick={() => setPage(page - 1)}>Назад</button><span>{page} / {Math.max(1, Math.ceil(drive.total / drive.pageSize))}</span><button className="secondary" disabled={page * drive.pageSize >= drive.total} onClick={() => setPage(page + 1)}>Далее</button></div></div>}
    </section>
   </>}
   {admin && view === 'subjects' && <div className="admin-columns"><section className="panel form-panel"><h2>Добавить предмет</h2><form onSubmit={addSubject}><label>Название<input name="name" placeholder="Например, органическая химия" maxLength={255} required /></label><label>Описание <small>необязательно</small><textarea name="description" maxLength={2000} rows={4} /></label><button className="primary" disabled={busy}>{busy ? 'Сохраняем…' : 'Добавить предмет'}</button></form></section><section className="panel subject-list"><h2>Все предметы · {subjects.length}</h2>{subjects.map(s => <article key={s.id}><div><h3>{s.name}</h3>{s.description && <p>{s.description}</p>}</div><button className="text-button danger" onClick={() => setPendingDelete({ id: s.id, name: s.name, kind: 'subject' })}>Удалить</button></article>)}{!subjects.length && <p className="muted">Список пока пуст.</p>}</section></div>}
   {admin && view === 'students' && <section className="panel table-scroll"><table><thead><tr><th>Студент</th><th>Email</th><th>Год поступления</th><th>Работ</th><th /></tr></thead><tbody>{students.map(s => <tr key={s.id}><td><strong>{s.fullName}</strong></td><td>{s.email}</td><td>{s.enrollmentYear}</td><td>{s.fileCount}</td><td><button className="text-button" onClick={() => { filter(setStudent, s.id); setView('files'); }}>Работы студента</button></td></tr>)}</tbody></table>{!students.length && <div className="empty-state">Студентов пока нет</div>}</section>}
  </main></div>
  {upload && <Upload subjects={subjects} types={types} onClose={() => setUpload(false)} onDone={() => { setUpload(false); setNotice('Работа загружена'); setView('files'); setQuery(''); setSearch(''); setSubject(''); setWorkType(''); setSemester(''); setPage(1); refresh(); }} />}
  {pendingDelete && <Modal title={pendingDelete.kind === 'file' ? 'Удалить работу?' : 'Удалить предмет?'} busy={busy} onClose={() => setPendingDelete(null)}><p><strong>{pendingDelete.name}</strong></p><p className="muted">{pendingDelete.kind === 'file' ? 'Файл будет удалён из хранилища. Это действие нельзя отменить.' : 'Предмет можно удалить, только если к нему не привязаны работы.'}</p><div className="modal-actions"><button className="secondary" disabled={busy} onClick={() => setPendingDelete(null)}>Отмена</button><button className="danger-button" disabled={busy} onClick={remove}>{busy ? 'Удаляем…' : 'Удалить'}</button></div></Modal>}
 </div>;
}
export default function App() {
 const [user, setUser] = useState<User | null>(null); const [loading, setLoading] = useState(true); const [error, setError] = useState(''); const [retry, setRetry] = useState(0);
 useEffect(() => { const onExpired = () => setUser(null); window.addEventListener('auth-expired', onExpired); return () => window.removeEventListener('auth-expired', onExpired); }, []);
 useEffect(() => {
  const controller = new AbortController(); setLoading(true); setError('');
  api<User>('/api/me', { signal: controller.signal }).then(setUser).catch(err => {
   if (!controller.signal.aborted && !(err instanceof ApiError && err.status === 401)) setError(message(err));
  }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
  return () => controller.abort();
 }, [retry]);
 if (loading) return <div className="boot"><Logo /><Spinner>Открываем портфолио…</Spinner></div>;
 if (error) return <div className="boot"><Logo /><Alert>{error}</Alert><button className="primary" onClick={() => setRetry(v => v + 1)}>Повторить подключение</button></div>;
 return user ? <Workspace key={user.id} user={user} onLogout={() => setUser(null)} /> : <Auth onUser={setUser} />;
}
