interface Window {
	marked: {
		parse: (content: string) => Promise<string>;
	};
	hljs: {
		highlightElement: (block: HTMLElement) => void;
	};
	mermaid: {
		parse: (text: string) => Promise<void>;
		render: (id: string, text: string) => Promise<{ svg: string; bindFunctions: unknown }>;
	};
	echarts: {
		init: (container: HTMLElement) => {
			setOption: (option: unknown) => void;
		};
	};
	katex: unknown;
	renderMathInElement: (element: HTMLElement, options: {
		delimiters: Array<{ left: string; right: string; display: boolean }>;
		throwOnError: boolean;
	}) => void;
	PIXI: {
		Application: new (options: {
			transparent?: boolean;
			width?: number;
			height?: number;
			view?: HTMLCanvasElement;
			antialias?: boolean;
		}) => unknown;
		live2d: {
			Live2DModel: {
				from: (url: string, options: { autoInteract?: boolean }) => Promise<unknown>;
			};
		};
	};
	app: unknown;
}

// type Timeout = ReturnType<typeof setTimeout>;
