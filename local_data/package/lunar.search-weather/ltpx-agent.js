/**
 * LTPX AtoA 专用智能体模块（实时天气与新闻）
 * 「Agent to Agent」：月华将自然语言指令交给本模块的专用智能体，
 * 由大语言模型（LLM）作为最终执行者完成意图识别与查询调度。
 *
 * 整合能力（来源）：
 * - 天气查询：lunar.skill.weather 的零配置兜底源 wttr.in（浏览器可直接请求，免 Key）
 * - 新闻摘要：lunar.skill.search 的当日要闻源 60s.7se.cn
 *
 * 特性：
 * - 独立上下文历史：保留最近 40 轮对话，超出丢弃最早的轮次
 * - 多轮工具调用循环：LLM 通过 OpenAI function calling 逐次调用 get_weather / get_news，
 *   观察工具结果并持续决策，直到给出最终答复
 * - 步骤回调：每个关键步骤通过 onStep 通知前端，实现「月华操作步骤与结构」的同步展示
 * - 模型调用走琉璃后端 /v1 代理（OpenAI v1 协议），由后端按 lunar_config.json 解析模型配置，
 *   前端不接触模型配置
 */

// ===== 智能体常量与状态 =====

/** 独立上下文历史：最多保留的对话轮数（1 轮 = 用户指令 + 智能体答复） */
const SW_AGENT_MAX_ROUNDS = 40;
/** 单条指令允许的最大工具调用循环次数（防模型无限调用工具） */
const SW_AGENT_MAX_TOOL_LOOPS = 8;

/** 已完成的对话历史：数组元素为 { user, assistant }（各为纯文本） */
let swAgentHistory = [];

/** 模型配置加载 Promise（从 lunar_config.json 的 agent 字段读取，不硬编码） */
let swModelConfigPromise = null;

/**
 * 读取 lunar_config.json 的 agent.multimodal_model 作为模型名
 * （通过 /file/read/ 文件接口；读取失败回退默认占位值，仍走同源 /v1 代理解析）
 * @returns {Promise<string>}
 */
async function loadSWAgentModel() {
    if (swModelConfigPromise) return swModelConfigPromise;
    swModelConfigPromise = (async () => {
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
    return swModelConfigPromise;
}

// ===== 数据获取（智能体工具与手动操作共用） =====

/**
 * 查询指定城市天气（wttr.in，零配置免 Key，浏览器可直接请求）
 * @param {string} city - 城市名称（中文或英文）
 * @returns {Promise<Object>} 结构化天气数据（失败时抛出异常）
 */
async function fetchWeather(city) {
    const resp = await fetch('https://wttr.in/' + encodeURIComponent(city) + '?format=j1');
    if (!resp.ok) throw new Error('天气服务返回状态 ' + resp.status);
    const data = await resp.json();
    const current = data.current_condition && data.current_condition[0];
    if (!current) throw new Error('天气数据解析失败');

    const area = data.nearest_area && data.nearest_area[0];
    const result = {
        city: (area && area.areaName && area.areaName[0] && area.areaName[0].value) || city,
        current: {
            temperature: parseFloat(current.temp_C) || 0,
            feels_like: parseFloat(current.FeelsLikeC) || parseFloat(current.temp_C) || 0,
            condition: (current.weatherDesc && current.weatherDesc[0] && current.weatherDesc[0].value) || '未知',
            humidity: parseInt(current.humidity, 10) || 0,
            wind: (current.winddir16Point || '') + ' ' + (current.windspeedKmph || '') + 'km/h'
        },
        forecast: [],
        source: 'wttr.in'
    };

    const weather = data.weather || [];
    for (let i = 0; i < Math.min(weather.length, 2); i++) {
        const day = weather[i];
        const hourly = day.hourly || [];
        result.forecast.push({
            date: day.date || '',
            high: parseFloat(day.maxtempC) || 0,
            low: parseFloat(day.mintempC) || 0,
            condition: (hourly[4] && hourly[4].weatherDesc && hourly[4].weatherDesc[0])
                ? hourly[4].weatherDesc[0].value : '未知'
        });
    }
    return result;
}

/**
 * 获取当日新闻摘要（60s.7se.cn）
 * @returns {Promise<Object>} { date, news: string[] }（失败时抛出异常）
 */
async function fetchNews() {
    const resp = await fetch('https://60s.7se.cn/v2/60s');
    if (!resp.ok) throw new Error('新闻服务返回状态 ' + resp.status);
    const data = await resp.json();
    if (!data || !data.data || !Array.isArray(data.data.news)) throw new Error('新闻数据解析失败');
    return { date: data.data.date || '', news: data.data.news };
}

// ===== 工具定义（OpenAI function calling 格式，供智能体 LLM 调用） =====

const swAgentTools = [
    {
        type: 'function',
        function: {
            name: 'get_weather',
            description: '查询指定城市的实时天气与未来2天预报，返回当前气温、体感温度、天气状况、湿度、风力等信息。',
            parameters: {
                type: 'object',
                properties: {
                    city: { type: 'string', description: '城市名称，中文或英文均可，示例：北京、上海、shenzhen、Tokyo' }
                },
                required: ['city']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'get_news',
            description: '获取当日新闻图文摘要（由权威信源整理的当日要闻列表），无需参数。',
            parameters: {
                type: 'object',
                properties: {},
                required: []
            }
        }
    }
];

// ===== 系统提示词（专用角色） =====

function buildSWSystemPrompt() {
    return [
        '你是星月智能「实时天气与新闻」的专用智能体（AtoA 执行者），负责解析月华发来的自然语言指令，',
        '并作为实时天气与新闻模块的最终执行者，通过工具完成新闻与天气查询。',
        '',
        '【能力范围】',
        '天气查询（指定城市当前天气与未来2天预报）、当日新闻摘要获取。',
        '',
        '【执行规则】',
        '1. 用户需要查询某地天气时，调用 get_weather 并传入城市名（中文或英文）。',
        '2. 用户需要了解今日新闻/要闻/资讯时，调用 get_news。',
        '3. 指令意图不明确时，用一句简洁中文向月华确认，不要臆测执行。',
        '4. 查询完成后，用一两句简洁中文总结结果（天气含城市与气温状况，新闻含条数与主题），不要输出额外内容或代码块。'
    ].join('\n');
}

// ===== 模型调用（OpenAI v1 协议，琉璃后端 /v1 代理解析模型配置） =====

/**
 * 调用 /v1/chat/completions（crystal_astral 将 /v1/ 代理到月华后端）
 * @param {Array<Object>} messages - 完整消息数组（含 system / user / assistant / tool）
 * @returns {Promise<Object>} OpenAI 响应中的 message 对象
 */
async function callSWModel(messages) {
    const model = await loadSWAgentModel();
    const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            model: model,
            messages: messages,
            tools: swAgentTools,
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

/** 天气查询 */
async function execSWWeather(args) {
    const city = String(args.city || '').trim();
    if (!city) return { success: false, error: '请提供城市名称' };
    const data = await fetchWeather(city);
    const lines = [`【${data.city}】当前天气（${data.source}）：`];
    lines.push(`- 气温：${data.current.temperature}°C（体感 ${data.current.feels_like}°C）`);
    lines.push(`- 天气状况：${data.current.condition}`);
    lines.push(`- 湿度：${data.current.humidity}%`);
    lines.push(`- 风力：${data.current.wind}`);
    data.forecast.forEach(f => {
        lines.push(`- 预报 ${f.date}：${f.condition}，${f.low}°C ~ ${f.high}°C`);
    });
    if (typeof renderResult === 'function') renderResult({ type: 'weather', data: data });
    return { success: true, text: lines.join('\n'), data: data };
}

/** 新闻摘要 */
async function execSWNews(args) {
    const data = await fetchNews();
    const lines = [`【${data.date || '今日'}】当日要闻（共 ${data.news.length} 条）：`];
    data.news.forEach((n, i) => lines.push(`${i + 1}. ${n}`));
    if (typeof renderResult === 'function') renderResult({ type: 'news', data: data });
    return { success: true, text: lines.join('\n'), data: data };
}

/** 工具名 → 执行器映射 */
const swToolExecutors = {
    get_weather: execSWWeather,
    get_news: execSWNews
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
    const messages = [{ role: 'system', content: buildSWSystemPrompt() }];
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
