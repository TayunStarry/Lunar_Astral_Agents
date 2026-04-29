import { FilePreview, SaveFileResponse } from './types';
import { encodeFileName } from './utils';

export function createFilePreview(file: File): FilePreview {
  return {
    file,
    url: URL.createObjectURL(file),
    type: getFileType(file),
    name: file.name,
  };
}

export function getFileType(file: File): 'image' | 'video' | 'text' {
  if (file.type.startsWith('image/')) return 'image';
  if (file.type.startsWith('video/')) return 'video';
  return 'text';
}

export function isMediaFile(file: File): boolean {
  return file.type.startsWith('image/') || file.type.startsWith('video/');
}

export function revokeFilePreview(preview: FilePreview): void {
  if (preview.url.startsWith('blob:')) {
    URL.revokeObjectURL(preview.url);
  }
}

export function revokeAllFilePreviews(previews: FilePreview[]): void {
  previews.forEach(revokeFilePreview);
}

export async function getVideoThumbnail(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const video = document.createElement('video');
    video.preload = 'metadata';
    video.muted = true;

    video.onloadeddata = () => {
      video.currentTime = 1;
    };

    video.onseeked = () => {
      const canvas = document.createElement('canvas');
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      const ctx = canvas.getContext('2d');
      if (ctx) {
        ctx.drawImage(video, 0, 0);
        resolve(canvas.toDataURL('image/jpeg'));
      } else {
        reject(new Error('Failed to get video context'));
      }
    };

    video.onerror = () => {
      reject(new Error('Failed to load video'));
    };

    video.src = URL.createObjectURL(file);
  });
}

export async function saveFile(file: File, overwrite = false): Promise<SaveFileResponse> {
  const encodedFileName = encodeFileName(file.name);

  const response = await fetch('/save', {
    method: 'POST',
    headers: {
      'X-File-Name': encodedFileName,
      'X-Overwrite': overwrite.toString(),
      'Content-Length': file.size.toString(),
    },
    body: file,
  });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({ message: 'Upload failed' }));
    throw new Error(errorData.message || `HTTP error! status: ${response.status}`);
  }

  return response.json();
}

export async function sendMessages(messages: { role: string; content: unknown }[]): Promise<{ success: boolean; length: number }> {
  const response = await fetch('/write/message', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ messages }),
  });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({ message: 'Request failed' }));
    throw new Error(errorData.message || `HTTP error! status: ${response.status}`);
  }

  return response.json();
}

export async function fetchLive2DSetting(): Promise<{ name?: string; url?: string; scale?: number; x?: number; y?: number; autoInteract?: boolean }> {
  try {
    const response = await fetch('/read/resources/live2d/setting.json');
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    const rawText = await response.text();
    const jsonText = rawText
      .replace(/\/\/.*$/gm, '')
      .replace(/\/\*[\s\S]*?\*\//g, '')
      .replace(/'/g, '"');
    return JSON.parse(jsonText);
  } catch (error) {
    console.error('Failed to fetch Live2D setting:', error);
    return {};
  }
}
