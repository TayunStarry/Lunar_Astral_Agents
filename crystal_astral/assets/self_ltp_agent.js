/* =====================================================================
 * SelfLTP Agent · 自主页面操作智能体（LLM 驱动，Self-LTP 自循环驾驶）
 * ---------------------------------------------------------------------
 * 遵循 LTPX 设计理念的「Self-LTP」分支：不接入 Lunar AtoA，由用户通过
 * 页面上的（开始/停止）按钮 + 文本框指定【初始任务】，智能体多轮自循环执行。
 *
 * 核心机制（区分于一次性批处理）：
 *   1. 计划阶段：收到初始任务后，先调用 set_plan 把任务拆解为一连串计划项，
 *      每项对应页面上的一个具体操作，并记录为「任务历史」。
 *   2. 执行阶段：多轮循环，**每轮只调用一个原子操作工具**（一次一个操作）。
 *   3. 验证：每个操作执行后都会重新观测页面（最新截图 + 元素），判断是否真正生效。
 *   4. 重试：某一步未命中/效果不正确时，用同一操作重试（可多次），或先 wait 再试；
 *      未确认完成绝不跳到下一步。
 *   5. 确认：关键步骤用 confirm_step(no, passed) 明确标记完成或需重试。
 *   6. 结束：全部完成时用 finish 总结。
 *
 * - 页面操作原语统一来自共享模块 window.SharedInput（/shared-input.js）。
 * - 模型调用走琉璃后端 /v1 代理，参数从 lunar_config.json 的 agent 字段读取，不硬编码。
 * - 自然语言理解完全由 LLM 完成，禁止用正则/规则引擎模拟。
 * ===================================================================== */
(function (global, factory) {
    var api = factory();
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    } else if (typeof define === 'function' && define.amd) {
        define([], function () { return api; });
    } else {
        global.SelfLTPAgent = api;
    }
})(typeof window !== 'undefined' ? window : (typeof globalThis !== 'undefined' ? globalThis : this), function () {
    'use strict';

    var VERSION = '2.0.0';

    // ================= 智能体常量与状态 =================

    /** 页面元素列表最多展示条目数（控制 token 消耗） */
    var AGENT_MAX_LIST_ITEMS = 60;
    /** 自主自循环的最大轮次（每轮一个原子操作，兜底防死循环；重试也在其内） */
    var SELF_MAX_STEPS = 60;

    /** 自主执行是否运行中（开始按钮置 true，停止按钮/结束/异常置 false） */
    var running = false;
    /** 用户是否请求停止（供循环各边界检查） */
    var stopRequested = false;

    /** 任务历史（计划项状态），供前端「可折叠任务历史」展示 */
    var taskHistory = [];
    /** 当前阶段：planning / executing / done */
    var stageFlag = 'planning';
    /** 外部日志回调（runSelfPlan 时注入） */
    var emitLog = null;

    // ================= 模型配置加载（从 lunar_config.json 的 agent 字段读取，不硬编码） =================

    /** 配置缓存与加载 Promise（一次加载，后续复用） */
    let modelConfigPromise = null;
    let modelConfig = null;

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
                modelConfig = { model: 'system-multimodal', base_url: '/v1', api_key: null };
                return modelConfig;
            }
        })();
        return modelConfigPromise;
    }

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


    /* ---------------- 工具定义（OpenAI function calling 格式） ----------------
       每个工具 = 一个具体原子操作。set_plan 用于计划，confirm_step 用于验证标记。 */

    var agentTools = [
        {
            type: 'function',
            function: {
                name: 'set_plan',
                description: '(计划阶段·首选调用) 根据【初始任务】与当前页面，把任务拆解为一连串具体的执行计划项，每项对应页面上的一个具体操作（如：点击某个按钮、向输入框输入文本后提交）。调用后系统会建立任务历史，随后进入执行阶段逐项操作。',
                parameters: {
                    type: 'object',
                    properties: {
                        plan: {
                            type: 'array',
                            description: '计划项列表（按执行顺序）',
                            items: {
                                type: 'object',
                                properties: {
                                    no: { type: 'integer', description: '步骤序号（从 1 起）' },
                                    action: { type: 'string', description: '该步骤操作描述（简短，如：点击提交按钮）' },
                                    detail: { type: 'string', description: '具体落点/定位说明（目标选择器或视口坐标）' }
                                },
                                required: ['no', 'action']
                            }
                        }
                    },
                    required: ['plan']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'capture_page',
                description: '捕获当前页面可见可交互的元素快照（含标签/文本/选择器/位置），用于定位可操作目标。不确定目标元素时调用确认。',
                parameters: { type: 'object', properties: { max: { type: 'integer', description: '最多返回元素数，默认 60' } }, required: [] }
            }
        },
        {
            type: 'function',
            function: {
                name: 'get_state',
                description: '查看当前页面状态：URL、标题、就绪状态、视口尺寸、滚动位置。',
                parameters: { type: 'object', properties: {}, required: [] }
            }
        },
        {
            type: 'function',
            function: {
                name: 'capture_screenshot',
                description: '刷新为最新一张视口截图（叠加 50px 坐标网格 + 元素编号框），用于核对元素网格坐标、观察操作结果。',
                parameters: { type: 'object', properties: {}, required: [] }
            }
        },
        {
            type: 'function',
            function: {
                name: 'click',
                description: '左键单击一个元素，或在画布上按视口坐标单击某点。普通按钮/链接用 target；画布/地图这类按坐标交互的目标用 x/y。',
                parameters: {
                    type: 'object',
                    properties: {
                        target: { type: 'string', description: '目标元素选择器或文本描述（可选，给了则点击其中心）' },
                        x: { type: 'integer', description: '视口 X 坐标（可选，与截图网格一致、最左为 0）' },
                        y: { type: 'integer', description: '视口 Y 坐标（可选，与截图网格一致、最顶为 0）' }
                    },
                    required: []
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'type_text',
                description: '向目标输入框输入文本。',
                parameters: {
                    type: 'object',
                    properties: {
                        target: { type: 'string', description: '目标输入框选择器或描述' },
                        text: { type: 'string', description: '要输入的文本' }
                    },
                    required: ['target', 'text']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'press_key',
                description: '模拟键盘按键，支持组合键（Ctrl+A、Cmd+Shift+P）与按住语义（short:/long: 前缀，如 long:W）。',
                parameters: {
                    type: 'object',
                    properties: { key: { type: 'string', description: '按键，组合键用 + 连接，如 W、Enter、Ctrl+A、long:W' } },
                    required: ['key']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'mouse_press',
                description: '通用鼠标按键：需要非左键（right/middle）或按住时长（hold short/long）时使用；普通左键单击请用 click。',
                parameters: {
                    type: 'object',
                    properties: {
                        button: { type: 'string', enum: ['left', 'right', 'middle'], description: '按键，默认 left' },
                        hold: { type: 'string', enum: ['tap', 'short', 'long'], description: '按住语义，默认 tap' },
                        target: { type: 'string', description: '目标元素（可选）' },
                        x: { type: 'integer', description: '视口 X 坐标（可选）' },
                        y: { type: 'integer', description: '视口 Y 坐标（可选）' }
                    },
                    required: []
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'hover',
                description: '悬停到目标元素上。',
                parameters: {
                    type: 'object',
                    properties: { target: { type: 'string', description: '目标元素选择器或描述' } },
                    required: ['target']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'select_option',
                description: '在下拉框中选择一个选项。',
                parameters: {
                    type: 'object',
                    properties: {
                        target: { type: 'string', description: '下拉框选择器或描述' },
                        value: { type: 'string', description: '选项值或文本' }
                    },
                    required: ['target', 'value']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'scroll_page',
                description: '页面滚动：direction 用 up/down/left/right/top/bottom；也支持用 target 滚动到某元素。',
                parameters: {
                    type: 'object',
                    properties: {
                        direction: { type: 'string', enum: ['up', 'down', 'left', 'right', 'top', 'bottom'], description: '滚动方向' },
                        amount: { type: 'integer', description: '像素（可选，缺省约一屏的 80%）' },
                        target: { type: 'string', description: '滚动到该元素（可选）' }
                    },
                    required: []
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'scroll_wheel',
                description: '鼠标滚轮滚动：direction 用 up/down，ticks 为格数（默认 3）。',
                parameters: {
                    type: 'object',
                    properties: {
                        direction: { type: 'string', enum: ['up', 'down'], description: '滚轮方向' },
                        ticks: { type: 'integer', description: '格数，默认 3' }
                    },
                    required: []
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'wait',
                description: '等待 ms 毫秒（页面动画/加载/反应完成用），之后会重新观测页面。',
                parameters: {
                    type: 'object',
                    properties: { ms: { type: 'integer', description: '毫秒数，如 2000' } },
                    required: ['ms']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'confirm_step',
                description: '确认某计划项已通过验证（passed=true）或仍未命中需重试（passed=false）。在一次操作执行并观察结果后，明确标记该关键步骤完成或需重试。',
                parameters: {
                    type: 'object',
                    properties: {
                        no: { type: 'integer', description: '计划步骤序号' },
                        passed: { type: 'boolean', description: 'true=该步已确认完成；false=未命中，需重试' },
                        note: { type: 'string', description: '说明（可选）' }
                    },
                    required: ['no', 'passed']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'finish',
                description: '结束本次自主任务并给出总结。当所有计划步骤完成、或确实无法继续推进时调用。',
                parameters: {
                    type: 'object',
                    properties: { summary: { type: 'string', description: '一两句简洁中文总结' } },
                    required: ['summary']
                }
            }
        }
    ];

    /* ---------------- 系统提示词（专用角色：多轮自循环·单步执行） ---------------- */

    function buildSystemPrompt() {
        return [
            '你是星月智能「自主页面操作」的专用智能体（Self-LTP）。你运行在目标应用页面内，接收用户的【初始任务】。',
            '你是「多轮自循环」智能体：不是一次调用把所有操作做完，而是先立计划，再一步步推进——每执行**一个**具体操作，',
            '都要观察页面变化、验证该操作是否生效，确认后才进入下一步；未命中就重试。',
            '',
            '【工作方式（关键，必须遵守）】',
            '1. 计划阶段：收到【初始任务】后，第一步先调用 set_plan，把任务拆解为一连串计划项，每项对应一个具体操作',
            '   （例如：点击一个按钮 → 确认按钮已生效 → 再向输入框输入文本后提交）。系统会将此记录为任务历史。',
            '2. 执行阶段：一次只调用**一个**工具（一个原子操作）。每轮你会收到最新视口截图 + 页面元素 + 任务历史与阶段提示。',
            '3. 验证：执行某个操作后，观察最新页面（截图/元素）判断它是否真正生效（如按钮是否被选中、画面是否出现预期变化）。',
            '4. 重试：若某一步未命中/效果不正确，就用同一个操作再试（允许多次重试），或先 wait 让页面反应完成再试；',
            '   绝不要带着没完成的步骤跳到下一步，也不要假装某一步已生效/已点击。',
            '5. 确认完成：关键步骤用 confirm_step(no, passed=true) 明确标记完成；passed=false 表示该步仍未命中、需重试。',
            '6. 结束：全部计划完成、或确实无法继续时，用 finish 给出简洁中文总结；未完成不得提前 finish。',
            '',
            '【纪律】',
            '- 一次循环只做一件事：每次只调用一个操作工具（一个工具调用），做完立即停下观察、验证，再决定下一步。',
            '- 不要在一次调用里连着发多个操作；不要在操作真正生效前就声称完成。',
            '- 合理使用 wait：动画/加载未完成时先等待，再验证。',
            '',
            '【能力范围】',
            '点击/输入/键盘/鼠标/滚轮/滚动/悬停/下拉/等待/截图识图/查看页面状态/查看元素。',
            '',
            '【如何读截图（视觉定位）】',
            '- 最新截图上覆盖 50px 坐标网格（顶部为 X、左侧为 Y，最左/最顶为 0），并把可见可交互元素画成与【页面元素】列表同序的编号框。',
            '- 定位元素时结合编号框位置与列表 selector 双重确认；点按钮/链接优先用 target；点画布/画面用 x/y 坐标。',
            '',
            '【鼠标分工】',
            '- click=左键单击（绝大多数点击）。普通按钮/链接用 target；画布/画面这类按坐标交互的目标用 x/y。',
            '- mouse_press=通用鼠标按键，仅用于需要 right/middle 或按住时长（hold: short/long）时；普通左键单击一律用 click。',
            '',
            '【键盘】',
            '- key 支持组合键（Ctrl+A、Cmd+Shift+P）与按住语义（short: 短按 / long: 长按，如 long:W）。'
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

    /* ---------------- 任务历史与日志 ---------------- */

    function resetTaskState() {
        taskHistory = [];
        stageFlag = 'planning';
    }

    function emit(evt) {
        if (typeof emitLog === 'function') { try { emitLog(evt); } catch (e) { /* 忽略 */ } }
    }

    /* ---------------- 工具执行器（每个工具 = 一个具体原子操作） ---------------- */

    function execCapturePage(args) {
        const items = captureElements({ max: (args && args.max) || AGENT_MAX_LIST_ITEMS });
        return items.length === 0
            ? { success: true, text: '页面上没有可见可交互元素' }
            : { success: true, text: buildCompactElements(items) };
    }

    function execGetState() { return { success: true, text: buildPageInfoText() }; }

    async function execCaptureScreenshot() {
        const items = captureElements({ max: AGENT_MAX_LIST_ITEMS });
        const shot = await captureScreenshot(items).catch(function () { return null; });
        if (!shot || !shot.dataUrl) return { success: false, error: '截图失败' };
        return {
            success: true,
            text: '已刷新视口截图（' + shot.width + '×' + shot.height + '，网格步长 ' + SCREEN_GRID_STEP + 'px），可见可交互元素 ' + shot.count + ' 个。'
        };
    }

    // 计划阶段：登记任务历史，进入执行阶段
    function execSetPlan(args) {
        const plan = Array.isArray(args && args.plan) ? args.plan : [];
        taskHistory = plan.filter(function (p) { return p && p.action; }).map(function (p) {
            return { no: p.no, action: p.action, detail: p.detail || '', status: 'pending', tries: 0 };
        });
        stageFlag = 'executing';
        emit({ kind: 'history', history: taskHistory.slice() });
        return { success: true, text: '已建立执行计划，共 ' + taskHistory.length + ' 项，进入执行阶段。开始逐项执行（一次一个操作，逐步验证）。' };
    }

    function execClick(args) {
        const coords = (typeof args.x === 'number' && typeof args.y === 'number') ? { x: args.x, y: args.y } : null;
        return clickEl(args.target, coords);
    }

    function execTypeText(args) { return typeEl(args.target, args.text); }

    function execPressKey(args) { return pressKeyEl(args.key); }

    async function execMousePress(args) {
        const spec = (args.hold === 'long' ? 'long:' : (args.hold === 'short' ? 'short:' : '')) + (args.button || 'left');
        const coords = (typeof args.x === 'number' && typeof args.y === 'number') ? { x: args.x, y: args.y } : null;
        return await mouseButtonEl(spec, args.target, coords);
    }

    function execHover(args) { return hoverEl(args.target); }

    function execSelect(args) { return selectEl(args.target, args.value); }

    function execScrollPage(args) {
        return args.target ? scrollEl(args.target, args.amount) : scrollEl(args.direction || 'down', args.amount);
    }

    function execScrollWheel(args) { return wheelEl(args.direction, args.ticks); }

    async function execWaitTool(args) {
        const ms = Math.max(0, parseInt(args.ms, 10) || 0);
        if (ms > 0) await sleep(ms);
        return { success: true, text: '已等待 ' + ms + 'ms，随后重新观测页面进行验证。' };
    }

    function execConfirmStep(args) {
        const no = parseInt(args.no, 10) || 0;
        const item = taskHistory.find(function (t) { return t.no === no; });
        if (!item) return { success: false, error: '未找到计划项 ' + no };
        if (args.passed) {
            item.status = 'done';
            emit({ kind: 'history', history: taskHistory.slice() });
            return { success: true, text: '计划项 ' + no + ' 已确认完成。' + (args.note ? ' 说明：' + args.note : '') };
        }
        item.status = 'retry';
        item.tries = (item.tries || 0) + 1;
        emit({ kind: 'history', history: taskHistory.slice() });
        return { success: true, text: '计划项 ' + no + ' 标记为未命中，将重试。' + (args.note ? ' 说明：' + args.note : '') };
    }

    function execFinishTool(args) {
        return { success: true, text: String((args && args.summary) || '任务已完成').trim() };
    }

    const toolExecutors = {
        set_plan: execSetPlan,
        capture_page: execCapturePage,
        get_state: execGetState,
        capture_screenshot: execCaptureScreenshot,
        click: execClick,
        type_text: execTypeText,
        press_key: execPressKey,
        mouse_press: execMousePress,
        hover: execHover,
        select_option: execSelect,
        scroll_page: execScrollPage,
        scroll_wheel: execScrollWheel,
        wait: execWaitTool,
        confirm_step: execConfirmStep,
        finish: execFinishTool
    };

    async function executeTool(name, args) {
        const executor = toolExecutors[name];
        if (!executor) return { success: false, error: '未知工具: ' + name };
        try { return await executor(args || {}); }
        catch (e) { return { success: false, error: (e && e.message) || String(e) }; }
    }

    /* ---------------- 自主自循环（多轮：一次一个操作 + 验证重试） ---------------- */

    // 每轮注入一张截图，累积会拖垮上下文；把较早截图替换为文字占位，只保留最新一张
    function trimOldScreenshots(messages) {
        var imageMsgIdx = [];
        messages.forEach(function (m, i) {
            if (m && m.role === 'user' && Array.isArray(m.content)) {
                var hasImg = m.content.some(function (p) { return p && p.type === 'image_url'; });
                if (hasImg) imageMsgIdx.push(i);
            }
        });
        if (imageMsgIdx.length <= 1) return;
        var keep = imageMsgIdx[imageMsgIdx.length - 1];
        imageMsgIdx.forEach(function (i) {
            if (i === keep) return;
            messages[i].content = messages[i].content.map(function (p) {
                if (p && p.type === 'image_url') {
                    return { type: 'text', text: '（较早的视口截图已省略，以最新截图为准）' };
                }
                if (p && p.type === 'text' && typeof p.text === 'string') {
                    return { type: 'text', text: p.text.replace(/<image>/g, '') };
                }
                return p;
            });
        });
    }

    function buildContextText(step, elems, shot) {
        var s = '【第 ' + step + ' 轮 · 页面信息】' + buildPageInfoText() + (shot && shot.dataUrl ? '\n<image>' : '');
        s += '\n【页面元素】\n' + buildCompactElements(elems);
        s += '\n【当前阶段】' + (stageFlag === 'planning' ? '计划阶段（请先调用 set_plan 建立计划）' : '执行阶段');
        if (taskHistory.length) {
            s += '\n【任务历史】\n' + taskHistory.map(function (t) {
                return t.no + '.[' + t.status + (t.tries ? '×' + t.tries : '') + '] ' + t.action + (t.detail ? '（' + t.detail + '）' : '');
            }).join('\n');
        }
        s += '\n【决策指令】请只规划并执行**一个**操作（本轮只调用一个工具）：';
        s += '\n- 若上一步操作刚执行完，先依据最新页面判断它是否真正生效（按钮是否被选中、画面是否出现预期变化）。';
        s += '\n- 生效：推进到下一步操作；关键验收步用 confirm_step(no, passed=true) 标记完成。';
        s += '\n- 未生效：用同一个操作重试（可多次），或 wait 后再试；未确认完成前绝不要跳到下一步。';
        s += '\n- 计划阶段则先调 set_plan。全部完成或无法继续时调用 finish。';
        return s;
    }

    async function runSelfPlan(initialTask, onStep) {
        const text = String(initialTask || '').trim();
        if (!text) throw new Error('空任务');

        running = true;
        stopRequested = false;
        resetTaskState();
        emitLog = onStep || null;
        emit({ kind: 'log', text: '开始自主执行任务：' + text });

        const messages = [{ role: 'system', content: buildSystemPrompt() }];
        messages.push({ role: 'user', content: '【初始任务】' + text + '\n\n请先调用 set_plan 将本任务拆解为具体执行计划（每项对应一个具体操作）。' });

        let finalSummary = '';

        outer:
        for (let step = 1; step <= SELF_MAX_STEPS; step++) {
            if (stopRequested || !running) break;

            const elems = captureElements({ max: AGENT_MAX_LIST_ITEMS });
            let shot = null;
            try { shot = await captureScreenshot(elems); } catch (e) { shot = null; }

            const stepParts = [{ type: 'text', text: buildContextText(step, elems, shot) }];
            if (shot && shot.dataUrl) stepParts.push({ type: 'image_url', image_url: { url: shot.dataUrl } });
            messages.push({ role: 'user', content: stepParts });
            trimOldScreenshots(messages);

            const message = await callAgentModel(messages);
            const tcs = Array.isArray(message.tool_calls) ? message.tool_calls : [];

            if (tcs.length === 0) {
                finalSummary = String(message.content || '').trim() || '任务已执行完成';
                emit({ kind: 'log', text: finalSummary });
                break;
            }

            // 强制：每轮只执行第一个工具调用（一个原子操作），一次只做一件事
            const tc = tcs[0];
            const args = safeParseArgs(tc.function.arguments);

            messages.push({
                role: 'assistant',
                content: message.content || '',
                tool_calls: [{ id: tc.id, type: 'function', function: { name: tc.function.name, arguments: tc.function.arguments } }]
            });

            // 执行操作前把首个 pending 计划项标记为运行中
            if (toolExecutors[tc.function.name]) {
                const pendingItem = taskHistory.find(function (t) { return t.status === 'pending'; });
                if (pendingItem) {
                    pendingItem.status = 'running';
                    emit({ kind: 'history', history: taskHistory.slice() });
                }
            }

            const result = await executeTool(tc.function.name, args);
            messages.push({ role: 'tool', tool_call_id: tc.id, content: JSON.stringify(result) });

            const resText = result && result.text ? result.text
                : (result && result.error ? '失败：' + result.error : '已执行');
            emit({ kind: 'log', text: '[' + tc.function.name + '] ' + resText });

            if (tc.function.name === 'finish') {
                finalSummary = String((args && args.summary) || (message.content || '').trim() || '任务已执行完成').trim();
                stageFlag = 'done';
                // 收尾：剩余「执行中/待执行」计划项随任务完成统一置为完成并推送
                taskHistory.forEach(function (t) {
                    if (t.status === 'running' || t.status === 'pending') t.status = 'done';
                });
                emit({ kind: 'history', history: taskHistory.slice() });
                emit({ kind: 'log', text: '任务结束：' + finalSummary });
                break;
            }
        }

        if (!finalSummary) finalSummary = stopRequested ? '任务已被手动停止' : '已达到最大执行轮次，任务暂停';
        running = false;
        // 结束兜底：再同步一次最终任务历史，确保 UI 显示与真实状态一致
        emit({ kind: 'history', history: taskHistory.slice() });
        emit({ kind: 'log', text: '任务结束：' + finalSummary });
        emitLog = null;
        return finalSummary;
    }

    function stop() {
        stopRequested = true;
        running = false;
    }

    /* ---------------- 控制面板（开始/停止 + 初始任务 + 可折叠任务历史 + 运行日志） ---------------- */

    var uiReady = false;

    function ensurePanelStyles() {
        if (document.getElementById('selfltp-style')) return;
        const style = document.createElement('style');
        style.id = 'selfltp-style';
        style.textContent = [
            /* 白灰磨砂半透明 · 胶囊横条（右上角，可拖动） */
            '#selfltp-root{position:fixed;top:12px;right:12px;z-index:2147483647;width:340px;max-width:calc(100vw - 16px);font-family:system-ui,-apple-system,"Segoe UI","Microsoft YaHei",sans-serif;font-size:13px;color:#3a4048;}',
            '#selfltp-bar{display:flex;align-items:center;gap:8px;padding:8px;border-radius:999px;background:rgba(255,255,255,0.72);-webkit-backdrop-filter:blur(14px) saturate(160%);backdrop-filter:blur(14px) saturate(160%);border:1px solid rgba(255,255,255,0.9);box-shadow:0 6px 24px rgba(110,120,140,0.18),inset 0 1px 0 rgba(255,255,255,0.9);cursor:grab;user-select:none;touch-action:none;}',
            '#selfltp-bar.dragging{cursor:grabbing;}',
            /* 左侧圆角矩形按钮：展开/收起历史记录 */
            '#selfltp-histbtn{display:flex;align-items:center;gap:6px;flex:0 0 auto;border:none;border-radius:14px;padding:8px 12px;background:rgba(233,236,240,0.9);color:#4a5260;font-size:12px;font-weight:600;white-space:nowrap;cursor:pointer;transition:background .15s ease;}',
            '#selfltp-histbtn:hover{background:rgba(219,223,229,0.95);}',
            '#selfltp-histbtn.active{background:#3a4048;color:#fff;}',
            /* 中间胶囊输入框 */
            '#selfltp-task{flex:1;min-width:120px;border:none;border-radius:999px;padding:8px 14px;background:rgba(240,242,245,0.85);color:#3a4048;font-size:12px;outline:none;transition:background .15s ease;}',
            '#selfltp-task::placeholder{color:#9aa2ad;}',
            '#selfltp-task:focus{background:rgba(255,255,255,0.95);}',
            '#selfltp-task:disabled{opacity:.6;}',
            /* 右侧圆形按钮：开始 / 停止 */
            '#selfltp-runbtn{flex:0 0 auto;width:36px;height:36px;border:none;border-radius:50%;display:flex;align-items:center;justify-content:center;color:#fff;font-size:13px;box-shadow:0 2px 8px rgba(0,0,0,0.14);cursor:pointer;transition:transform .12s ease,background .15s ease;}',
            '#selfltp-runbtn.idle{background:linear-gradient(135deg,#4f7cff,#3a63e0);}',
            '#selfltp-runbtn.running{background:linear-gradient(135deg,#ff5a4e,#ff3b30);}',
            '#selfltp-runbtn:hover{transform:scale(1.06);}',
            '#selfltp-runbtn:active{transform:scale(0.95);}',
            /* 历史记录下拉（任务历史 + 运行日志） */
            '#selfltp-drop{position:absolute;top:calc(100% + 10px);right:0;width:340px;max-width:calc(100vw - 24px);display:none;background:rgba(255,255,255,0.82);-webkit-backdrop-filter:blur(16px) saturate(160%);backdrop-filter:blur(16px) saturate(160%);border:1px solid rgba(255,255,255,0.9);border-radius:16px;box-shadow:0 10px 32px rgba(110,120,140,0.22);overflow:hidden;}',
            '#selfltp-drop.show{display:block;}',
            '#selfltp-drophead{display:flex;align-items:center;gap:6px;padding:10px 14px;font-size:11px;font-weight:700;color:#6a7280;letter-spacing:.5px;border-bottom:1px solid rgba(60,70,90,0.08);}',
            '#selfltp-history{margin:0;padding:8px 12px 8px 26px;font-size:11px;line-height:1.6;max-height:180px;overflow:auto;color:#4a5260;}',
            '#selfltp-history li{white-space:pre-wrap;word-break:break-all;padding:1px 0;}',
            '#selfltp-history .st-done{color:#22a06b;}',
            '#selfltp-history .st-retry{color:#d98a00;}',
            '#selfltp-history .st-running{color:#3a63e0;}',
            '#selfltp-status{margin:0;padding:8px 14px;font-size:11px;line-height:1.6;color:#6a7280;max-height:150px;overflow:auto;white-space:pre-wrap;word-break:break-all;border-top:1px solid rgba(60,70,90,0.08);}'
        ].join('\n');
        document.head.appendChild(style);
    }

    function statusLabel(status) {
        if (status === 'done') return '完成';
        if (status === 'running') return '执行中';
        if (status === 'retry') return '重试';
        return '待执行';
    }

    function buildPanel() {
        ensurePanelStyles();

        // 胶囊横条：左侧「历史」按钮 + 中间胶囊输入框 + 右侧圆形开始/停止按钮，无标题
        const root = document.createElement('div');
        root.id = 'selfltp-root';
        root.innerHTML =
            '<div id="selfltp-bar">' +
            '  <button id="selfltp-histbtn" title="展开/收起历史记录"><span>&#9202;</span>历史</button>' +
            '  <input id="selfltp-task" type="text" placeholder="输入初始任务…" autocomplete="off" spellcheck="false">' +
            '  <button id="selfltp-runbtn" class="idle" title="开始">&#9654;</button>' +
            '</div>' +
            '<div id="selfltp-drop">' +
            '  <div id="selfltp-drophead"><span>&#9201;</span>任务历史 · 运行日志</div>' +
            '  <ol id="selfltp-history"></ol>' +
            '  <pre id="selfltp-status">就绪，输入任务后点击 ▶ 开始。</pre>' +
            '</div>';
        document.body.appendChild(root);

        const bar = document.getElementById('selfltp-bar');
        const task = document.getElementById('selfltp-task');
        const runBtn = document.getElementById('selfltp-runbtn');
        const histBtn = document.getElementById('selfltp-histbtn');
        const drop = document.getElementById('selfltp-drop');
        const status = document.getElementById('selfltp-status');
        const history = document.getElementById('selfltp-history');

        function renderHistory(hist) {
            history.innerHTML = '';
            (hist || []).forEach((it) => {
                const li = document.createElement('li');
                const span = document.createElement('span');
                span.className = 'st-' + it.status;
                span.textContent = '[' + statusLabel(it.status) + (it.tries ? '×' + it.tries : '') + '] ';
                li.appendChild(span);
                li.appendChild(document.createTextNode(it.no + '. ' + it.action + (it.detail ? '（' + it.detail + '）' : '')));
                history.appendChild(li);
            });
        }

        function renderLog(txt) {
            const line = String(txt || '');
            status.textContent = status.textContent + line + '\n';
            const lines = status.textContent.split('\n');
            if (lines.length > 60) status.textContent = lines.slice(lines.length - 60).join('\n');
            status.scrollTop = status.scrollHeight;
        }

        function onSelfttpEvt(evt) {
            if (!evt) return;
            if (evt.kind === 'history') renderHistory(evt.history);
            else renderLog(evt.text || '');
        }

        const setRunning = (on) => {
            task.disabled = on;
            runBtn.classList.toggle('idle', !on);
            runBtn.classList.toggle('running', on);
            runBtn.innerHTML = on ? '&#9632;' : '&#9654;';
            runBtn.title = on ? '停止' : '开始';
        };

        // 左侧圆角矩形按钮：展开/收起历史记录
        histBtn.addEventListener('click', () => {
            const show = drop.classList.toggle('show');
            histBtn.classList.toggle('active', show);
        });

        // 右侧圆形按钮：开始 / 停止
        runBtn.addEventListener('click', async () => {
            if (running) {
                stop();
                renderLog('正在停止…');
                return;
            }
            const t = task.value.trim();
            if (!t) { renderLog('请先输入初始任务。'); return; }
            setRunning(true);
            status.textContent = '';
            history.innerHTML = '';
            try {
                const result = await runSelfPlan(t, onSelfttpEvt);
                renderLog('完成：' + result);
            } catch (e) {
                renderLog('执行出错：' + String(e && e.message || e));
            } finally {
                setRunning(false);
            }
        });

        // 拖动：整条横条可拖动（输入框/按钮除外），并限制在视口内
        let dragging = false, offX = 0, offY = 0;
        bar.addEventListener('mousedown', (e) => {
            if (e.target.closest('input,button')) return;
            dragging = true;
            offX = e.clientX - root.offsetLeft;
            offY = e.clientY - root.offsetTop;
            bar.classList.add('dragging');
        });
        window.addEventListener('mousemove', (e) => {
            if (!dragging) return;
            const left = Math.max(0, Math.min(window.innerWidth - root.offsetWidth, e.clientX - offX));
            const top = Math.max(0, Math.min(window.innerHeight - root.offsetHeight, e.clientY - offY));
            root.style.left = left + 'px';
            root.style.top = top + 'px';
            root.style.right = 'auto';
        });
        window.addEventListener('mouseup', () => {
            if (!dragging) return;
            dragging = false;
            bar.classList.remove('dragging');
        });

        return { root, task, runBtn, status };
    }

    function enableSelfUI() {
        if (uiReady) return publicApi;
        uiReady = true;
        const mount = () => { if (!document.getElementById('selfltp-root')) buildPanel(); };
        if (document.body) mount();
        else document.addEventListener('DOMContentLoaded', mount);
        return publicApi;
    }

    /* ---------------- 对外接口 ---------------- */

    var publicApi = {
        version: VERSION,
        run: runSelfPlan,        // 自循环执行：初始任务 → 最终总结（Promise<string>）
        stop: stop,              // 手动停止
        enableUI: enableSelfUI,  // 挂载控制面板（开始/停止 + 输入框 + 可折叠任务历史）
        isRunning: function () { return running; },
        getHistory: function () { return taskHistory.slice(); },
        getStage: function () { return stageFlag; },
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
        getModelConfig: loadModelConfig,
        refreshModelConfig: refreshModelConfig
    };

    // 位于 iframe 中（被琉璃等宿主嵌入）时自动挂载自循环驾驶控制面板
    if (typeof window !== 'undefined' && window !== window.top && window.parent) {
        enableSelfUI();
    }

    return publicApi;
});