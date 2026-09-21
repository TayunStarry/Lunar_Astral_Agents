/**
 * LTPX AtoA 专用智能体模块（图表工坊）
 * 「Agent to Agent」：月华将自然语言指令交给本模块的专用智能体，
 * 由大语言模型（LLM）作为最终执行者完成图表选型与构建。
 *
 * 特性（遵循 LTPX AtoA 规范）：
 * - 独立上下文历史：保留最近 40 轮对话，超出丢弃最早的轮次
 * - 多轮工具调用循环：LLM 通过 OpenAI function calling 逐次调用 render_echart /
 *   render_mermaid / read_editor，观察工具结果并持续修正，直到给出最终答复（上限 8 次）
 * - 步骤回调：每个关键步骤通过 onStep 通知前端，同步展示到「月华操作步骤」面板
 * - 模型调用走同源 /v1 代理（OpenAI v1 协议），模型名读取 lunar_config.json 的 agent 字段，不硬编码
 */

// ===== 智能体常量与状态 =====

/** 独立上下文历史：最多保留的对话轮数（1 轮 = 用户指令 + 智能体答复） */
const CS_AGENT_MAX_ROUNDS = 40;
/** 单条指令允许的最大工具调用循环次数（防模型无限调用工具） */
const CS_AGENT_MAX_TOOL_LOOPS = 8;
/** read_editor 返回源码的最大长度（超出截断，避免撑爆模型上下文） */
const CS_EDITOR_SNIPPET_MAX = 4000;

/** 已完成的对话历史：数组元素为 { user, assistant }（各为纯文本） */
let csAgentHistory = [];

/** 模型配置加载 Promise（从 lunar_config.json 的 agent 字段读取，不硬编码） */
let csModelConfigPromise = null;

/**
 * 读取 lunar_config.json 的 agent.multimodal_model 作为模型名
 * （通过 /file/read/ 文件接口；读取失败回退默认占位值，仍走同源 /v1 代理解析）
 * @returns {Promise<string>}
 */
async function loadCSAgentModel() {
    if (csModelConfigPromise) return csModelConfigPromise;
    csModelConfigPromise = (async () => {
        try {
            const resp = await fetch('/file/read/lunar_config.json', { cache: 'no-store' });
            if (!resp.ok) throw new Error('读取配置失败 HTTP ' + resp.status);
            const cfg = await resp.json();
            const agent = (cfg && cfg.agent) || {};
            return (agent.multimodal_model && String(agent.multimodal_model)) || 'system-multimodal';
        } catch (e) {
            return 'system-multimodal';
        }
    })();
    return csModelConfigPromise;
}

// ===== 工具定义（OpenAI function calling 格式，供智能体 LLM 调用） =====

const csAgentTools = [
    {
        type: 'function',
        function: {
            name: 'render_echart',
            description: '渲染一张 ECharts 数据图表（折线/柱状/饼图/雷达/散点/热力等）。将完整的 ECharts option 以 JSON 对象传入 config，图表会实时显示在预览区并写入编辑器源码。',
            parameters: {
                type: 'object',
                properties: {
                    title: { type: 'string', description: '图表标题（简短中文，用于历史记录与回复摘要）' },
                    config: { type: 'object', description: '完整的 ECharts option JSON 对象，必须包含 xAxis/yAxis 或 series 等必要字段；不要使用函数或字符串脚本' }
                },
                required: ['title', 'config']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'render_mermaid',
            description: '渲染一张 Mermaid 结构图（flowchart 流程图 / sequenceDiagram 时序图 / classDiagram / stateDiagram / gantt 甘特图 / pie 饼图 / mindmap 等）。图表会实时显示在预览区并写入编辑器源码。',
            parameters: {
                type: 'object',
                properties: {
                    title: { type: 'string', description: '图表标题（简短中文，用于历史记录与回复摘要）' },
                    code: { type: 'string', description: '完整的 Mermaid 图表源码，首行必须是图表类型声明（如 flowchart TD、sequenceDiagram、gantt）' }
                },
                required: ['title', 'code']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'read_editor',
            description: '读取当前编辑器中的图表源码（Markdown 文本，含 echarts/mermaid 代码块），用于在修改或追加图表前了解已有内容。',
            parameters: {
                type: 'object',
                properties: {},
                required: []
            }
        }
    }
];

// ===== 系统提示词（专用角色） =====

function buildCSSystemPrompt() {
    return [
        '你是星月智能「图表工坊」的专用智能体（AtoA 执行者），负责解析月华发来的自然语言指令，',
        '并作为图表模块的最终执行者，通过工具把图表渲染出来。',
        '',
        '【工作流程（必须遵守）】',
        '在输出任何图表内容之前，你必须先声明两件事：1) 技术格式（优先 ECharts，其次 Mermaid/Markdown 代码块）；2) 具体图表类型。',
        '系统会根据你的声明自动载入对应的《图表样式参考》（含样式骨架示例），你必须依据参考中的配色体系、排版布局与专业底线修正和完善图表输出。',
        '样式骨架仅示范样式字段与格式结构：骨架中的样式参数照用，数据一律替换为指令对应的真实数据，禁止照抄占位数据。',
        '',
        '【能力范围】',
        '1. ECharts 数据图表：柱状/折线/饼图/环形/雷达/散点/仪表盘等，适合展示量化数据。',
        '2. Mermaid 结构图：flowchart 流程图、sequenceDiagram 时序图、gantt 甘特图、树状图/思维导图、stateDiagram 状态图等。',
        '',
        '【执行规则】',
        '1. 数据对比/占比/趋势类需求 → 调用 render_echart，config 必须是语法合法的完整 ECharts option JSON 对象；',
        '   数值必须由你根据指令内容给出（指令没有数据时可构造合理的示例数据并向月华说明）。',
        '2. 流程/时序/状态/计划/层级类需求 → 调用 render_mermaid，code 首行必须是合法的图表类型声明，并注入样式参考给出的主题变量。',
        '3. 图表选型冲突时优先 ECharts；多条图表需求可多次调用工具分别渲染。',
        '4. 构建每个图表时，严格遵循已载入的《图表样式参考》：色彩取共享色板、布局按规范参数、遵守专业底线（禁 3D/阴影/彩虹色）。',
        '5. 若渲染失败（工具返回 error），根据失败原因修正参数重试，最多重试 2 次，仍失败则如实告知月华。',
        '6. 全部渲染完成后，用一两句简洁中文总结渲染了哪些图表，不要在回复里输出图表源码或代码块。'
    ].join('\n');
}

// ===== 声明阶段：图表格式与类型判定（正式输出图表内容前必须先行声明） =====

/** 声明阶段系统提示词：只输出格式与类型两行，禁止输出图表内容 */
function buildCSDeclSystemPrompt() {
    return [
        '你是星月智能「图表工坊」的图表选型分析器。根据月华的指令，判定构建图表所用的技术格式与具体图表类型。',
        '',
        '【判定规则】',
        '1. 数据对比/占比/趋势/分布/评估类需求优先选择 ECharts；',
        '2. 流程/时序/计划排期/层级树状/状态机等结构类需求选择 Mermaid（以 Markdown 代码块渲染）。',
        '',
        '【输出格式（必须严格遵循，禁止输出任何其他内容或图表）】',
        '格式：ECharts 或 Mermaid',
        '类型：<具体图表类型，如 折线图/面积图/柱状图/饼状图/环形图/散点图/雷达图/仪表盘/流程图/时序图/甘特图/树状图/状态图，不确定时写最接近的类型>'
    ].join('\n');
}

/**
 * 解析声明阶段的模型输出
 * @param {string} text - 模型原始输出
 * @returns {{ format:'echarts'|'mermaid', formatLabel:string, type:string } | null}
 */
function parseCSDeclaration(text) {
    const fmtMatch = text.match(/格式\s*[:：]\s*(.+)/);
    const typeMatch = text.match(/类型\s*[:：]\s*(.+)/);
    if (!typeMatch) return null;
    const fmtRaw = fmtMatch ? fmtMatch[1].trim() : '';
    let format = 'echarts', formatLabel = 'ECharts';
    if (/mermaid|markdown|md|结构/i.test(fmtRaw)) { format = 'mermaid'; formatLabel = /markdown/i.test(fmtRaw) ? 'Mermaid（Markdown 代码块）' : 'Mermaid'; }
    else if (/echarts/i.test(fmtRaw)) { format = 'echarts'; formatLabel = 'ECharts'; }
    const type = typeMatch[1].trim().slice(0, 24);
    return { format: format, formatLabel: formatLabel, type: type };
}

// ===== 模型调用（OpenAI v1 协议，同源 /v1 代理解析模型配置） =====

/**
 * 调用 /v1/chat/completions（同源代理，避免 CORS）
 * @param {Array<Object>} messages - 完整消息数组（含 system / user / assistant / tool）
 * @param {boolean} includeTools - 是否附带工具定义（声明阶段不附带，强制纯文本输出）
 * @returns {Promise<Object>} OpenAI 响应中的 message 对象
 */
async function callCSModel(messages, includeTools) {
    const model = await loadCSAgentModel();
    const body = { model: model, messages: messages, stream: false };
    if (includeTools !== false) body.tools = csAgentTools;
    const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
    });
    if (!response.ok) throw new Error('AI 调用失败: HTTP ' + response.status);
    const data = await response.json();
    // OpenAI 兼容原始响应
    if (data.choices && data.choices[0] && data.choices[0].message) {
        return data.choices[0].message;
    }
    // 代理包装响应
    if (data.success && data.data && data.data.choices && data.data.choices[0]) {
        return data.data.choices[0].message;
    }
    throw new Error('AI 响应格式异常');
}

/**
 * 安全解析工具参数（兼容 JSON 字符串与已解析对象，解析失败回退空对象）
 */
function safeParseCSArgs(jsonStr) {
    if (jsonStr && typeof jsonStr === 'object') return jsonStr;
    if (!jsonStr) return {};
    try {
        const parsed = JSON.parse(jsonStr);
        return parsed && typeof parsed === 'object' ? parsed : {};
    } catch (e) {
        return {};
    }
}

// ===== 工具执行器 =====

/**
 * 追加一个 Markdown 代码块到编辑器并实时渲染
 * （appendChartBlock / getEditorSource 由 script.js 提供）
 */
async function execCSRenderEchart(args) {
    const title = String(args.title || '').trim() || '未命名图表';
    let config = args.config;
    if (typeof config === 'string') {
        try { config = JSON.parse(config); } catch (e) {
            return { success: false, error: 'config 不是合法 JSON：' + e.message };
        }
    }
    if (!config || typeof config !== 'object' || Array.isArray(config)) {
        return { success: false, error: 'config 必须是 ECharts option JSON 对象' };
    }
    if (typeof appendChartBlock !== 'function') return { success: false, error: '页面渲染能力未就绪' };
    appendChartBlock('echarts', JSON.stringify(config, null, 2), title);
    return { success: true, text: '已渲染 ECharts 图表「' + title + '」', title: title };
}

async function execCSRenderMermaid(args) {
    const title = String(args.title || '').trim() || '未命名图表';
    const code = String(args.code || '').trim();
    if (!code) return { success: false, error: 'code 不能为空' };
    if (typeof appendChartBlock !== 'function') return { success: false, error: '页面渲染能力未就绪' };
    appendChartBlock('mermaid', code, title);
    return { success: true, text: '已渲染 Mermaid 图表「' + title + '」', title: title };
}

async function execCSReadEditor() {
    if (typeof getEditorSource !== 'function') return { success: false, error: '页面渲染能力未就绪' };
    const src = getEditorSource();
    const snippet = src.length > CS_EDITOR_SNIPPET_MAX
        ? src.slice(0, CS_EDITOR_SNIPPET_MAX) + '\n…（已截断，共 ' + src.length + ' 字符）'
        : (src || '（编辑器当前为空）');
    return { success: true, text: snippet, data: { length: src.length } };
}

/** 工具名 → 执行器映射 */
const csToolExecutors = {
    render_echart: execCSRenderEchart,
    render_mermaid: execCSRenderMermaid,
    read_editor: execCSReadEditor
};

/**
 * 执行单个工具并返回结构化结果（供回填给模型）
 * @param {string} name - 工具名
 * @param {Object} args - 结构化参数
 * @returns {Promise<Object>} { success, text?, error? }
 */
async function executeCSTool(name, args) {
    const executor = csToolExecutors[name];
    if (!executor) return { success: false, error: '未知工具: ' + name };
    try {
        return await executor(args || {});
    } catch (e) {
        return { success: false, error: (e && e.message) || String(e) };
    }
}

// ===== 主流程：多轮工具调用循环 =====

/**
 * 运行图表工坊 LLM 智能体，处理一条月华指令
 * @param {string} instruction - 月华发来的自然语言指令
 * @param {Function} onStep - 步骤回调 (step) => void，step 为 { kind, text }
 * @returns {Promise<Object>} { success, text, error }
 */
async function runCSAgent(instruction, onStep) {
    const text = String(instruction || '').trim();
    if (!text) throw new Error('空指令');

    if (onStep) onStep({ kind: 'info', text: '收到月华指令：' + text });

    // ===== 阶段一：格式与类型声明（正式输出图表内容前必须先行声明） =====
    let decl = null;
    try {
        if (onStep) onStep({ kind: 'thinking', text: '智能体正在判定图表格式与类型...' });
        const declMsg = await callCSModel([
            { role: 'system', content: buildCSDeclSystemPrompt() },
            { role: 'user', content: '【月华指令】' + text }
        ], false);
        decl = parseCSDeclaration(String(declMsg.content || ''));
    } catch (e) {
        console.warn('图表声明阶段失败，回退通用规范:', e);
    }
    if (!decl) {
        decl = { format: 'echarts', formatLabel: 'ECharts（默认）', type: '' };
        if (onStep) onStep({ kind: 'warn', text: '未能解析图表声明，按默认 ECharts 处理' });
    } else {
        if (onStep) onStep({ kind: 'info', text: '图表声明 → 格式：' + decl.formatLabel + ' · 类型：' + decl.type });
    }

    // ===== 阶段二：按声明检索样式参考库，完整载入智能体上下文 =====
    const styleGuide = lookupCSStyleGuide(decl.format, decl.type);
    if (onStep) onStep({ kind: 'tool-result', text: '已载入样式参考：《' + styleGuide.name + '》' });

    // 消息骨架：系统提示 + 样式参考（含样式骨架示例）+ 独立上下文历史（最近 40 轮）+ 本轮指令（附带编辑器现状作为业务上下文）
    const messages = [
        { role: 'system', content: buildCSSystemPrompt() },
        { role: 'system', content: '【图表样式参考（系统按声明自动载入，构建图表时必须遵循）】\n' + buildCSStyleGuideText(styleGuide) }
    ];
    for (const round of csAgentHistory) {
        messages.push({ role: 'user', content: round.user });
        messages.push({ role: 'assistant', content: round.assistant });
    }
    let bizContext = '';
    try {
        if (typeof getEditorSource === 'function') {
            const src = getEditorSource();
            bizContext = '\n【业务上下文】编辑器当前' + (src.trim() ? '已有 ' + src.length + ' 字符的图表源码，可用 read_editor 查看' : '为空');
        }
    } catch (e) { /* 上下文读取失败不阻塞主流程 */ }
    messages.push({ role: 'user', content: '【月华指令】' + text + bizContext });

    let lastReply = '';
    for (let loop = 0; loop < CS_AGENT_MAX_TOOL_LOOPS; loop++) {
        if (onStep) onStep({ kind: 'thinking', text: '智能体正在分析指令并决策...' });
        const message = await callCSModel(messages);
        const toolCalls = Array.isArray(message.tool_calls) ? message.tool_calls : [];

        // 无工具调用：智能体给出最终答复
        if (toolCalls.length === 0) {
            lastReply = String(message.content || '').trim() || '已完成';
            if (onStep) onStep({ kind: 'done', text: '生成答复：' + lastReply });
            break;
        }

        // 记录助手工具调用（供工具结果回填时关联）
        messages.push({
            role: 'assistant',
            content: message.content || '',
            tool_calls: toolCalls.map(tc => ({
                id: tc.id,
                type: 'function',
                function: { name: tc.function.name, arguments: tc.function.arguments }
            }))
        });

        // 依次执行工具，将结果作为 tool 消息回填
        for (const tc of toolCalls) {
            const args = safeParseCSArgs(tc.function.arguments);
            if (onStep) onStep({ kind: 'tool', text: '调用工具 ' + tc.function.name + '(' + JSON.stringify(args).slice(0, 120) + ')' });
            const result = await executeCSTool(tc.function.name, args);
            if (onStep) {
                const preview = String(result.text || result.error || '').slice(0, 120);
                onStep({ kind: result.success ? 'tool-result' : 'error', text: (result.success ? '工具结果：' : '工具失败：') + preview });
            }
            messages.push({ role: 'tool', tool_call_id: tc.id, content: JSON.stringify(result) });
        }
    }

    // 工具循环耗尽仍未给出最终答复（模型持续调用工具）
    if (!lastReply) lastReply = '已完成图表渲染';

    // 记录本轮对话并裁剪历史（保留最近 40 轮，超出丢弃最早的）
    csAgentHistory.push({ user: text, assistant: lastReply });
    if (csAgentHistory.length > CS_AGENT_MAX_ROUNDS) {
        csAgentHistory.splice(0, csAgentHistory.length - CS_AGENT_MAX_ROUNDS);
    }

    return { success: true, text: lastReply, error: '' };
}
