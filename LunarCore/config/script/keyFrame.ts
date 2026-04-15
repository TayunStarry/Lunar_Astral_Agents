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