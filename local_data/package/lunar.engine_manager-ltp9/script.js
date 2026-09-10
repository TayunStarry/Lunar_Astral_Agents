// ============================================================
// 『 星月智能 』LTP9 引擎可视化测试（蓝图编辑器 v2）
// 特性：
//  - 启动节点：运行全部从启动节点开始，链式顺序执行（上一节点无错方运行下一节点）
//  - 端到端数据连线：从源节点输出端口连到目标节点输入端口，按端口把数据带给目标参数；<顺序>端口仅传放行不传数据
//  - 逻辑门：AND / OR / NOT / NAND / NOR，仅作门控（不传数据）
// 后端信封：crystal_astral/ltp9_debug.go（ltp9/event|call|tool|broadcast|stats|probe|test）
// test 动作覆盖：llm / embed / tool / emoji / platform / db / memory / file / crypto / jwt / command / network / async / http / sleep
// ============================================================

// ==== 常量 ====
const FLOW_FILE = 'database/lunar.engine_manager_ltp9.graph.json'; // 画布持久化到本地文件（local_data/database/）
const WS_MAX_RETRY = 5;
const WS_RETRY_INTERVAL = 3000;
const NODE_W = 200;
const HEADER_H = 34;  // 节点头部高度，端口从下方起排
const PORT_STEP = 22; // 端口纵向间距
// 判定回执的有效类型：只有这些类型 + 匹配 request_id 才算真实回执
// （/ws 集线器会把客户端发出的请求原样广播回显，若不区分类型会把"发送的请求"误当作回执）
const ACK_TYPES = { 'ltp9/result': 1, 'ltp9/test_ack': 1, 'ltp9/stats_ack': 1, 'ltp9/broadcast_ack': 1, 'ltp9/pong': 1 };

// ==== 节点类型定义 ====
// 每类节点：
//   ins     数据输入端口 [{k,label}]；k 对应可被连线填写的参数字段
//   out     数据输出 {k,label}（null 表示无数据输出，仅提供"顺序"空输出）
//   gate    是否为逻辑门
//   fields  参数 schema（表单自动生成）
// 每节点始终额外提供"顺序"输入端口与"顺序"输出端口（仅传放行，不传数据）。
const GATE_TYPES = { and: 1, or: 1, not: 1, nand: 1, nor: 1 };

const NODES = {
    start: {
        label: '启动节点', icon: 'fa-play-circle', color: '#22c55e',
        desc: '运行全部时从本节点开始，链式向下执行；无数据输入输出',
        ins: [], out: null, fields: []
    },
    event_start: {
        label: '事件启动', icon: 'fa-bolt', color: '#f97316',
        desc: '等待目标事件触发后启动逻辑链，并把事件载荷作为输出（事件由页面上「事件触发」或「时钟启动」投递）',
        ins: [], out: { k: 'payload', label: '事件载荷' },
        fields: [
            { key: 'topic', label: '监听事件(topic，留空=任意；下拉=引擎真实已订阅)', type: 'text', def: 'weather.query', dynamic: 'events' },
            { key: 'timeout', label: '等待超时(毫秒)', type: 'number', def: 8000 }
        ]
    },
    clock_start: {
        label: '时钟启动', icon: 'fa-clock', color: '#f59e0b',
        desc: '每隔设定时间启动一次其下游逻辑链（直至点击「停止运行」）',
        ins: [], out: { k: 'tick', label: 'tick' },
        fields: [{ key: 'interval', label: '启动间隔(毫秒)', type: 'number', def: 2000 }]
    },
    event: {
        label: '事件触发', icon: 'fa-bolt', color: '#f97316',
        desc: '触发订阅事件（weather.query / art.generate 等）；主题从引擎已订阅列表动态填充',
        ins: [{ k: 'payload', label: '载荷' }], out: { k: 'result', label: '回执' },
        fields: [
            { key: 'topic', label: '事件主题（下拉=引擎真实已订阅）', type: 'text', ph: 'weather.query', dynamic: 'events' },
            { key: 'payload', label: '载荷 payload (JSON)', type: 'json', rows: 3, def: '{\n  "city": "北京"\n}' }
        ]
    },
    broadcast_all: {
        label: '全局广播', icon: 'fa-share-alt', color: '#8b5cf6',
        desc: 'engine.signal.all 向所有插件广播',
        ins: [{ k: 'payload', label: '载荷' }], out: { k: 'result', label: '回执' },
        fields: [{ key: 'payload', label: '广播载荷 (JSON)', type: 'json', rows: 3, def: '{\n  "from": "engine_manager",\n  "hello": "广播测试"\n}' }]
    },
    broadcast_target: {
        label: '定向广播', icon: 'fa-bullseye', color: '#a855f7',
        desc: 'engine.signal.target 仅向目标插件广播；目标包从引擎真实加载列表动态填充',
        ins: [{ k: 'payload', label: '载荷' }], out: { k: 'result', label: '回执' },
        fields: [
            { key: 'target', label: '目标包ID（下拉=引擎真实加载）', type: 'text', ph: 'com.yaraflow.weather-ltp9', dynamic: 'plugins' },
            { key: 'payload', label: '广播载荷 (JSON)', type: 'json', rows: 3, def: '{\n  "to": "weather",\n  "msg": "定向测试"\n}' }
        ]
    },
    call: {
        label: '跨包调用', icon: 'fa-phone-alt', color: '#14b8a6',
        desc: 'engine.call(包ID).run(函数名, 参数)；单参数框：JSON数组=多参数，单个文本/JSON=首个参数；【输入数据】连线则覆盖为首参',
        ins: [{ k: 'input', label: '输入数据' }], out: { k: 'result', label: '返回值' },
        fields: [
            { key: 'plugin', label: '目标包（下拉=引擎真实加载）', type: 'text', ph: 'com.yaraflow.weather-ltp9', dynamic: 'plugins' },
            { key: 'fn', label: '函数名（下拉=该包真实导出；先选包）', type: 'text', ph: 'queryWeather', dynamic: 'exports' },
            { key: 'args', label: '参数（JSON数组=多参数；单个文本/JSON=首个参数）', type: 'textarea', rows: 1, def: '["北京"]' }
        ]
    },
    command: {
        label: '指令触发', icon: 'fa-terminal', color: '#0d9488',
        desc: 'engine.command：向目标插件派发指令文本（插件留空=向全部插件派发）；插件从引擎真实加载列表动态填充',
        ins: [{ k: 'text', label: '指令文本' }, { k: 'context', label: '上下文' }], out: { k: 'result', label: '结果' },
        fields: [
            { key: 'plugin', label: '目标包ID（留空=全部插件；下拉=引擎真实加载）', type: 'text', ph: '留空派发给全部插件', dynamic: 'plugins' },
            { key: 'text', label: '指令文本', type: 'textarea', rows: 1, def: '/ping' },
            { key: 'context', label: '上下文 context (JSON)', type: 'json', rows: 1, def: '{}' }
        ]
    },
    agent: {
        label: 'LTPX', icon: 'fa-puzzle-piece', color: '#06b6d4',
        desc: 'engine.agent(包ID).run(指令) → Mini-LTP / Node-LTP；【指令输入】作为自然语言',
        ins: [{ k: 'instruction', label: '指令文本' }], out: { k: 'text', label: '返回文本' },
        fields: [
            { key: 'plugin', label: 'LTPX 包ID（下拉=Mini-LTP/Node-LTP 包）', type: 'text', ph: 'com.example.webagent', dynamic: 'agentPlugins' },
            { key: 'instruction', label: '自然语言指令', type: 'textarea', rows: 2, def: '查询当前时间并回复' }
        ]
    },
    db: {
        label: '数据库', icon: 'fa-database', color: '#0ea5e9',
        desc: 'engine.database.query/exec（共享 SQLite）；<SQL/参数> 可来自上游',
        ins: [{ k: 'sql', label: 'SQL' }, { k: 'params', label: '参数' }], out: { k: 'rows', label: '结果' },
        fields: [
            { key: 'op', label: '操作', type: 'select', options: ['query', 'exec'], def: 'query' },
            { key: 'sql', label: 'SQL', type: 'textarea', rows: 2, def: 'SELECT 1 AS ok' },
            { key: 'params', label: '绑定参数 (JSON数组)', type: 'json', rows: 1, def: '[]' }
        ]
    },
    memory: {
        label: '记忆库', icon: 'fa-brain', color: '#7c3aed',
        desc: 'engine.memory.store/search（向量记忆库）；<参数对象> 可来自上游',
        ins: [{ k: 'params', label: '参数对象' }], out: { k: 'result', label: '结果' },
        fields: [
            { key: 'op', label: '操作', type: 'select', options: ['store', 'search'], def: 'store' },
            { key: 'params', label: '参数 (JSON)', type: 'json', rows: 2, def: '{\n  "content": "测试记忆",\n  "tags": ["test"]\n}' }
        ]
    },
    file: {
        label: '文件', icon: 'fa-file-alt', color: '#f59e0b',
        desc: 'engine.file.write/read/delete；<路径/内容> 可来自上游，读出内容经输出传递',
        ins: [{ k: 'path', label: '路径' }, { k: 'data', label: '内容' }], out: { k: 'text', label: '内容' },
        fields: [
            { key: 'op', label: '操作', type: 'select', options: ['write', 'read', 'delete'], def: 'write' },
            { key: 'path', label: '路径', type: 'text', ph: 'probe.txt', def: 'probe.txt' },
            { key: 'data', label: '内容 (write 时)', type: 'textarea', rows: 2, def: 'hello ltp9 probe' }
        ]
    },
    crypto: {
        label: '加密/解密', icon: 'fa-lock', color: '#16a34a',
        desc: 'engine.encoder/decoder；encode 输入<明文+密钥>输出<密文>，decode 输入<密文+密钥>输出<明文>',
        ins: [{ k: 'content', label: '明文/密文' }, { k: 'key', label: '密钥' }], out: { k: 'text', label: '密文/明文' },
        fields: [
            { key: 'op', label: '操作', type: 'select', options: ['encode', 'decode'], def: 'encode' },
            { key: 'key', label: '密钥', type: 'text', def: 'secret-key-123' },
            { key: 'content', label: '明文 (encode) / 密文 (decode)', type: 'textarea', rows: 2, def: '这是一段用于加密的文本' }
        ]
    },
    jwt: {
        label: 'JWT 签名', icon: 'fa-id-badge', color: '#e11d48',
        desc: 'engine.crypto.signJWT（HS256/EdDSA/none）；<负载/KeyID> 可来自上游',
        ins: [{ k: 'claims', label: '负载' }, { k: 'kid', label: 'KeyID' }], out: { k: 'token', label: '令牌' },
        fields: [
            { key: 'claims', label: '负载 claims (JSON)', type: 'json', rows: 2, def: '{\n  "sub": "project",\n  "iat": 1700000000,\n  "exp": 1700000900\n}' },
            { key: 'secret', label: '密钥 / Ed25519 PEM', type: 'textarea', rows: 1, def: 'super-secret' },
            { key: 'alg', label: '算法', type: 'select', options: ['HS256', 'EdDSA', 'none'], def: 'HS256' },
            { key: 'kid', label: 'KeyID (可选)', type: 'text', def: '' }
        ]
    },
    llm: {
        label: 'LLM 对话', icon: 'fa-comment-dots', color: '#22c55e',
        desc: 'engine.llm.chat；单消息框：无法解析为 JSON 视为一条用户消息；能解析且符合 [{role,content}] 消息数组格式视为消息列表；系统提示词自动前置',
        ins: [{ k: 'content', label: '消息内容' }], out: { k: 'answer', label: 'AI 应答' },
        fields: [
            { key: 'content', label: '消息内容（文本=一条用户消息；JSON数组 [{role,content}]=消息列表）', type: 'textarea', rows: 2, def: '用一句话介绍你自己' },
            { key: 'system', label: '系统提示词（可选，自动前置为 system 消息）', type: 'textarea', rows: 1, def: '' },
            { key: 'opts', label: '参数 opts (JSON，如 temperature)', type: 'json', rows: 1, def: '{\n  "temperature": 0.7\n}' }
        ]
    },
    http: {
        label: '同步 HTTP', icon: 'fa-globe', color: '#0891b2',
        desc: 'engine.http.get/post；<URL/请求体/请求头> 可来自上游',
        ins: [{ k: 'url', label: 'URL' }, { k: 'body', label: '请求体' }, { k: 'headers', label: '请求头' }], out: { k: 'resp', label: '响应' },
        fields: [
            { key: 'method', label: '方法', type: 'select', options: ['GET', 'POST'], def: 'GET' },
            { key: 'url', label: 'URL', type: 'text', def: 'https://wttr.in/?format=j1' },
            { key: 'body', label: '请求体 (POST)', type: 'textarea', rows: 1, def: '' },
            { key: 'headers', label: '请求头 (JSON)', type: 'json', rows: 1, def: '{}' }
        ]
    },
    network: {
        label: '网络 TCP/UDP/DNS', icon: 'fa-network-wired', color: '#0891b2',
        desc: 'engine.network：resolveDNS/resolveSRV 即时返回；tcpConnect/udpConnect/udpListen 建立套接字并返回句柄；sockSend/Receive/SendTo/Close 按句柄操作；<句柄/数据> 可来自上游',
        ins: [{ k: 'handle', label: '套接字句柄' }, { k: 'data', label: '发送数据' }], out: { k: 'result', label: '结果' },
        fields: [
            { key: 'op', label: '操作', type: 'select', options: ['resolveDNS', 'resolveSRV', 'tcpConnect', 'udpConnect', 'udpListen', 'sockSend', 'sockReceive', 'sockSendTo', 'sockClose'], def: 'resolveDNS' },
            { key: 'host', label: '主机', type: 'text', def: '127.0.0.1' },
            { key: 'port', label: '端口', type: 'number', def: 80 },
            { key: 'service', label: '服务名 (resolveSRV)', type: 'text', def: '_sip' },
            { key: 'proto', label: '协议 (resolveSRV)', type: 'text', def: '_tcp' },
            { key: 'handle', label: '套接字句柄（connect/listen 输出）', type: 'text', ph: 's1' },
            { key: 'data', label: '发送数据 (sockSend/SendTo)', type: 'textarea', rows: 1, def: 'hello' },
            { key: 'timeout', label: '超时(秒, 读/DNS/连接)', type: 'number', def: 5 }
        ]
    },
    async: {
        label: '异步子任务', icon: 'fa-tasks', color: '#6366f1',
        desc: 'engine.async：run 以指定包导出的函数为任务体后台执行（不填包/函数则耗时模拟）；reportProgress 上报进度；getStatus/list 查询状态；<任务ID/进度> 可连线',
        ins: [{ k: 'taskId', label: '任务ID' }, { k: 'progress', label: '进度' }], out: { k: 'result', label: '结果' },
        fields: [
            { key: 'op', label: '操作', type: 'select', options: ['run', 'reportProgress', 'getStatus', 'list'], def: 'run' },
            { key: 'plugin', label: '任务体包ID（run 可选；下拉=引擎真实加载）', type: 'text', ph: '留空=耗时模拟', dynamic: 'plugins' },
            { key: 'fn', label: '任务体函数名（run 可选；下拉=该包真实导出）', type: 'text', ph: 'queryWeather', dynamic: 'exports' },
            { key: 'args', label: '任务体参数 (JSON数组，可选)', type: 'json', rows: 1, def: '[]' },
            { key: 'ms', label: '耗时(毫秒, 无任务体时的模拟)', type: 'number', def: 1500 },
            { key: 'timeout', label: '超时(毫秒, run 看门狗)', type: 'number', def: 8000 },
            { key: 'taskId', label: '任务ID (report/getStatus；可连线)', type: 'number', def: 0 },
            { key: 'progress', label: '进度 (reportProgress)', type: 'json', rows: 1, def: '50' }
        ]
    },
    transform: {
        label: '格式转换', icon: 'fa-code-merge', color: '#38bdf8',
        desc: '接收多路输入，逐路设置提取字段（留空取全部），按 {{inN.字段}} 占位符模板组合输出；路径支持数组下标（如 result.outcomes[0].result.data.city）；激活需时钟线接入（信号线仅传数据）；模板必填',
        ins: [{ k: 'in1', label: '输入1' }, { k: 'in2', label: '输入2' }, { k: 'in3', label: '输入3' }, { k: 'in4', label: '输入4' }],
        out: { k: 'text', label: '结果' },
        fields: [
            { key: 'e1', label: '输入1 提取字段（留空=取全部）', type: 'text', def: '' },
            { key: 'e2', label: '输入2 提取字段（留空=取全部）', type: 'text', def: '' },
            { key: 'e3', label: '输入3 提取字段（留空=取全部）', type: 'text', def: '' },
            { key: 'e4', label: '输入4 提取字段（留空=取全部）', type: 'text', def: '' },
            { key: 'template', label: '输出模板（必填）：用 {{in1}} / {{in1.字段}} 等占位符，支持数组下标 a.b[0].c', type: 'textarea', rows: 3, def: '{{in1}} | {{in2}}' }
        ]
    },
    display: {
        label: '文本显示', icon: 'fa-eye', color: '#f472b6',
        desc: '接收并显示上游文本内容；卡片仅展示描述标签（说明本节点用途），真实文本可点节点头部「气泡」图标查看；激活需时钟线接入（信号线仅传数据），未连线时显示并输出本节点填写的文本',
        ins: [{ k: 'text', label: '文本' }], out: { k: 'text', label: '文本' },
        fields: [
            { key: 'text', label: '文本内容（未连线时使用）', type: 'textarea', rows: 2, def: '你好，星月智能！' },
            { key: 'hint', label: '卡片描述标签（说明本节点展示什么）', type: 'text', def: '用于展示文本内容' }
        ]
    },
    wait: {
        label: '同步等待', icon: 'fa-hourglass-half', color: '#94a3b8',
        desc: 'engine.sleep 语义：阻塞等待指定毫秒；无数据输入输出',
        ins: [], out: null, fields: [{ key: 'ms', label: '等待毫秒', type: 'number', def: 800 }]
    },
    stats: {
        label: '插件状态', icon: 'fa-cubes', color: '#f43f5e',
        desc: '查询引擎在线 + 已加载插件（含事件/导出/智能体包，供下游提取）', ins: [], out: { k: 'rows', label: '插件列表' }, fields: []
    },
    and: { label: '与 AND', icon: 'fa-gate', color: '#22c55e', gate: true, desc: '所有输入成功才放行', ins: port_ins4(), out: null, fields: [] },
    or: { label: '或 OR', icon: 'fa-gate-open', color: '#a3e635', gate: true, desc: '任一路径成功即放行', ins: port_ins4(), out: null, fields: [] },
    not: { label: '非 NOT', icon: 'fa-gate-xmark', color: '#facc15', gate: true, desc: '输入取反（单输入）', ins: port_ins4(), out: null, fields: [] },
    nand: { label: '与非 NAND', icon: 'fa-gate-xmark', color: '#eab308', gate: true, desc: '非(全部成功)', ins: port_ins4(), out: null, fields: [] },
    nor: { label: '或非 NOR', icon: 'fa-gate-xmark', color: '#ca8a04', gate: true, desc: '非(任一成功)', ins: port_ins4(), out: null, fields: [] },
    tool: {
        label: '调用工具', icon: 'fa-wrench', color: '#0ea5e9',
        desc: 'engine.tool：调用目标插件注册的 LLM/AtoA 工具；目标包+工具名从引擎真实注册动态填充；<参数对象> 可来自上游',
        ins: [{ k: 'params', label: '参数对象' }], out: { k: 'result', label: '结果' },
        fields: [
            { key: 'plugin', label: '目标包（下拉=引擎真实加载）', type: 'text', ph: 'com.yaraflow.weather-ltp9', dynamic: 'plugins' },
            { key: 'tool', label: '工具名（下拉=该包真实注册工具；先选包）', type: 'text', ph: 'get_weather', dynamic: 'tools' },
            { key: 'params', label: '参数对象 (JSON)', type: 'json', rows: 2, def: '{\n  "city": "北京"\n}' }
        ]
    },
    platform: {
        label: '平台上下文', icon: 'fa-cube', color: '#6366f1',
        desc: 'engine.platform：getName / getGroupId / lookupUser（宿主注入解析）；lookupUser 需配置群ID与昵称',
        ins: [], out: { k: 'result', label: '结果' },
        fields: [
            { key: 'method', label: '方法', type: 'select', options: ['getName', 'getGroupId', 'lookupUser'], def: 'getName' },
            { key: 'groupId', label: '群ID (lookupUser)', type: 'text', def: '' },
            { key: 'name', label: '昵称 (lookupUser)', type: 'text', def: '' }
        ]
    },
    emoji: {
        label: '表情包', icon: 'fa-face-grin-hearts', color: '#ec4899',
        desc: 'engine.emoji：search / store / random（复用记忆库 stickers 集合）；search 按语义检索、random 随机返回一张',
        ins: [{ k: 'query', label: '检索文本' }, { k: 'image', label: '图片 (store)' }], out: { k: 'result', label: '结果' },
        fields: [
            { key: 'op', label: '操作', type: 'select', options: ['search', 'store', 'random'], def: 'search' },
            { key: 'query', label: '检索文本 (search/random)', type: 'text', def: '开心' },
            { key: 'limit', label: '最多返回 (search)', type: 'number', def: 5 },
            { key: 'image', label: '图片 base64 (store)', type: 'textarea', rows: 2, def: '' }
        ]
    },
    embed: {
        label: '文本嵌入', icon: 'fa-vector-square', color: '#22c55e',
        desc: 'engine.llm.embed：计算文本向量（agent.embedding 文本嵌入模型）；<输入文本> 可来自上游，多文本传 JSON 字符串数组',
        ins: [{ k: 'input', label: '输入文本' }], out: { k: 'result', label: '向量' },
        fields: [{ key: 'input', label: '输入文本（字符串 或 JSON 字符串数组）', type: 'textarea', rows: 2, def: '星月智能' }]
    },
    kokoro_tts: {
        label: 'Kokoro 语音合成', icon: 'fa-volume-up', color: '#e879f9',
        desc: 'engine：Kokoro 引擎合成语音（内嵌引擎，首次请求懒加载），返回 base64 WAV 音频；voice 音色、speed 语速(0.5~2.0)、lang 语言(zh/en/auto)；<文本>可来自上游',
        ins: [{ k: 'text', label: '文本' }], out: { k: 'audio', label: '音频(base64)', kind: 'audio' },
        fields: [
            { key: 'text', label: '合成文本', type: 'textarea', rows: 2, def: '你好，星月智能！' },
            { key: 'voice', label: '音色（如 zf_001 / zm_031，留空=默认）', type: 'text', def: '' },
            { key: 'speed', label: '语速倍率 (0.5~2.0)', type: 'number', def: 1 },
            { key: 'lang', label: '语言 (zh/en/auto)', type: 'text', def: 'auto' }
        ]
    },
    qwen_tts: {
        label: 'Qwen 语音合成', icon: 'fa-music', color: '#f59e0b',
        desc: 'engine：调用 qwenTTS（经月华 /tts 代理合成）返回 base64 WAV 音频；需月华服务在线；<文本>可来自上游',
        ins: [{ k: 'text', label: '文本' }], out: { k: 'audio', label: '音频(base64)', kind: 'audio' },
        fields: [
            { key: 'text', label: '合成文本', type: 'textarea', rows: 2, def: '你好，星月智能！' },
            { key: 'ref_audio', label: '参考音频路径（相对 local_data）', type: 'text', def: 'local_data/audios/crystal-template.wav' }
        ]
    },
    qwen_asr: {
        label: 'Qwen 语音识别', icon: 'fa-microphone', color: '#22d3ee',
        desc: 'engine：Qwen3-ASR 识别 base64 音频为文本（内嵌引擎，首次请求懒加载；wav 直接识别，其他格式经 ffmpeg 转 16k 单声道）；<音频>可来自上游',
        ins: [{ k: 'audio', label: '音频(base64)' }], out: { k: 'text', label: '识别文本' },
        fields: [
            { key: 'audio', label: '音频 base64（WAV；其他格式需 ffmpeg）', type: 'textarea', rows: 3, def: '' },
            { key: 'format', label: '源音频格式（非 wav 时转码用）', type: 'select', options: ['wav', 'webm', 'ogg', 'mp4', 'm4a', 'weba'], def: 'wav' }
        ]
    },
    microphone: {
        label: '麦克风录音', icon: 'fa-microphone-lines', color: '#10b981',
        desc: '本地录音：运行后请求麦克风权限，录制指定时长并输出 base64 音频（webm/opus），可连线到 Qwen 识别或扬声器节点；需 https/localhost',
        ins: [], out: { k: 'audio', label: '音频(base64)' },
        fields: [{ key: 'duration', label: '录音时长(秒)', type: 'number', def: 3 }]
    },
    speaker: {
        label: '扬声器播放', icon: 'fa-volume-high', color: '#f472b6',
        desc: '本地播放：取上游音频（或本节点填写的 base64）通过扬声器播放；激活需时钟线接入，信号线仅传音频',
        ins: [{ k: 'audio', label: '音频(base64)' }], out: null,
        fields: [{ key: 'audio', label: '音频 base64（未连线时使用）', type: 'textarea', rows: 2, def: '' }]
    },
};
function port_ins4() { return [{ k: 'in1', label: '输入1' }, { k: 'in2', label: '输入2' }, { k: 'in3', label: '输入3' }, { k: 'in4', label: '输入4' }]; }

function metaOf(type) { return NODES[type] || { label: type, icon: 'fa-cube', color: '#6d5ce7', ins: [], out: null, fields: [] }; }
function isGate(type) { return !!GATE_TYPES[type]; }
// 时钟线：连接目标「顺序」输入口，或目标为逻辑门（门的任意输入都参与执行）；决定执行链/门控
// 信号线：连接目标数据输入口，仅用于传入参数，不参与执行顺序判断
function isClockLink(l) {
    if (l.to.port === 'gate') return true;
    const to = nodeById(l.to.node);
    return !!to && isGate(to.type);
}
function clockLinksOf(id) { return state.links.filter(l => l.to.node === id && isClockLink(l)); }
// 前端事件总线：事件启动节点 await；事件触发/时钟启动投递
function awaitEvent(topic, timeout) {
    return new Promise(res => {
        const h = { topic: topic || null, res, _t: null };
        state.evWaiters.push(h);
        h._t = setTimeout(() => { state.evWaiters = state.evWaiters.filter(x => x !== h); res({ __timeout: true }); }, timeout || 8000);
    });
}
function emitEventLocal(topic, payload) {
    const waiters = state.evWaiters; state.evWaiters = [];
    waiters.forEach(h => {
        if (h.topic && topic && h.topic !== topic) { state.evWaiters.push(h); return; }
        clearTimeout(h._t); h.res({ topic: topic || (h.topic || ''), payload });
    });
}
function clearClocks() { state.clocks.forEach(t => clearInterval(t)); state.clocks = []; }
// 解析取值路径：支持 a.b[0].c 或 a.b.0.c 混合写法 → ['a','b','0','c']
function pathKeys(path) {
    return String(path).replace(/\[(\d+)\]/g, '.$1').split('.').filter(Boolean);
}
// 按路径从对象取值；path 为空返回对象本身；数组下标用 [n] 或 .n 均可
function extractField(obj, path) {
    if (obj == null) return undefined;
    if (!path) return obj;
    let v = obj;
    for (const k of pathKeys(path)) { if (v == null) return undefined; v = v[k]; }
    return v;
}
function gateFunc(type, oks) {
    switch (type) {
        case 'and': return oks.every(Boolean);
        case 'nand': return !oks.every(Boolean);
        case 'or': return oks.some(Boolean);
        case 'nor': return !oks.some(Boolean);
        case 'not': return !(oks[0] === true);
        default: return oks.every(Boolean);
    }
}
// 节点端口集合：输入 = [顺序] + 数据ins；输出 = [顺序] + (out?)
function inPortsOf(type) { const d = metaOf(type); return [{ k: 'gate', label: '时钟线' }].concat(d.ins || []); }
function outPortsOf(type) { const d = metaOf(type); return [{ k: 'gate', label: '时钟线' }].concat(d.out ? [d.out] : []); }

// ==== 状态 ====
const state = {
    ws: null, retry: 0, connected: false,
    seq: 0, running: false, abort: false,
    nodes: [], links: [], selectedNodeId: null,
    pending: new Map(),
    evWaiters: [],   // 事件等待者（事件启动）{topic,res,_t}
    clocks: [],      // 时钟定时器句柄（时钟启动）
    filter: 'all', unread: 0, log: [],
    linkDrag: { from: null, x: 0, y: 0 },
    // 引擎真实注册信息（从 ltp9/probe|stats 回执填充，供下拉动态选项）：
    //   plugins       已加载 LTP9 插件ID 数组
    //   events        全部插件已订阅事件主题（去重）
    //   exports       插件ID → 该包导出的函数名数组
    //   tools         插件ID → 该包注册的工具名数组
    //   agentPlugins  Mini-LTP/Node-LTP 前端智能体包ID（engine.agent 目标）
    catalog: { plugins: [], events: [], exports: {}, tools: {}, agentPlugins: [] },
    modalNodeId: null // 当前浮窗编辑的节点（catalog 刷新后重渲染用）
};

// ==== DOM ====
const $ = id => document.getElementById(id);
const modal = $('modal'), modalTop = $('modalTop'), modalBody = $('modalBody');
const canvas = $('canvas'), linksSvg = $('linksSvg'), nodesLayer = $('nodesLayer');

function toast(msg, type) {
    const el = document.createElement('div');
    el.className = 'toast' + (type ? ' ' + type : '');
    el.textContent = msg; $('toastWrap').appendChild(el);
    setTimeout(() => { el.style.opacity = '0'; setTimeout(() => el.remove(), 300); }, 2200);
}
function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }
function newId() { return 'n' + Date.now() + '_' + (++state.seq); }
function seg() { return 'r' + Date.now() + '_' + (++state.seq); }
function nodeById(id) { return state.nodes.find(n => n.id === id); }
function incomingLinks(id) { return state.links.filter(l => l.to.node === id); }
function outgoingLinks(id) { return state.links.filter(l => l.from.node === id); }

function defaultParams(type) {
    const p = {};
    (metaOf(type).fields || []).forEach(f => { if (f.def !== undefined) p[f.key] = f.def; });
    return p;
}

// ==== WebSocket ====
function wsUrl() { const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'; return proto + '//' + window.location.host + '/ws'; }
function connect() {
    try { state.ws = new WebSocket(wsUrl()); } catch (e) { scheduleReconnect(); return; }
    state.ws.onopen = () => { state.connected = true; state.retry = 0; renderWs(); toast('已连接引擎', 'success'); send({ type: 'ltp9/probe' }, '探测'); };
    state.ws.onmessage = ev => handleMessage(ev.data);
    state.ws.onclose = () => { state.connected = false; renderWs(); scheduleReconnect(); };
    state.ws.onerror = () => { try { state.ws.close(); } catch (e) {} };
}
function scheduleReconnect() { if (state.retry < WS_MAX_RETRY) { state.retry++; renderWs(); setTimeout(connect, WS_RETRY_INTERVAL); } else $('wsText').textContent = '连接失败'; }
function renderWs() {
    const dot = $('wsDot'), txt = $('wsText');
    if (state.connected) { dot.className = 'status-dot on'; txt.textContent = '已连接'; }
    else if (state.retry > 0 && state.retry < WS_MAX_RETRY) { dot.className = 'status-dot off'; txt.textContent = `重连中(${state.retry}/${WS_MAX_RETRY})`; }
    else if (state.retry >= WS_MAX_RETRY) { dot.className = 'status-dot off'; txt.textContent = '连接失败'; }
    else { dot.className = 'status-dot'; txt.textContent = '未连接'; }
}

// ==== 信封发送 / 等待 ====
function send(env, label) {
    if (!state.ws || state.ws.readyState !== WebSocket.OPEN) { toast('尚未连接引擎', 'error'); return null; }
    env.request_id = seg();
    const p = new Promise((resolve) => state.pending.set(env.request_id, { resolve }));
    addLog({ dir: 'send', type: env.type, label: label || '', req: env.request_id, env, isError: false });
    try { state.ws.send(JSON.stringify(env)); } catch (e) { toast('发送失败: ' + e.message, 'error'); state.pending.delete(env.request_id); return null; }
    return p;
}
// 等待回执并带超时：仅在真正收到匹配 request_id 的 ack 时才算完成（默认最长 5 分钟，供画图等慢操作）
function sendWait(env, label, ms) {
    const p = send(env, label);
    if (!p) return Promise.resolve(null);
    return new Promise(res => {
        const t = setTimeout(() => { state.pending.delete(env.request_id); res(null); }, ms || 300000);
        p.then(v => { clearTimeout(t); res(v); });
    });
}
function handleMessage(raw) {
    let m; try { m = JSON.parse(raw); } catch (e) { return; }
    if (!m || typeof m.type !== 'string') return;
    const isAck = !!ACK_TYPES[m.type];
    if (m.request_id && state.pending.has(m.request_id)) {
        if (isAck) { const p = state.pending.get(m.request_id); state.pending.delete(m.request_id); p.resolve(m); }
        else { return; } // 自己发送的请求被集线器回显，不是回执，忽略
    }
    if (!m.type.startsWith('ltp9/')) return;
    // 引擎真实注册信息（probe/stats 回执携带 plugins/agent_plugins）：刷新下拉动态选项
    if (Array.isArray(m.plugins) || Array.isArray(m.agent_plugins)) updateCatalog(m.plugins, m.agent_plugins);
    const sm = summarize(m);
    addLog({ dir: isAck ? 'recv' : 'recv', type: m.type, label: '', req: m.request_id || '', env: m, isError: sm.isError, summary: sm.text });
}
// updateCatalog 用引擎回执的插件状态填充动态选项：LTP9包ID / 已订阅事件主题 / 各包导出函数 / Mini-LTP·Node-LTP 智能体包
function updateCatalog(plugins, agentPlugins) {
    const c = { plugins: [], events: [], exports: {}, tools: {} };
    const seen = new Set();
    (plugins || []).forEach(pl => {
        if (!pl || !pl.id) return;
        c.plugins.push(pl.id);
        c.exports[pl.id] = (pl.exports || []).slice().sort();
        c.tools[pl.id] = (pl.tools || []).slice().sort();
        (pl.events || []).forEach(t => seen.add(t));
    });
    c.plugins.sort();
    c.events = [...seen].sort();
    c.agentPlugins = (agentPlugins || []).slice().sort();
    state.catalog = c;
    // 浮窗打开中：重渲染参数表单，让新选项立即生效
    if (state.modalNodeId) { const n = nodeById(state.modalNodeId); if (n) openNodeModal(n); }
}
function summarize(m) {
    let text = m.type.replace('ltp9/', ''); let isError = false;
    switch (m.type) {
        case 'ltp9/result': {
            const r = m.result;
            if (r && r.error) { text = 'LTP9 错误: ' + r.error; isError = true; }
            else if (r && r.value && r.value.ok === false) { text = '调用失败: ' + (r.value.text || safeJson(r.value)); isError = true; }
            else if (r && r.summary) text = `事件回执 subscribed=${r.summary.subscribed}` + (r.summary.intercepted ? ' · intercepted' : '') + (r.summary.canceled ? ' · canceled' : '');
            else text = '结果: ' + safeJson(r);
            break;
        }
        case 'ltp9/test_ack': {
            const a = m.action || '';
            if (m.ok === false) { text = `测试[${a}] 失败: ${m.error || ''}`; isError = true; }
            else text = `测试[${a}]: ` + safeJson(m.value);
            break;
        }
        case 'ltp9/stats_ack': case 'ltp9/pong':
            text = '引擎在线 · 插件 ' + (m.plugins && m.plugins.length ? m.plugins.length : '?') + ' 个'; break;
        case 'ltp9/broadcast_ack': text = '广播已发送'; break;
        default: text = safeJson(m);
    }
    return { text, isError };
}
function safeJson(v) { try { const s = JSON.stringify(v); return s === undefined ? String(v) : (s.length > 220 ? s.slice(0, 220) + '…' : s); } catch (e) { return String(v); } }
function escHtml(s) { return String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])); }
function extractOut(ack) {
    if (!ack) return null;
    if (ack.value !== undefined) return ack.value;
    if (ack.result !== undefined) return ack.result;
    return ack;
}

// ==== 回显日志 + 日志侧边栏 ====
function addLog(entry) { entry.time = new Date(); state.log.unshift(entry); if (state.log.length > 400) state.log.pop(); state.unread++; updateFab(); renderLog(); }
function updateFab() { const fab = $('logFab'), cnt = $('logCount'); if (state.unread > 0) fab.classList.add('glow'); else fab.classList.remove('glow'); cnt.textContent = state.unread > 99 ? '99+' : String(state.unread); }
function closeLogModal() { $('logSidebar').classList.remove('open'); }
function toggleLogSidebar() { const s = $('logSidebar'); s.classList.toggle('open'); if (s.classList.contains('open')) { state.unread = 0; updateFab(); renderLog(); } }

function renderLog() {
    const list = $('logList'); list.innerHTML = '';
    const items = state.log.filter(en => {
        if (state.filter === 'all') return true;
        if (state.filter === 'send') return en.dir === 'send';
        if (state.filter === 'recv') return en.dir === 'recv';
        if (state.filter === 'error') return en.isError;
        return true;
    });
    if (!items.length) { list.innerHTML = '<div class="empty-tip">暂无日志</div>'; return; }
    items.forEach(en => {
        const el = document.createElement('div'); el.className = 'log-entry' + (en.isError ? ' error' : '');
        const head = document.createElement('div'); head.className = 'log-head';
        const badge = document.createElement('span'); badge.className = 'log-type ' + (en.isError ? 't-error' : (en.dir === 'send' ? 't-send' : 't-response'));
        badge.textContent = en.type; head.appendChild(badge);
        const time = document.createElement('span'); time.className = 'log-time';
        const d = en.time; time.textContent = `${String(d.getHours()).padStart(2,'0')}:${String(d.getMinutes()).padStart(2,'0')}:${String(d.getSeconds()).padStart(2,'0')}`; head.appendChild(time);
        const tag = document.createElement('span'); tag.className = 'log-tag'; tag.textContent = en.dir === 'send' ? '发送' : '回执'; head.appendChild(tag);
        el.appendChild(head);
        const sum = document.createElement('div'); sum.className = 'log-summary'; sum.textContent = en.summary || (en.label ? en.label + ' · ' + en.type : en.type); el.appendChild(sum);
        const hint = document.createElement('div'); hint.className = 'log-json-hint'; hint.innerHTML = '<i class="fas fa-expand"></i> 查看 JSON'; el.appendChild(hint);
        el.addEventListener('click', () => openDetail('日志详情', en.env || en));
        list.appendChild(el);
    });
}

// ==== 节点渲染（含输入/输出端口） ====
// 节点卡片动作按钮辅助：消息气泡 / 图片显示 / 音频播放 / 音频输入。
// 依 magic bytes 识别二进制 base64 的图片与音频，既可据输出实时检测，也支持按节点 out.kind 预置按钮。
function sniffMime(bin) {
    const b = i => bin.charCodeAt(i);
    if (bin.length > 12 && b(0) === 0x52 && bin.slice(8, 12) === 'WAVE') return 'audio/wav';
    if (bin.slice(0, 4) === 'fLaC') return 'audio/flac';
    if (bin.slice(0, 4) === 'OggS') return 'audio/ogg';
    if (bin.slice(0, 4) === 'ID3' || (b(0) === 0xff && (b(1) & 0xe0) === 0xe0)) return 'audio/mpeg';
    if (bin.slice(0, 4) === '\x1aE\xdf\xa3') return 'audio/webm';
    if (b(0) === 0x89 && bin.slice(1, 4) === 'PNG') return 'image/png';
    if (b(0) === 0xff && b(1) === 0xd8 && b(2) === 0xff) return 'image/jpeg';
    if (bin.slice(0, 6) === 'GIF87a' || bin.slice(0, 6) === 'GIF89a') return 'image/gif';
    if (bin.slice(0, 4) === 'RIFF' && bin.slice(8, 12) === 'WEBP') return 'image/webp';
    return null;
}
// 递归扫描输出值，提取 { image, audio }（data URI 形式）
function findMedia(v, found) {
    found = found || {};
    if (!v) return found;
    if (typeof v === 'string') {
        const t = v.trim();
        if (/^data:image\//i.test(t)) { if (!found.image) found.image = t; return found; }
        if (/^data:audio\//i.test(t)) { if (!found.audio) found.audio = t; return found; }
        if (t.length > 60 && t.length % 4 === 0 && /^[A-Za-z0-9+/=\s]+$/.test(t)) {
            try {
                const bin = atob(t);
                const mime = sniffMime(bin);
                if (mime && !found[mime.startsWith('image') ? 'image' : 'audio']) found[mime.startsWith('image') ? 'image' : 'audio'] = 'data:' + mime + ';base64,' + t;
            } catch (e) { /* 非合法 base64，忽略 */ }
        }
        return found;
    }
    if (Array.isArray(v) || (typeof v === 'object')) Object.values(v).forEach(i => findMedia(i, found));
    return found;
}
// 节点应提供的媒体按钮：按 out.kind 预置（即便未运行也显示），并兜底实时检测输出里的媒体
function mediaButtonsFor(node) {
    const meta = metaOf(node.type);
    const md = node.outValue ? findMedia(node.outValue) : null;
    const wantImg = meta.out && meta.out.kind === 'image';
    const wantAud = meta.out && meta.out.kind === 'audio';
    const b = [];
    if (wantImg || (md && md.image)) b.push({ kind: 'image', dataUri: (md && md.image) || null });
    if (wantAud || (md && md.audio)) b.push({ kind: 'audio', dataUri: (md && md.audio) || null });
    return b;
}
function nodeEl(node) { return nodesLayer.querySelector('.node[data-id="' + node.id + '"]'); }
function closeNodePop(node) { const el = nodeEl(node); if (el) el.querySelectorAll('.node-pop').forEach(p => p.remove()); }
// 消息气泡/图片展示：点击图标在节点上方弹出，再点或点别处关闭
function toggleNodePop(node, html) {
    const el = nodeEl(node); if (!el) return;
    closeNodePop(node);
    const pop = document.createElement('div'); pop.className = 'node-pop';
    if (typeof html === 'string' && /^</.test(html)) pop.innerHTML = html; else pop.textContent = String(html);
    el.appendChild(pop);
    const off = e => { if (e.target.closest && e.target.closest('.node-pop')) return; document.removeEventListener('click', off, true); closeNodePop(node); };
    document.addEventListener('click', off, true);
}
// 「文本显示」节点：气泡展示当前文本/输出内容
function showNodeBubble(node) {
    const v = node.outValue !== undefined && node.outValue !== null ? node.outValue : node.params.text;
    const txt = typeof v === 'string' ? v : (v == null ? '' : JSON.stringify(v, null, 2));
    toggleNodePop(node, `<pre class="pop-text">${escHtml(txt) || '（空）'}</pre>`);
}
// 「图片显示」按钮：气泡展示节点输出里的图片
function showNodeImage(node, dataUri) {
    let d = dataUri;
    if (!d) { const md = node.outValue ? findMedia(node.outValue) : null; if (md && md.image) d = md.image; }
    if (!d) { toast('无图片输出，请先运行此节点', 'error'); return; }
    toggleNodePop(node, `<img class="pop-img" src="${d}">`);
}
// 「播放音频」按钮：播放节点输出里的音频（base64 → data URI）
function playNodeAudio(node, dataUri) {
    let d = dataUri;
    if (!d) { const md = node.outValue ? findMedia(node.outValue) : null; if (md && md.audio) d = md.audio; }
    if (!d) { toast('无音频输出，请先运行此节点', 'error'); return; }
    const a = new Audio(d);
    a.play().catch(() => toast('音频播放失败', 'error'));
}
// 「音频输入」按钮（qwen_asr）：选本地音频文件 → base64 写入本节点音频参数
function pickAudioInput(node) {
    const inp = document.createElement('input');
    inp.type = 'file'; inp.accept = 'audio/*,.wav,.mp3,.webm,.ogg,.m4a'; inp.style.display = 'none';
    inp.addEventListener('change', () => {
        const f = inp.files && inp.files[0]; if (!f) return;
        const fr = new FileReader();
        fr.onload = () => { node.params.audio = String(fr.result).split(',')[1]; toast(`已载入音频（base64 ${(node.params.audio.length / 1024).toFixed(0)}KB）`, 'success'); renderNodes(); };
        fr.onerror = () => toast('读取音频失败', 'error');
        fr.readAsDataURL(f);
    });
    document.body.appendChild(inp); inp.click(); inp.remove();
}
// 从任意输出值里取音频 data URI（递归 detect 图片/音频）
function audioDataUriOf(v) { const md = findMedia(v); return md ? md.audio : null; }
// 裸 base64 → data URI（按 magic bytes 推断 MIME；非法返回 null）
function b64ToDataUri(b64) {
    if (typeof b64 !== 'string' || !b64.trim()) return null;
    const t = b64.trim();
    try { const m = sniffMime(atob(t)); return m ? 'data:' + m + ';base64,' + t : null; } catch (e) { return null; }
}
// 麦克风录音：MediaRecorder 录制 duration 秒，返回 base64 音频（webm/opus）；失败/未授权返回 null
function recordAudio(durationSec) {
    return new Promise(resolve => {
        if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia || typeof MediaRecorder === 'undefined') { resolve(null); return; }
        navigator.mediaDevices.getUserMedia({ audio: true }).then(stream => {
            let rec = null;
            try {
                if (MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported('audio/webm;codecs=opus')) rec = new MediaRecorder(stream, { mimeType: 'audio/webm;codecs=opus' });
                else rec = new MediaRecorder(stream);
            } catch (e) { try { rec = new MediaRecorder(stream); } catch (e2) { stream.getTracks().forEach(t => t.stop()); resolve(null); return; } }
            const chunks = [];
            rec.ondataavailable = e => { if (e.data && e.data.size) chunks.push(e.data); };
            rec.onstop = () => {
                stream.getTracks().forEach(t => t.stop());
                const blob = new Blob(chunks, { type: rec.mimeType || 'audio/webm' });
                const fr = new FileReader();
                fr.onload = () => resolve(String(fr.result).split(',')[1]);
                fr.onerror = () => resolve(null);
                fr.readAsDataURL(blob);
            };
            rec.onerror = () => { stream.getTracks().forEach(t => t.stop()); resolve(null); };
            rec.start();
            setTimeout(() => { if (rec.state !== 'inactive') rec.stop(); }, durationSec * 1000);
        }).catch(() => resolve(null));
    });
}
// 生成节点头部动作按钮（stopPropagation，避免误触发编辑/拖动）
function makeActionBtn(icon, title, fn) {
    const s = document.createElement('span'); s.className = 'node-act'; s.innerHTML = `<i class="fas ${icon}"></i>`; s.title = title;
    s.addEventListener('mousedown', e => e.stopPropagation());
    s.addEventListener('click', e => { e.stopPropagation(); fn(); });
    return s;
}

function renderGraph() { renderNodes(); renderLinks(); $('canvasHint').style.display = state.nodes.length ? 'none' : 'block'; }
function renderNodes() {
    nodesLayer.innerHTML = '';
    state.nodes.forEach(node => {
        const meta = metaOf(node.type);
        const ins = inPortsOf(node.type), outs = outPortsOf(node.type);
        const el = document.createElement('div');
        el.className = 'node' + statusClass(node, 'el');
        el.style.left = node.x + 'px'; el.style.top = node.y + 'px'; el.style.width = NODE_W + 'px'; el.style.height = '100px';
        el.style.setProperty('--nc', meta.color);
        el.dataset.id = node.id;
        el.dataset.run = node._s || 'pending';
        const head = document.createElement('div'); head.className = 'node-head';
        head.innerHTML = `<i class="fas ${meta.icon}"></i><span class="node-label">${meta.label}</span>`;
        // 节点动作按钮：消息气泡（文本显示）· 音频输入（识别音频）· 图片显示 · 音频播放
        const acts = document.createElement('div'); acts.className = 'node-actions';
        if (node.type === 'display') acts.appendChild(makeActionBtn('fa-comment-dots', '查看文本内容（消息气泡）', () => showNodeBubble(node)));
        if (node.type === 'qwen_asr') acts.appendChild(makeActionBtn('fa-file-audio', '载入音频 → base64 注入本节点音频参数', () => pickAudioInput(node)));
        mediaButtonsFor(node).forEach(b => {
            if (b.kind === 'audio') acts.appendChild(makeActionBtn('fa-play', '播放本节点输出的音频', () => playNodeAudio(node, b.dataUri)));
            else acts.appendChild(makeActionBtn('fa-image', '显示本节点输出的图片', () => showNodeImage(node, b.dataUri)));
        });
        head.appendChild(acts);
        const copy = document.createElement('span'); copy.className = 'node-copy'; copy.innerHTML = '<i class="fas fa-copy"></i>'; copy.title = '复制节点（含参数与连线）';
        copy.addEventListener('click', e => { e.stopPropagation(); duplicateNode(node.id); });
        head.appendChild(copy);
        const close = document.createElement('span'); close.className = 'node-x'; close.innerHTML = '&times;'; close.title = '删除';
        close.addEventListener('click', e => { e.stopPropagation(); removeNode(node.id); });
        head.appendChild(close); head.addEventListener('mousedown', e => startDragNode(e, node));
        el.appendChild(head);
        const body = document.createElement('div'); body.className = 'node-body';
        body.textContent = statusText(node); el.appendChild(body);
        // 文本显示节点：卡片仅显示描述标签（不受链路控制，用于说明本节点展示什么），真实文本经消息气泡图标查看
        if (node.type === 'display') {
            const cap = document.createElement('div'); cap.className = 'pv pv-display';
            cap.textContent = String(node.params.hint != null && node.params.hint !== '' ? node.params.hint : '用于展示文本内容');
            body.appendChild(cap);
        }
        // 输入端口（顶部横排），输出端口（底部横排）
        ins.forEach((p, i) => el.appendChild(makePort(el, node.id, 'in', p, i, ins.length)));
        outs.forEach((p, i) => el.appendChild(makePort(el, node.id, 'out', p, i, outs.length)));
        el.addEventListener('click', () => { if (!el.getAttribute('data-dragging')) openNodeModal(node); });
        nodesLayer.appendChild(el);
    });
}
function statusClass(node, kind) {
    const s = node._s || 'pending';
    if (node.running) return ' running';
    if (s === 'ok') return ' done';
    if (s === 'err') return ' done error';
    if (s === 'skip') return ' skip';
    if (s === 'idle') return ' idle';
    return '';
}
function statusText(node) {
    const s = node._s || 'pending';
    if (node.running) return '运行中…';
    if (s === 'ok') return '成功';
    if (s === 'err') return '错误';
    if (s === 'skip') return '跳过';
    if (s === 'idle') return '就绪';
    return '待运行';
}
function makePort(el, nodeId, side, p, i, n) {
    const pt = document.createElement('div');
    pt.className = 'port port-' + (side === 'in' ? 'in' : 'out');
    pt.dataset.node = nodeId; pt.dataset.side = side; pt.dataset.port = p.k;
    pt.title = (side === 'in' ? '输入' : '输出') + '：' + p.label;
    // 上下端口：横向均分
    pt.style.left = ((i + 1) * NODE_W / (n + 1) - 6) + 'px';
    if (side === 'out') pt.addEventListener('mousedown', e => startLinkDrag(e, nodeId, p.k));
    else pt.addEventListener('mouseup', e => tryDropLink(e, nodeId, p.k));
    return pt;
}
function portPosOf(portEl) {
    const r = portEl.getBoundingClientRect(), b = nodesLayer.getBoundingClientRect();
    return { x: r.left - b.left + r.width / 2, y: r.top - b.top + r.height / 2 };
}
function renderLinks() {
    linksSvg.innerHTML = '';
    let maxX = 600, maxY = 300;
    state.links.forEach(link => {
        const aEl = nodesLayer.querySelector('.port-out[data-node="' + link.from.node + '"][data-port="' + link.from.port + '"]');
        const bEl = nodesLayer.querySelector('.port-in[data-node="' + link.to.node + '"][data-port="' + link.to.port + '"]');
        if (!aEl || !bEl) return;
        const a = portPosOf(aEl), b = portPosOf(bEl);
        const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        path.setAttribute('d', linkPath(a, b));
        path.setAttribute('class', 'link ' + (isClockLink(link) ? 'clock' : 'signal') + (link.running ? ' running' : ''));
        path.dataset.id = link.id;
        path.addEventListener('dblclick', e => { e.stopPropagation(); removeLink(link.id); });
        linksSvg.appendChild(path);
        maxX = Math.max(maxX, a.x + 40, b.x + 40); maxY = Math.max(maxY, a.y + 40, b.y + 40);
    });
    if (state.linkDrag.from) {
        const src = state.linkDrag.from;
        const aEl = nodesLayer.querySelector('.port-out[data-node="' + src.node + '"][data-port="' + src.port + '"]');
        let a = { x: state.linkDrag.x, y: state.linkDrag.y };
        if (aEl) a = portPosOf(aEl);
        const tmp = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        tmp.setAttribute('d', linkPath(a, { x: state.linkDrag.x, y: state.linkDrag.y }));
        tmp.setAttribute('class', 'link temp'); linksSvg.appendChild(tmp);
        maxX = Math.max(maxX, state.linkDrag.x + 40); maxY = Math.max(maxY, state.linkDrag.y + 40);
    }
    linksSvg.setAttribute('viewBox', '0 0 ' + maxX + ' ' + maxY);
    linksSvg.style.width = maxX + 'px'; linksSvg.style.height = maxY + 'px';
}
// 方向感知的贝塞尔连线：以主要位移方向决定控制点。
// 左右布局（水平位移为主）→ 横向贝塞尔；上下布局（垂直位移为主）→ 纵向贝塞尔，
// 避免上下排列的节点间因固定横向控制点而画出 S 形。
function linkPath(a, b) {
    const dx = b.x - a.x, dy = b.y - a.y;
    if (Math.abs(dy) > Math.abs(dx)) {
        const c = Math.max(24, Math.abs(dy) * 0.5), sy = Math.sign(dy) || 1;
        return `M ${a.x} ${a.y} C ${a.x} ${a.y + sy * c}, ${b.x} ${b.y - sy * c}, ${b.x} ${b.y}`;
    }
    const c = Math.max(24, Math.abs(dx) * 0.5), sx = Math.sign(dx) || 1;
    return `M ${a.x} ${a.y} C ${a.x + sx * c} ${a.y}, ${b.x - sx * c} ${b.y}, ${b.x} ${b.y}`;
}

// ==== 节点/连线 增删查 ====
function addNode(type, x, y) { const n = { id: newId(), type, x, y, params: defaultParams(type), running: false, _s: 'pending', outValue: null }; state.nodes.push(n); renderGraph(); return n; }
function removeNode(id) {
    state.nodes = state.nodes.filter(n => n.id !== id);
    state.links = state.links.filter(l => l.from.node !== id && l.to.node !== id);
    renderGraph();
}
// 复制节点：参数深拷贝 + 向右下偏移放置，并把原节点进/出连线一并复制到副本
function duplicateNode(id) {
    const n = nodeById(id); if (!n) return null;
    const copy = { id: newId(), type: n.type, x: n.x + 34, y: n.y + 34, params: JSON.parse(JSON.stringify(n.params || {})), running: false, _s: 'pending', outValue: null, enabled: n.enabled };
    state.nodes.push(copy);
    state.links.slice().forEach(l => {
        if (l.from.node === id) state.links.push({ id: 'l' + seg(), from: { node: copy.id, port: l.from.port }, to: { node: l.to.node, port: l.to.port } });
        else if (l.to.node === id) state.links.push({ id: 'l' + seg(), from: { node: l.from.node, port: l.from.port }, to: { node: copy.id, port: l.to.port } });
    });
    renderGraph();
    toast('已复制节点', 'success');
    return copy;
}
function removeLink(id) { state.links = state.links.filter(l => l.id !== id); renderLinks(); }
function ensureLink(from, to) {
    if (!from || !to || from.node === to.node) return;
    if (state.links.some(l => l.from.node === from.node && l.from.port === from.port && l.to.node === to.node && l.to.port === to.port)) return;
    state.links.push({ id: 'l' + seg(), from, to }); renderLinks();
}

// ==== 拖动 ====
function startDragNode(e, node) {
    if (e.target.closest('.port') || e.target.closest('.node-x')) return;
    e.preventDefault();
    const startX = e.clientX, startY = e.clientY, ox = node.x, oy = node.y;
    const move = ev => { node.x = Math.max(10, ox + (ev.clientX - startX)); node.y = Math.max(10, oy + (ev.clientY - startY)); renderNodes(); renderLinks(); };
    const up = () => {
        window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up);
        // 松开鼠标时把节点位置吸附到 5 的倍数（网格对齐）
        node.x = Math.max(10, Math.round(node.x / 5) * 5);
        node.y = Math.max(10, Math.round(node.y / 5) * 5);
        renderNodes(); renderLinks();
    };
    window.addEventListener('mousemove', move); window.addEventListener('mouseup', up);
}
function startLinkDrag(e, nodeId, port) {
    e.preventDefault(); e.stopPropagation();
    const aEl = e.currentTarget, a = portPosOf(aEl);
    state.linkDrag = { from: { node: nodeId, port }, x: a.x, y: a.y };
    renderLinks();
    const move = ev => { const r = canvas.getBoundingClientRect(); state.linkDrag.x = ev.clientX - r.left + canvas.scrollLeft; state.linkDrag.y = ev.clientY - r.top + canvas.scrollTop; renderLinks(); };
    const up = ev => {
        window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up);
        const inEl = document.elementFromPoint(ev.clientX, ev.clientY);
        const p = inEl && (inEl.closest ? inEl.closest('.port-in') : null);
        if (p) { const to = { node: p.dataset.node, port: p.dataset.port }; ensureLink(state.linkDrag.from, to); }
        state.linkDrag = { from: null, x: 0, y: 0 }; renderLinks();
    };
    window.addEventListener('mousemove', move); window.addEventListener('mouseup', up);
}
function tryDropLink(e, nodeId, port) { /* 输入端口也响应鼠标释放做兜底命中 */ e.preventDefault(); }

// ==== 参数编辑（浮窗面板） ====
function clearNodePanel() { nodesLayer.querySelectorAll('.node.panel-active').forEach(el => el.classList.remove('panel-active')); }
function openNodeModal(node) {
    const meta = metaOf(node.type);
    clearNodePanel();
    modalTop.innerHTML = '';
    const t = document.createElement('span'); t.className = 'modal-title';
    t.innerHTML = '<i class="fas ' + meta.icon + '" style="color:' + meta.color + '"></i> ' + meta.label + ' · 参数'; t.title = meta.desc;
    modalTop.appendChild(t);
    const dup = document.createElement('button'); dup.className = 'btn-ghost btn-mini'; dup.innerHTML = '<i class="fas fa-copy"></i> 复制';
    dup.addEventListener('click', () => { const c = duplicateNode(node.id); closeModal(); if (c) openNodeModal(c); });
    modalTop.appendChild(dup);
    const run = document.createElement('button'); run.className = 'btn-primary btn-mini'; run.innerHTML = '<i class="fas fa-play"></i> 仅运行';
    run.addEventListener('click', () => { closeModal(); runSingle(node.id); });
    modalTop.appendChild(run); modalTop.appendChild(closeBtn());
    modalBody.innerHTML = '';
    const wrap = document.createElement('div'); wrap.className = 'form';
    if (meta.desc) { const d = document.createElement('div'); d.className = 'field-hint'; d.textContent = meta.desc; wrap.appendChild(d); }
    const hint = document.createElement('div'); hint.className = 'field-hint';
    hint.innerHTML = '端口：上方=输入、下方=输出；<时钟线>决定执行链，其余为数据信号线。拖动标题栏可移动本面板。';
    wrap.appendChild(hint);
    (meta.fields || []).forEach(f => wrap.appendChild(buildField(f, node.params)));
    modalBody.appendChild(wrap);
    modal.classList.add('open');
    state.modalNodeId = node.id;
    // 切换「目标包」后重渲染表单：让「函数名」下拉跟随所选包的真实导出变化
    wrap.querySelectorAll('input[data-dynamic="plugins"]').forEach(inp => {
        inp.addEventListener('change', () => {
            const n = nodeById(state.modalNodeId); if (n) openNodeModal(n);
        });
    });
    const card = nodesLayer.querySelector('.node[data-id="' + node.id + '"]');
    if (card) card.classList.add('panel-active'); // 被设置节点闪烁
}
// 字段的可选值：dynamic 选项从引擎真实注册信息（state.catalog）取，静态 options 兜底
function fieldOptions(f, p) {
    if (f.dynamic === 'plugins') return state.catalog.plugins;
    if (f.dynamic === 'agentPlugins') return state.catalog.agentPlugins;
    if (f.dynamic === 'events') return state.catalog.events;
    if (f.dynamic === 'exports') return (state.catalog.exports[(p && p.plugin) || ''] || []);
    if (f.dynamic === 'tools') return (state.catalog.tools[(p && p.plugin) || ''] || []);
    return f.options || [];
}
function buildField(f, p) {
    const box = document.createElement('div'); box.className = 'field';
    const lab = document.createElement('label'); lab.className = 'field-label'; lab.textContent = f.label; box.appendChild(lab);
    const setter = v => { p[f.key] = v; };
    let ctl;
    switch (f.type) {
        case 'select': {
            const sel = document.createElement('select'); sel.className = 'sel';
            fieldOptions(f, p).forEach(o => { const op = document.createElement('option'); op.value = o; op.textContent = o; if (String(p[f.key]) === o) op.selected = true; sel.appendChild(op); });
            sel.addEventListener('change', () => setter(sel.value)); ctl = sel; break;
        }
        case 'number': {
            ctl = document.createElement('input'); ctl.type = 'number'; ctl.className = 'inp'; ctl.value = p[f.key] != null ? p[f.key] : (f.def != null ? f.def : '');
            ctl.addEventListener('input', () => setter(Number(ctl.value))); break;
        }
        case 'json': case 'textarea': {
            const ta = document.createElement('textarea'); ta.className = 'ta' + (f.type === 'json' ? ' json' : ''); ta.rows = f.rows || 2; ta.spellcheck = false;
            ta.value = p[f.key] != null && typeof p[f.key] === 'string' ? p[f.key] : (p[f.key] != null ? JSON.stringify(p[f.key]) : (f.def != null ? f.def : ''));
            // 按换行符数量决定高度：每多一个换行就多加一行高度（上限 320px，超出滚动）
            const lineH = 18, pad = 16;
            const autosize = () => {
                const lines = Math.max(1, String(ta.value).split('\n').length);
                ta.style.height = Math.min(lines * lineH + pad, 320) + 'px';
            };
            ta.addEventListener('input', () => { setter(ta.value); autosize(); });
            autosize();
            ctl = ta; break;
        }
        default: {
            const inp = document.createElement('input'); inp.type = 'text'; inp.className = 'inp';
            inp.value = p[f.key] != null && typeof p[f.key] === 'string' ? p[f.key] : (p[f.key] != null ? JSON.stringify(p[f.key]) : (f.def != null ? f.def : ''));
            inp.placeholder = f.ph || '';
            const opts = fieldOptions(f, p);
            if (opts.length) { const dlBase = document.createElement('datalist'); dlBase.id = 'dl_' + (++state.seq); opts.forEach(o => { const op = document.createElement('option'); op.value = o; dlBase.appendChild(op); }); inp.setAttribute('list', dlBase.id); document.body.appendChild(dlBase); }
            if (f.dynamic) inp.dataset.dynamic = f.dynamic; // 供「切换包后重渲染」定位
            inp.addEventListener('input', () => setter(inp.value)); ctl = inp; break;
        }
    }
    box.appendChild(ctl); return box;
}
function closeBtn() { const b = document.createElement('button'); b.className = 'modal-close'; b.innerHTML = '&times;'; b.title = '关闭'; b.addEventListener('click', closeModal); return b; }
function closeModal() { modal.classList.remove('open'); clearNodePanel(); modalTop.innerHTML = ''; modalBody.innerHTML = ''; state.modalNodeId = null; }
function initDragParamPop() {
    const el = modal, head = modalTop; if (!head) return;
    head.addEventListener('mousedown', ev => {
        if (ev.target.closest('button')) return;
        const r = el.getBoundingClientRect(), dx = ev.clientX - r.left, dy = ev.clientY - r.top;
        const move = e2 => { el.style.left = (e2.clientX - dx) + 'px'; el.style.top = (e2.clientY - dy) + 'px'; el.style.right = 'auto'; };
        const up = () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up); };
        window.addEventListener('mousemove', move); window.addEventListener('mouseup', up);
    });
}
function openDetail(title, payload) {
    clearNodePanel();
    modalTop.innerHTML = '';
    const t = document.createElement('span'); t.className = 'modal-title'; t.textContent = title;
    modalTop.appendChild(t); modalTop.appendChild(closeBtn());
    modalBody.innerHTML = '';
    const pre = document.createElement('pre'); pre.className = 'modal-json'; pre.textContent = JSON.stringify(payload, null, 2);
    modalBody.appendChild(pre); modal.classList.add('open');
}

// ==== 信封构建 ====
function gp(p, key) { return p[key]; }
function parseVal(v, arr) {
    if (typeof v !== 'string') return v; // 已是对象/数组
    const t = v.trim(); if (!t) return arr ? [] : {};
    try { return JSON.parse(t); } catch (e) { return v; }
}
function buildEnvelope(node) {
    const meta = metaOf(node.type), p = node.params || {};
    const s = (k) => { const v = gp(p, k); return v == null ? '' : (typeof v === 'string' ? v : JSON.stringify(v)); };
    switch (node.type) {
        case 'start': case 'event_start': case 'clock_start': case 'wait': case 'transform': case 'display': case 'and': case 'or': case 'not': case 'nand': case 'nor':
            return null;
        case 'event': {
            if (!(p.topic || '').trim()) { toast('请填写事件主题', 'error'); return null; }
            return { type: 'ltp9/event', topic: p.topic.trim(), payload: parseVal(gp(p, 'payload')) };
        }
        case 'broadcast_all':
            if (!gp(p, 'payload')) { toast('请填写广播载荷', 'error'); return null; }
            return { type: 'ltp9/broadcast', payload: parseVal(gp(p, 'payload')) };
        case 'broadcast_target': {
            if (!(p.target || '').trim()) { toast('请填写目标包ID', 'error'); return null; }
            return { type: 'ltp9/test', action: 'broadcast_target', target: p.target.trim(), payload: parseVal(gp(p, 'payload')) };
        }
        case 'call': {
            if (!(p.plugin || '').trim() || !(p.fn || '').trim()) { toast('请填写目标包与函数名', 'error'); return null; }
            let args = [];
            // 输入数据连线注入优先：作为首个参数
            const linked = gp(p, 'input');
            if (linked !== undefined && linked !== null && linked !== '') {
                args = [linked];
            } else {
                const raw = String(gp(p, 'args') != null ? gp(p, 'args') : '').trim();
                if (raw) {
                    try { const j = JSON.parse(raw); args = Array.isArray(j) ? j : [j]; }
                    catch (e) { args = [raw]; } // 单个文本 → 首个参数
                }
            }
            return { type: 'ltp9/call', plugin: p.plugin.trim(), fn: p.fn.trim(), args };
        }
        case 'agent': {
            if (!(p.plugin || '').trim() || !gp(p, 'instruction')) { toast('请填写智能体包ID与指令', 'error'); return null; }
            return { type: 'ltp9/test', action: 'agent', plugin: p.plugin.trim(), instruction: String(gp(p, 'instruction')) };
        }
        case 'db': {
            let params = parseVal(gp(p, 'params'), true); if (!Array.isArray(params)) params = [];
            return { type: 'ltp9/test', action: 'db', op: p.op || 'query', sql: s('sql'), params };
        }
        case 'memory':
            return { type: 'ltp9/test', action: 'memory', op: p.op || 'store', params: parseVal(gp(p, 'params')) };
        case 'file':
            return { type: 'ltp9/test', action: 'file', op: p.op || 'write', path: s('path'), data: s('data') };
        case 'crypto':
            return { type: 'ltp9/test', action: 'crypto', op: p.op || 'encode', key: s('key'), content: s('content') };
        case 'jwt':
            return { type: 'ltp9/test', action: 'jwt', claims: parseVal(gp(p, 'claims')) || {}, secret: s('secret'), alg: p.alg || 'HS256', kid: s('kid') };
        case 'llm': {
            const opts = parseVal(gp(p, 'opts')) || {};
            const raw = String(gp(p, 'content') != null ? gp(p, 'content') : '').trim();
            if (!raw) { toast('请填写消息内容', 'error'); return null; }
            // 单消息框：可解析且符合 [{role,content}] 消息数组格式 → 视为消息列表；否则视为一条用户消息
            let messages = null;
            try {
                const j = JSON.parse(raw);
                if (Array.isArray(j) && j.length && j.every(mm => mm && typeof mm.role === 'string')) messages = j;
            } catch (e) { /* 非 JSON → 单条用户消息 */ }
            if (!messages) messages = [{ role: 'user', content: raw }];
            if ((p.system || '').trim()) opts.system = p.system.trim();
            return { type: 'ltp9/test', action: 'llm', messages, opts };
        }
        case 'command': {
            if (!(p.text || '').trim()) { toast('请填写指令文本', 'error'); return null; }
            const env = { type: 'ltp9/test', action: 'command', text: p.text.trim(), context: parseVal(gp(p, 'context')) || {} };
            if ((p.plugin || '').trim()) env.plugin = p.plugin.trim();
            return env;
        }
        case 'tool': {
            if (!(p.plugin || '').trim() || !(p.tool || '').trim()) { toast('请填写目标包与工具名', 'error'); return null; }
            return { type: 'ltp9/tool', plugin: p.plugin.trim(), tool: p.tool.trim(), params: parseVal(gp(p, 'params')) || {} };
        }
        case 'platform': {
            const method = p.method || 'getName';
            const args = { groupId: s('groupId'), name: s('name') };
            return { type: 'ltp9/test', action: 'platform', method, params: args };
        }
        case 'emoji': {
            const op = p.op || 'search';
            const params = { query: s('query'), limit: Number(p.limit) || 5, image: s('image') };
            return { type: 'ltp9/test', action: 'emoji', op, params };
        }
        case 'embed': {
            if (!gp(p, 'input') && String(s('input')).trim() === '') { toast('请填写待嵌入文本', 'error'); return null; }
            return { type: 'ltp9/test', action: 'embed', input: parseVal(gp(p, 'input')) };
        }
        case 'kokoro_tts': {
            if (!(p.text || '').trim()) { toast('请填写合成文本', 'error'); return null; }
            return { type: 'ltp9/test', action: 'kokoro_tts', text: s('text'), voice: (p.voice || '').trim(), speed: Number(p.speed) || 1, lang: s('lang') || 'auto' };
        }
        case 'qwen_tts': {
            if (!(p.text || '').trim()) { toast('请填写合成文本', 'error'); return null; }
            const env = { type: 'ltp9/test', action: 'qwen_tts', text: s('text') };
            if ((p.ref_audio || '').trim()) env.ref_audio = p.ref_audio.trim();
            return env;
        }
        case 'qwen_asr': {
            if (!(p.audio || '').trim()) { toast('请填写音频 base64', 'error'); return null; }
            return { type: 'ltp9/test', action: 'qwen_asr', audio: s('audio'), format: p.format || 'wav' };
        }
        case 'network':
            return { type: 'ltp9/test', action: 'network', op: p.op || 'resolveDNS', args: {
                host: s('host'), port: Number(p.port) || 0, service: s('service'), proto: s('proto'),
                handle: gp(p, 'handle') != null ? String(gp(p, 'handle')) : '',
                data: gp(p, 'data') != null ? String(gp(p, 'data')) : '',
                timeout: Number(p.timeout) || 0
            } };
        case 'async':
            return { type: 'ltp9/test', action: 'async', op: p.op || 'run', params: {
                plugin: (p.plugin || '').trim(),
                fn: (p.fn || '').trim(),
                args: parseVal(gp(p, 'args'), true),
                ms: Number(p.ms) || 0,
                timeout: Number(p.timeout) || 0,
                task_id: Number(p.taskId) || 0,
                progress: parseVal(gp(p, 'progress'))
            } };
        case 'http':
            return { type: 'ltp9/test', action: 'http', method: p.method || 'GET', url: s('url'), body: s('body'), headers: parseVal(gp(p, 'headers')) || {} };
        case 'stats':
            return { type: 'ltp9/stats' };
    }
    return null;
}

// ==== 执行 ====
function markRunning(node, on) {
    node.running = on;
    outgoingLinks(node.id).forEach(l => l.running = on);
    renderNodes(); renderLinks();
}
async function runSingle(id) {
    const n = nodeById(id); if (!n) return;
    if (!state.connected) { toast('尚未连接引擎', 'error'); return; }
    if (isGate(n.type)) { toast('逻辑门需随主流程运行', 'error'); return; }
    n._s = 'pending'; n.outValue = null;
    await runNode(n);
}
// 拓扑排序（仅沿时钟线）：节点激活只由时钟线传递；数据信号线仅传参、不触发执行
function topoOrder(hasStart, startIds) {
    const deps = state.links.filter(isClockLink);
    const out = {}; deps.forEach(l => { (out[l.from.node] = out[l.from.node] || []).push(l); });
    let seeds;
    if (hasStart && startIds.length) seeds = startIds.slice();
    else seeds = state.nodes.filter(n => clockLinksOf(n.id).length === 0).map(n => n.id);
    const reachable = new Set(seeds); const stack = seeds.slice();
    while (stack.length) { const id = stack.pop(); (out[id] || []).forEach(l => { if (!reachable.has(l.to.node)) { reachable.add(l.to.node); stack.push(l.to.node); } }); }
    const present = state.nodes.filter(n => reachable.has(n.id));
    const indeg = {}; present.forEach(n => indeg[n.id] = 0);
    deps.forEach(l => { if (reachable.has(l.to.node) && reachable.has(l.from.node)) indeg[l.to.node]++; });
    const q = present.filter(n => indeg[n.id] === 0).map(n => n.id);
    const order = [];
    while (q.length) { const id = q.shift(); order.push(id); (out[id] || []).forEach(l => { if (!reachable.has(l.to.node)) return; indeg[l.to.node]--; if (indeg[l.to.node] === 0) q.push(l.to.node); }); }
    if (order.length !== present.length) return null;
    return { order, reachable };
}
async function runAll() {
    if (state.running) return;
    if (!state.connected) { toast('尚未连接引擎', 'error'); return; }
    if (!state.nodes.length) { toast('画布为空', 'error'); return; }
    const TRIGGER = { start: 1, event_start: 1, clock_start: 1 };
    const startIds = state.nodes.filter(n => TRIGGER[n.type] && n.enabled !== false).map(n => n.id);
    const hasStart = startIds.length > 0;
    const tp = topoOrder(hasStart, startIds);
    if (!tp) { toast('连线存在环，无法链式执行', 'error'); return; }
    state.running = true; state.abort = false; setRunBtn(true);
    state.nodes.forEach(n => { n._s = 'pending'; n.outValue = null; n.running = false; });
    state.links.forEach(l => l.running = false);
    renderGraph();
    state.nodes.forEach(n => { if (!tp.reachable.has(n.id)) n._s = 'skip'; });
    // 严格串行：按拓扑顺序，一次只运行一个节点；依时钟线判断前置（上一节点未收到回执不算完成）
    for (const id of tp.order) {
        if (state.abort) break;
        const n = nodeById(id); if (!n || n._s === 'skip') continue;
        if (isGate(n.type)) { // 逻辑门：综合所有输入（时钟线/信号线都算其执行输入）的成败
            const inl = incomingLinks(id);
            const oks = inl.map(l => nodeById(l.from.node)._s === 'ok');
            n._s = gateFunc(n.type, oks) ? 'ok' : 'skip';
            renderNodes(); continue;
        }
        const clk = clockLinksOf(id); // 仅时钟线决定执行门槛
        const predsAllOk = clk.length === 0 || clk.every(l => nodeById(l.from.node)._s === 'ok');
        if (!predsAllOk) { n._s = 'skip'; renderNodes(); continue; }
        await runNode(n); // 阻塞直到该节点真正收到回执并完成
        renderNodes();
    }
    // 整条链执行完成后：复位全部节点为「就绪」，取消所有状态边框
    state.nodes.forEach(n => { n._s = 'idle'; n.outValue = null; });
    renderGraph();
    setRunBtn(false); state.running = false;
}
// ==== 时钟启动：每隔 interval 从该节点沿时钟线重跑其下游子链 ====
function outgoingClock(id) { return state.links.filter(l => l.from.node === id && isClockLink(l)); }
async function runScope(ids) {
    const set = new Set(ids);
    const indeg = {}; ids.forEach(id => indeg[id] = 0);
    const out = {};
    state.links.forEach(l => { if (isClockLink(l) && set.has(l.from.node) && set.has(l.to.node)) { indeg[l.to.node]++; (out[l.from.node] = out[l.from.node] || []).push(l); } });
    const q = ids.filter(id => indeg[id] === 0); const order = [];
    while (q.length) { const id = q.shift(); order.push(id); (out[id] || []).forEach(l => { indeg[l.to.node]--; if (indeg[l.to.node] === 0) q.push(l.to.node); }); }
    if (order.length !== ids.length) { toast('链存在环', 'error'); return; }
    for (const id of order) {
        if (state.abort) break;
        const n = nodeById(id); if (!n) continue;
        if (isGate(n.type)) { const oks = incomingLinks(id).map(l => nodeById(l.from.node)._s === 'ok'); n._s = gateFunc(n.type, oks) ? 'ok' : 'skip'; continue; }
        const clk = clockLinksOf(id); const ok = clk.length === 0 || clk.every(l => nodeById(l.from.node)._s === 'ok');
        if (!ok) { n._s = 'skip'; continue; }
        await runNode(n);
    }
}
async function runSubAfter(nodeId) {
    if (state.abort) return;
    const reach = new Set(); const st = [];
    outgoingClock(nodeId).forEach(l => { if (!reach.has(l.to.node)) { reach.add(l.to.node); st.push(l.to.node); } });
    while (st.length) { const id = st.pop(); outgoingClock(id).forEach(l => { if (!reach.has(l.to.node)) { reach.add(l.to.node); st.push(l.to.node); } }); }
    if (reach.size === 0) return;
    const ids = [...reach];
    ids.forEach(id => { const n = nodeById(id); if (n) { n._s = 'pending'; n.outValue = null; } });
    renderGraph();
    await runScope(ids);
    if (!state.abort) ids.forEach(id => { const n = nodeById(id); if (n) n._s = 'idle'; });
    renderGraph();
}
function setRunBtn(on) {
    const b = $('runBtn');
    b.innerHTML = on ? '<i class="fas fa-stop"></i> 停止运行' : '<i class="fas fa-play"></i> 运行全部';
}
function stopRun() {
    state.abort = true;
    clearClocks();
    state.evWaiters.forEach(h => { clearTimeout(h._t); h.res({ __abort: true }); });
    state.evWaiters = [];
    state.nodes.forEach(n => { n.running = false; n._s = 'idle'; n.outValue = null; });
    state.links.forEach(l => l.running = false);
    renderGraph(); setRunBtn(false); state.running = false;
}
async function runNode(node) {
    node.running = true; markRunning(node, true);
    const meta = metaOf(node.type);
    addLog({ dir: 'send', type: 'node:' + node.type, label: meta.label, req: '', env: { node: node.id }, isError: false, summary: '执行节点: ' + meta.label });
    try {
        if (node.type === 'start') { node.outValue = { started: true }; node._s = 'ok'; }
        else if (node.type === 'wait') { const ms = Number(node.params.ms) || 500; await sleep(ms); node.outValue = { slept_ms: ms }; node._s = 'ok'; }
        else if (node.type === 'transform') {
            const p = node.params || {};
            const template = String(p.template != null ? p.template : '').trim();
            if (!template) { node.outValue = { error: '输出模板不能为空' }; node._s = 'err'; }
            else {
                const values = {};
                ['in1', 'in2', 'in3', 'in4'].forEach((port, i) => {
                    const l = incomingLinks(node.id).find(x => x.to.port === port);
                    if (!l) return;
                    const src = nodeById(l.from.node);
                    if (!src || src.outValue === undefined) return;
                    const path = String(p['e' + (i + 1)] || '').trim();
                    values[port] = extractField(src.outValue, path);
                });
                let out = template.replace(/\{\{([^}]+)\}\}/g, (m, key) => {
                    const parts = pathKeys(key); const root = parts.shift();
                    if (!(root in values)) return m;
                    let v = values[root];
                    for (const s of parts) { if (v == null) break; v = v[s]; }
                    if (v == null) return '';
                    return typeof v === 'string' ? v : JSON.stringify(v);
                });
                node.outValue = out; node._s = 'ok';
            }
        }
        else if (node.type === 'display') { // 文本显示：取上游连线文本（无连线则用本节点填写内容），原样输出并回写参数供节点内预览
            const l = incomingLinks(node.id).find(x => x.to.port === 'text');
            let val = node.params.text;
            if (l) { const src = nodeById(l.from.node); if (src && src.outValue !== undefined && src.outValue !== null) val = src.outValue; }
            if (val !== undefined) node.params.text = val; // 回写，节点内预览与下游保持一致
            node.outValue = val; node._s = 'ok';
        }
        else if (node.type === 'event_start') { // 等待目标事件触发后启动
            const topic = String(node.params.topic || '').trim() || null;
            const r = await awaitEvent(topic, Number(node.params.timeout) || 8000);
            if (r && r.__abort) { node._s = 'skip'; }
            else if (r && r.__timeout) { node.outValue = { error: '等待事件超时' }; node._s = 'err'; }
            else { node.outValue = (r && r.payload !== undefined) ? r.payload : (r || {}); node._s = 'ok'; }
        }
        else if (node.type === 'clock_start') { // 每隔 interval 启动一次下游逻辑链
            const ms = Math.max(50, Number(node.params.interval) || 2000);
            const fire = () => { if (state.abort) { clearClocks(); return; } runSubAfter(node.id); };
            state.clocks.push(setInterval(fire, ms));
            node.outValue = { tick: true }; node._s = 'ok';
        }
        else if (node.type === 'speaker') { // 扬声器：取上游音频（或本节点填写的 base64）通过扬声器播放
            const l = incomingLinks(node.id).find(x => x.to.port === 'audio');
            let uri = null;
            if (l) { const src = nodeById(l.from.node); if (src && src.outValue) uri = audioDataUriOf(src.outValue); }
            if (!uri && typeof node.params.audio === 'string' && node.params.audio.trim()) uri = b64ToDataUri(node.params.audio);
            if (!uri) { node.outValue = { error: '无音频输入（请连线或在参数中填入 base64 音频）' }; node._s = 'err'; }
            else {
                try { await new Audio(uri).play(); node.outValue = { played: true }; node._s = 'ok'; }
                catch (e) { node.outValue = { error: '播放失败: ' + String(e) }; node._s = 'err'; }
            }
        }
        else if (node.type === 'microphone') { // 麦克风：请求授权并录制指定时长，输出 base64 音频（webm/opus）
            const sec = Math.max(0.3, Number(node.params.duration) || 3);
            addLog({ dir: 'send', type: 'node:microphone', label: '麦克风录音', req: '', env: {}, isError: false, summary: `请求麦克风授权，录音 ${sec} 秒…` });
            const b64 = await recordAudio(sec);
            if (!b64) { node.outValue = { error: '录音失败或麦克风未授权' }; node._s = 'err'; }
            else {
                node.params.audio = b64; // 回写，供连线/查看
                node.outValue = b64; // 裸 base64，连线到 Qwen 识别 / 扬声器皆可直接使用
                node._s = 'ok';
            }
        }
        else {
            // 注入上游数据：对每条数据输入连线，把上游输出写入本节点对应参数
            incomingLinks(node.id).forEach(l => {
                if (l.to.port === 'gate') return;
                const src = nodeById(l.from.node);
                if (src && src.outValue !== undefined && src.outValue !== null) node.params[l.to.port] = src.outValue;
            });
            if (isGate(node.type)) { node._s = gateFunc(node.type, incomingLinks(node.id).map(l => nodeById(l.from.node)._s === 'ok')) ? 'ok' : 'skip'; }
            else {
                const env = buildEnvelope(node);
                if (!env) { node.outValue = { error: '参数未完整配置' }; node._s = 'err'; }
                else {
                    if (env.type === 'ltp9/event') emitEventLocal(env.topic, env.payload); // 事件触发时点亮事件启动链
                    const ack = await sendWait(env, meta.label);
                    node.outValue = ack; // outValue = 原始回执，字段路径与日志/展示的 JSON 完全一致
                    // 失败判定：引擎层错误（result.error）、调用层 ok:false（result.value.ok）、或未收到回执
                    const rv = ack && ack.result;
                    const err = ack && (ack.ok === false || (rv && rv.error) || (rv && rv.value && rv.value.ok === false));
                    node._s = (err || !ack) ? 'err' : 'ok';
                }
            }
        }
    } catch (e) { node._s = 'err'; node.outValue = { error: String(e) }; }
    markRunning(node, false);
}

// ==== 持久化 / 示例 / 清空 ====
// 画布以 JSON 文件形式保存到 local_data/database/（同项目数据库目录）
function graphJson() {
    return JSON.stringify({
        nodes: state.nodes.map(n => ({ id: n.id, type: n.type, x: n.x, y: n.y, params: n.params, enabled: n.enabled })),
        links: state.links.map(l => ({ id: l.id, from: l.from, to: l.to }))
    }, null, 2);
}
async function save() {
    try {
        const resp = await fetch('/file/write', {
            method: 'POST',
            headers: {
                'X-File-Name': btoa(unescape(encodeURIComponent(FLOW_FILE))),
                'X-Overwrite': 'true'
            },
            body: new Blob([graphJson()], { type: 'application/json' })
        });
        if (!resp.ok) { toast('保存失败: ' + resp.status, 'error'); return; }
        toast('画布已保存 → ' + FLOW_FILE, 'success');
    } catch (e) { toast('保存异常: ' + e.message, 'error'); }
}
function applyGraph(g) {
    state.nodes = (g.nodes || []).map(n => Object.assign({ enabled: true, running: false, _s: 'pending', outValue: null }, n));
    state.links = g.links || []; renderGraph();
}
async function load() {
    try {
        const resp = await fetch('/file/read/' + FLOW_FILE);
        if (!resp.ok) { toast('没有已保存的画布（' + FLOW_FILE + '）', 'error'); return; }
        const g = await resp.json();
        applyGraph(g); toast('已加载画布', 'success');
    } catch (e) { toast('加载失败: ' + e.message, 'error'); }
}
function clearGraph() { if (!state.nodes.length) return; if (!confirm('确定清空画布？')) return; state.nodes = []; state.links = []; renderGraph(); closeModal(); closeLogModal(); toast('已清空'); }
function demoGraph() {
    state.nodes = []; state.links = [];
    const cx = 60, cy = 60, gapX = 240, gapY = 150;
    const defs = [
        { type: 'start', x: cx, y: cy },
        { type: 'stats', x: cx, y: cy + gapY },
        { type: 'wait', x: cx + gapX, y: cy },
        { type: 'crypto', x: cx + gapX, y: cy + gapY },
        { type: 'llm', x: cx + gapX * 2, y: cy },
        { type: 'broadcast_all', x: cx + gapX * 2, y: cy + gapY },
        { type: 'transform', x: cx + gapX * 3, y: cy }
    ];
    defs.forEach(d => state.nodes.push({ id: newId(), type: d.type, x: d.x, y: d.y, params: defaultParams(d.type), running: false, _s: 'pending', outValue: null }));
    const lk = (a, b, ap, bp) => state.links.push({ id: 'l' + seg(), from: { node: state.nodes[a].id, port: ap }, to: { node: state.nodes[b].id, port: bp } });
    // 时钟链：start → stats → wait → crypto → llm → broadcast
    lk(0, 1, 'gate', 'gate');
    lk(1, 2, 'rows', 'gate');
    lk(2, 3, 'gate', 'gate');
    lk(3, 4, 'text', 'gate');
    lk(4, 5, 'answer', 'gate');
    // 时钟：start → 格式转换；同时 stats 结果经信号线（rows→in1）喂入 格式转换
    lk(0, 6, 'gate', 'gate');
    lk(1, 6, 'rows', 'in1');
    renderGraph();
}
function ensureLinkByIdx(a, b, ap, bp) { /* 兼容预留 */ }

// ==== 事件绑定 ====
// 节点模块分类：类型 → 分页
const NODE_CATS = [
    { key: 'start', label: '入口启动', types: ['start', 'event_start', 'clock_start'] },
    { key: 'engine', label: '引擎交互', types: ['event', 'broadcast_all', 'broadcast_target', 'call', 'tool', 'command', 'agent', 'platform', 'async'] },
    { key: 'data', label: '数据安全', types: ['db', 'memory', 'file', 'crypto', 'jwt', 'emoji'] },
    { key: 'ai', label: '网络通讯', types: ['llm', 'embed', 'http', 'network'] },
    { key: 'media', label: '语音系统', types: ['kokoro_tts', 'qwen_tts', 'qwen_asr', 'microphone', 'speaker'] },
    { key: 'flow', label: '流程处理', types: ['wait', 'transform', 'display', 'stats'] },
    { key: 'gates', label: '逻辑控制', types: ['and', 'or', 'not', 'nand', 'nor'] }
];
let nodeCatActive = 'start';
function openNodeModalPanel() { $('nodeSidebar').classList.add('open'); renderNodeModal(); }
function closeNodeModalPanel() { $('nodeSidebar').classList.remove('open'); }
function toggleNodeSidebar() { const s = $('nodeSidebar'); s.classList.toggle('open'); if (s.classList.contains('open')) renderNodeModal(); }
function renderNodeModal() {
    const tabs = $('nodeTabs'), body = $('nodeContent');
    tabs.innerHTML = '';
    NODE_CATS.forEach(c => {
        const b = document.createElement('button'); b.className = 'node-tab' + (c.key === nodeCatActive ? ' active' : '');
        b.textContent = c.label;
        b.addEventListener('click', () => { nodeCatActive = c.key; renderNodeModal(); });
        tabs.appendChild(b);
    });
    body.innerHTML = '';
    const cat = NODE_CATS.find(c => c.key === nodeCatActive) || NODE_CATS[0];
    cat.types.forEach(type => {
        const meta = metaOf(type);
        const chip = document.createElement('div'); chip.className = 'node-chip';
        chip.draggable = true; // 支持拖拽到画布释放
        chip.title = (meta.desc || '') + '\n拖拽到画布释放，或点击快速添加';
        const icon = document.createElement('i'); icon.className = 'fas ' + meta.icon; icon.style.color = meta.color || 'var(--accent)';
        chip.appendChild(icon);
        const label = document.createElement('span'); label.textContent = meta.label; chip.appendChild(label);
        chip.addEventListener('dragstart', e => {
            e.dataTransfer.setData('text/plain', type);
            e.dataTransfer.effectAllowed = 'copy';
        });
        chip.addEventListener('click', () => {
            const n = addNode(type, 80 + Math.random() * 120, 80 + Math.random() * 80);
            openNodeModal(n);
        });
        body.appendChild(chip);
    });
}
function bindNodeModal() {
    $('nodeFab').addEventListener('click', toggleNodeSidebar);
    $('nodeSidebarClose').addEventListener('click', closeNodeModalPanel);
}
function bindActions() {
    $('runBtn').addEventListener('click', () => { if (state.running || state.clocks.length) stopRun(); else runAll(); });
    $('demoBtn').addEventListener('click', demoGraph);
    $('saveBtn').addEventListener('click', save);
    $('loadBtn').addEventListener('click', load);
    $('clearBtn').addEventListener('click', clearGraph);
    $('reconnectBtn').addEventListener('click', () => { state.retry = 0; if (state.ws) { try { state.ws.close(); } catch (e) {} } connect(); });
    $('logFilters').addEventListener('click', e => { const chip = e.target.closest('.filter-chip'); if (!chip) return; document.querySelectorAll('.filter-chip').forEach(c => c.classList.remove('active')); chip.classList.add('active'); state.filter = chip.dataset.filter; renderLog(); });
    $('clearLogBtn').addEventListener('click', () => { state.log = []; renderLog(); });
    $('logFab').addEventListener('click', toggleLogSidebar);
    $('closeLogBtn').addEventListener('click', closeLogModal);
    // 节点模块：拖拽节点芯片到画布释放，生成节点
    const canvasEl = $('canvas');
    canvasEl.addEventListener('dragover', e => { e.preventDefault(); e.dataTransfer.dropEffect = 'copy'; canvasEl.classList.add('drag-over'); });
    canvasEl.addEventListener('dragleave', e => { if (!canvasEl.contains(e.relatedTarget)) canvasEl.classList.remove('drag-over'); });
    canvasEl.addEventListener('drop', e => {
        e.preventDefault(); canvasEl.classList.remove('drag-over');
        const type = e.dataTransfer.getData('text/plain');
        if (!type || !NODES[type]) return;
        const r = canvasEl.getBoundingClientRect();
        const x = e.clientX - r.left + canvasEl.scrollLeft - NODE_W / 2;
        const y = e.clientY - r.top + canvasEl.scrollTop - 15;
        addNode(type, Math.max(10, x), Math.max(10, y));
    });
    initDragParamPop();
    document.addEventListener('keydown', e => { if (e.key === 'Escape') { closeModal(); closeLogModal(); closeNodeModalPanel(); } });
}

// ==== 启动 ====
async function init() {
    bindNodeModal(); bindActions();
    renderGraph(); renderLog(); renderWs(); updateFab();
    connect();
    try {
        const resp = await fetch('/file/read/' + FLOW_FILE);
        if (resp.ok) applyGraph(await resp.json()); else demoGraph();
    } catch (e) { demoGraph(); }
}
init();