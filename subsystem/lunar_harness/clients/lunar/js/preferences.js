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

// ---------- 用户名（浏览器缓存） ----------
// 首次访问/缓存缺失时显示称呼覆盖层；输入后缓存到 localStorage，刷新后直接复用。
const USER_NAME_STORAGE_KEY = 'lunar_user_name';

function readCachedUserName() {
    try {
        const saved = localStorage.getItem(USER_NAME_STORAGE_KEY);
        return saved && saved.trim() ? saved.trim() : '';
    } catch (e) {
        return '';
    }
}

function writeCachedUserName(name) {
    try {
        localStorage.setItem(USER_NAME_STORAGE_KEY, name);
    } catch (e) {
        // 隐私模式等场景下写入失败：本次会话仍可使用
    }
}

function injectUserNameOverlayStyle() {
    if (document.getElementById('lunarUserNameStyle')) return;
    const style = document.createElement('style');
    style.id = 'lunarUserNameStyle';
    style.textContent = `
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
    document.head.appendChild(style);
}

// 首次访问时提示称呼；已有缓存则直接使用。
function initUserName() {
    const cached = readCachedUserName();
    if (cached) {
        backendUserName = cached;
        return Promise.resolve();
    }
    injectUserNameOverlayStyle();
    return new Promise((resolve) => {
        const overlay = document.createElement('div');
        overlay.className = 'lunar-name-overlay';
        overlay.innerHTML = `
            <div class="lunar-name-card" role="dialog" aria-modal="true" aria-labelledby="lunarNameTitle">
                <h2 id="lunarNameTitle">希望月华如何称呼您？</h2>
                <p>这个名字会用于对话与消息气泡，并只保存在当前浏览器。</p>
                <input class="lunar-name-input" id="lunarNameInput" type="text" maxlength="24" placeholder="请输入您的称呼" autocomplete="nickname">
                <div class="lunar-name-actions">
                    <button class="lunar-name-confirm" id="lunarNameConfirm" type="button" disabled>确认</button>
                </div>
            </div>
        `;
        document.body.appendChild(overlay);

        const input = overlay.querySelector('#lunarNameInput');
        const button = overlay.querySelector('#lunarNameConfirm');
        const submit = () => {
            const name = input.value.trim();
            if (!name) {
                input.focus();
                return;
            }
            writeCachedUserName(name);
            backendUserName = name;
            overlay.remove();
            resolve();
        };
        input.addEventListener('input', () => {
            button.disabled = !input.value.trim();
        });
        input.addEventListener('keydown', (event) => {
            if (event.key === 'Enter') {
                event.preventDefault();
                submit();
            }
        });
        button.addEventListener('click', submit);
        requestAnimationFrame(() => input.focus());
    });
}

// 构造发送给月华的用户名前缀，与 QQ 适配器的入站格式一致
function buildUserNamePrefix() {
    return `[用户: ${backendUserName}]: `;
}

// ---------- 角色渲染器 ----------
// 从 lunar_config.json 的 system.character_renderer 读取；
// 非法或未定义时默认 bedrock（保持既有行为）
async function loadCharacterRenderer() {
    try {
        const res = await fetch('/file/read/lunar_config.json');
        if (!res.ok) return;
        const config = await res.json();
        const renderer = config && config.system ? config.system.character_renderer : null;
        if (renderer === 'live2d' || renderer === 'bedrock') {
            characterRenderer = renderer;
        }
    } catch (e) {
        // 配置读取失败：保持默认 bedrock
    }
}
