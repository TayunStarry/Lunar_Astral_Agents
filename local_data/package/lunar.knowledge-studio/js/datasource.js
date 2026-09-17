// ============================================================
// 星月智能 · 知识库 — datasource.js
// 数据库探查（唯一数据源入口）：
//   启动时自动执行；/file/list/database 递归 BFS 收集全部 *.db
//   → /file/read/（Range bytes=0-15）验证 SQLite 魔数
//   → /knowledge/ 执行 SELECT count(*) FROM sqlite_master 二次确认可用
//   → 可用/不可用分组展示，点击可用项即切换数据源；按钮仅用于重新扫描
// ============================================================

// 递归收集目录下全部 *.db（BFS 子目录；path 统一归一化为 / 分隔，规避 Windows 反斜杠）
async function collectDbFiles(relDir, acc = []) {
    const res = await fetch('/file/list/' + relDir, { method: 'POST' });
    if (!res.ok) throw new Error(`目录列表失败 (${relDir}): HTTP ${res.status}`);
    const list = await res.json();
    for (const f of (list || [])) {
        const p = String(f.path || '').replace(/\\/g, '/');
        if (f.isDir) {
            await collectDbFiles(p, acc);
        } else if (/\.db$/i.test(f.name)) {
            acc.push({ ...f, path: p });
        }
    }
    return acc;
}

// dbKey 由相对 LocalDir 的路径（database/xxx.db）转为相对 database 目录且去 .db 的键
function dbKeyOf(relPath) {
    return String(relPath).replace(/\\/g, '/').replace(/^database\//i, '').replace(/\.db$/i, '');
}

// 探测单个 .db：魔数校验（前 16 字节）→ SQL 可用性复验
async function probeDatabase(fileInfo) {
    const item = {
        name: fileInfo.name,
        path: fileInfo.path,
        key: dbKeyOf(fileInfo.path),
        valid: false,
        reason: ''
    };
    try {
        // 1) 魔数校验：SQLite 文件头为 "SQLite format 3\0"
        const res = await fetch('/file/read/' + fileInfo.path, { headers: { Range: 'bytes=0-15' } });
        if (!res.ok && res.status !== 206) throw new Error('HTTP ' + res.status);
        const buf = new Uint8Array(await res.arrayBuffer());
        const magic = 'SQLite format 3\0';
        let magicOk = buf.length >= 16;
        for (let i = 0; magicOk && i < 16; i++) {
            if (buf[i] !== magic.charCodeAt(i)) magicOk = false;
        }
        if (!magicOk) {
            item.reason = '非 SQLite 文件';
            return item;
        }
        // 2) SQL 复验：能查询 sqlite_master 即为可用库
        const check = await fetch(API_BASE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ sql: 'SELECT count(*) AS n FROM sqlite_master', params: [], db: item.key })
        });
        const data = await check.json();
        if (!data.success) {
            item.reason = data.error || '无法执行 SQL';
            return item;
        }
        item.valid = true;
    } catch (e) {
        item.reason = e.message;
    }
    return item;
}

// 扫描入口：收集 → 探测 → 渲染
async function scanDatabases() {
    const btn = elements.btnScanDb;
    btn.disabled = true;
    elements.dbScanResult.innerHTML = '<p class="message info">正在扫描…</p>';
    try {
        const files = await collectDbFiles('database');
        const results = [];
        for (const f of files) {
            results.push(await probeDatabase(f));
        }
        results.sort((a, b) => (a.valid === b.valid ? a.key.localeCompare(b.key) : (a.valid ? -1 : 1)));
        renderDbScan(results);
    } catch (e) {
        elements.dbScanResult.innerHTML = `<p class="message error">扫描失败：${escapeHtml(e.message)}</p>`;
        showToast('扫描数据库失败: ' + e.message, 'error');
    } finally {
        btn.disabled = false;
    }
}

// 渲染探查结果（可用在前、不可用在后；可用项点击即切换数据源）
function renderDbScan(results) {
    if (!results.length) {
        elements.dbScanResult.innerHTML = '<p class="message info">未发现 *.db 文件</p>';
        return;
    }
    elements.dbScanResult.innerHTML = results.map(it => `
        <div class="db-scan-item ${it.valid ? '' : 'unusable'} ${it.key === currentDB ? 'active' : ''}"
             data-db="${escapeHtml(it.key)}" data-valid="${it.valid}" title="${escapeHtml(it.path)}">
            <i class="fas ${it.valid ? 'fa-database' : 'fa-ban'}"></i>
            <span class="ds-name">${escapeHtml(it.path)}</span>
            <span class="ds-status ${it.valid ? 'ok' : 'bad'}" title="${escapeHtml(it.reason || '可用的 SQLite 库')}">${it.valid ? '可用' : '不可用'}</span>
        </div>
    `).join('');
    elements.dbScanResult.querySelectorAll('.db-scan-item[data-valid="true"]').forEach(el => {
        el.addEventListener('click', () => selectScannedDb(el.dataset.db));
    });
}

// 点击可用项切换数据源
function selectScannedDb(key) {
    if (key === currentDB) return;
    currentDB = key;
    selectedTable = null;
    clearCurrentTable();
    refreshTables().then(() => showToast('已切换数据源：' + key, 'success'));
    markActiveDbItem();
}

// 高亮当前数据源对应的探查项
function markActiveDbItem() {
    elements.dbScanResult.querySelectorAll('.db-scan-item').forEach(el => {
        el.classList.toggle('active', el.dataset.db === currentDB);
    });
}
