import { saveFile } from './api';
import type { FileUploadResult } from './types';

export interface FilePreview {
    file: File;
    url: string;
    type: 'image' | 'video' | 'text';
    name: string;
}

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

export async function uploadFile(file: File, overwrite: boolean = false): Promise<FileUploadResult> {
    return saveFile(file, overwrite);
}

export async function uploadFiles(files: File[], overwrite: boolean = false): Promise<FileUploadResult[]> {
    return Promise.all(files.map(file => uploadFile(file, overwrite)));
}

export function revokeFilePreview(preview: FilePreview): void {
    if (preview.url.startsWith('blob:')) {
        URL.revokeObjectURL(preview.url);
    }
}

export function revokeAllFilePreviews(previews: FilePreview[]): void {
    previews.forEach(revokeFilePreview);
}

export function getAcceptedFileTypes(): string {
    return '.jpg,.jpeg,.png,.gif,.webp,.mp4,.avi,.mov,.wmv,.txt,.md,.json';
}

export function isImageFile(file: File): boolean {
    return file.type.startsWith('image/');
}

export function isVideoFile(file: File): boolean {
    return file.type.startsWith('video/');
}

export function getVideoThumbnail(file: File): Promise<string> {
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