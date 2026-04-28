import type { FileUploadResult, APIResponse, OpenAIMessage } from './types';

export async function saveFile(file: File, overwrite: boolean = false): Promise<FileUploadResult> {
    const encodedFileName = btoa(unescape(encodeURIComponent(file.name)));

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

export async function fetchJSON<T>(url: string): Promise<T> {
    const response = await fetch(url);
    if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
    }
    return response.json();
}

export async function fetchText(url: string): Promise<string> {
    const response = await fetch(url);
    if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
    }
    return response.text();
}

export async function sendMessages(messages: OpenAIMessage[]): Promise<APIResponse> {
    const response = await fetch('/message/batch', {
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

export async function sendVideoUrls(urls: string[]): Promise<APIResponse> {
    const response = await fetch('/videourl/batch', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({ urls }),
    });

    if (!response.ok) {
        const errorData = await response.json().catch(() => ({ message: 'Request failed' }));
        throw new Error(errorData.message || `HTTP error! status: ${response.status}`);
    }

    return response.json();
}

export async function fetchLive2DSetting(): Promise<any> {
    try {
        const response = await fetch('/read/resources/live2d/setting.json');
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const rawText = await response.text();
        const jsonText = removeCodeComments(rawText);
        return JSON.parse(jsonText);
    } catch (error) {
        console.error('Failed to fetch Live2D setting:', error);
        return {};
    }
}

export function removeCodeComments(text: string): string {
    return text
        .replace(/\/\/.*$/gm, '')
        .replace(/\/\*[\s\S]*?\*\//g, '')
        .replace(/'/g, '"');
}