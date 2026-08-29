// ============================================================
//  星月智能 · 消息终端 — 主题切换 / 语音自动播放开关
// ============================================================

// ---------- 主题切换 ----------
function loadTheme() {
    const saved = localStorage.getItem('message_terminal_theme');
    if (saved === 'dark') {
        isDarkMode = true;
        document.body.classList.add('dark-mode');
        themeToggle.innerHTML = '<i class="fas fa-sun"></i>';
    }
}

function toggleTheme() {
    isDarkMode = !isDarkMode;
    document.body.classList.toggle('dark-mode', isDarkMode);
    themeToggle.innerHTML = isDarkMode ? '<i class="fas fa-sun"></i>' : '<i class="fas fa-moon"></i>';
    localStorage.setItem('message_terminal_theme', isDarkMode ? 'dark' : 'light');
    mermaidInitialized = false;
    initMermaid();
    if (musicReady) musicChannel.postMessage({ type: 'theme', darkMode: isDarkMode });
}

function setupThemeToggle() {
    themeToggle.addEventListener('click', toggleTheme);
}

// ---------- 语音自动播放开关 ----------
function loadVoiceAutoPlay() {
    autoPlayVoice = localStorage.getItem('message_terminal_autoplay') !== 'off';
    updateVoiceToggleUI();
}

function toggleVoiceAutoPlay() {
    autoPlayVoice = !autoPlayVoice;
    localStorage.setItem('message_terminal_autoplay', autoPlayVoice ? 'on' : 'off');
    updateVoiceToggleUI();
    if (!autoPlayVoice) AudioQueue.stop();
}

function updateVoiceToggleUI() {
    voiceToggleBtn.classList.toggle('active', autoPlayVoice);
    voiceToggleBtn.title = autoPlayVoice ? '自动播放语音：开' : '自动播放语音：关';
    voiceToggleBtn.innerHTML = autoPlayVoice ? '<i class="fas fa-volume-up"></i>' : '<i class="fas fa-volume-mute"></i>';
}

function setupVoiceToggle() {
    voiceToggleBtn.addEventListener('click', toggleVoiceAutoPlay);
}
