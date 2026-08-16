/**
 * 小说工作室 Pro — 核心纯逻辑单元测试
 * 使用 Node 内置 node:test + vm 加载 IIFE 模块，无第三方依赖。
 * 运行方式：node --test test/core.test.js
 */

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const PKG_DIR = path.resolve(__dirname, '..');

// ==== 通过 vm 加载 IIFE 模块，暴露到沙箱全局 ====
function loadModule(filename) {
    const code = fs.readFileSync(path.join(PKG_DIR, filename), 'utf8');
    const sandbox = { console };
    const context = vm.createContext(sandbox);
    vm.runInContext(code, context, { filename });
    return sandbox;
}

// ==== 跨 realm 归一化：将 vm 上下文中创建的对象转为宿主 realm 普通对象 ====
function plain(value) {
    return JSON.parse(JSON.stringify(value));
}

const configModule = loadModule('config.js');
const creativeModule = loadModule('creative.js');

const ConfigManager = configModule.ConfigManager;
const CreativeBuilder = creativeModule.CreativeBuilder;

// ==== ConfigManager 测试 ====
test('ConfigManager._buildMessages 无图片时原样返回', () => {
    const cm = new ConfigManager({});
    const messages = [
        { role: 'system', content: 'sys' },
        { role: 'user', content: 'hello' }
    ];
    const result = cm._buildMessages(messages, null);
    assert.strictEqual(result, messages);
    assert.deepStrictEqual(plain(result), messages);
});

test('ConfigManager._buildMessages 图片附加到末条 user 消息', () => {
    const cm = new ConfigManager({});
    const imageUrl = 'data:image/png;base64,AAAA';
    const messages = [
        { role: 'system', content: 'sys' },
        { role: 'user', content: '请分析这张图' }
    ];
    const result = cm._buildMessages(messages, imageUrl);

    assert.strictEqual(result[0], messages[0]);
    assert.strictEqual(result[1].role, 'user');
    assert.ok(Array.isArray(result[1].content));
    assert.deepStrictEqual(plain(result[1].content[0]), { type: 'text', text: '请分析这张图' });
    assert.deepStrictEqual(plain(result[1].content[1]), {
        type: 'image_url',
        image_url: { url: imageUrl }
    });
});

test('ConfigManager._buildMessages 多条 user 消息时仅末条携带图片', () => {
    const cm = new ConfigManager({});
    const messages = [
        { role: 'user', content: '第一条' },
        { role: 'user', content: '第二条' }
    ];
    const result = cm._buildMessages(messages, 'data:image/png;base64,BBBB');
    assert.strictEqual(result[0], messages[0]);
    assert.ok(Array.isArray(result[1].content));
});

test('ConfigManager.getConfig 默认兜底配置', () => {
    const cm = new ConfigManager({});
    cm.agentConfig = null;
    const cfg = cm.getConfig();
    assert.strictEqual(cfg.name, 'system-multimodal');
    assert.strictEqual(cfg.url, 'http://127.0.0.1:36789/v1');
    assert.strictEqual(cfg.key, '');
});

test('ConfigManager.getConfig 读取 agent 配置（兼容 {url,name,key} 形态）', () => {
    const cm = new ConfigManager({});
    cm.agentConfig = {
        multimodal_model: 'my-mm',
        multimodal_url: 'https://example.com/v1',
        multimodal_key: 'sk-abc'
    };
    assert.deepStrictEqual(plain(cm.getConfig()), {
        url: 'https://example.com/v1',
        name: 'my-mm',
        key: 'sk-abc'
    });
});

test('ConfigManager.getEmbeddingModel 默认与自定义', () => {
    const cm = new ConfigManager({});
    cm.agentConfig = null;
    assert.strictEqual(cm.getEmbeddingModel(), 'system-embedding');
    cm.agentConfig = { embedding_model: 'my-embed' };
    assert.strictEqual(cm.getEmbeddingModel(), 'my-embed');
});

// ==== CreativeBuilder 测试 ====
test('CreativeBuilder._detectExportFormat 识别系统导出格式', () => {
    const cb = new CreativeBuilder({});
    assert.strictEqual(cb._detectExportFormat(JSON.stringify({
        version: 3, chapters: [{ index: 1, content: 'x' }]
    })), true);
    assert.strictEqual(cb._detectExportFormat(JSON.stringify({
        version: 3, chapters: []
    })), true);
});

test('CreativeBuilder._detectExportFormat 拒绝非导出 JSON 与非法输入', () => {
    const cb = new CreativeBuilder({});
    assert.strictEqual(cb._detectExportFormat(JSON.stringify({ foo: 1 })), false);
    assert.strictEqual(cb._detectExportFormat('not json'), false);
    assert.strictEqual(cb._detectExportFormat(''), false);
});

test('CreativeBuilder._parseChapterOutlines 解析 JSON 字符串数组', () => {
    const cb = new CreativeBuilder({});
    const out = cb._parseChapterOutlines(JSON.stringify(['开篇', '冲突', '高潮']));
    assert.deepStrictEqual(plain(out), [
        { index: 1, outline: '开篇' },
        { index: 2, outline: '冲突' },
        { index: 3, outline: '高潮' }
    ]);
});

test('CreativeBuilder._parseChapterOutlines 解析 JSON 对象数组', () => {
    const cb = new CreativeBuilder({});
    const out = cb._parseChapterOutlines(JSON.stringify([
        { index: 1, outline: '第一章大纲' },
        { index: 2, outline: '第二章大纲' }
    ]));
    assert.strictEqual(out.length, 2);
    assert.strictEqual(out[0].outline, '第一章大纲');
});

test('CreativeBuilder._parseChapterOutlines 解析 code block 包裹内容', () => {
    const cb = new CreativeBuilder({});
    const text = '```json\n["A", "B"]\n```';
    const out = cb._parseChapterOutlines(text);
    assert.deepStrictEqual(plain(out), [
        { index: 1, outline: 'A' },
        { index: 2, outline: 'B' }
    ]);
});

test('CreativeBuilder._parseKnowledgeArray 解析 JSON 字符串数组', () => {
    const cb = new CreativeBuilder({});
    const out = cb._parseKnowledgeArray(JSON.stringify(['知识点1', '知识点2']));
    assert.deepStrictEqual(plain(out), ['知识点1', '知识点2']);
});

test('CreativeBuilder._parseKnowledgeArray 解析对象数组并提取文本字段', () => {
    const cb = new CreativeBuilder({});
    const out = cb._parseKnowledgeArray(JSON.stringify([
        { content: '内容A' },
        { text: '内容B' }
    ]));
    assert.deepStrictEqual(plain(out), ['内容A', '内容B']);
});
