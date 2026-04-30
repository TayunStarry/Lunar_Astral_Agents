export interface MessageContentBlock {
	type: 'text' | 'image_url';
	text?: string;
	image_url?: {
		url: string;
	};
}

export interface Message {
	role: 'user' | 'assistant';
	content: string | MessageContentBlock[];
	imageUrl?: string;
	imageUrls?: string[];
	timestamp?: number;
}

export interface OpenAIMessage {
	role: 'user' | 'assistant' | 'system';
	content: string | MessageContentBlock[];
}

export interface FilePreview {
	file: File;
	url: string;
	type: 'image' | 'video' | 'text';
	name: string;
}

export interface SaveFileResponse {
	filename: string;
	path: string;
	overwrite: boolean;
	size: number;
	success: boolean;
}

export interface SendMessagesResponse {
	success: boolean;
	length: number;
}

export interface WebSocketMessage {
	type: 'context' | 'image' | 'error';
	data: {
		type?: string;
		content?: string;
		images?: string[];
	};
}

export interface Live2DSetting {
	name: string;
	url: string;
	scale: number;
	x: number;
	y: number;
	autoInteract: boolean;
}

export type EmotionalState =
	| 'IDLE'
	| 'THINKING'
	| 'AWAIT'
	| 'SPEAKING'
	| 'HAPPY'
	| 'SAD'
	| 'ANGRY';

export interface LunarCoreAppConfig {
	wsUrl?: string;
}
