// ============================================================
// 星月智能 · 知识库 — detect.js
// 输入识别：SQL 还是自然语言（驱动按钮形态与提示）
// ============================================================

function isLikelySQL(text) {
    const t = (text || '').trim();
    if (t.length < 4) return false;
    // 去掉注释前缀后判断首关键字
    const clean = t.replace(/^\/\*[\s\S]*?\*\//, '').trim().toUpperCase();
    if (/^(SELECT|INSERT|UPDATE|DELETE|CREATE|DROP|ALTER|TRUNCATE|WITH|PRAGMA|EXPLAIN|REPLACE|BEGIN|COMMIT|ROLLBACK)\b/.test(clean)) {
        return true;
    }
    // 多行且含明显 SQL 关键字
    if (t.includes('\n') && /\b(SELECT|INSERT|UPDATE|DELETE|CREATE|DROP|ALTER|FROM|WHERE|INTO|VALUES|SET)\b/i.test(t)) {
        return true;
    }
    return false;
}

function updateDetectState() {
    const text = elements.sqlInput.value;
    const isSQL = isLikelySQL(text);
    const btn = elements.btnRun;
    const hint = elements.detectHint;

    if (isSQL) {
        btn.className = 'btn btn-primary btn-run';
        btn.innerHTML = '<i class="fas fa-bolt"></i> 执行操作';
        hint.innerHTML = '<i class="fas fa-code"></i> 识别为 <b>SQL 语句</b>，点击可直接审核并执行';
    } else {
        btn.className = 'btn btn-primary btn-run';
        btn.innerHTML = '<i class="fas fa-wand-magic-sparkles"></i> 生成 SQL';
        hint.innerHTML = '<i class="fas fa-circle-info"></i> 自动识别 SQL / 自然语言';
    }
}
