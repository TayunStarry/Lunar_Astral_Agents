// ---------- 聚合导出（保持原公共 API） ----------
export { descriptionRole, searcherRole, painterRole, musicianRole, actorRole, viewerRole, memorizerRole, randomDefaultMessage } from './roles/roles';
export { LiteImageFile } from './capabilities/media';
export { extractTextFromMessage, ensureMemoryReady } from './capabilities/memory';

// ---------- 启动副作用：加载自定义配置，并每秒驱动一次思考循环 ----------
import { GlobalConfig, fetchDocumentCallback } from '../index';
import { thoughtLoopTickEvent } from './loop';

// 初始化 自定义配置 信息
fetchDocumentCallback('lunar_config.json').then(content => GlobalConfig.customConfig = content);
// 每秒执行一次思考循环
setInterval(() => thoughtLoopTickEvent(), 1000);
