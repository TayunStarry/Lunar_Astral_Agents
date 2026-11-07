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

// ---------- 用户名（浏览器缓存，共享层适配） ----------
// 首次访问/缓存缺失时显示称呼覆盖层；输入后缓存到 localStorage，刷新后直接复用。
const DESKTOP_NAME_STYLE = `
    .lunar-name-overlay { position: fixed; inset: 0; z-index: 100000; display: flex; align-items: center; justify-content: center; padding: 20px; background: rgba(10, 14, 24, 0.38); backdrop-filter: blur(8px); -webkit-backdrop-filter: blur(8px); }
    .lunar-name-card { width: min(92vw, 420px); padding: 28px 26px; border-radius: var(--radius-lg, 18px); background: var(--panel-bg, rgba(250, 250, 250, 0.92)); border: 1px solid var(--border-color, rgba(255, 255, 255, 0.45)); box-shadow: var(--shadow-glass, 0 16px 48px rgba(0, 0, 0, 0.18)); color: var(--text-primary, #0f1e0f); font-family: inherit; }
    .lunar-name-card h2 { margin: 0 0 10px; font-size: 1.25rem; font-weight: 600; }
    .lunar-name-card p { margin: 0 0 18px; color: var(--text-muted, #6b7280); font-size: 0.9rem; line-height: 1.5; }
    .lunar-name-input { width: 100%; box-sizing: border-box; padding: 12px 14px; border-radius: var(--radius-md, 10px); border: 1px solid var(--border-color, rgba(0, 0, 0, 0.12)); background: var(--panel-header-bg, rgba(255, 255, 255, 0.6)); color: inherit; font-size: 1rem; outline: none; transition: border-color 0.2s ease, box-shadow 0.2s ease; }
    .lunar-name-input:focus { border-color: var(--brand-color, #6c9bcf); box-shadow: 0 0 0 3px rgba(108, 155, 207, 0.18); }
    .lunar-name-actions { display: flex; justify-content: flex-end; margin-top: 18px; }
    .lunar-name-confirm { padding: 10px 22px; border: none; border-radius: var(--radius-md, 10px); background: var(--brand-color, #6c9bcf); color: #fff; font-size: 0.95rem; cursor: pointer; transition: background 0.2s ease, transform 0.2s ease; }
    .lunar-name-confirm:hover { background: var(--accent-hover, #5a8abf); transform: translateY(-1px); }
    .lunar-name-confirm:disabled { opacity: 0.5; cursor: not-allowed; transform: none; }
`;

const desktopUserName = window.LunarShared
    ? new window.LunarShared.UserName({ styleText: DESKTOP_NAME_STYLE })
    : null;

// 首次访问时提示称呼；已有缓存则直接使用。
function initUserName() {
    if (!desktopUserName) return Promise.resolve();
    return desktopUserName.init().then(name => {
        backendUserName = name;
    });
}

// 构造发送给月华的用户名前缀，与 QQ 适配器的入站格式一致
function buildUserNamePrefix() {
    return desktopUserName ? desktopUserName.prefix() : '';
}

// ---------- 角色渲染器（共享层适配） ----------
// 从 config/general.json 的 system.character_renderer 读取；
// 非法或未定义时默认 bedrock（保持既有行为）
async function loadCharacterRenderer() {
    characterRenderer = await window.LunarShared.renderer.loadRendererChoice();
}
