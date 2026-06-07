document.addEventListener('DOMContentLoaded', async function () {
    // 等待标准依赖库加载完成（包含qrcode.min.js）
    await waitForDependencies();

    // 获取服务器信息
    const serverInfo = await fetchServerInfo();

    if (serverInfo) {
        // 生成二维码
        generateQRCode(serverInfo.url);
        // 显示服务器URL
        document.getElementById('server-url').textContent = serverInfo.url;
    }

    // 检查健康状态
    checkHealth();
    // 绑定复制按钮
    bindCopyButton();
});

// ==== 依赖等待 ====

/**
 * 等待QRCode库加载完成
 */
async function waitForDependencies() {
    let retries = 0;
    while (typeof QRCode === 'undefined' && retries < 50) {
        await new Promise(resolve => setTimeout(resolve, 100));
        retries++;
    }
    if (typeof QRCode === 'undefined') {
        console.warn('QRCode库加载超时，将使用备用方案');
    }
}

// ==== 数据获取 ====

/**
 * 获取服务器信息
 */
async function fetchServerInfo() {
    try {
        const response = await fetch('/api/server-info');
        if (response.ok) {
            return await response.json();
        }
    } catch (e) {
        console.error('获取服务器信息失败:', e);
    }
    // 回退到window.location
    return {
        url: window.location.origin,
        ip: window.location.hostname,
        port: window.location.port || '443'
    };
}

// ==== 二维码 ====

/**
 * 生成二维码，内容为服务器访问URL
 */
function generateQRCode(url) {
    const container = document.getElementById('qrcode-container');
    if (typeof QRCode !== 'undefined') {
        container.innerHTML = '';
        new QRCode(container, {
            text: url,
            width: 200,
            height: 200,
            colorDark: '#1a1a2e',
            colorLight: '#ffffff',
            correctLevel: QRCode.CorrectLevel.H
        });
    } else {
        container.innerHTML = '<p class="error-text">二维码库加载失败，请手动复制链接</p>';
    }
}

// ==== 健康检查 ====

/**
 * 检查服务健康状态
 */
async function checkHealth() {
    const statusEl = document.querySelector('.status-text');
    const dotEl = document.querySelector('.status-dot');
    try {
        const response = await fetch('/health');
        if (response.ok) {
            statusEl.textContent = '服务运行中';
            dotEl.classList.add('online');
        } else {
            statusEl.textContent = '服务异常';
            dotEl.classList.remove('online');
        }
    } catch (e) {
        statusEl.textContent = '服务离线';
        dotEl.classList.remove('online');
    }
}

// ==== 复制功能 ====

/**
 * 绑定复制按钮事件
 */
function bindCopyButton() {
    document.getElementById('copy-btn').addEventListener('click', function () {
        const url = document.getElementById('server-url').textContent;
        if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(url).then(() => {
                showCopySuccess();
            }).catch(() => {
                fallbackCopy(url);
            });
        } else {
            fallbackCopy(url);
        }
    });
}

/**
 * 显示复制成功反馈 - 使用CSS类驱动
 */
function showCopySuccess() {
    const btn = document.getElementById('copy-btn');
    btn.classList.add('success');
    btn.innerHTML = '<i class="fas fa-check"></i>';
    showToast('链接已复制到剪贴板');
    setTimeout(() => {
        btn.classList.remove('success');
        btn.innerHTML = '<i class="fas fa-copy"></i>';
    }, 2000);
}

/**
 * 备用复制方法
 */
function fallbackCopy(text) {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    try {
        document.execCommand('copy');
        showCopySuccess();
    } catch (e) {
        console.error('复制失败:', e);
    }
    document.body.removeChild(textarea);
}

// ==== Toast 提示 ====

/**
 * 显示 Toast 提示
 */
function showToast(message) {
    const toast = document.getElementById('toast');
    toast.textContent = message;
    toast.classList.add('visible');
    setTimeout(() => {
        toast.classList.remove('visible');
    }, 2500);
}
