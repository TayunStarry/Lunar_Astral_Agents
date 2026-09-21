/**
 * LTPX AtoA 专用智能体模块（代码工坊）
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
            name: 'write_code',
            description: '将完整代码写入工作区文件并同步到页面编辑器。JS 必须是可独立运行的单文件脚本（ES2023+，浏览器环境，可使用 DOM/Canvas/Fetch 等 WebAPI）；HTML 必须是完整文档。内容必须是完整文件，禁止占位符或省略。',
            parameters: {
                type: 'object',
                properties: {
                    filename: { type: 'string', description: '文件名，以 .js 或 .html 结尾，如 hello.js、index.html；无扩展名时按内容自动补' },
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
            description: '读取工作区指定文件的当前内容（同时载入页面编辑器，用户可见）。修改已有代码前必须先调用。',
            parameters: {
                type: 'object',
                properties: {
                    filename: { type: 'string', description: '文件名，如 hello.js' }
                },
                required: ['filename']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'run_code',
            description: '运行代码并返回控制台输出。指定 filename 则运行工作区文件（同时载入编辑器），否则运行编辑器当前内容。JS 每次运行固定收集约 2 秒的控制台输出；错误（异常/Promise 拒绝）也会返回。',
            parameters: {
                type: 'object',
                properties: {
                    filename: { type: 'string', description: '可选，要运行的工作区文件名；省略则运行编辑器当前内容' }
                },
                required: []
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'list_code',
            description: '列出工作区已有的代码文件。',
            parameters: { type: 'object', properties: {}, required: [] }
        }
    }
];

// ===== 系统提示词（专用角色） =====

function buildCSSystemPrompt() {
    return [
        '你是星月智能「代码工坊」的专用智能体（AtoA 执行者），负责根据月华的自然语言指令编写、修改并运行代码。',
        '',
        '【能力范围】',
        '- 单文件 JavaScript：ES2023+ 标准，运行于浏览器（Chromium），可使用全部 WebAPI（DOM、Canvas、Fetch、localStorage、定时器、Audio/Video 等）；脚本必须能直接执行并通常通过 console.log 输出结果或操作页面 DOM。',
        '- 完整 HTML 文档：单文件，内联 CSS/JS，运行后直接渲染在预览区。',
        '',
        '【执行规则】',
        '1. 修改已有代码前，必须先调用 read_code 获取当前内容，在其基础上修改，不要凭空臆测已有代码。',
        '2. 用 write_code 写入完整文件内容：代码必须完整可运行，禁止占位符、注释省略（如 "... 其余同理"）。JS 文件如果需要展示结果，用 console.log 输出或操作 document.body。',
        '3. 写入后调用 run_code 运行，仔细阅读返回的控制台输出：有错误时修正代码并重新 write_code + run_code，直到运行正常（注意工具调用轮次有限，尽量一次写对）。',
        '4. 不确定工作区有哪些文件时先调用 list_code。',
        '5. 全部完成后，用简短中文总结：创建了/修改了什么、文件名、运行输出关键结果或效果说明。不要在答复中粘贴完整代码。',
        '6. 人机协同：代码已同步显示在页面编辑器，用户可以随时手动修改并再次运行；如指令中包含用户对代码的修改意见，按意见落实。'
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

// ===== 工具执行器（复用 script.js 的共享操作函数，保证与人手动操作行为一致） =====

/** 写入代码 */
async function execCSWrite(args) {
    const filename = String(args.filename || '').trim();
    const content = String(args.content || '');
    if (!filename) return { success: false, error: '请提供文件名' };
    if (!content.trim()) return { success: false, error: '代码内容为空' };
    const saved = await window.writeCodeFile(filename, content);
    return { success: true, text: '已写入文件 ' + saved + '（' + content.length + ' 字符）并同步到编辑器', data: { filename: saved } };
}

/** 读取代码 */
async function execCSRead(args) {
    const filename = String(args.filename || '').trim();
    if (!filename) return { success: false, error: '请提供文件名' };
    const result = await window.readCodeFile(filename);
    let content = result.content;
    let note = '';
    if (content.length > 16000) {
        content = content.slice(0, 16000);
        note = '\n...（内容过长已截断，共 ' + result.content.length + ' 字符）';
    }
    return { success: true, text: '文件 ' + result.name + ' 当前内容：\n' + content + note, data: { filename: result.name } };
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
    // 借用文件列表接口（与页面刷新一致）
    const resp = await fetch('/file/list/package/lunar.code-studio/workspace', { cache: 'no-store' });
    const files = resp.ok ? await resp.json() : [];
    const list = (Array.isArray(files) ? files : []).filter(f => !f.isDir).map(f => String(f.name));
    return { success: true, text: list.length ? '工作区文件：' + list.join('、') : '工作区暂无文件', data: { files: list } };
}

/** 工具名 → 执行器映射 */
const csToolExecutors = {
    write_code: execCSWrite,
    read_code: execCSRead,
    run_code: execCSRun,
    list_code: execCSList
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
 * 运行代码工坊 LLM 智能体，处理一条月华指令
 * @param {string} instruction - 月华发来的自然语言指令
 * @param {Function} onStep - 步骤回调 (step) => void，step 为 { kind, text }
 * @returns {Promise<Object>} { success, text, error }
 */
async function runCodeAgent(instruction, onStep) {
    const text = String(instruction || '').trim();
    if (!text) throw new Error('空指令');

    if (onStep) onStep({ kind: 'info', text: '收到月华指令：' + text });

    // 消息骨架：系统提示 + 独立上下文历史（最近 40 轮）+ 本轮指令
    const messages = [{ role: 'system', content: buildCSSystemPrompt() }];
    for (const round of csAgentHistory) {
        messages.push({ role: 'user', content: round.user });
        messages.push({ role: 'assistant', content: round.assistant });
    }
    messages.push({ role: 'user', content: '【月华指令】' + text });

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
            // write_code 参数过长，步骤展示只显示文件名
            const stepArgs = tc.function.name === 'write_code'
                ? { filename: args.filename, contentLength: String(args.content || '').length }
                : args;
            if (onStep) onStep({ kind: 'tool', text: '调用工具 ' + tc.function.name + '(' + JSON.stringify(stepArgs) + ')' });
            const result = await executeCSTool(tc.function.name, args);
            if (onStep) {
                const preview = String(result.text || result.error || '').slice(0, 160);
                onStep({ kind: result.success ? 'tool-result' : 'error', text: (result.success ? '工具结果：' : '工具失败：') + preview });
            }
            messages.push({ role: 'tool', tool_call_id: tc.id, content: JSON.stringify(result) });
        }
    }

    // 工具循环耗尽仍未给出最终答复（模型持续调用工具）
    if (!lastReply) lastReply = '已完成相关代码操作';

    // 记录本轮对话并裁剪历史（保留最近 40 轮，超出丢弃最早的）
    csAgentHistory.push({ user: text, assistant: lastReply });
    if (csAgentHistory.length > CS_AGENT_MAX_ROUNDS) {
        csAgentHistory.splice(0, csAgentHistory.length - CS_AGENT_MAX_ROUNDS);
    }

    return { success: true, text: lastReply, error: '' };
}
