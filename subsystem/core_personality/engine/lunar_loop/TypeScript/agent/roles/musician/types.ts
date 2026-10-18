/** 音乐创作详情记录（用于向对话者传递作品信息） */
export interface MusicPieceDetail {
	/** 音乐作品标题 */
	title: string;
	/** 使用的乐器列表，多个乐器用逗号分隔 */
	instruments: string;
	/** 演奏速度（BPM） */
	tempo: number;
	/** 音乐段落结构描述 */
	structure: string;
	/** 调式，如 C大调、a小调 */
	key: string;
	/** 拍号，如 4/4、3/4 */
	meter: string;
	/** ABC记谱法格式的完整乐谱长度（小节） */
	abcLength: number;
}

/** 分阶段创作草稿：主旋律 → 伴奏 → 和弦进行 */
export interface MusicDraft {
	/** 作品标题 */
	title: string;
	/** 段落结构描述 */
	structure: string;
	/** 调式，如 C、a */
	key: string;
	/** 拍号，如 4/4 */
	meter: string;
	/** 速度（BPM） */
	tempo: number;
	/** 主旋律声部（ABC 片段） */
	melody: string;
	/** 主旋律乐器 */
	melodyInstrument: string;
	/** 主旋律小节数 */
	melodyBars: number;
	/** 伴奏声部（ABC 片段） */
	accompaniment: string;
	/** 伴奏乐器 */
	accompanimentInstrument: string;
	/** 和弦进行声部（ABC 片段） */
	chords: string;
	/** 和弦声部乐器 */
	chordInstrument: string;
}
