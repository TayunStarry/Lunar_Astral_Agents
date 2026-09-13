/// <reference path="../globals.d.ts" />

// ---------- 模块入口（原 TypeScript/index.ts 的职责由本文件承接） ----------
// LunarGoja 直接执行打包产物、只依赖副作用（工具注册 / 思考循环），不读取导出成员；
// 因此本文件除聚合导出外，必须显式引入所有带模块级副作用的工具模块。

// ---------- 工具模块副作用：向 GlobalConfig 注册 LTPfunction / LTPdefinition ----------
import '../tool/agent-control';
import '../tool/schedule';
import '../tool/screenshot';

// ---------- 聚合导出（保持原公共 API） ----------
export { descriptionRole, painterRole, musicianRole, actorRole, viewerRole, memorizerRole, randomDefaultMessage } from './roles/roles';
export { LiteImageFile } from './capabilities/media';
export { extractTextFromMessage, ensureMemoryReady } from './capabilities/memory';

// ---------- 启动副作用：加载自定义配置，并每秒驱动一次思考循环 ----------
import { GlobalConfig } from '../config/global';
import { fetchDocumentCallback } from '../file/io/read';
import { thoughtLoopTickEvent } from './loop';

// 初始化 自定义配置 信息
fetchDocumentCallback('lunar_config.json').then(content => GlobalConfig.customConfig = content);
// 每秒执行一次思考循环
setInterval(() => thoughtLoopTickEvent(), 1000);
