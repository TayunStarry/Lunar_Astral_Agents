/** 提取关键帧响应结构 */
export interface ExtractKeyframesResponse {
    /** 关键帧 URL 数组 */
    keyFrames: ExtractKeyframesData[];
    /** 关键帧数量 */
    count: number;
}

/** 关键帧数据结构 */
export interface ExtractKeyframesData {
    /** 关键帧文件路径 */
    filePath: string;
    /** 关键帧时间戳，格式：HH:mm:ss */
    timestamp: string;
    /** 关键帧帧号 */
    frameNum: number;
    /** 关键帧数据 */
    data: string;
};

/**
 * 关键帧结构
 */
export interface KeyFrame {
    /** 关键帧文件名 */
    filePath: string;
    /** 关键帧时间戳 */
    timestamp: string;
    /** 关键帧编号 */
    frameNum: number;
    /** 关键帧图像数据 */
    data: string;
}

/**
 * 缩放图片结果接口
 */
export interface ResizeImageResult {
    /** 缩放后的图片数据 */
    image: Uint8Array;
    /** 缩放后的图片base64编码 */
    base64: string;
    /** 图片格式 */
    format: string;
    /** 缩放后的图片宽度 */
    width: number;
    /** 缩放后的图片高度 */
    height: number;
}

/**
 * 图片生成参数接口
 */
export interface GenerateImageParams {
    /** 提示词 */
    prompt: string;
    /** 负面提示词 */
    negativePrompt?: string;
    /** 批处理数量 */
    batchSize?: number;
    /** 图片宽度 */
    width?: number;
    /** 图片高度 */
    height?: number;
    /** 生成步数 */
    steps?: number;
    /** 图生图强度 */
    strength?: number;
    /** 提示词引导系数 */
    cfgScale?: number;
    /** 随机数种子 */
    seed?: number;
    /** 初始图片路径 */
    initImg?: string;
}

/**
 * 图片生成结果接口
 */
export interface GenerateImageResult {
    /** 生成的图片路径 */
    path: string;
    /** 生成的图片base64编码 */
    base64: string;
    /** 图片宽度 */
    width: number;
    /** 图片高度 */
    height: number;
    /** 随机数种子 */
    seed: number;
}