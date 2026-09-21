/**
 * LTPX AtoA 专用智能体模块（数学工坊）
 * 「Agent to Agent」：月华将自然语言指令交给本模块的专用智能体，
 * 由大语言模型（LLM）作为最终执行者，将自然语言转换为合适的数学表达式并调用工具执行。
 *
 * 工具能力（来源）：
 * - 表达式求值：math.js（四则运算、函数求值、单位换算、复杂数学表达式）
 * - 方程求解：Nerdamer Solve 模块（多元多次方程与方程组符号求解）
 * - 函数绘图：本地 ECharts 图表渲染
 *
 * 特性：
 * - 独立上下文历史：保留最近 40 轮对话，超出丢弃最早的轮次
 * - 多轮工具调用循环：LLM 通过 OpenAI function calling 逐次调用工具，
 *   观察工具结果并持续决策，直到给出最终答复；上限 8 次防死循环
 * - 步骤回调：每个关键步骤通过 onStep 通知前端，实现「月华操作步骤」的同步展示
 * - 模型调用走琉璃后端 /v1 代理（OpenAI v1 协议），模型名从 lunar_config.json 读取，不硬编码
 */

// ===== 智能体常量与状态 =====

/** 独立上下文历史：最多保留的对话轮数（1 轮 = 用户指令 + 智能体答复） */
const MS_AGENT_MAX_ROUNDS = 40;
/** 单条指令允许的最大工具调用循环次数（防模型无限调用工具） */
const MS_AGENT_MAX_TOOL_LOOPS = 8;

/** 已完成的对话历史：数组元素为 { user, assistant }（各为纯文本） */
let msAgentHistory = [];

/** 模型配置加载 Promise（从 lunar_config.json 的 agent 字段读取，不硬编码） */
let msModelConfigPromise = null;

/**
 * 读取 lunar_config.json 的 agent.multimodal_model 作为模型名
 * （通过 /file/read/ 文件接口；读取失败回退默认占位值，仍走同源 /v1 代理解析）
 * @returns {Promise<string>}
 */
async function loadMSAgentModel() {
    if (msModelConfigPromise) return msModelConfigPromise;
    msModelConfigPromise = (async () => {
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
    return msModelConfigPromise;
}

// ===== 工具定义（OpenAI function calling 格式，供智能体 LLM 调用） =====

const msAgentTools = [
    {
        type: 'function',
        function: {
            name: 'evaluate_expression',
            description: '求解数学表达式的值。支持四则运算、幂运算、对数指数、三角函数、常数（pi/e）、单位换算等，如 (1+2)*3、sin(pi/4)、5 km + 2 km。不适用于含等号的方程。',
            parameters: {
                type: 'object',
                properties: {
                    expression: { type: 'string', description: '数学表达式，使用 math.js 语法，示例：sqrt(2)^2、log(100, 10)、2 inch to cm' }
                },
                required: ['expression']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'solve_equation',
            description: '符号求解方程或方程组，支持多元多次（如 x^2+2*x=3、x^3-2*x+1=0、方程组 x+y=5 与 x-y=1）。注意：必须包含等号。',
            parameters: {
                type: 'object',
                properties: {
                    equations: {
                        type: 'array',
                        items: { type: 'string' },
                        description: '方程列表，每条形如 "x^2+2*x=3"；方程组传多条，如 ["x+y=5", "x-y=1"]'
                    },
                    variable: { type: 'string', description: '可选，求解变量名（单变量方程可省略）' }
                },
                required: ['equations']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'plot_function',
            description: '在页面绘制函数图表（y = f(x)），支持同时绘制多条曲线，用户可直接观察与交互（缩放/平移）。涉及「画图」「图像」「曲线」「可视化」等诉求时使用。',
            parameters: {
                type: 'object',
                properties: {
                    functions: {
                        type: 'array',
                        items: { type: 'string' },
                        description: '函数表达式列表（以 x 为自变量），如 ["sin(x)"] 或 ["sin(x)", "cos(x)"]'
                    },
                    x_min: { type: 'number', description: '可选，绘图范围下界，默认 -10' },
                    x_max: { type: 'number', description: '可选，绘图范围上界，默认 10' }
                },
                required: ['functions']
            }
        }
    }
];

// ===== 系统提示词（专用角色） =====

function buildMSSystemPrompt() {
    return [
        '你是星月智能「数学工坊」的专用智能体（AtoA 执行者），负责将月华发来的自然语言计算需求',
        '转换为准确的数学表达式，并调用工具完成计算、求解与绘图。',
        '',
        '【能力范围】',
        '四则运算与复杂数学表达式求值（math.js 语法，支持三角函数、对数指数、常数 pi/e、单位换算）、',
        '多元多次方程与方程组的符号求解（Nerdamer）、函数图表绘制（y = f(x)，以 x 为自变量）。',
        '',
        '【执行规则】',
        '1. 先把自然语言转换为准确的数学表达式：注意运算优先级、括号补全、函数名规范化（如「根号」→ sqrt，「log以10为底」→ log(x, 10)）。',
        '2. 需求含等号或为「解方程/求根/求方程组的解」时，调用 solve_equation，equations 传方程数组。',
        '3. 需求含「画图/图像/曲线/可视化/变化趋势」时，调用 plot_function；根据需求选择合理的绘图范围（默认 [-10, 10]）。',
        '4. 其余数值计算调用 evaluate_expression。',
        '5. 一条指令包含多个计算需求时，可依次调用多个工具分别完成。',
        '6. 工具失败时先检查表达式是否规范并修正重试一次；仍失败则如实向月华汇报失败原因，不要编造结果。',
        '7. 全部完成后，用简短中文总结计算结果（数值给出精确或近似值，方程给出全部解，绘图说明所绘函数与范围），不要输出额外内容或代码块。'
    ].join('\n');
}

// ===== 模型调用（OpenAI v1 协议，琉璃后端 /v1 代理解析模型配置） =====

/**
 * 调用 /v1/chat/completions（crystal_astral 将 /v1/ 代理到月华后端）
 * @param {Array<Object>} messages - 完整消息数组（含 system / user / assistant / tool）
 * @returns {Promise<Object>} OpenAI 响应中的 message 对象
 */
async function callMSModel(messages) {
    const model = await loadMSAgentModel();
    const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            model: model,
            messages: messages,
            tools: msAgentTools,
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
function safeParseMSArgs(jsonStr) {
    if (jsonStr && typeof jsonStr === 'object') return jsonStr;
    if (!jsonStr) return {};
    try {
        const parsed = JSON.parse(jsonStr);
        return parsed && typeof parsed === 'object' ? parsed : {};
    } catch (e) {
        return {};
    }
}

// ===== 工具执行器（复用 script.js 的共享执行函数，保证与直接输入行为一致） =====

/** 表达式求值 */
async function execMSEvaluate(args) {
    const expression = String(args.expression || '').trim();
    if (!expression) return { success: false, error: '请提供数学表达式' };
    const result = await window.runCalc(expression);
    return { success: true, text: '表达式 ' + result.expr + ' 的求值结果为 ' + result.value, data: result };
}

/** 方程求解 */
async function execMSSolve(args) {
    let equations = args.equations;
    if (typeof equations === 'string') equations = [equations];
    if (!Array.isArray(equations) || equations.length === 0) {
        return { success: false, error: '请提供至少一条方程（必须包含等号）' };
    }
    const result = await window.runSolve(equations);
    return { success: true, text: '方程 ' + result.equations.join(' , ') + ' 的解：' + result.display, data: result };
}

/** 函数绘图 */
async function execMSPlot(args) {
    const functions = args.functions;
    if (!Array.isArray(functions) || functions.length === 0) {
        return { success: false, error: '请提供至少一个函数表达式（以 x 为自变量）' };
    }
    const summary = await window.runPlot(functions, args.x_min, args.x_max);
    return { success: true, text: summary, data: { type: 'plot', functions: functions, x_min: args.x_min, x_max: args.x_max } };
}

/** 工具名 → 执行器映射 */
const msToolExecutors = {
    evaluate_expression: execMSEvaluate,
    solve_equation: execMSSolve,
    plot_function: execMSPlot
};

/**
 * 执行单个工具并返回结构化结果（供回填给模型）
 * @param {string} name - 工具名
 * @param {Object} args - 结构化参数
 * @returns {Promise<Object>} { success, text?, error? }
 */
async function executeMSTool(name, args) {
    const executor = msToolExecutors[name];
    if (!executor) return { success: false, error: '未知工具: ' + name };
    try {
        return await executor(args || {});
    } catch (e) {
        return { success: false, error: (e && e.message) || String(e) };
    }
}

// ===== 主流程：多轮工具调用循环 =====

/**
 * 运行数学工坊 LLM 智能体，处理一条月华指令
 * @param {string} instruction - 月华发来的自然语言指令
 * @param {Function} onStep - 步骤回调 (step) => void，step 为 { kind, text }
 * @returns {Promise<Object>} { success, text, error }
 */
async function runMathAgent(instruction, onStep) {
    const text = String(instruction || '').trim();
    if (!text) throw new Error('空指令');

    if (onStep) onStep({ kind: 'info', text: '收到月华指令：' + text });

    // 消息骨架：系统提示 + 独立上下文历史（最近 40 轮）+ 本轮指令
    const messages = [{ role: 'system', content: buildMSSystemPrompt() }];
    for (const round of msAgentHistory) {
        messages.push({ role: 'user', content: round.user });
        messages.push({ role: 'assistant', content: round.assistant });
    }
    messages.push({ role: 'user', content: '【月华指令】' + text });

    let lastReply = '';
    for (let loop = 0; loop < MS_AGENT_MAX_TOOL_LOOPS; loop++) {
        if (onStep) onStep({ kind: 'thinking', text: '智能体正在分析指令并转换为数学表达式...' });
        const message = await callMSModel(messages);
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
            const args = safeParseMSArgs(tc.function.arguments);
            if (onStep) onStep({ kind: 'tool', text: '调用工具 ' + tc.function.name + '(' + JSON.stringify(args) + ')' });
            const result = await executeMSTool(tc.function.name, args);
            if (onStep) {
                const preview = String(result.text || result.error || '').slice(0, 120);
                onStep({ kind: result.success ? 'tool-result' : 'error', text: (result.success ? '工具结果：' : '工具失败：') + preview });
            }
            messages.push({ role: 'tool', tool_call_id: tc.id, content: JSON.stringify(result) });
        }
    }

    // 工具循环耗尽仍未给出最终答复（模型持续调用工具）
    if (!lastReply) lastReply = '已完成相关计算';

    // 记录本轮对话并裁剪历史（保留最近 40 轮，超出丢弃最早的）
    msAgentHistory.push({ user: text, assistant: lastReply });
    if (msAgentHistory.length > MS_AGENT_MAX_ROUNDS) {
        msAgentHistory.splice(0, msAgentHistory.length - MS_AGENT_MAX_ROUNDS);
    }

    return { success: true, text: lastReply, error: '' };
}
