export interface HistoryMessage {
    role: 'user' | 'assistant' | 'system';
    content: string;
    imageUrl?: string;
    videoUrl?: string;
    timestamp?: number;
    noRender?: boolean;
    isPrompt?: boolean;
    deletable?: boolean;
}

export interface Live2DModelConfig {
    name: string;
    url: string;
    scale: number;
    x: number;
    y: number;
    autoInteract: boolean;
}

export interface WebSocketMessage {
    type: 'context' | 'image' | 'error';
    data: {
        type?: 'response' | 'active';
        content?: string;
        images?: string[];
    };
}

export interface FileUploadResult {
    filename: string;
    path: string;
    overwrite: boolean;
    size: number;
    success: boolean;
}

export interface APIResponse {
    success: boolean;
    length?: number;
    error?: {
        code: number;
        message: string;
        details?: string;
    };
}

export interface ContentBlock {
    type: 'text' | 'image_url';
    text?: string;
    image_url?: {
        url: string;
    };
}

export interface OpenAIMessage {
    role: 'user' | 'assistant' | 'system';
    content: string | ContentBlock[];
}

export type EmotionalState = 'IDLE' | 'THINKING' | 'AWAIT' | 'SPEAKING' | 'HAPPY' | 'SAD' | 'ANGRY';

export interface AppState {
    isConnected: boolean;
    isLoading: boolean;
    currentModel: Live2DModelConfig | null;
    emotionState: EmotionalState;
    historyMessages: HistoryMessage[];
    uploadedFiles: File[];
}