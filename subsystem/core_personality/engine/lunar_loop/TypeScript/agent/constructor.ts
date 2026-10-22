/// <reference path="../globals.d.ts" />

// ---------- 模块入口（原 TypeScript/index.ts 的职责由本文件承接） ----------
// LunarGoja 直接执行打包产物、只依赖副作用（工具注册 / 对话循环），不读取导出成员；
// 因此本文件除聚合导出外，必须显式引入所有带模块级副作用的工具模块。

// ---------- 工具模块副作用：向 GlobalConfig 注册 LTPfunction（工具定义由对话者静态聚合） ----------
import '../tools/agent-control';
import '../tools/schedule/index';
// ---------- 聚合导出（保持原公共 API） ----------
export { painterRole, musicianRole, memorizerRole, randomDefaultMessage } from '../roles/index';
export { LiteImageFile } from '../capabilities/media';
export { extractTextFromMessage, ensureMemoryReady } from '../capabilities/memory';

// ---------- 启动副作用：加载自定义配置，并驱动对话链 ----------
import { GlobalConfig } from '../config/global';
import { fetchDocumentCallback } from '../io/read';
import { initConversationCache } from '../io/conversation';
import { dialogueRole } from '../roles/index';
import { bindDumpBridge } from './data-dump';
import { dialogueTick } from './loop';

// 初始化自定义配置
fetchDocumentCallback('lunar_config.json').then(content => {
	GlobalConfig.customConfig = content;
});
// 向宿主暴露上下文导出入口（TUI「数据导出」经 dumpForJS 调用）
bindDumpBridge();
// 载入缓存文件中的近期记忆（前端刷新时读取同一份文件）
initConversationCache(messages => dialogueRole.coverContext(messages));
// 每秒一次对话链 tick：计划表到期项与未读消息都由它消费。
setInterval(() => { void dialogueTick(); }, 1000);
