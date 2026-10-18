import { ToolCall } from '../../../../config/tool';
import { generationTools } from './generation';
import { portraitTools } from './portrait';
import { groupTools } from './group';
import { runtimeTools } from './runtime';

/** 绘制者完整工具集（顺序即工具描述顺序） */
export const painterTools: ToolCall[] = [
	...generationTools,
	...portraitTools,
	...groupTools,
	...runtimeTools,
];
