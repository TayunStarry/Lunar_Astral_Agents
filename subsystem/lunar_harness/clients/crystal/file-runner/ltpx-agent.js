/**
 * LTPX AtoA 专用智能体模块（编辑运行）
 * 「Agent to Agent」：月华将自然语言指令交给本模块的专用智能体，
 * 由大语言模型（LLM）作为最终执行者完成代码的编写、修改与运行。
 *
 * 能力范围：
 * - 单文件 JavaScript：ES2023+ 标准，运行于浏览器沙箱页面（可使用全部 WebAPI：DOM、Canvas、
 *   Fetch、localStorage、定时器、音频视频等），控制台输出实时回传
 * - 完整 HTML 文档：内联 CSS/JS 的单文件页面，直接渲染预览
 *
 * 人机协同原则：
 * - 智能体写入/读取的代码同步显示在页面编辑器，用户可随时查看、修改后手动再次运行
 * - 智能体遵循「读现状 → 写代码 → 运行 → 看输出 → 修正」循环，控制台错误驱动自修复
 *
 * 特性：
 * - 独立上下文历史：保留最近 40 轮对话，超出丢弃最早的轮次
 * - 多轮工具调用循环：OpenAI function calling，上限 8 次防死循环
 * - 模型调用走琉璃后端 /v1 代理（OpenAI v1 协议），模型名从 lunar_config.json 读取，不硬编码
 */

// ===== 智能体常量与状态 =====

/** 独立上下文历史：最多保留的对话轮数（1 轮 = 用户指令 + 智能体答复） */
const CS_AGENT_MAX_ROUNDS = 40;
/** 单条指令允许的最大工具调用循环次数（防模型无限调用工具） */
const CS_AGENT_MAX_TOOL_LOOPS = 8;
/** read_code 返回内容的最大字符数（防止超出模型上下文） */
const CS_READ_MAX_CHARS = 16000;
/** 上下文压缩阈值：历史 + 工具结果超过该字符数时自动压缩 */
const CS_CONTEXT_MAX_CHARS = 24000;
/** 自动压缩历史时保留的最近轮数 */
const CS_HISTORY_KEEP_ROUNDS = 6;
/** 工具结果压缩时保留的最近条数 */
const CS_TOOL_KEEP_RESULTS = 4;
/** 单条工具结果允许进入上下文的最大字符数 */
const CS_TOOL_RESULT_MAX_CHARS = 6000;
/** 早前上下文摘要的最大字符数 */
const CS_HISTORY_DIGEST_MAX_CHARS = 2400;

/** 已完成的对话历史：数组元素为 { user, assistant }（各为纯文本） */
let csAgentHistory = [];
/** 被压缩历史生成的摘要（作为 system 消息注入） */
let csHistoryDigest = '';
/** 主动遗忘工具请求：>=0 时在工具返回后压缩工具结果，保留指定条数 */
let csForgetToolResultsKeep = -1;
/** 主动压缩/遗忘历史后，是否需要在当前工具循环里重建历史消息 */
let csRebuildHistoryRequested = false;

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
            return (agent.multimodal_model && String(agent.multimodal_model)) || 'primary_reasoning';
        } catch (e) {
            return 'primary_reasoning';
        }
    })();
    return csModelConfigPromise;
}

// ===== 工具定义（OpenAI function calling 格式，供智能体 LLM 调用） =====

const csAgentTools = [
    {
        type: 'function',
        function: {
            name: 'write_code',
            description: '将完整文本内容写入文档库文件（支持 .js/.html/.md/.txt/.py/.go 等文本文件；按原文件名保存，自动登记资源句柄）并同步到页面编辑器。JS 必须是可独立运行的单文件脚本；HTML 必须是完整文档；Markdown/纯文本按原样写入。内容必须是完整文件，禁止占位符或省略。',
            parameters: {
                type: 'object',
                properties: {
                    filename: { type: 'string', description: '文件名或资源句柄（如 @D.006），支持 .js/.html/.md/.txt/.py/.go 等文本文件；无扩展名时按内容自动补' },
                    content: { type: 'string', description: '完整文件内容' }
                },
                required: ['filename', 'content']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'read_code',
            description: '读取文档库指定文本文件的当前内容（同时载入页面编辑器，用户可见）。修改已有文件前必须先调用。',
            parameters: {
                type: 'object',
                properties: {
                    filename: { type: 'string', description: '文件名或资源句柄（如 @D.006）' }
                },
                required: ['filename']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'read_lines',
            description: '按行读取文档片段并返回带行号的内容。行区间写法与资源句柄的 |L 锚点一致：L10-20 表示第 10 到 20 行（1 基、含端点），单行写 L10。修改代码前用它精确查看目标区域，避免整文件读入上下文。',
            parameters: {
                type: 'object',
                properties: {
                    filename: { type: 'string', description: '文件名或资源句柄（如 @D.006）' },
                    range: { type: 'string', description: '行区间，如 L10-20；单行写 L10' }
                },
                required: ['filename', 'range']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'write_lines',
            description: '按行原位写入：用 content 覆盖指定行区间，content 的行数必须与区间长度完全一致。单行区间（如 L10）则 content 只能有一行。content 是纯内容，不要带 read_lines 返回的行号前缀。若行数需要增减，请改用 replace_lines。',
            parameters: {
                type: 'object',
                properties: {
                    filename: { type: 'string', description: '文件名或资源句柄（如 @D.006）' },
                    range: { type: 'string', description: '目标行区间，如 L10-12 或 L10' },
                    content: { type: 'string', description: '新内容；行数必须与区间长度一致（末尾换行不计入额外行）' }
                },
                required: ['filename', 'range', 'content']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'replace_lines',
            description: '按行置换：用 content 替换指定行区间，content 行数可以任意增减。适合插入、删除或替换长度不同的代码块；行区间写法与 read_lines 相同。content 是纯内容，不要带行号前缀。',
            parameters: {
                type: 'object',
                properties: {
                    filename: { type: 'string', description: '文件名或资源句柄（如 @D.006）' },
                    range: { type: 'string', description: '目标行区间，如 L10-20 或 L10' },
                    content: { type: 'string', description: '替换后的完整内容；可为空（删除该区间）' }
                },
                required: ['filename', 'range', 'content']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'swap_paragraphs',
            description: '互换两个不重叠的行段落。两个 range 都以交换前的文件行号为准，例如 range_a="L1-10"、range_b="L30-40"；单行段落写 L5。',
            parameters: {
                type: 'object',
                properties: {
                    filename: { type: 'string', description: '文件名或资源句柄（如 @D.006）' },
                    range_a: { type: 'string', description: '第一个段落，如 L1-10 或 L5' },
                    range_b: { type: 'string', description: '第二个段落，如 L30-40 或 L42' }
                },
                required: ['filename', 'range_a', 'range_b']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'run_code',
            description: '运行代码并返回控制台输出。指定 filename 则运行文档库文件（同时载入编辑器），否则运行编辑器当前内容。JS 每次运行固定收集约 2 秒的控制台输出；错误（异常/Promise 拒绝）也会返回。',
            parameters: {
                type: 'object',
                properties: {
                    filename: { type: 'string', description: '可选，文件名或资源句柄（如 @D.006）；省略则运行编辑器当前内容' }
                },
                required: []
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'list_code',
            description: '列出文档库已有的可读文本文件（仅显示映射表中登记的文件）。',
            parameters: { type: 'object', properties: {}, required: [] }
        }
    },
    {
        type: 'function',
        function: {
            name: 'compress_context',
            description: '主动压缩较早的对话历史：把超出 keep_recent 的轮次折叠成简短摘要，保留最近若干轮原文。上下文较长但还需要延续任务时使用。',
            parameters: {
                type: 'object',
                properties: {
                    keep_recent: { type: 'number', description: '保留最近多少轮原文，默认 6' }
                },
                required: []
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'forget_context',
            description: '主动遗忘上下文。scope=history 清空/裁剪对话历史；scope=tool_results 压缩较早的工具结果；scope=all 同时处理两者。适合切换到全新任务、避免旧上下文干扰时使用。',
            parameters: {
                type: 'object',
                properties: {
                    scope: { type: 'string', enum: ['history', 'tool_results', 'all'], description: '遗忘范围，默认 all' },
                    keep_recent: { type: 'number', description: '保留最近多少轮历史 / 多少条工具结果，默认 4；传 0 表示全部清空' }
                },
                required: []
            }
        }
    }
];

// ===== 系统提示词（专用角色） =====

function buildCSSystemPrompt() {
    return [
        '你是星月智能「编辑运行」的专用智能体（AtoA 执行者），负责根据月华的自然语言指令编写、修改文档库中的文本文件与可运行代码。',
        '',
        '【能力范围】',
        '- 文档库文本文件：可读取、按行编辑、整文件重写 .js/.html/.md/.txt/.py/.go 等全部文本类文件。',
        '- 单文件 JavaScript：ES2023+ 标准，运行于浏览器（Chromium），可使用全部 WebAPI（DOM、Canvas、Fetch、localStorage、定时器、Audio/Video 等）；脚本必须能直接执行并通常通过 console.log 输出结果或操作页面 DOM。',
        '- 完整 HTML 文档：单文件，内联 CSS/JS，运行后会按需弹出悬浮预览。',
        '- Markdown / 纯文本：只做内容编辑，不需要也不应该调用 run_code。',
        '',
        '【行区间文法】',
        '- 与资源句柄的 |L 锚点一致：L10-20 表示第 10 到 20 行（1 基、含端点），单行写 L10。',
        '- 文件名参数既可以是普通文件名，也可以是资源句柄（如 @D.006）；工具会自动解析为文档库中的实际文件，修改后仍是同一个资源。',
        '- read_lines：读取指定区间，返回带行号内容。',
        '- write_lines：原位覆盖区间，content 行数必须与区间长度一致。',
        '- replace_lines：替换区间，content 行数可任意增减（空内容表示删除）。',
        '- swap_paragraphs：互换两个不重叠的行段落，行号以交换前为准。',
        '',
        '【执行规则】',
        '1. 修改已有代码前，必须先读取目标区域：小范围改动优先 read_lines，需要整体理解时才 read_code。',
        '2. 小改动优先用 write_lines / replace_lines，避免整文件重写；只有创建新文件或大范围重构才用 write_code。代码必须完整可运行，禁止占位符或省略。',
        '3. 写入后：JS/HTML 调用 run_code 运行并检查控制台输出；Markdown / 纯文本只编辑内容，不调用 run_code。',
        '4. 不确定文档库有哪些文件时先调用 list_code。',
        '5. 上下文管理：工具结果过长时系统会自动压缩较早内容；需要主动腾出空间时调用 compress_context；切换到全新任务时调用 forget_context。',
        '6. 全部完成后，用简短中文总结：创建/修改了什么、文件名与行区间、运行输出关键结果。不要在答复中粘贴完整代码。',
        '7. 人机协同：代码已同步显示在页面编辑器，用户可以随时手动修改并再次运行；如指令中包含用户修改意见，按意见落实。'
    ].join('\n');
}

// ===== 模型调用（OpenAI v1 协议，琉璃后端 /v1 代理解析模型配置） =====

/**
 * 调用 /v1/chat/completions（crystal_astral 将 /v1/ 代理到月华后端）
 * @param {Array<Object>} messages - 完整消息数组（含 system / user / assistant / tool）
 * @returns {Promise<Object>} OpenAI 响应中的 message 对象
 */
async function callCSModel(messages) {
    const model = await loadCSAgentModel();
    const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            model: model,
            messages: messages,
            tools: csAgentTools,
            stream: false
        })
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

// ===== 上下文压缩与主动遗忘 =====

/** 当前历史（含摘要）字符数 */
function historyChars() {
    const digest = String(csHistoryDigest || '').length;
    return csAgentHistory.reduce((total, round) => {
        return total + String(round.user || '').length + String(round.assistant || '').length;
    }, digest);
}

/**
 * 压缩较早的对话历史为摘要，只保留最近 keepRecent 轮原文
 * @param {number} keepRecent - 保留的最近轮数
 * @returns {number} 被压缩的轮数
 */
function compressCSHistory(keepRecent) {
    const keep = Math.max(0, Math.trunc(Number(keepRecent == null ? CS_HISTORY_KEEP_ROUNDS : keepRecent)));
    if (csAgentHistory.length <= keep) return 0;
    const older = csAgentHistory.slice(0, csAgentHistory.length - keep);
    const lines = older.map((round, index) => {
        const user = String(round.user || '').replace(/\s+/g, ' ').slice(0, 80);
        const answer = String(round.assistant || '').replace(/\s+/g, ' ').slice(0, 120);
        return '- 第' + (index + 1) + '轮 用户：' + user + '；答复：' + answer;
    });
    let digest = (csHistoryDigest ? csHistoryDigest + '\n' : '')
        + '【已压缩的早前上下文】\n' + lines.join('\n');
    if (digest.length > CS_HISTORY_DIGEST_MAX_CHARS) {
        digest = '【已压缩的早前上下文（截断）】\n' + digest.slice(-CS_HISTORY_DIGEST_MAX_CHARS);
    }
    csHistoryDigest = digest;
    csAgentHistory = keep > 0 ? csAgentHistory.slice(-keep) : [];
    return older.length;
}

/**
 * 主动遗忘对话历史（摘要一并清除）
 * @param {number} keepRecent - 保留的最近轮数，0 表示全部清空
 * @returns {number} 被清除的轮数
 */
function forgetCSHistory(keepRecent) {
    const keep = Math.max(0, Math.trunc(Number(keepRecent == null ? 4 : keepRecent)));
    const removed = csAgentHistory.length - Math.min(keep, csAgentHistory.length);
    csAgentHistory = keep > 0 ? csAgentHistory.slice(-keep) : [];
    csHistoryDigest = '';
    return removed;
}

/** 历史超过预算时自动压缩（保留最近若干轮） */
function autoCompressHistory() {
    if (csAgentHistory.length <= CS_HISTORY_KEEP_ROUNDS) return 0;
    if (historyChars() <= CS_CONTEXT_MAX_CHARS) return 0;
    return compressCSHistory(CS_HISTORY_KEEP_ROUNDS);
}

/** 消息数组字符数（用于工具结果压缩判据） */
function messagesChars(messages) {
    return messages.reduce((total, message) => total + String(message && message.content || '').length, 0);
}

/**
 * 压缩较早的工具结果，只保留最近 keepRecent 条原文
 * @param {Array<Object>} messages - 模型消息数组
 * @param {number} keepRecent - 保留的最近工具结果条数
 * @param {string} [reason] - 压缩说明
 * @returns {number} 被压缩的条数
 */
function compactCSToolMessages(messages, keepRecent, reason) {
    const keep = Math.max(0, Math.trunc(Number(keepRecent == null ? CS_TOOL_KEEP_RESULTS : keepRecent)));
    const indexes = [];
    messages.forEach((message, index) => {
        if (message && message.role === 'tool') indexes.push(index);
    });
    if (indexes.length <= keep) return 0;
    const compactCount = indexes.length - keep;
    for (let i = 0; i < compactCount; i++) {
        const index = indexes[i];
        const content = String(messages[index].content || '');
        if (content.length <= 160) continue;
        messages[index] = {
            role: 'tool',
            tool_call_id: messages[index].tool_call_id,
            content: JSON.stringify({
                success: true,
                text: (reason || '较早的工具结果已压缩') + '（原长 ' + content.length + ' 字符）',
                compressed: true
            })
        };
    }
    return compactCount;
}

/** 应用 forget_context 对工具结果产生的待处理请求 */
function applyPendingForget(messages) {
    if (csForgetToolResultsKeep < 0) return 0;
    const keep = csForgetToolResultsKeep;
    csForgetToolResultsKeep = -1;
    return compactCSToolMessages(messages, keep, '已主动遗忘的工具结果');
}

/** 单条工具结果超长时截断，避免一次工具调用撑爆上下文 */
function limitToolResult(result) {
    const text = String(result && result.text || '');
    if (text.length <= CS_TOOL_RESULT_MAX_CHARS) return result;
    return {
        ...result,
        text: text.slice(0, CS_TOOL_RESULT_MAX_CHARS)
            + '\n...（工具结果过长已截断，原长 ' + text.length + ' 字符）'
    };
}

// ===== 工具执行器（复用 script.js 的共享操作函数，保证与人手动操作行为一致） =====

/** 写入代码 */
async function execCSWrite(args) {
    const filename = String(args.filename || '').trim();
    const content = String(args.content || '');
    if (!filename) return { success: false, error: '请提供文件名' };
    if (!content.trim()) return { success: false, error: '代码内容为空' };
    const saved = await window.writeCodeFile(filename, content);
    const handleText = saved.handle ? '，句柄 ' + saved.handle : '（句柄登记失败）';
    return {
        success: true,
        text: '已写入文件 ' + saved.name + '（' + content.length + ' 字符' + handleText + '）并同步到编辑器',
        data: { filename: saved.name, handle: saved.handle }
    };
}

/** 读取代码 */
async function execCSRead(args) {
    const filename = String(args.filename || '').trim();
    if (!filename) return { success: false, error: '请提供文件名' };
    const result = await window.readCodeFile(filename);
    let content = result.content;
    let note = '';
    if (content.length > CS_READ_MAX_CHARS) {
        content = content.slice(0, CS_READ_MAX_CHARS);
        note = '\n...（内容过长已截断，共 ' + result.content.length + ' 字符）';
    }
    return { success: true, text: '文件 ' + result.name + ' 当前内容：\n' + content + note, data: { filename: result.name } };
}

/** 按行读取 */
async function execCSReadLines(args) {
    const filename = String(args.filename || '').trim();
    const range = String(args.range || '').trim();
    if (!filename) return { success: false, error: '请提供文件名' };
    if (!range) return { success: false, error: '请提供行区间，如 L10-20 或 L10' };
    const text = await window.readCodeLines(filename, range);
    return { success: true, text: text, data: { filename: filename, range: range } };
}

/** 按行原位写入（content 行数必须与区间一致） */
async function execCSWriteLines(args) {
    const filename = String(args.filename || '').trim();
    const range = String(args.range || '').trim();
    const content = String(args.content == null ? '' : args.content);
    if (!filename) return { success: false, error: '请提供文件名' };
    if (!range) return { success: false, error: '请提供行区间' };
    const result = await window.writeCodeLines(filename, range, content);
    return {
        success: true,
        text: '已按行写入 ' + result.name + ' ' + result.range + '，文件现有 ' + result.lines + ' 行'
            + (result.handle ? '，句柄 ' + result.handle : ''),
        data: result
    };
}

/** 按行置换（content 行数可任意增减） */
async function execCSReplaceLines(args) {
    const filename = String(args.filename || '').trim();
    const range = String(args.range || '').trim();
    const content = String(args.content == null ? '' : args.content);
    if (!filename) return { success: false, error: '请提供文件名' };
    if (!range) return { success: false, error: '请提供行区间' };
    const result = await window.replaceCodeLines(filename, range, content);
    return {
        success: true,
        text: '已置换 ' + result.name + ' ' + result.range + '，文件现有 ' + result.lines + ' 行'
            + (result.handle ? '，句柄 ' + result.handle : ''),
        data: result
    };
}

/** 互换两个行段落 */
async function execCSSwapParagraphs(args) {
    const filename = String(args.filename || '').trim();
    const rangeA = String(args.range_a || '').trim();
    const rangeB = String(args.range_b || '').trim();
    if (!filename) return { success: false, error: '请提供文件名' };
    if (!rangeA || !rangeB) return { success: false, error: '请提供两个段落区间' };
    const result = await window.swapCodeParagraphs(filename, rangeA, rangeB);
    return {
        success: true,
        text: '已互换 ' + result.name + ' 的 ' + result.first + ' 与 ' + result.second
            + '，文件现有 ' + result.lines + ' 行' + (result.handle ? '，句柄 ' + result.handle : ''),
        data: result
    };
}

/** 运行代码 */
async function execCSRun(args) {
    const filename = String(args.filename || '').trim();
    const result = await window.runWorkspaceFile(filename || undefined);
    const errCount = result.lines.filter(l => l.startsWith('error:')).length;
    const head = '已运行 ' + (filename || '编辑器当前内容') + '，控制台输出 ' + result.lines.length + ' 行' + (errCount ? '（含 ' + errCount + ' 个错误）' : '') + '：\n' + result.summary;
    return { success: true, text: head, data: { outputLines: result.lines.length, hasError: errCount > 0 } };
}

/** 列出文件 */
async function execCSList() {
    // 直接列出文档库中的可读文件
    const list = await window.listDocumentFiles();
    return { success: true, text: list.length ? '文档库文件：' + list.join('、') : '文档库暂无文件', data: { files: list } };
}

/** 主动压缩较早的历史 */
function execCSCompressContext(args) {
    const rawKeep = Number(args.keep_recent);
    const keep = Number.isFinite(rawKeep) ? Math.max(0, Math.trunc(rawKeep)) : CS_HISTORY_KEEP_ROUNDS;
    const compressed = compressCSHistory(keep);
    if (compressed > 0) csRebuildHistoryRequested = true;
    return {
        success: true,
        text: compressed
            ? '已压缩 ' + compressed + ' 轮较早历史，保留最近 ' + keep + ' 轮原文'
            : '当前历史无需压缩（保留轮数 ' + keep + '）',
        data: { compressed: compressed, keep_recent: keep }
    };
}

/** 主动遗忘历史与/或工具结果 */
function execCSForgetContext(args) {
    const scope = String(args.scope || 'all').toLowerCase();
    const rawKeep = Number(args.keep_recent);
    const keep = Number.isFinite(rawKeep) ? Math.max(0, Math.trunc(rawKeep)) : 4;
    let removed = 0;
    let toolNote = '';
    if (scope === 'history' || scope === 'all') {
        removed = forgetCSHistory(keep);
        csRebuildHistoryRequested = true;
    }
    if (scope === 'tool_results' || scope === 'all') {
        csForgetToolResultsKeep = keep;
        toolNote = '；工具结果保留最近 ' + keep + ' 条（其余在回填后遗忘）';
    }
    return {
        success: true,
        text: '已主动遗忘：历史清理 ' + removed + ' 轮' + toolNote,
        data: { scope: scope, removed: removed, keep_recent: keep }
    };
}

/** 工具名 → 执行器映射 */
const csToolExecutors = {
    write_code: execCSWrite,
    read_code: execCSRead,
    read_lines: execCSReadLines,
    write_lines: execCSWriteLines,
    replace_lines: execCSReplaceLines,
    swap_paragraphs: execCSSwapParagraphs,
    run_code: execCSRun,
    list_code: execCSList,
    compress_context: execCSCompressContext,
    forget_context: execCSForgetContext
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
 * 运行「编辑运行」的 LLM 智能体，处理一条月华指令
 * @param {string} instruction - 月华发来的自然语言指令
 * @param {Function} onStep - 步骤回调 (step) => void，step 为 { kind, text }
 * @returns {Promise<Object>} { success, text, error }
 */
async function runCodeAgent(instruction, onStep) {
    const text = String(instruction || '').trim();
    if (!text) throw new Error('空指令');

    if (onStep) onStep({ kind: 'info', text: '收到月华指令：' + text });

    // 历史超预算时先自动压缩，再组装消息
    const autoCompressed = autoCompressHistory();
    if (autoCompressed && onStep) {
        onStep({ kind: 'info', text: '上下文自动压缩：折叠了 ' + autoCompressed + ' 轮较早历史' });
    }

    // 消息骨架：系统提示 + 早前摘要 + 独立上下文历史 + 本轮指令
    const messages = [{ role: 'system', content: buildCSSystemPrompt() }];
    if (csHistoryDigest) messages.push({ role: 'system', content: csHistoryDigest });
    for (const round of csAgentHistory) {
        messages.push({ role: 'user', content: round.user });
        messages.push({ role: 'assistant', content: round.assistant });
    }
    const currentUserMessage = { role: 'user', content: '【月华指令】' + text };
    messages.push(currentUserMessage);

    /** 主动压缩/遗忘历史后，立即重建当前请求里的历史部分（保留本轮已发生的工具对话） */
    function rebuildHistoryMessages() {
        const currentIndex = messages.indexOf(currentUserMessage);
        if (currentIndex < 0) return;
        const tail = messages.slice(currentIndex);
        const rebuilt = [{ role: 'system', content: buildCSSystemPrompt() }];
        if (csHistoryDigest) rebuilt.push({ role: 'system', content: csHistoryDigest });
        for (const round of csAgentHistory) {
            rebuilt.push({ role: 'user', content: round.user });
            rebuilt.push({ role: 'assistant', content: round.assistant });
        }
        rebuilt.push(...tail);
        messages.splice(0, messages.length, ...rebuilt);
    }

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
            // 带正文的工具参数过长，步骤展示只显示文件名与内容长度
            const hasContent = tc.function.name === 'write_code'
                || tc.function.name === 'write_lines'
                || tc.function.name === 'replace_lines';
            const stepArgs = hasContent
                ? { filename: args.filename, range: args.range, contentLength: String(args.content || '').length }
                : args;
            if (onStep) onStep({ kind: 'tool', text: '调用工具 ' + tc.function.name + '(' + JSON.stringify(stepArgs) + ')' });
            const result = limitToolResult(await executeCSTool(tc.function.name, args));
            if (onStep) {
                const preview = String(result.text || result.error || '').slice(0, 160);
                onStep({ kind: result.success ? 'tool-result' : 'error', text: (result.success ? '工具结果：' : '工具失败：') + preview });
            }
            messages.push({ role: 'tool', tool_call_id: tc.id, content: JSON.stringify(result) });
            // forget_context 生效：压缩较早的工具结果
            const forgotten = applyPendingForget(messages);
            if (forgotten && onStep) {
                onStep({ kind: 'info', text: '已按遗忘请求压缩 ' + forgotten + ' 条较早工具结果' });
            }
            // compress_context / forget_context(history) 生效：立即重建历史部分
            if (csRebuildHistoryRequested) {
                csRebuildHistoryRequested = false;
                rebuildHistoryMessages();
                if (onStep) onStep({ kind: 'info', text: '已按请求重建历史上下文' });
            }
        }

        // 工具结果总量超预算时自动压缩较早部分
        if (messagesChars(messages) > CS_CONTEXT_MAX_CHARS) {
            const compacted = compactCSToolMessages(messages, CS_TOOL_KEEP_RESULTS);
            if (compacted && onStep) {
                onStep({ kind: 'info', text: '工具结果超出上下文预算，已压缩 ' + compacted + ' 条较早结果' });
            }
        }
    }

    // 工具循环耗尽仍未给出最终答复（模型持续调用工具）
    if (!lastReply) lastReply = '已完成相关代码操作';

    // 记录本轮对话并裁剪历史（保留最近 40 轮，超出丢弃最早的）
    csAgentHistory.push({ user: text, assistant: lastReply });
    if (csAgentHistory.length > CS_AGENT_MAX_ROUNDS) {
        csAgentHistory.splice(0, csAgentHistory.length - CS_AGENT_MAX_ROUNDS);
    }
    autoCompressHistory();

    return { success: true, text: lastReply, error: '' };
}
