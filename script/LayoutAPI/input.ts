/**
 * TTS文本输入框
 */
export const speechModelText = document.getElementById("speechModelText") as HTMLInputElement;
/**
 * 用户消息截断长度输入框
 */
export const messageSliceLength = document.getElementById("messageSliceLength") as HTMLInputElement;
/**
 * 常规聊天模式 聊天输入框
 */
export const chatWriteArea = document.getElementById("chatWriteArea") as HTMLInputElement;
/**
 * 角色互动模式 聊天输入框
 */
export const live2dWriteArea = document.getElementById("live2dWriteArea") as HTMLInputElement;
/**
 * 轻量渲染 输入框
 */
export const renderWriteArea = document.getElementById('renderWriteArea') as HTMLInputElement;
/**
 * 共享视觉 输入框
 */
export const screenshotWriteArea = document.getElementById('screenshotWriteArea') as HTMLInputElement;
/**
 * TTS语速显示值
 */
export const speechSpeedValue = document.getElementById("speechSpeedValue") as HTMLInputElement;
/**
 * TTS音量显示值
 */
export const speechVolumeValue = document.getElementById("speechVolumeValue") as HTMLInputElement;
/**
 * 知识库输入框
 */
export const noteWriteArea = document.getElementById("noteWriteArea") as HTMLInputElement;

import * as EntryAPI from '../EntryAPI/code';

// 为输入框添加按键监听事件
renderWriteArea.addEventListener("keypress",
    event => {
        // 当按下的键是 Enter 且没有同时按下 Shift 键时
        if (event.key === "Enter" && !event.shiftKey) {
            // 阻止默认的换行行为
            event.preventDefault();
            // 调用创建轻量渲染的函数
            EntryAPI.createSimpleRendering();
        }
    }
);

/**
 * 初始化所有带有 auto-resize-textarea 类的文本框的自动调整高度功能
 */
export function initAutoResizeTextareas() {
    /**
     * 获取所有带有 auto-resize-textarea 类的文本框元素
     */
    const textareas = document.querySelectorAll('.auto-resize-textarea') as NodeListOf<HTMLTextAreaElement>;
    // 遍历每个文本框元素，为其添加自动调整高度的功能
    textareas.forEach(
        textarea => {
            /**
             * 自动调整文本框高度的函数
             * 先将文本框高度设为 auto 以获取实际内容高度，
             * 再取内容高度和最大高度中的较小值作为新高度
             */
            function autoResize() {
                // 临时将文本框高度设为 auto，以便获取准确的 scrollHeight
                textarea.style.height = 'auto';
                /**
                 * 计算新高度，取文本框内容高度和最大高度中的较小值
                 */
                const newHeight = Math.min(textarea.scrollHeight, parseInt(getComputedStyle(textarea).maxHeight));
                // 设置文本框的新高度
                textarea.style.height = newHeight + 'px';
            }
            // 为文本框的 input 事件添加自动调整高度的监听
            textarea.addEventListener('input', autoResize);
            // 为文本框的 focus 事件添加自动调整高度的监听
            textarea.addEventListener('focus', autoResize);
            // 初始化时立即执行一次自动调整高度
            autoResize();
        }
    );
};

// 为输入框添加按键监听事件
screenshotWriteArea.addEventListener("keypress", (event: KeyboardEvent) => {
    // 当按下的键是 Enter 且没有同时按下 Shift 键时
    if (event.key === "Enter" && !event.shiftKey) {
        // 阻止默认的换行行为
        event.preventDefault();
        // 调用创建共享视觉的函数
        EntryAPI.createSimpleVisual();
    }
});