// ============================================================
//  星月智能 · 消息终端 — 共享层就绪守卫
//
// 桌面端后续脚本都依赖 /shared/desktop.js 挂载的 window.LunarShared。
// 桥接脚本按文档顺序排在本脚本之前，但「能执行到」不等于「加载成功」：
// /shared/ 路由缺失、模块语法错误、网络失败都会让桥接静默缺席。
//
// 这里不假设网络时序：
// - 优先响应桥接脚本显式派发的 lunar-shared-ready
// - 以 window.load 作为确定性边界（模块脚本会阻塞 load），load 后仍缺失即判定失败
// - 失败时渲染可见错误层，而不是白屏或半死状态
// ============================================================

(function () {
    let errorShown = false;

    function showError() {
        if (errorShown || document.getElementById('lunarSharedError')) return;
        errorShown = true;
        console.error('[星月智能] 共享模块加载失败：window.LunarShared 不存在，请检查 /shared/ 静态路由与 /shared/desktop.js');

        const overlay = document.createElement('div');
        overlay.id = 'lunarSharedError';
        overlay.setAttribute('role', 'alert');
        overlay.style.cssText = [
            'position:fixed', 'inset:0', 'z-index:2147483647',
            'display:flex', 'align-items:center', 'justify-content:center',
            'padding:24px', 'background:rgba(8,10,20,0.82)',
            'backdrop-filter:blur(10px)', '-webkit-backdrop-filter:blur(10px)',
            'font-family:system-ui,-apple-system,"Segoe UI",sans-serif'
        ].join(';');

        const card = document.createElement('div');
        card.style.cssText = [
            'max-width:460px', 'width:100%', 'padding:24px 26px',
            'border-radius:16px', 'background:rgba(30,30,44,0.96)',
            'border:1px solid rgba(255,255,255,0.18)',
            'box-shadow:0 18px 48px rgba(0,0,0,0.45)', 'color:#f2f2f5'
        ].join(';');

        const title = document.createElement('div');
        title.textContent = '共享模块加载失败';
        title.style.cssText = 'font-size:17px;font-weight:600;margin-bottom:10px;color:#ffd7d7;';

        const detail = document.createElement('div');
        detail.textContent = '页面依赖的 /shared/ 共享模块没有加载成功，界面无法完整启动。请确认服务端已重新构建（/shared/ 静态路由已注册），然后重试。';
        detail.style.cssText = 'font-size:13px;line-height:1.65;color:rgba(255,255,255,0.78);margin-bottom:18px;';

        const retry = document.createElement('button');
        retry.type = 'button';
        retry.textContent = '重新加载';
        retry.style.cssText = [
            'padding:9px 18px', 'border-radius:999px', 'cursor:pointer',
            'border:1px solid rgba(157,107,255,0.5)',
            'background:rgba(157,107,255,0.22)', 'color:#ffffff',
            'font-size:13px', 'font-weight:600'
        ].join(';');
        retry.addEventListener('click', () => window.location.reload());

        card.appendChild(title);
        card.appendChild(detail);
        card.appendChild(retry);
        overlay.appendChild(card);

        const mount = () => document.body
            ? document.body.appendChild(overlay)
            : document.addEventListener('DOMContentLoaded', () => document.body.appendChild(overlay), { once: true });
        mount();
    }

    /**
     * 等待共享层就绪
     *
     * @returns {Promise<boolean>} 就绪返回 true；确定缺失（load 后仍无）返回 false 并展示错误层
     */
    window.waitForLunarShared = function () {
        if (window.LunarShared) return Promise.resolve(true);
        return new Promise(resolve => {
            let settled = false;
            const finish = (ok) => {
                if (settled) return;
                settled = true;
                resolve(ok);
            };
            window.addEventListener('lunar-shared-ready', () => finish(!!window.LunarShared), { once: true });
            if (document.readyState === 'complete') {
                finish(!!window.LunarShared);
                return;
            }
            window.addEventListener('load', () => {
                // load 后再等一拍，让同批次派发的 ready 事件落定
                setTimeout(() => finish(!!window.LunarShared), 0);
            }, { once: true });
        }).then(ok => {
            if (!ok) showError();
            return ok;
        });
    };

    // 兜底：即使入口脚本没有调用 waitForLunarShared，加载失败也要显式报错
    window.addEventListener('load', () => {
        setTimeout(() => {
            if (!window.LunarShared) showError();
        }, 0);
    }, { once: true });
})();
