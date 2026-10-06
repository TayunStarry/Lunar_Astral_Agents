/**
 * 节点类型预设与配色：左侧面板按此表构建，新建节点从这里取默认外观。
 * 纯数据模块，无 DOM。
 */

export const NODE_TYPES = [
	{
		key: "start",
		label: "开始",
		icon: "fa-play",
		title: "开始",
		text: "流程起点",
		w: 168,
		h: 78,
		color: "#2fb59a",
		shape: "pill",
		fontSize: 14,
	},
	{
		key: "process",
		label: "处理",
		icon: "fa-gears",
		title: "处理步骤",
		text: "双击节点即可编辑这段文字",
		w: 232,
		h: 132,
		color: "#5b8dff",
		shape: "round",
		fontSize: 13,
	},
	{
		key: "decision",
		label: "判断",
		icon: "fa-code-branch",
		title: "条件判断",
		text: "成立？",
		w: 208,
		h: 118,
		color: "#f0a63c",
		shape: "round",
		fontSize: 13,
	},
	{
		key: "data",
		label: "数据",
		icon: "fa-database",
		title: "数据",
		text: "输入 / 输出数据",
		w: 224,
		h: 112,
		color: "#8f7dff",
		shape: "round",
		fontSize: 13,
	},
	{
		key: "io",
		label: "输入输出",
		icon: "fa-right-left",
		title: "输入输出",
		text: "外部接口",
		w: 224,
		h: 112,
		color: "#4bb8f0",
		shape: "round",
		fontSize: 13,
	},
	{
		key: "note",
		label: "便签",
		icon: "fa-note-sticky",
		title: "便签",
		text: "写下备注或说明…",
		w: 208,
		h: 150,
		color: "#ffcc66",
		shape: "note",
		fontSize: 13,
	},
	{
		key: "group",
		label: "分组",
		icon: "fa-layer-group",
		title: "分组",
		text: "同一类节点的集合",
		w: 268,
		h: 178,
		color: "#9aa6bf",
		shape: "rect",
		fontSize: 13,
	},
];

const BY_KEY = new Map(NODE_TYPES.map((t) => [t.key, t]));

export function getNodeType(key) {
	return BY_KEY.get(key) || BY_KEY.get("process");
}

export function isNodeType(key) {
	return BY_KEY.has(key);
}

export const SHAPES = ["round", "rect", "pill", "note"];

export const SHAPE_LABEL = {
	round: "圆角",
	rect: "直角",
	pill: "胶囊",
	note: "便签",
};

export const COLOR_SWATCHES = [
	"#5b8dff",
	"#4bb8f0",
	"#2fb59a",
	"#f0a63c",
	"#e5657f",
	"#8f7dff",
	"#ffcc66",
	"#9aa6bf",
];

export const FONT_SIZES = [12, 13, 14, 15, 16, 18, 20];

/** 节点尺寸下限，保证端口/文字始终可见；高度下限刻意留得小，好让折叠态能收成一条标题 */
export const MIN_NODE_W = 96;
export const MIN_NODE_H = 40;
export const MAX_NODE_W = 2400;
export const MAX_NODE_H = 1800;

/** 标题栏高度（与 css/canvas.css 的 .node-head 保持一致），折叠态高度与内容高度都从它算起 */
export const HEADER_HEIGHT = 34;
