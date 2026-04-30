interface Window {
	/** Markdown解析器 */
	marked: {
		/** 解析Markdown内容 */
		parse: (content: string) => Promise<string>;
	};
	/** 代码高亮器 */
	hljs: {
		/** 高亮代码块 */
		highlightElement: (block: HTMLElement) => void;
	};
	/** Mermaid渲染器 */
	mermaid: {
		/** 解析Mermaid文本 */
		parse: (text: string) => Promise<void>;
		/** 渲染Mermaid图表 */
		render: (id: string, text: string) => Promise<{ svg: string; bindFunctions: unknown }>;
	};
	/** ECharts图表器 */
	echarts: {
		/** 初始化图表 */
		init: (container: HTMLElement) => {
			/** 设置图表选项 */
			setOption: (option: unknown) => void;
		};
	};
	/** Katex渲染器 */
	katex: unknown;
	/** 渲染Katex数学公式 */
	renderMathInElement: (element: HTMLElement, options: { delimiters: Array<{ left: string; right: string; display: boolean }>; throwOnError: boolean; }) => void;
	/** PIXI渲染器 */
	PIXI: {
		/** 创建PIXI应用 */
		Application: new (options: { transparent?: boolean; width?: number; height?: number; view?: HTMLCanvasElement; antialias?: boolean; }) => unknown;
		/** Live2D模型 */
		live2d: {
			/** 从URL加载Live2D模型 */
			Live2DModel: {
				/** 从URL加载Live2D模型 */
				from: (url: string, options: { autoInteract?: boolean }) => Promise<unknown>;
			};
		};
	};
	/** 核心应用实例 */
	app: unknown;
}

// type Timeout = ReturnType<typeof setTimeout>;
