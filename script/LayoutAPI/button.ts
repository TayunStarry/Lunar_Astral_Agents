/**
 * 自动播放语音按钮
 */
export const autoPlaySpeechButton = document.getElementById("autoPlaySpeechButton") as HTMLButtonElement;
/**
 * 导出聊天交互数据按钮
 */
export const exportChatInteractionButton = document.getElementById("exportChatInteractionButton") as HTMLButtonElement;
/**
 * 导入聊天交互数据按钮
 */
export const importChatInteractionButton = document.getElementById("importChatInteractionButton") as HTMLButtonElement;
/**
 * 语音识别按钮
 */
export const voiceRecognitionButton = document.getElementById("voiceRecognitionButton") as HTMLButtonElement;
/**
 * 调试模式按钮
 */
export const debugModeButton = document.getElementById("debugModeButton") as HTMLButtonElement;
/**
 * 输入文件按钮
 */
export const inputFileButton = document.getElementById("inputFileButton") as HTMLButtonElement;
/**
 * 播放语音模型按钮
 */
export const playSpeechModelButton = document.getElementById("playSpeechModelButton") as HTMLButtonElement;
/**
 * 渲染输入按钮
 */
export const renderReleaseButton = document.getElementById("renderReleaseButton") as HTMLButtonElement;
/**
 * 截图输入按钮
 */
export const screenshotReleaseButton = document.getElementById("screenshotReleaseButton") as HTMLButtonElement;
/**
 * 停止语音模型按钮
 */
export const stopSpeechModelButton = document.getElementById("stopSpeechModelButton") as HTMLButtonElement;
/**
 * 简单渲染按钮
 */
export const simpleRenderingButton = document.getElementById("simpleRenderingButton") as HTMLButtonElement;
/**
 * 视觉共享按钮
 */
export const shareScreenButton = document.getElementById("shareScreenButton") as HTMLButtonElement;
/**
 * 对外交流按钮
 */
export const externalDialogueButton = document.getElementById("externalDialogueButton") as HTMLButtonElement;
/**
 * 月华笔记按钮
 */
export const lunarNotesButton = document.getElementById("lunarNotesButton") as HTMLButtonElement;
/**
 * 文枢阁按钮
 */
export const FileVaultButton = document.getElementById("FileVaultButton") as HTMLButtonElement;
/**
 * 灵绘坊按钮
 */
export const ImageStudioButton = document.getElementById("ImageStudioButton") as HTMLButtonElement;
/**
 * 智存库按钮
 */
export const DataKeeperButton = document.getElementById("DataKeeperButton") as HTMLButtonElement;
/**
 * 聊天输入按钮
 */
export const chatReleaseButton = document.getElementById("chatReleaseButton") as HTMLButtonElement;
/**
 * Live2D输入按钮
 */
export const live2dReleaseButton = document.getElementById("live2dReleaseButton") as HTMLButtonElement;
/**
 * 自定义语音引擎按钮
 */
export const customSpeechEngineButton = document.getElementById("customSpeechEngineButton") as HTMLButtonElement;
/**
 * 系统语音引擎按钮
 */
export const systemSpeechEngineButton = document.getElementById("systemSpeechEngineButton") as HTMLButtonElement;
/**
 * 语音配置按钮
 */
export const voiceConfigureButton = document.getElementById("voiceConfigureButton") as HTMLButtonElement;
/**
 * 主题按钮
 */
export const themeButton = document.getElementById("themeButton") as HTMLButtonElement;
/**
 * 语音输入按钮
 */
export const voiceReleaseButton = document.getElementById("voiceReleaseButton") as HTMLButtonElement;
/**
 * 触发Live2D状态按钮
 */
export const triggerLive2DStateButton = document.getElementById("triggerLive2DStateButton") as HTMLButtonElement;
/**
 * 头部触摸按钮
 */
export const headTouchButton = document.getElementById("headTouchButton") as HTMLButtonElement;
/**
 * 身体触摸按钮
 */
export const bodyTouchButton = document.getElementById("bodyTouchButton") as HTMLButtonElement;
/**
 * 腿部触摸按钮
 */
export const legTouchButton = document.getElementById("legTouchButton") as HTMLButtonElement;
/**
 * 脚部触摸按钮
 */
export const footTouchButton = document.getElementById("footTouchButton") as HTMLButtonElement;
/**
 * 刷新知识库按钮
 */
export const refreshNoteButton = document.getElementById("refreshNoteButton") as HTMLButtonElement;
/**
 * 上传知识库按钮
 */
export const noteReleaseButton = document.getElementById("noteReleaseButton") as HTMLButtonElement;
/**
 * 功能控制按钮
 */
export const functionControlButton = document.getElementById("functionControlButton") as HTMLButtonElement;
/**
 * 聊天记录按钮
 */
export const chatHistoryButton = document.getElementById("chatHistoryButton") as HTMLButtonElement;
/**
 * 连续记忆模式切换按钮
 */
export const longTermMemoryButton = document.getElementById("longTermMemoryButton") as HTMLButtonElement;
/**
 * 二维码按钮
 */
export const qrcodeButton = document.getElementById("qrcodeButton") as HTMLButtonElement;
/**
 * 主动消息模式切换按钮
 */
export const activeMessageButton = document.getElementById("activeMessageButton") as HTMLButtonElement;

import * as EntryAPI from '../EntryAPI/code';

/**
 * 批量启用或禁用输入按钮
 *
 * @param {boolean} disabled - 一个布尔值，用于指定是否禁用按钮。true 表示禁用，false 表示启用
 */
export function disabledReleaseButton(disabled: boolean) {
    // 设置截图输入按钮的禁用状态
    screenshotReleaseButton.disabled = disabled;
    // 设置Live2D输入按钮的禁用状态
    live2dReleaseButton.disabled = disabled;
    // 设置聊天输入按钮的禁用状态
    chatReleaseButton.disabled = disabled;
};

/**
 * 获取所有输入按钮的禁用状态
 *
 * @returns {boolean} 如果所有按钮都被禁用则返回 true，否则返回 false
 */
export function getReleaseButtonsDisabledState(): boolean {
    return (
        screenshotReleaseButton.disabled &&
        live2dReleaseButton.disabled &&
        chatReleaseButton.disabled
    );
};

//* 绑定 导出聊天记录 按钮点击事件
exportChatInteractionButton.addEventListener("click", () => EntryAPI.exportChatInteractionWithFetch());
//* 绑定 导入聊天记录 按钮点击事件
importChatInteractionButton.addEventListener("click", () => EntryAPI.importChatInteractionEvent());
//* 绑定 角色互动模式 聊天输入按钮点击事件
live2dReleaseButton.addEventListener("click", () => EntryAPI.sendChatMessageToBackendModel());
//* 绑定 常规聊天模式 聊天输入按钮点击事件
chatReleaseButton.addEventListener("click", () => EntryAPI.sendChatMessageToBackendModel());
//* 绑定 自定义语音引擎 按钮点击事件
customSpeechEngineButton.addEventListener("click", () => EntryAPI.switchSpeechEngineMode("custom"));
//* 绑定 系统语音引擎 按钮点击事件
systemSpeechEngineButton.addEventListener("click", () => EntryAPI.switchSpeechEngineMode("system"));
//* 绑定 播放TTS 按钮点击事件
playSpeechModelButton.addEventListener("click", () => EntryAPI.playSpeechModel(EntryAPI.speechModelText.value.trim()));
//* 绑定 停止TTS 按钮点击事件
stopSpeechModelButton.addEventListener("click", () => EntryAPI.stopSpeechModel());

//* 绑定 摸头按钮 点击事件
headTouchButton.addEventListener("click",
    async function () {
        /**
         * 定义< 动态提示词 >
         */
        const markdown = '用户摸了摸你的头, 请做出合适的反应';
        // 若调试模式开启，则渲染< 动态提示词 >
        if (EntryAPI.OnlyData.isDebugMode) {
            /**
             * 渲染< 动态提示词 >
             */
            const messageElement = await EntryAPI.tracelessRenderMessage('<think>\n' + markdown + '\n</think>', EntryAPI.chatHistoryPanel);
            // 为think区块添加折叠功能
            (messageElement?.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>).forEach(EntryAPI.bindFoldingButton);
        };
        // 从API加载对话内容
        EntryAPI.executeDialogueAndParse(EntryAPI.chatHistoryPanel, markdown);
    }
);
//* 绑定 摸身体按钮 点击事件
bodyTouchButton.addEventListener("click",
    async function () {
        /**
         * 定义< 动态提示词 >
         */
        const markdown = '用户摸了摸你的身体或胸部, 请做出合适的反应';
        // 若调试模式开启，则渲染< 动态提示词 >
        if (EntryAPI.OnlyData.isDebugMode) {
            /**
             * 渲染< 动态提示词 >
             */
            const messageElement = await EntryAPI.tracelessRenderMessage('<think>\n' + markdown + '\n</think>', EntryAPI.chatHistoryPanel);
            // 为think区块添加折叠功能
            (messageElement?.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>).forEach(EntryAPI.bindFoldingButton);
        };
        // 从API加载对话内容
        EntryAPI.executeDialogueAndParse(EntryAPI.chatHistoryPanel, markdown);
    }
);
//* 绑定 摸腿按钮 点击事件
legTouchButton.addEventListener("click",
    async function () {
        /**
         * 定义< 动态提示词 >
         */
        const markdown = '用户摸了摸你的大腿, 请做出合适的反应';
        // 若调试模式开启，则渲染< 动态提示词 >
        if (EntryAPI.OnlyData.isDebugMode) {
            /**
             * 渲染< 动态提示词 >
             */
            const messageElement = await EntryAPI.tracelessRenderMessage('<think>\n' + markdown + '\n</think>', EntryAPI.chatHistoryPanel);
            // 为think区块添加折叠功能
            (messageElement?.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>).forEach(EntryAPI.bindFoldingButton);
        };
        // 从API加载对话内容
        EntryAPI.executeDialogueAndParse(EntryAPI.chatHistoryPanel, markdown);
    }
);
//* 绑定 摸脚按钮 点击事件
footTouchButton.addEventListener("click",
    async function () {
        /**
         * 定义< 动态提示词 >
         */
        const markdown = '用户摸了摸你的小腿或脚部, 请做出合适的反应';
        // 若调试模式开启，则渲染< 动态提示词 >
        if (EntryAPI.OnlyData.isDebugMode) {
            /**
             * 渲染< 动态提示词 >
             */
            const messageElement = await EntryAPI.tracelessRenderMessage('<think>\n' + markdown + '\n</think>', EntryAPI.chatHistoryPanel);
            // 为think区块添加折叠功能
            (messageElement?.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>).forEach(EntryAPI.bindFoldingButton);
        };
        // 从API加载对话内容
        EntryAPI.executeDialogueAndParse(EntryAPI.chatHistoryPanel, markdown);
    }
);
//* 绑定 Live2D 动作触发按钮事件
triggerLive2DStateButton.addEventListener("click",
    function () {
        /**
         * 获取 Live2D 状态选择框中当前选中的值
         */
        const selectedState = EntryAPI.live2dStateDropdown.value;
        // 调用函数设置 Live2D 的情感状态
        EntryAPI.setEmotionState(selectedState);
        // 显示系统状态面板，提示已成功触发指定状态
        EntryAPI.showSystemMessage(`已触发状态: ${selectedState}`, "success");
    }
);
//* 绑定 切换轻量渲染面板 按钮点击事件
simpleRenderingButton.addEventListener('click',
    function () {
        // 若当前屏幕宽度不足，显示错误提示并结束事件响应
        if (window.innerWidth <= EntryAPI.smallScreenWidthThreshold) return EntryAPI.showSystemMessage("< 轻量渲染 >不可在小屏幕下使用", "error");
        // 清除所有配置面板的显示状态
        EntryAPI.eraseAllConfigurePanel();
        // 若当前已显示轻量渲染面板
        if (EntryAPI.OnlyData.configurePanelOption === 'simpleRenderingButton') {
            // 显示对话和历史记录面板
            EntryAPI.chatHistoryContainerPanel.style.display = "flex";
            // 隐藏轻量渲染面板
            EntryAPI.simpleRenderingContainerPanel.style.display = "none";
            // 改变全局变量，表示无配置面板显示
            EntryAPI.OnlyData.configurePanelOption = 'any';
            // 结束事件响应
            return;
        }
        // 隐藏对话和历史记录面板
        EntryAPI.chatHistoryContainerPanel.style.display = "none";
        // 显示轻量渲染面板
        EntryAPI.simpleRenderingContainerPanel.style.display = "flex";
        // 改变全局变量，表示当前显示轻量渲染面板
        EntryAPI.OnlyData.configurePanelOption = 'simpleRenderingButton';
    }
);
//* 绑定 切换视觉共享面板 按钮点击事件
shareScreenButton.addEventListener('click',
    function () {
        // 若当前屏幕宽度不足，显示错误提示并结束事件响应
        if (window.innerWidth <= EntryAPI.smallScreenWidthThreshold) return EntryAPI.showSystemMessage("< 共享视觉 >不可在小屏幕下使用", "error");
        /**
         * 获取文档中所有的配置面板按钮元素
         */
        const configurePanelButton = document.documentElement.querySelectorAll('.power-button.live2d');
        // 遍历所有配置面板按钮，移除按钮上的点击中的样式类，恢复按钮初始样式
        configurePanelButton.forEach(button => button.classList.remove("clicking"));
        // 清除所有配置面板的显示状态
        EntryAPI.eraseAllConfigurePanel();
        // 若当前已显示轻量渲染面板
        if (EntryAPI.OnlyData.configurePanelOption === 'shareScreenButton') {
            // 变更按钮样式
            this.innerHTML = '<i class="fas fa-camera"></i>';
            this.classList.remove("clicking");
            // 显示对话和历史记录面板
            EntryAPI.chatHistoryContainerPanel.style.display = "flex";
            // 隐藏视觉共享面板
            EntryAPI.shareScreenContainerPanel.style.display = "none";
            // 改变全局变量，表示无配置面板显示
            EntryAPI.OnlyData.configurePanelOption = 'any';
            // 结束事件响应
            return;
        }
        // 变更按钮样式
        this.innerHTML = '<i class="fas fa-eye"></i>';
        this.classList.add("clicking");
        // 隐藏对话和历史记录面板
        EntryAPI.chatHistoryContainerPanel.style.display = "none";
        // 显示视觉共享面板
        EntryAPI.shareScreenContainerPanel.style.display = "flex";
        // 改变全局变量，表示当前显示视觉共享面板
        EntryAPI.OnlyData.configurePanelOption = 'shareScreenButton';
        // 调用截图核心函数，截图当前屏幕
        EntryAPI.screenshotCore.captureScreen()
    }
);
//* 绑定 切换< 外部通讯 >面板 按钮点击事件
externalDialogueButton.addEventListener('click',
    async function () {
        /** 获取文档中所有的配置面板按钮元素 */
        const configurePanelButton = document.documentElement.querySelectorAll('.power-button.live2d');
        // 遍历所有配置面板按钮，移除按钮上的点击中的样式类，恢复按钮初始样式
        configurePanelButton.forEach(button => button.classList.remove("clicking"));
        // 清除所有配置面板的显示状态
        EntryAPI.eraseAllConfigurePanel();
        // 若当前已显示< 外部通讯 >面板
        if (EntryAPI.OnlyData.configurePanelOption === 'externalDialogueButton') {
            // 关闭WebSocket服务
            EntryAPI.managerExchanges.close();
            EntryAPI.showSystemMessage("关闭< 外部通讯 >", "success");
            // 变更按钮样式
            this.innerHTML = '<i class="fas fa-exchange-alt"></i>';
            this.classList.remove("clicking");
            // 显示对话和历史记录面板
            EntryAPI.chatHistoryContainerPanel.style.display = "flex";
            // 改变全局变量，表示无配置面板显示
            EntryAPI.OnlyData.configurePanelOption = 'any';
            // 加载系统提示词
            EntryAPI.OnlyData.systemPrompt = await EntryAPI.fetchMarkdown('/read/resources/prompts/systemPrompt.md');
            // 结束事件响应
            return;
        }
        // 启动WebSocket服务
        EntryAPI.managerExchanges.start();
        EntryAPI.showSystemMessage("开启< 外部通讯 >", "success");
        // 变更按钮样式
        this.innerHTML = '<i class="fas fa-globe"></i>';
        this.classList.add("clicking");
        // 隐藏对话和历史记录面板
        EntryAPI.chatHistoryContainerPanel.style.display = "none";
        // 改变全局变量，表示当前显示对外交流面板
        EntryAPI.OnlyData.configurePanelOption = 'externalDialogueButton';
        // 加载系统提示词
        EntryAPI.OnlyData.systemPrompt = await EntryAPI.fetchMarkdown('/read/resources/prompts/externalDialogue.md');
    }
);
//* 绑定 切换月华笔记面板 按钮点击事件
lunarNotesButton.addEventListener('click',
    function () {
        // 若当前屏幕宽度不足，显示错误提示并结束事件响应
        if (window.innerWidth <= EntryAPI.smallScreenWidthThreshold) return EntryAPI.showSystemMessage("< 月华笔记 >不可在小屏幕下使用", "error");
        // 清除所有配置面板的显示状态
        EntryAPI.eraseAllConfigurePanel();
        // 若当前已显示月华笔记面板
        if (EntryAPI.OnlyData.configurePanelOption === 'lunarNotesButton') {
            // 显示对话和历史记录面板
            EntryAPI.chatHistoryContainerPanel.style.display = "flex";
            // 隐藏月华笔记面板
            EntryAPI.lunarNotesContainerPanel.style.display = "none";
            // 改变全局变量，表示无配置面板显示
            EntryAPI.OnlyData.configurePanelOption = 'any';
            // 结束事件响应
            return;
        }
        // 隐藏对话和历史记录面板
        EntryAPI.chatHistoryContainerPanel.style.display = "none";
        // 显示月华笔记面板
        EntryAPI.lunarNotesContainerPanel.style.display = "flex";
        // 改变全局变量，表示当前显示月华笔记面板
        EntryAPI.OnlyData.configurePanelOption = 'lunarNotesButton';
        // 刷新知识库页面
        EntryAPI.refreshKnowledgePage('knowledge/lunar_notes.json');
    }
);
//* 绑定 切换语音配置 按钮点击事件
voiceConfigureButton.addEventListener('click',
    function () {
        // 若当前屏幕宽度不足
        if (window.innerWidth <= EntryAPI.smallScreenWidthThreshold) return EntryAPI.showSystemMessage("< 语音配置 >不可在小屏幕下使用", "error");
        // 清除所有配置面板的显示状态
        EntryAPI.eraseAllConfigurePanel();
        // 若当前未显示系统配置
        if (EntryAPI.OnlyData.configurePanelOption === 'voiceConfigureButton') {
            // 显示对话和历史记录面板
            EntryAPI.chatHistoryContainerPanel.style.display = "flex";
            // 改变全局变量
            EntryAPI.OnlyData.configurePanelOption = 'any';
            // 结束事件响应
            return;
        }
        // 隐藏对话和历史记录面板
        EntryAPI.chatHistoryContainerPanel.style.display = "none";
        // 显示模型配置面板
        EntryAPI.speechConfigContainerPanel.style.display = "flex";
        // 改变全局变量
        EntryAPI.OnlyData.configurePanelOption = 'voiceConfigureButton';
    }
);
//* 绑定 功能控制面板 按钮点击事件
functionControlButton.addEventListener('click',
    function () {
        // 若当前屏幕宽度不足，显示错误提示并结束事件响应
        if (window.innerWidth <= EntryAPI.smallScreenWidthThreshold) return EntryAPI.showSystemMessage("< 功能控制 >不可在小屏幕下使用", "error");
        // 清除所有配置面板的显示状态
        EntryAPI.eraseAllConfigurePanel();
        // 若当前已显示功能控制面板
        if (EntryAPI.OnlyData.configurePanelOption === 'functionControlButton') {
            // 变更按钮图标，使用扳手图标表示功能控制关闭状态
            this.innerHTML = '<i class="fas fa-cog"></i>';
            // 显示对话和历史记录面板
            EntryAPI.chatHistoryContainerPanel.style.display = "flex";
            // 隐藏功能控制面板
            EntryAPI.functionControlContainerPanel.style.display = "none";
            // 隐藏聊天记录按钮
            chatHistoryButton.style.display = "none";
            // 改变全局变量，表示无配置面板显示
            EntryAPI.OnlyData.configurePanelOption = 'any';
            // 结束事件响应
            return;
        }
        // 变更按钮图标，使用齿轮组图标表示功能控制打开状态
        this.innerHTML = '<i class="fas fa-wrench"></i>';
        // 显示功能控制面板
        EntryAPI.functionControlContainerPanel.style.display = "flex";
        // 隐藏对话和历史记录面板
        EntryAPI.chatHistoryContainerPanel.style.display = "none";
        // 显示聊天记录按钮
        chatHistoryButton.style.display = "flex";
        // 变更聊天记录按钮透明度，使其可见
        chatHistoryButton.style.opacity = "0.8";
        // 变更按钮样式, 添加点击中的样式类
        this.classList.add("clicking");
        // 改变全局变量，表示当前显示功能控制面板
        EntryAPI.OnlyData.configurePanelOption = 'functionControlButton';
    }
);
//* 绑定 聊天记录面板 按钮点击事件
chatHistoryButton.addEventListener('click',
    function () {
        // 若当前屏幕宽度不足，显示错误提示并结束事件响应
        if (window.innerWidth <= EntryAPI.smallScreenWidthThreshold) return EntryAPI.showSystemMessage("< 聊天记录 >不可在小屏幕下使用", "error");
        // 清除所有配置面板的显示状态
        EntryAPI.eraseAllConfigurePanel();
        // 显示对话和历史记录面板
        EntryAPI.chatHistoryContainerPanel.style.display = "flex";
        // 隐藏功能控制面板
        EntryAPI.functionControlContainerPanel.style.display = "none";
        // 改变全局变量，表示无配置面板显示
        EntryAPI.OnlyData.configurePanelOption = 'any';
        // 隐藏聊天记录按钮
        chatHistoryButton.style.display = "none";
    }
);
//* 绑定 自动播放 按钮点击事件
autoPlaySpeechButton.addEventListener('click',
    function () {
        if (EntryAPI.OnlyData.autoPlaySpeech) {
            // 变更按钮样式
            this.innerHTML = '<i class="fas fa-volume-off"></i> 启用自动朗读';
            this.classList.add("disable");
            // 改变全局变量
            EntryAPI.OnlyData.autoPlaySpeech = false;
            EntryAPI.showSystemMessage("禁用< 消息自动朗读 >", "success");
        }
        else {
            // 变更按钮样式
            this.innerHTML = '<i class="fas fa-volume-up"></i> 禁用自动朗读';
            this.classList.remove("disable");
            // 改变全局变量
            EntryAPI.OnlyData.autoPlaySpeech = true;
            EntryAPI.showSystemMessage("启用< 消息自动朗读 >", "success");

        };
        // 重载Live2D容器
        EntryAPI.reloadLive2DContainer();
    }
);
//* 绑定 禁用语音识别自动发送 按钮点击事件
voiceRecognitionButton.addEventListener('click',
    function () {
        if (EntryAPI.OnlyData.isDisableVoiceRecognition) {
            // 变更按钮样式
            this.innerHTML = '<i class="fas fa-microphone"></i> 禁用语音发送';
            this.classList.remove("disable");
            // 改变全局变量
            EntryAPI.OnlyData.isDisableVoiceRecognition = false;
            EntryAPI.showSystemMessage("启用< 语音识别并发送 >", "success");
        }
        else {
            // 变更按钮样式
            this.innerHTML = '<i class="fas fa-microphone-slash"></i> 启用语音发送';
            this.classList.add("disable");
            // 改变全局变量
            EntryAPI.OnlyData.isDisableVoiceRecognition = true;
            EntryAPI.showSystemMessage("禁用< 语音识别并发送 >", "success");
        };
        // 重载Live2D容器
        EntryAPI.reloadLive2DContainer();
    }
);
//* 绑定 切换调试模式 按钮点击事件
debugModeButton.addEventListener('click',
    function () {
        if (EntryAPI.OnlyData.isDebugMode) {
            // 变更按钮样式
            this.innerHTML = '<i class="fas fa-star-and-crescent"></i> 启用 调试模式';
            // 改变全局变量
            EntryAPI.OnlyData.isDebugMode = false;
            EntryAPI.showSystemMessage("禁用< 调试模式 >", "success");
        }
        else {
            // 变更按钮样式
            this.innerHTML = '<i class="fas fa-code"></i> 禁用 调试模式';
            // 改变全局变量
            EntryAPI.OnlyData.isDebugMode = true;
            EntryAPI.showSystemMessage("启用< 调试模式 >", "success");
        };
        // 重载Live2D容器
        EntryAPI.reloadLive2DContainer();
    }
);
//* 绑定 切换连续记忆模式 按钮点击事件
longTermMemoryButton.addEventListener('click',
    function () {
        /**
         * 获取文档中所有的配置面板按钮元素
         */
        const configurePanelButton = document.documentElement.querySelectorAll('.power-button.live2d');
        // 遍历所有配置面板按钮，移除按钮上的点击中的样式类，恢复按钮初始样式
        configurePanelButton.forEach(button => button.classList.remove("clicking"));
        if (EntryAPI.OnlyData.isContinuousMemory) {
            // 变更按钮样式
            this.innerHTML = '<i class="fas fa-memory"></i>';
            this.classList.remove("clicking");
            // 改变全局变量
            EntryAPI.OnlyData.isContinuousMemory = false;
            EntryAPI.showSystemMessage("禁用< 连续记忆模式 >", "success");
        }
        else {
            // 变更按钮样式
            this.innerHTML = '<i class="fas fa-infinity"></i>';
            this.classList.add("clicking");
            // 改变全局变量
            EntryAPI.OnlyData.isContinuousMemory = true;
            EntryAPI.showSystemMessage("启用< 连续记忆模式 >", "success");
        };
        // 重载Live2D容器
        EntryAPI.reloadLive2DContainer();
    }
);
//* 绑定 切换主动消息模式 按钮点击事件
activeMessageButton.addEventListener('click',
    function () {
        /**
         * 获取文档中所有的配置面板按钮元素
         */
        const configurePanelButton = document.documentElement.querySelectorAll('.power-button.live2d');
        // 遍历所有配置面板按钮，移除按钮上的点击中的样式类，恢复按钮初始样式
        configurePanelButton.forEach(button => button.classList.remove("clicking"));
        if (EntryAPI.OnlyData.isActiveMessageMode) {
            // 变更按钮样式，使用无消息图标表示主动消息模式禁用状态
            this.innerHTML = '<i class="fas fa-comment-slash"></i>';
            this.classList.remove("clicking");
            // 改变全局变量
            EntryAPI.OnlyData.isActiveMessageMode = false;
            EntryAPI.showSystemMessage("禁用< 主动消息模式 >", "success");
        }
        else {
            // 变更按钮样式，使用聊天图标表示主动消息模式启用状态
            this.innerHTML = '<i class="fas fa-comment-dots"></i>';
            this.classList.add("clicking");
            // 改变全局变量
            EntryAPI.OnlyData.isActiveMessageMode = true;
            EntryAPI.showSystemMessage("启用< 主动消息模式 >", "success");
        };
        // 重载Live2D容器
        EntryAPI.reloadLive2DContainer();
    }
);
//* 绑定 切换主题风格 按钮点击事件
themeButton.addEventListener("click",
    function () {
        // 切换页面根元素(html)的暗色模式类名
        document.documentElement.classList.toggle("dark-mode");
        /**
         * 获取当前是否为暗色模式
         */
        const isDarkMode = document.documentElement.classList.contains("dark-mode");
        // 存储当前主题到本地存储中
        localStorage.setItem("theme", isDarkMode ? "dark" : "light");
        /**
         * 获取文档中所有的配置面板按钮元素
         */
        const configurePanelButton = document.documentElement.querySelectorAll('.power-button.live2d');
        // 遍历所有配置面板按钮，移除按钮上的点击中的样式类，恢复按钮初始样式
        configurePanelButton.forEach(button => button.classList.remove("clicking"));
        // 更新按钮图标
        if (isDarkMode) {
            // 变更按钮样式
            this.classList.add("clicking");
            this.innerHTML = '<i class="fas fa-sun"></i>';
        }
        else {
            // 变更按钮样式
            this.classList.remove("clicking");
            this.innerHTML = '<i class="fas fa-moon"></i>';
        };
    }
);
//* 绑定 二维码切换按钮 点击事件
qrcodeButton.addEventListener('click',
    function () {
        // 如果二维码已经显示，则关闭它
        if (EntryAPI.qrcodeStatusPanel.className.includes('show')) {
            // 变更按钮样式
            this.innerHTML = '<i class="fas fa-qrcode"></i> 显示 远程连接';
            // 设置系统状态面板的类名，移除显示类名
            EntryAPI.qrcodeStatusPanel.className = 'system-message qrcode';
        }
        else {
            // 变更按钮样式
            this.innerHTML = '<i class="fas fa-network-wired"></i> 隐藏 远程连接';
            // 设置系统状态面板的类名，包含基础类名、消息类型类名和显示类名
            EntryAPI.qrcodeStatusPanel.className = 'system-message qrcode show';
            // 拖动配置面板
            EntryAPI.dragElement(EntryAPI.qrcodeStatusPanel);
        }
    }
);

/**
 * 播放按钮点击音效
 */
function playButtonClickSound() {
    /** 随机选择一个按钮点击音效URL */
    const audio = new Audio(`/read/resources/audios/button-${EntryAPI.RandomFloor(0, 11)}.mp3`);
    // 设置音频音量为最大
    audio.volume = 1.0;
    // 播放音频
    audio.play()
};

//* 绑定 轻量渲染 按钮点击事件
renderReleaseButton.addEventListener("click", () => EntryAPI.createSimpleRendering());
//* 为输入按钮添加点击监听事件，点击时调用创建共享视觉的函数
screenshotReleaseButton.addEventListener("click", () => EntryAPI.createSimpleVisual());
//* 监听刷新按钮的点击事件，触发 refreshNoteButtonClickEvent 函数处理事件
refreshNoteButton.addEventListener('click', () => { EntryAPI.refreshKnowledgePage('knowledge/lunar_notes.json') });
//* 监听上传按钮的点击事件，触发 uploadKnowledgeBase 函数处理事件
noteReleaseButton.addEventListener("click", () => EntryAPI.uploadKnowledgeBase());
//* 监听 文枢阁 按钮点击事件，点击时跳转到[ 文枢阁 ]页面
FileVaultButton.addEventListener("click", () => setTimeout(() => window.location.href = '/file-vault', 10));
//* 监听 灵绘坊 按钮点击事件，点击时跳转到[ 灵绘坊 ]页面
ImageStudioButton.addEventListener("click", () => setTimeout(() => window.location.href = '/image-studio', 10));
//* 监听 智存库 按钮点击事件，点击时跳转到[ 智存库 ]页面
DataKeeperButton.addEventListener("click", () => setTimeout(() => window.location.href = '/data-keeper', 10));

//* 为所有矩形按钮添加点击音效事件监听器
(document.querySelectorAll(".rectangle-button") as NodeListOf<HTMLPreElement>).forEach(button => button.addEventListener('click', playButtonClickSound));

//* 为所有电源按钮添加点击音效事件监听器
(document.querySelectorAll(".power-button") as NodeListOf<HTMLPreElement>).forEach(button => button.addEventListener('click', playButtonClickSound));