// ============================================================
// 星月智能 · 知识库 — ai.js
// AI 生成 SQL（依据当前表结构）/ 模型调用 / 结果解析
// ============================================================

// AI 生成 SQL 主流程
async function generateSql() {
    const prompt = elements.sqlInput.value.trim();
    if (!prompt) { showToast('请先输入需求描述', 'info'); elements.sqlInput.focus(); return; }

    // 加载态
    elements.reviewBox.innerHTML = `
        <div class="ai-card">
            <div class="ai-loading">
                <div class="spinner"></div>
                <span>正在调用模型，依据当前表结构生成 SQL...</span>
            </div>
        </div>
    `;

    const schemaContext = await buildSchemaContext();

    const systemPrompt = `你是 SQLite 数据库的 SQL 专家。请根据用户的需求和数据表结构，生成一条 SQLite 可执行的 SQL 语句。
要求：
1. 只能使用提供的表与字段名，列名/表名使用反引号包裹。
2. 只返回一条 SQL。INSERT/UPDATE/DELETE 属于写入操作，SELECT 属于查询。
3. 对于写入类需求，确保字段名严格匹配表结构。
4. 返回值必须是合法的 JSON：{"sql":"SQL语句","explanation":"用一两句中文说明这条 SQL 的含义与作用"}。
5. 不要输出任何 JSON 之外的文字。

用户需求：${prompt}

数据表结构：
${schemaContext}`;

    try {
        const content = await callAI([
            { role: 'system', content: systemPrompt },
            { role: 'user', content: prompt }
        ]);

        const parsed = parseAIResult(content);
        if (!parsed || !parsed.sql) {
            throw new Error('模型未返回有效的 SQL');
        }

        aiResult = parsed;
        currentSQL = parsed.sql;
        renderReview({ from: 'ai', sql: parsed.sql, explanation: parsed.explanation, cls: classifySql(parsed.sql) });
    } catch (error) {
        elements.reviewBox.innerHTML = `
            <div class="ai-card">
                <div class="ai-error"><i class="fa-solid fa-circle-exclamation"></i><div>生成 SQL 失败：${escapeHtml(error.message)}</div></div>
            </div>
        `;
    }
}

// 解析 AI 返回（兼容代码块包裹的 JSON）
function parseAIResult(content) {
    if (!content) return null;
    let text = content.trim();
    const fence = text.match(/```(?:json)?\s*([\s\S]*?)```/i);
    if (fence) text = fence[1].trim();
    try {
        return JSON.parse(text);
    } catch (e) {
        const start = text.indexOf('{');
        const end = text.lastIndexOf('}');
        if (start >= 0 && end > start) {
            try { return JSON.parse(text.slice(start, end + 1)); } catch (_) { /* fallthrough */ }
        }
        return null;
    }
}

// 汇总当前库全部表结构，作为生成 SQL 的上下文
async function buildSchemaContext() {
    if (tables.length === 0) return '';
    const lines = [];
    for (const t of tables) {
        try {
            const op = await runSQL(`PRAGMA table_info(\`${t.replace(/`/g, '')}\`)`);
            const fields = (op.rows || []).map(c => `${c.name} ${c.type}`).join(', ');
            lines.push(`表 ${t}(${fields})`);
        } catch (e) { /* skip */ }
    }
    return lines.join('\n');
}

// 调用模型（OpenAI 兼容 /v1/chat/completions）
async function callAI(messages) {
    const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ model: AI_MODEL, messages, stream: false, temperature: 0.2 })
    });
    if (!response.ok) throw new Error('API 请求失败 (' + response.status + ')');
    const data = await response.json();
    if (!data.choices?.[0]?.message?.content) throw new Error('API 返回格式异常');
    return data.choices[0].message.content;
}
