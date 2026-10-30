/**
 * LTPX AtoA 专用智能体模块（实时天气与新闻）
 * 「Agent to Agent」：月华将自然语言指令交给本模块的专用智能体，
 * 由大语言模型（LLM）作为最终执行者完成意图识别与查询调度。
 *
 * 依赖：
 * - weather-providers.js：wttr.in / Open-Meteo / 和风天气 数据源
 * - weather-service.js：配置读取、auto 回退调度、新闻解析、文本总结
 *
 * 特性：
 * - 独立上下文历史：保留最近 40 轮对话，超出丢弃最早的轮次
 * - 多轮工具调用循环：LLM 通过 OpenAI function calling 调用 get_weather / get_news /
 *   get_weather_sources，观察工具结果并持续决策，直到给出最终答复
 * - 智能体可通过 get_weather 的 provider 参数选择和风 / Open-Meteo / wttr 策略
 * - 步骤回调：每个关键步骤通过 onStep 通知前端，实现「月华操作步骤与结构」的同步展示
 * - 模型调用走琉璃后端 /agent 端点（非流式），模型为后端固定值
 */

// ===== 智能体常量与状态 =====

/** 独立上下文历史：最多保留的对话轮数（1 轮 = 用户指令 + 智能体答复） */
const SW_AGENT_MAX_ROUNDS = 40;
/** 单条指令允许的最大工具调用循环次数（防模型无限调用工具） */
const SW_AGENT_MAX_TOOL_LOOPS = 8;

/** 已完成的对话历史：数组元素为 { user, assistant }（各为纯文本） */
let swAgentHistory = [];

// ===== 工具定义（OpenAI function calling 格式，供智能体 LLM 调用） =====

const swAgentTools = [
    {
        type: 'function',
        function: {
            name: 'get_weather',
            description: '查询指定城市的实时天气与未来2-3天预报。provider 为查询策略：auto=自动回退（默认）；qweather=和风天气，中文最准且支持空气质量；openmeteo=Open-Meteo，免费全球覆盖；wttr=wttr.in，免费轻量。用户关心空气质量/AQI 或需要更准确的中文天气时优先 qweather；普通查询用 auto。',
            parameters: {
                type: 'object',
                properties: {
                    city: {
                        type: 'string',
                        description: '城市名称，中文或英文均可，示例：北京、上海、shenzhen、Tokyo'
                    },
                    provider: {
                        type: 'string',
                        enum: ['auto', 'qweather', 'openmeteo', 'wttr'],
                        description: '天气查询策略，默认 auto'
                    }
                },
                required: ['city']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'get_news',
            description: '获取当日新闻要闻列表（60s.7se.cn 整理），无需参数。接口不可用时返回错误，不要编造新闻。',
            parameters: {
                type: 'object',
                properties: {},
                required: []
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'get_weather_sources',
            description: '查看当前可用的天气数据源、默认策略、和风天气是否配置、AQI 是否开启、默认城市与新闻可用性。用于在不确定该选哪个 provider 时先做能力确认。',
            parameters: {
                type: 'object',
                properties: {},
                required: []
            }
        }
    }
];

// ===== 系统提示词（专用角色，动态注入当前能力） =====

async function buildSWSystemPrompt() {
    const cfg = await loadSWConfig();
    const qweatherState = qweatherAvailable(cfg)
        ? '可用（' + cfg.qweatherHost + '）'
        : '不可用（' + qweatherUnavailableReason(cfg) + '）';
    return [
        '你是星月智能「实时天气与新闻」的专用智能体（AtoA 执行者），负责解析月华发来的自然语言指令，',
        '并通过工具完成天气与新闻查询。',
        '',
        '【能力范围】',
        '天气查询（当前天气、未来2-3天预报、空气质量）与当日新闻摘要。',
        '',
        '【天气查询策略】',
        '- auto：默认；和风（若可用）→ Open-Meteo → wttr.in 自动回退。',
        '- qweather：和风天气，中文城市/天气最准确，支持 AQI；当前：' + qweatherState + '。',
        '- openmeteo：Open-Meteo，免费、全球覆盖、3 天预报。',
        '- wttr：wttr.in，免费、轻量、2 天预报。',
        '默认策略：' + cfg.provider + '。',
        '选择规则：用户关心空气质量/AQI、或需要更准确的中文天气时优先 qweather；普通查询用 auto；',
        '用户明确指定数据源时按用户要求；某个源查不到城市时换另一个源或 auto 重试。',
        '',
        '【执行规则】',
        '1. 查天气时调用 get_weather，city 传城市名，provider 传你选择的策略。',
        '2. 不确定数据源是否可用时，可先调用 get_weather_sources。',
        '3. 用户问今日新闻/要闻/资讯时调用 get_news。',
        '4. 指令意图不明确时，用一句简洁中文向月华确认，不要臆测执行。',
        '5. 查询完成后用一两句简洁中文总结：天气含城市、气温、天气状况与数据源；新闻含条数与主题。',
        '6. 工具返回错误时如实说明，不要编造数据，也不要输出代码块。'
    ].join('\n');
}

// ===== 模型调用（/agent 协议，非流式；模型名由后端固定） =====

/**
 * 调用 /agent/chat/completions（后端固定模型，不接受模型名与流式）
 * @param {Array<Object>} messages - 完整消息数组（含 system / user / assistant / tool）
 * @returns {Promise<Object>} OpenAI 响应中的 message 对象
 */
async function callSWModel(messages) {
    const response = await fetch('/agent/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            messages: messages,
            tools: swAgentTools,
            stream: false
        })
    });
    if (!response.ok) throw new Error('AI 调用失败: HTTP ' + response.status);
    const data = await response.json();
    if (data.choices && data.choices[0] && data.choices[0].message) {
        return data.choices[0].message;
    }
    throw new Error('AI 响应格式异常');
}

/**
 * 安全解析工具参数（兼容 JSON 字符串与已解析对象，解析失败回退空对象）
 */
function safeParseSWArgs(jsonStr) {
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

/** 天气查询（智能体可指定 provider） */
async function execSWWeather(args) {
    const city = String(args.city || '').trim();
    if (!city) return { success: false, error: '请提供城市名称' };
    const provider = String(args.provider || '').trim().toLowerCase();
    const result = await queryWeather(city, provider || null);
    if (!result.ok) return { success: false, error: result.error };
    const data = result.data;
    if (typeof renderResult === 'function') renderResult({ type: 'weather', data: data });
    return { success: true, text: generateWeatherText(data), data: data };
}

/** 新闻摘要 */
async function execSWNews() {
    const data = await fetchNews();
    if (typeof renderResult === 'function') renderResult({ type: 'news', data: data });
    return { success: true, text: generateNewsText(data), data: data };
}

/** 数据源能力确认 */
async function execSWWeatherSources() {
    const cfg = await loadSWConfig();
    const lines = [
        '天气查询能力：',
        '- 默认策略：' + cfg.provider,
        '- 和风天气：' + (qweatherAvailable(cfg)
            ? '可用（' + cfg.qweatherHost + '）'
            : '不可用（' + qweatherUnavailableReason(cfg) + '）'),
        '- Open-Meteo：可用',
        '- wttr.in：可用',
        '- AQI：' + (cfg.showAQI ? '开启（仅和风天气）' : '关闭'),
        '- 默认城市：' + (cfg.defaultCity || '未设置'),
        '- 新闻查询：' + (cfg.newsEnabled ? '可用' : '关闭')
    ];
    return { success: true, text: lines.join('\n') };
}

/** 工具名 → 执行器映射 */
const swToolExecutors = {
    get_weather: execSWWeather,
    get_news: execSWNews,
    get_weather_sources: execSWWeatherSources
};

/**
 * 执行单个工具并返回结构化结果（供回填给模型）
 * @param {string} name - 工具名
 * @param {Object} args - 结构化参数
 * @returns {Promise<Object>} { success, text?, error? }
 */
async function executeSWTool(name, args) {
    const executor = swToolExecutors[name];
    if (!executor) return { success: false, error: '未知工具: ' + name };
    try {
        return await executor(args || {});
    } catch (e) {
        return { success: false, error: (e && e.message) || String(e) };
    }
}

// ===== 主流程：多轮工具调用循环 =====

/**
 * 运行实时天气与新闻 LLM 智能体，处理一条月华指令
 * @param {string} instruction - 月华发来的自然语言指令
 * @param {Function} onStep - 步骤回调 (step) => void，step 为 { kind, text }
 * @returns {Promise<Object>} { success, text, error }
 */
async function runSWAgent(instruction, onStep) {
    const text = String(instruction || '').trim();
    if (!text) throw new Error('空指令');

    if (onStep) onStep({ kind: 'info', text: '收到月华指令：' + text });

    // 消息骨架：系统提示 + 独立上下文历史（最近 40 轮）+ 本轮指令
    const messages = [{ role: 'system', content: await buildSWSystemPrompt() }];
    for (const round of swAgentHistory) {
        messages.push({ role: 'user', content: round.user });
        messages.push({ role: 'assistant', content: round.assistant });
    }
    messages.push({ role: 'user', content: '【月华指令】' + text });

    let lastReply = '';
    for (let loop = 0; loop < SW_AGENT_MAX_TOOL_LOOPS; loop++) {
        if (onStep) onStep({ kind: 'thinking', text: '智能体正在分析指令并决策...' });
        const message = await callSWModel(messages);
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
            const args = safeParseSWArgs(tc.function.arguments);
            if (onStep) onStep({ kind: 'tool', text: '调用工具 ' + tc.function.name + '(' + JSON.stringify(args) + ')' });
            const result = await executeSWTool(tc.function.name, args);
            if (onStep) {
                const preview = String(result.text || result.error || '').slice(0, 120);
                onStep({ kind: result.success ? 'tool-result' : 'error', text: (result.success ? '工具结果：' : '工具失败：') + preview });
            }
            messages.push({ role: 'tool', tool_call_id: tc.id, content: JSON.stringify(result) });
        }
    }

    // 工具循环耗尽仍未给出最终答复（模型持续调用工具）
    if (!lastReply) lastReply = '已完成相关查询';

    // 记录本轮对话并裁剪历史（保留最近 40 轮，超出丢弃最早的）
    swAgentHistory.push({ user: text, assistant: lastReply });
    if (swAgentHistory.length > SW_AGENT_MAX_ROUNDS) {
        swAgentHistory.splice(0, swAgentHistory.length - SW_AGENT_MAX_ROUNDS);
    }

    return { success: true, text: lastReply, error: '' };
}
