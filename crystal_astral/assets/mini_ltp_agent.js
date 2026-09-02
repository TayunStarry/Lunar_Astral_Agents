/* =====================================================================
 * MiniLTP Agent · 通用页面操作智能体（LLM 驱动，LTPX AtoA）
 * ---------------------------------------------------------------------
 * 遵循 LTPX AtoA 设计理念：月华将自然语言指令交给本智能体，
 * 由大语言模型（LLM）作为最终执行者完成意图识别与页面操作调度。
 * 与文件管理器 ltpx-agent.js 同构，仅能力域不同（通用页面操作，非业务定制）。
 *
 * - 独立上下文历史：保留最近 40 轮对话，超出丢弃最早的轮次
 * - 专用系统提示词：定义「通用页面操作执行者」角色与操作边界
 * - 多轮工具调用循环：LLM 通过 OpenAI function calling 逐次调用页面操作工具，
 *   观察工具结果并持续决策，直到给出最终答复
 * - 模型调用走琉璃后端 /v1 代理（OpenAI v1 协议，由后端按 lunar_config.json
 *   解析模型 name/key/url，前端不接触模型配置）
 * - 内嵌 AtoA 集成层：位于 iframe 中时自动监听 ltpx_run 并回执 ltpx_result
 * - LLM 不可用时降级到最小离线规则，保证基础可用（非主路径）
 * ===================================================================== */
(function (global, factory) {
    var api = factory();
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    } else if (typeof define === 'function' && define.amd) {
        define([], function () { return api; });
    } else {
        global.MiniLTPAgent = api;
    }
})(typeof window !== 'undefined' ? window : (typeof globalThis !== 'undefined' ? globalThis : this), function () {
    'use strict';

    var VERSION = '3.1.0';

    // ================= 智能体常量与状态 =================

    /** 独立上下文历史：最多保留的对话轮数（1 轮 = 用户指令 + 智能体答复） */
    var AGENT_MAX_ROUNDS = 40;
    /** 单条指令允许的最大工具调用循环次数（防模型无限调用工具） */
    var AGENT_MAX_TOOL_LOOPS = 20;
    /** 页面元素列表最多展示条目数（控制 token 消耗） */
    var AGENT_MAX_LIST_ITEMS = 60;

    /** 独立上下文历史：数组元素为 { user, assistant }（各为纯文本） */
    var agentHistory = [];

    // ================= 模型配置加载（从 lunar_config.json 的 agent 字段读取，不硬编码） =================

    /** 配置缓存与加载 Promise（一次加载，后续复用） */
    let modelConfigPromise = null;
    let modelConfig = null;

    /**
     * 读取 lunar_config.json 的 agent 配置（通过 /file/read/ 文件接口）。
     * 模型名取 agent.multimodal_model；base_url / api_key 一并返回供自定义代理使用。
     * @returns {Promise<{model:string, base_url:string, api_key:string|null}>}
     */
    async function loadModelConfig() {
        if (modelConfigPromise) return modelConfigPromise;
        modelConfigPromise = (async () => {
            try {
                const resp = await fetch('/file/read/lunar_config.json', { cache: 'no-store' });
                if (!resp.ok) throw new Error('读取配置失败 HTTP ' + resp.status);
                const cfg = await resp.json();
                const agent = (cfg && cfg.agent) || {};
                modelConfig = {
                    model: (agent.multimodal_model && String(agent.multimodal_model)) || 'system-multimodal',
                    base_url: (agent.multimodal_url && String(agent.multimodal_url)) || '/v1',
                    api_key: agent.multimodal_key || null
                };
                return modelConfig;
            } catch (e) {
                // 读取失败回退默认（与配置中的占位值一致，仍走同源 /v1 代理解析）
                modelConfig = { model: 'system-multimodal', base_url: '/v1', api_key: null };
                return modelConfig;
            }
        })();
        return modelConfigPromise;
    }

    /** 强制重新加载模型配置（配置变更后调用） */
    async function refreshModelConfig() {
        modelConfigPromise = null;
        modelConfig = null;
        return loadModelConfig();
    }

    /* ---------------- 共享键鼠操作模块（SharedInput） ----------------
       页面操作原语统一来自 window.SharedInput（见 shared_input.js）；
       宿主注入顺序：先注 /shared-input.js，再注本智能体脚本。 */

    var SharedInput = (typeof window !== 'undefined' && window.SharedInput) || null;
    if (!SharedInput) throw new Error('SharedInput 共享键鼠模块未加载（需先注入 /shared-input.js）');
    var isVisible = SharedInput.isVisible, escapeCss = SharedInput.escapeCss,
        query = SharedInput.query, queryAll = SharedInput.queryAll,
        textOf = SharedInput.textOf, describe = SharedInput.describe,
        findByText = SharedInput.findByText, resolveTarget = SharedInput.resolveTarget,
        uniqueSelector = SharedInput.uniqueSelector, captureElements = SharedInput.captureElements,
        buildCompactElements = SharedInput.buildCompactElements,
        buildPageInfoText = SharedInput.buildPageInfoText,
        dispatchAt = SharedInput.dispatchAt, resolvePoint = SharedInput.resolvePoint,
        fireMouse = SharedInput.fireMouse, focusEl = SharedInput.focusEl,
        clickEl = SharedInput.clickEl, hoverEl = SharedInput.hoverEl,
        setInputValue = SharedInput.setInputValue, typeEl = SharedInput.typeEl,
        keyToCode = SharedInput.keyToCode, normalizeKeyToken = SharedInput.normalizeKeyToken,
        pressKeyEl = SharedInput.pressKeyEl, parseHoldSpec = SharedInput.parseHoldSpec,
        mouseButtonEl = SharedInput.mouseButtonEl, wheelEl = SharedInput.wheelEl,
        sleep = SharedInput.sleep, scrollEl = SharedInput.scrollEl,
        selectEl = SharedInput.selectEl, getState = SharedInput.getState,
        tryDrawPagePixels = SharedInput.tryDrawPagePixels, drawGrid = SharedInput.drawGrid,
        drawElementBoxes = SharedInput.drawElementBoxes, captureScreenshot = SharedInput.captureScreenshot,
        SCREEN_GRID_STEP = SharedInput.SCREEN_GRID_STEP;


    /* ---------------- 工具定义（OpenAI function calling 格式） ---------------- */

    // 操作序列模型：智能体先做语义理解与元素分析（capture_page / get_state），
    // 再用**一条** execute_operations 提交完整操作队列，程序按 0.5s 间隔逐个执行。
    var agentTools = [
        {
            type: 'function',
            function: {
                name: 'capture_page',
                description: '捕获当前页面可见可交互的元素快照（含标签/文本/选择器/位置），用于分析定位可操作目标。执行 execute_operations 前若不确定目标元素，先调用本工具确认。',
                parameters: {
                    type: 'object',
                    properties: { max: { type: 'integer', description: '最多返回元素数，默认 60' } },
                    required: []
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'get_state',
                description: '查看当前页面状态：URL、标题、就绪状态、视口尺寸、滚动位置，用于确认页面加载完成。',
                parameters: { type: 'object', properties: {}, required: [] }
            }
        },
        {
            type: 'function',
            function: {
                name: 'capture_screenshot',
                description: '截取当前页面视口截图并叠加 50px 坐标网格与元素编号框（编号与【页面元素】列表一一对应），用于让视觉模型核对各元素在页面中的网格坐标与位置。调用后刷新为最新一张截图。',
                parameters: { type: 'object', properties: {}, required: [] }
            }
        },
        {
            type: 'function',
            function: {
                name: 'execute_operations',
                description: '一次性提交需按顺序执行的一连串页面操作队列。operations 为操作数组，程序会从前往后逐个执行，执行完上一个操作后等待 0.5 秒再执行下一个。鼠标点击优先用 click=左键单击（普通点击按钮/链接/画布目标点）；只有当需要指定其它按键（右键/中键）或按住时长时才用 mouse=鼠标按键；其余：type=向输入框输入文本、key=键盘按键、wheel=滚轮滚动、scroll=页面滚动、hover=悬停、select=下拉选择、wait=等待。分析完【月华指令】后，把最理想的连续操作用一条 execute_operations 提交（尽量合并到同一次调用）。',
                parameters: {
                    type: 'object',
                    properties: {
                        operations: {
                            type: 'array',
                            description: '按执行顺序排列的操作队列',
                            items: {
                                type: 'object',
                                properties: {
                                    action: { type: 'string', enum: ['click', 'type', 'key', 'mouse', 'wheel', 'scroll', 'hover', 'select', 'wait'], description: '操作类型' },
                                    target: { type: 'string', description: '点击/输入/悬停/下拉目标元素的选择器或文本描述（click/type/hover/select 需要）' },
                                    x: { type: 'integer', description: '点击/鼠标的视口 X 坐标（click/mouse 可选，与截图网格坐标一致、最左为 0；缺省点击元素中心）' },
                                    y: { type: 'integer', description: '点击/鼠标的视口 Y 坐标（click/mouse 可选，与截图网格坐标一致、最顶为 0；缺省点击元素中心）' },
                                    text: { type: 'string', description: '要输入的文本（type 需要）' },
                                    key: { type: 'string', description: '按键名。组合键用 + 连接修饰键与主键，如 Ctrl+A、Ctrl+Shift+Z、Cmd+Shift+P（修饰键可用 ctrl/control、alt/option、shift、cmd/meta/win）；按住语义用前缀表达：short:/long:/hold:/tap:，如 W、short:W、long:Ctrl+A（key 需要）' },
                                    button: { type: 'string', enum: ['left', 'right', 'middle'], description: '鼠标按键（mouse，默认 left；普通左键单击请直接用 click 而非 mouse）' },
                                    hold: { type: 'string', enum: ['tap', 'short', 'long'], description: '鼠标按住语义（mouse，默认 tap；short=短按1秒/long=长按10秒）' },
                                    direction: { type: 'string', enum: ['up', 'down', 'left', 'right', 'top', 'bottom'], description: '滚动方向：wheel 用 up/down 表示滚轮上/下滚动；scroll 用 up/down/left/right/top/bottom' },
                                    ticks: { type: 'integer', description: '滚轮格数（wheel，默认 3）' },
                                    amount: { type: 'integer', description: '页面滚动像素（scroll，可选）' },
                                    value: { type: 'string', description: '下拉选项值或文本（select 需要）' },
                                    ms: { type: 'integer', description: '等待毫秒数（wait）' }
                                },
                                required: ['action']
                            }
                        }
                    },
                    required: ['operations']
                }
            }
        }
    ];

    // ================= 系统提示词（专用角色） =================

    function buildSystemPrompt() {
        return [
            '你是星月智能「通用页面操作」的专用智能体（AtoA 执行者），负责解析月华发来的自然语言指令，',
            '并作为当前页面的最终执行者：先进行语义理解与元素分析，再输出一连串操作序列（操作队列），',
            '由程序按队列逐个执行，完成用户要求的点击、输入、按键、滚轮、滑动、悬停、下拉选择等操作。',
            '',
            '【能力范围】',
            '点击页面元素、向输入框输入文本、模拟键盘按键（含组合键、短按/长按）、模拟鼠标左右键（含短按/长按）、',
            '鼠标滚轮上下滚动、页面滚动、悬停、下拉选择、捕获可见可交互元素、查看页面状态、等待。',
            '',
            '【执行规则】',
            '1. 每轮用户消息会附上【页面信息】、一张叠加网格的最新视口截图、【页面元素】【月华指令】；【页面元素】是当前可见可交互元素的紧凑列表（含标签/文本/选择器）。',
            '2. 分析阶段：首先理解【月华指令】并用视觉定位目标。每轮都会附带一张「最新状态的视口截图」（有且只有一张），截图上覆盖 50px 坐标网格，并把可见可交互元素画成与【页面元素】列表同序的编号框。若不确定目标元素，先调用 capture_page（或 capture_screenshot、get_state）确认；确实不存在则如实说明，不要臆造。',
            '3. 执行阶段：分析就绪后，用**一条** execute_operations 提交完整操作队列（operations 数组），把需要连续执行的点击、按键、滚轮、输入等按顺序放入队列。',
            '   程序会从前往后逐个执行，执行完上一个操作后等待 0.5 秒再执行下一个。',
            '4. 操作队列支持的 action：click=左键单击、type=输入文本、key=键盘按键、mouse=鼠标按键、wheel=滚轮滚动、scroll=页面滚动、hover=悬停、select=下拉选择、wait=等待。',
            '5. 鼠标交互：**click=左键单击**，覆盖绝大多数点击（按钮/链接/画布目标点）；**mouse** 仅当需要非左键（right=右键/middle=中键）或按住时长（hold: short=短按/long=长按）时才用，普通左键单击一律用 click、不要用 mouse 代替。',
            '6. key 支持组合键与按住语义。组合键用 + 连接修饰键与主键：修饰键可用 ctrl/control、alt/option、shift、cmd/meta/win，主键为单个字母/数字/方向键/回车等（如 Ctrl+A、Ctrl+Shift+Z、Cmd+Shift+P）；按住语义用前缀表达：默认/无前缀=键入（瞬间按下即松开）、short:=短按（1 秒）、long: 或 hold:=长按（10 秒），可与组合键叠加，如 short:Ctrl+A、long:W。',
            '7. wheel 的 direction 用 up/down 表示滚轮向上/向下滚动，ticks 为格数（默认 3）；scroll 的 direction 用 up/down/left/right/top/bottom，也可用 target 定位到元素。',
            '8. 若一次操作队列不足以完成任务（如需根据前一步页面反馈再决策），可分多次 execute_operations；每条指令允许至多 20 轮工具调用循环，未完成不要提前总结，工具执行结果会回传给你。',
            '9. 全部操作完成后，用一两句简洁的中文总结做了什么与结果，不要输出额外内容或代码块。',
            '',
            '【如何读截图（视觉定位）】',
            '- 每轮附带的那张截图，其顶部横排数字是 X 轴网格坐标（以最左为 0），左侧竖排数字是 Y 轴网格坐标（以最顶为 0），一格 50px；坐标表示页面元素在视口中的像素位置。',
            '- 截图上的橙色编号框与【页面元素】列表编号一一对应：列表中第 n 个元素，就是截图里编号 n 的框。',
            '- 定位元素时结合编号框位置与列表中的 selector 双重确认；target 优先用列表里的 selector，能精确命中则不用坐标。',
            '- 点击「画布/画面/地图」这类需要按坐标定位的目标时（这类目标通常监听 pointer 事件并用坐标拾取，而非普通 click 回调），用 click 或 mouse 都可用 x/y 指定视口坐标（与截图网格一致、最左/最顶为 0）；坐标由你自主决定，读截图网格后在目标交互区域内任意选点、命中需要操作的坐标即可，不必固定在中心；只点普通按钮/链接则用 selector 即可。'
        ].join('\n');
    }

    /* ---------------- 模型调用（OpenAI v1 协议，琉璃后端 /v1 代理） ---------------- */

    async function callAgentModel(messages) {
        const cfg = await loadModelConfig();
        const response = await fetch('/v1/chat/completions', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                model: cfg.model,
                messages: messages,
                tools: agentTools,
                stream: false
            })
        });
        if (!response.ok) throw new Error('AI 调用失败: HTTP ' + response.status);
        const data = await response.json();
        if (data.choices && data.choices[0] && data.choices[0].message) {
            return data.choices[0].message;
        }
        if (data.success && data.data && data.data.choices && data.data.choices[0]) {
            return data.data.choices[0].message;
        }
        throw new Error('AI 响应格式异常');
    }

    function safeParseArgs(jsonStr) {
        if (jsonStr && typeof jsonStr === 'object') return jsonStr;
        if (!jsonStr) return {};
        try {
            const parsed = JSON.parse(jsonStr);
            return parsed && typeof parsed === 'object' ? parsed : {};
        } catch (e) {
            return {};
        }
    }

    /* ---------------- 工具执行器（结构化参数 → 页面操作原语） ---------------- */

    function execCapturePage(args) {
        const items = captureElements({ max: (args && args.max) || AGENT_MAX_LIST_ITEMS });
        if (items.length === 0) return { success: true, text: '页面上没有可见可交互元素' };
        return { success: true, text: buildCompactElements(items) };
    }

    function execGetState() {
        const s = getState();
        return { success: true, text: buildPageInfoText() };
    }

    // capture_screenshot：刷新最新截图并回读当前视口规模与元素编号规模。
    async function execCaptureScreenshot() {
        const items = captureElements({ max: AGENT_MAX_LIST_ITEMS });
        const shot = await captureScreenshot(items).catch(function () { return null; });
        if (!shot || !shot.dataUrl) return { success: false, error: '截图失败' };
        return {
            success: true,
            text: '已刷新当前视口截图（' + shot.width + '×' + shot.height + '，网格步长 ' + SCREEN_GRID_STEP + 'px，X0=最左/Y0=最顶），可见可交互元素 ' + shot.count + ' 个，橙色编号框与【页面元素】列表序号一一对应。'
        };
    }

    function execWait(args) {
        const ms = Math.max(0, parseInt(args && args.ms, 10) || 0);
        return new Promise((resolve) => {
            setTimeout(() => resolve({ success: true, text: '已等待 ' + ms + 'ms' }), ms);
        });
    }

    // 执行单条操作：把操作对象路由到对应底层页面操作原语
    async function execSingleOperation(op) {
        if (!op || typeof op !== 'object') return { ok: false, reason: '非法操作项' };
        const action = String(op.action || '').toLowerCase();
        switch (action) {
            case 'click': {
                const coords = (typeof op.x === 'number' && typeof op.y === 'number') ? { x: op.x, y: op.y } : null;
                return clickEl(op.target, coords);
            }
            case 'type': return typeEl(op.target, op.text);
            case 'key': return await pressKeyEl(op.key);
            case 'mouse': {
                const holdPrefix = op.hold === 'long' ? 'long:' : (op.hold === 'short' ? 'short:' : '');
                const spec = holdPrefix + (op.button || 'left');
                const coords = (typeof op.x === 'number' && typeof op.y === 'number') ? { x: op.x, y: op.y } : null;
                return await mouseButtonEl(spec, op.target, coords);
            }
            case 'wheel': return wheelEl(op.direction, op.ticks);
            case 'scroll': return op.target ? scrollEl(op.target, op.amount) : scrollEl(op.direction || 'down', op.amount);
            case 'hover': return hoverEl(op.target);
            case 'select': return selectEl(op.target, op.value);
            case 'wait': {
                const ms = Math.max(0, parseInt(op.ms, 10) || 0);
                if (ms > 0) await sleep(ms);
                return { ok: true, info: '已等待 ' + ms + 'ms' };
            }
            default: return { ok: false, reason: '未知操作: ' + action };
        }
    }

    // execute_operations：一次性执行完整操作队列。从前往后逐个执行，
    // 执行完上一个操作后等待 gapMs(0.5s) 再执行下一个；任一步失败即中断并返回已执行记录。
    async function execExecuteOperations(args) {
        const ops = Array.isArray(args && args.operations) ? args.operations : [];
        if (ops.length === 0) return { success: true, text: '操作队列为空，无需执行' };
        const gapMs = 500; // 相邻操作间隔 0.5 秒
        const lines = [];
        let failed = null;
        for (let i = 0; i < ops.length; i++) {
            if (i > 0) await sleep(gapMs); // 执行完上一个操作后等待 0.5 秒再执行下一个
            const op = ops[i];
            const res = await execSingleOperation(op);
            const label = String(op.action || '?')
                + (op.target ? ':' + op.target : (op.key ? ':' + op.key : (op.direction ? ':' + op.direction + (op.ticks ? '×' + op.ticks : '') : '')));
            lines.push((i + 1) + '. ' + label + ' → ' + (res.ok ? res.info : '失败(' + res.reason + ')'));
            if (!res.ok) { failed = op; break; }
        }
        const summary = lines.join('\n');
        if (failed) {
            const who = failed.target ? '「' + failed.target + '」' : ':' + failed.action;
            return { success: false, error: '操作序列中断于第 ' + lines.length + ' 步' + who + '\n执行记录:\n' + summary };
        }
        return { success: true, text: '操作队列执行完成（共 ' + lines.length + ' 步，步间间隔 0.5s）:\n' + summary };
    }

    const toolExecutors = {
        capture_page: execCapturePage,
        get_state: execGetState,
        capture_screenshot: execCaptureScreenshot,
        execute_operations: execExecuteOperations
    };

    async function executeTool(name, args) {
        const executor = toolExecutors[name];
        if (!executor) return { success: false, error: '未知工具: ' + name };
        try {
            return await executor(args || {});
        } catch (e) {
            return { success: false, error: (e && e.message) || String(e) };
        }
    }

    /* ---------------- 主流程：多轮工具调用循环 ---------------- */

    async function runLTPXAgent(instruction) {
        const text = String(instruction || '').trim();
        if (!text) throw new Error('空指令');

        // 每次运行只采集一次最新状态截图 + 元素列表，保证上下文中"有且只有一张"当前截图；
        // 编号框与【页面元素】列表同序，视觉模型据此读取元素网格坐标。
        const elems = captureElements({ max: AGENT_MAX_LIST_ITEMS });
        let shot = null;
        try { shot = await captureScreenshot(elems); } catch (e) { shot = null; }

        // 组装本轮用户消息：页面信息(+<image> 标记，供多模态/Llama 视觉对齐) + 截图 + 元素列表 + 月华指令
        const userParts = [
            { type: 'text', text: '【页面信息】' + buildPageInfoText() + (shot && shot.dataUrl ? '\n<image>' : '') },
            ...(shot && shot.dataUrl ? [{ type: 'image_url', image_url: { url: shot.dataUrl } }] : []),
            { type: 'text', text: '【页面元素】\n' + buildCompactElements(elems) },
            { type: 'text', text: '【月华指令】' + text }
        ];

        // 消息骨架：系统提示 + 独立上下文历史（最近 N 轮）+ 本轮
        const messages = [{ role: 'system', content: buildSystemPrompt() }];
        for (const round of agentHistory) {
            messages.push({ role: 'user', content: round.user });
            messages.push({ role: 'assistant', content: round.assistant });
        }
        messages.push({ role: 'user', content: userParts });

        let lastReply = '';
        for (let loop = 0; loop < AGENT_MAX_TOOL_LOOPS; loop++) {
            const message = await callAgentModel(messages);
            const toolCalls = Array.isArray(message.tool_calls) ? message.tool_calls : [];

            // 无工具调用：智能体给出最终答复
            if (toolCalls.length === 0) {
                lastReply = String(message.content || '').trim() || '已完成';
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
                const args = safeParseArgs(tc.function.arguments);
                const result = await executeTool(tc.function.name, args);
                messages.push({
                    role: 'tool',
                    tool_call_id: tc.id,
                    content: JSON.stringify(result)
                });
            }
        }

        if (!lastReply) lastReply = '已完成相关页面操作';

        // 记录本轮对话并裁剪历史
        agentHistory.push({ user: text, assistant: lastReply });
        if (agentHistory.length > AGENT_MAX_ROUNDS) {
            agentHistory.splice(0, agentHistory.length - AGENT_MAX_ROUNDS);
        }
        return lastReply;
    }

    /* ---------------- 对外主接口：自然语言输入 → 自然语言输出 ---------------- */

    async function run(instruction, opts) {
        opts = opts || {};
        if (!instruction || !instruction.trim()) return '未提供指令，请输入要执行的操作。';
        const text = String(instruction).trim();
        // LTPX AtoA 主路径：LLM 智能体（独立上下文 + 系统提示词 + function calling 多轮执行）。
        // 月华在线时模型服务必然在线（调用由月华经 /v1 转发），因此不保留离线规则回退。
        return await runLTPXAgent(text);
    }

    /* ---------------- AtoA 集成层 ---------------- */

    function enableAtoA(opts) {
        if (window.__miniLTPAtoAReady) return publicApi;
        window.__miniLTPAtoAReady = true;
        window.addEventListener('message', async function (event) {
            const data = event.data;
            if (!data || typeof data !== 'object' || data.type !== 'ltpx_run') return;
            const requestId = data.request_id;
            const args = data.arguments || {};
            const instruction = args.instruction || '';
            try {
                const text = await run(instruction, opts || {});
                postResult(requestId, true, text, '');
            } catch (e) {
                postResult(requestId, false, '', String(e && e.message || e));
            }
        });
        return publicApi;
    }

    function postResult(requestId, success, text, error) {
        try {
            if (!window.parent) return;
            window.parent.postMessage({
                type: 'ltpx_result',
                request_id: requestId,
                success: !!success,
                text: text || '',
                error: error || '',
                // 页面操作型智能体执行后保持页面打开：让用户看到操作结果、继续观察/交互
                keep_open: true
            }, '*');
        } catch (e) { /* 父窗口不可达时静默 */ }
    }

    var publicApi = {
        version: VERSION,
        run: run,               // 自然语言输入接口 → 自然语言输出（Promise<string>）
        enableAtoA: enableAtoA, // 启用/再次启用 AtoA 集成
        // ---- 底层页面操作原语（供工具执行器与宿主扩展/测试） ----
        capture: captureElements,
        click: clickEl,
        type: typeEl,
        pressKey: pressKeyEl,
        scroll: scrollEl,
        hover: hoverEl,
        select: selectEl,
        mouseButton: mouseButtonEl,
        wheel: wheelEl,
        getState: getState,
        captureScreenshot: captureScreenshot,
        // ---- 内部工具 ----
        query: query,
        queryAll: queryAll,
        isVisible: isVisible,
        resolveTarget: resolveTarget,
        // ---- 智能体状态 ----
        resetHistory: function () { agentHistory = []; return agentHistory.length; },
        // ---- 模型配置（从 lunar_config.json 读取） ----
        getModelConfig: loadModelConfig,
        refreshModelConfig: refreshModelConfig
    };

    // 位于 iframe 中（即被琉璃等宿主嵌入）时自动启用 AtoA 集成
    if (typeof window !== 'undefined' && window !== window.top && window.parent) {
        enableAtoA();
    }

    return publicApi;
});
