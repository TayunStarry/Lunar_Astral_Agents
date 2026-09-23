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

// ---------- 后端用户名（QQ 适配器同款标注，仅用于发送给月华，前端展示不使用） ----------
// 从 lunar_config.json 的 server.user_name 读取；未定义或为空时默认「子幽」
async function loadBackendUserName() {
    try {
        const res = await fetch('/file/read/lunar_config.json');
        if (!res.ok) return;
        const config = await res.json();
        const name = config && config.server ? config.server.user_name : null;
        if (typeof name === 'string' && name.trim()) {
            backendUserName = name.trim();
        }
    } catch (e) {
        // 配置读取失败：保持默认用户名
    }
}

// 构造发送给月华的用户名前缀，与 QQ 适配器的入站格式一致
function buildUserNamePrefix() {
    return `[用户: ${backendUserName}]: `;
}
