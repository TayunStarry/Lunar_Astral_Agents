import { defineConfig } from 'vitest/config';

export default defineConfig({
    test: {
        // 测试文件匹配模式
        include: ['server_side/**/__tests__/**/*.test.ts'],
        // 全局设置
        globals: true,
        // 环境：Node.js（Goja 模拟）
        environment: 'node',
        // setup 文件 — 在每个测试文件加载前执行，用于注册全局 mock
        setupFiles: ['./server_side/agent/__tests__/setup.ts'],
    },
});