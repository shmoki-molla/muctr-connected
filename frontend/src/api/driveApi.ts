export type User = { id: string; email: string; role: 'student' | 'admin'; studentId: string | null; fullName: string };
export type Discipline = { id: string; name: string; description: string };
export type WorkType = { id: number; name: string };
export type Student = { id: string; fullName: string; email: string; enrollmentYear: number; fileCount: number };
export type DriveFile = {
 id: string; title: string; name: string; owner: string; studentId: string;
 disciplineId: string; disciplineName: string; workTypeId: number; workTypeName: string;
 semester: number; academicYearStart: number; fileSize: number; mimeType: string; uploadedAt: string;
};
export type DriveResponse = { files: DriveFile[]; total: number; page: number; pageSize: number; stats: { count: number; bytes: number } };
export class ApiError extends Error { constructor(message: string, public status: number) { super(message); } }
const base = import.meta.env.VITE_API_URL ?? '';
function expired(status: number) { if (status === 401) window.dispatchEvent(new Event('auth-expired')); }
export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
 const headers = new Headers(options.headers);
 headers.set('Accept', 'application/json');
 if (options.body && !(options.body instanceof FormData)) headers.set('Content-Type', 'application/json');
 if (options.method && options.method !== 'GET') headers.set('X-MUCTR-Request', '1');
 const response = await fetch(base + path, { ...options, headers, credentials: 'include' });
 if (!response.ok) {
  expired(response.status);
  const data = await response.json().catch(() => ({}));
  throw new ApiError(data.error || 'Сервер недоступен. Повторите попытку.', response.status);
 }
 if (response.status === 204) return undefined as T;
 return response.json();
}
export function uploadFile(data: FormData, onProgress: (percent: number) => void): Promise<void> {
 return new Promise((resolve, reject) => {
  const xhr = new XMLHttpRequest();
  xhr.open('POST', base + '/api/files');
  xhr.withCredentials = true;
  xhr.timeout = 180000;
  xhr.setRequestHeader('X-MUCTR-Request', '1');
  xhr.upload.onprogress = (event) => { if (event.lengthComputable) onProgress(Math.round(event.loaded / event.total * 100)); };
  xhr.onerror = () => reject(new Error('Соединение прервано. Обновите список перед повторной загрузкой.'));
  xhr.ontimeout = () => reject(new Error('Сервер не ответил вовремя. Обновите список перед повторной загрузкой.'));
  xhr.onload = () => {
   if (xhr.status >= 200 && xhr.status < 300) { resolve(); return; }
   expired(xhr.status);
   let message = 'Не удалось загрузить файл';
   try { message = JSON.parse(xhr.responseText).error || message; } catch { /* nginx can return HTML */ }
   reject(new ApiError(message, xhr.status));
  };
  xhr.send(data);
 });
}
export async function downloadFile(file: DriveFile): Promise<void> {
 const response = await fetch(base + '/api/files/' + file.id + '/download', { credentials: 'include' });
 if (!response.ok) {
  expired(response.status);
  const data = await response.json().catch(() => ({}));
  throw new ApiError(data.error || 'Не удалось скачать файл', response.status);
 }
 const url = URL.createObjectURL(await response.blob());
 const link = document.createElement('a'); link.href = url; link.download = file.name;
 document.body.appendChild(link); link.click(); link.remove();
 window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
