import type { DriveFile, DriveFolder } from '../data/mockDrive';

export type DriveApiResponse = {
  folders: DriveFolder[];
  files: DriveFile[];
  source: string;
  databaseConnected: boolean;
  message?: string;
};

const API_BASE_URL = import.meta.env.VITE_API_URL ?? '';

export async function loadDrive(): Promise<DriveApiResponse> {
  const response = await fetch(`${API_BASE_URL}/api/drive`, {
    headers: { Accept: 'application/json' },
  });

  if (!response.ok) {
    throw new Error(`API вернул ${response.status}`);
  }

  return response.json();
}
