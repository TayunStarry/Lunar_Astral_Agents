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
        desc: '运行全部时与其它启动节点同时开始，沿时钟线并行向下传播激活；无数据输入输出',
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
        desc: '每隔设定时间沿时钟线激活一次下游节点（各分支并行独立传播，直至点击「停止运行」）',
        ins: [], out: { k: 'tick', label: 'tick' },
        fields: [{ key: 'interval', label: '启动间隔(毫秒)', type: 'number', def: 2000 }]
    },
    limit: {
        label: '调用上限', icon: 'fa-gauge-high', color: '#06b6d4',
        desc: '动态调整本次运行的最大节点调用次数（默认 300，防无限递归）；被时钟激活时立即全局生效，需接入时钟链',
        ins: [], out: null,
        fields: [{ key: 'max', label: '最大调用次数', type: 'number', def: 300 }]
    },
    event: {
        label: '事件触发', icon: 'fa-bolt', color: '#f97316',
        desc: '触发订阅事件（weather.query / art.generate 等）；主题从引擎已订阅列表动态填充；输出原始回执（含订阅器返回与改写后载荷），不做简化提取',
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
        desc: 'engine.call(包ID).run(函数名, 参数)；单参数框：JSON数组=多参数，单个文本/JSON=首个参数；【输入数据】连线则覆盖为首参；输出原始回执（ltp9/result 完整结构），不做简化提取',
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
        desc: 'engine.file.write/read/delete；write_b64 把上游 base64 解码为二进制落盘（兼容 data URI 前缀）；<路径/内容> 可来自上游，读出内容经输出传递',
        ins: [{ k: 'path', label: '路径' }, { k: 'data', label: '内容' }], out: { k: 'text', label: '内容' },
        fields: [
            { key: 'op', label: '操作', type: 'select', options: ['write', 'write_b64', 'read', 'delete'], def: 'write' },
            { key: 'path', label: '路径', type: 'text', ph: 'probe.txt', def: 'probe.txt' },
            { key: 'data', label: '内容 (write) / base64 (write_b64)', type: 'textarea', rows: 2, def: 'hello ltp9 probe' }
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
    base64: {
        label: 'Base64 编解码', icon: 'fa-file-code', color: '#0891b2',
        desc: 'engine.base64：encode 文本 → base64 字符串，decode base64 → 文本（兼容 data:xxx;base64, 前缀，自动剥离）；<内容>可来自上游，常用于传输二进制/文本',
        ins: [{ k: 'content', label: '内容' }], out: { k: 'text', label: '结果' },
        fields: [
            { key: 'op', label: '操作', type: 'select', options: ['encode', 'decode'], def: 'encode' },
            { key: 'content', label: '明文 (encode) / base64 (decode)', type: 'textarea', rows: 2, def: '星月智能' }
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
        label: '提取合并', icon: 'fa-code-merge', color: '#38bdf8',
        desc: '多路合并：接收多路输入，逐路设置提取字段（留空取全部），按 {{inN.字段}} 占位符模板组合为单路输出；路径支持数组下标（如 result.outcomes[0].result.data.city）；激活需时钟线接入（信号线仅传数据）；模板必填',
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
    extract: {
        label: '提取拆分', icon: 'fa-scissors', color: '#fbbf24',
        desc: '单路拆分：从一个输入按多条提取路径拆分为多路输出；每个输出端口对应一条提取路径（留空=原样透传整值），路径支持数组下标 a.b[0].c；激活需时钟线接入（信号线仅传数据）；输出1 亦作为节点默认输出（兼容单值消费）',
        ins: [{ k: 'input', label: '输入数据' }],
        outs: [
            { k: 'out1', label: '输出1' }, { k: 'out2', label: '输出2' },
            { k: 'out3', label: '输出3' }, { k: 'out4', label: '输出4' }
        ],
        fields: [
            { key: 'e1', label: '输出1 提取路径（留空=原样透传）', type: 'text', def: '' },
            { key: 'e2', label: '输出2 提取路径（留空=不输出）', type: 'text', def: '' },
            { key: 'e3', label: '输出3 提取路径（留空=不输出）', type: 'text', def: '' },
            { key: 'e4', label: '输出4 提取路径（留空=不输出）', type: 'text', def: '' }
        ]
    },
    display: {
        label: '文本显示', icon: 'fa-eye', color: '#f472b6',
        desc: '接收并显示上游文本内容；卡片仅展示描述标签（说明本节点用途），真实文本可点节点头部「气泡」图标查看；激活时若有内容自动弹出消息气泡；激活需时钟线接入（信号线仅传数据），未连线时显示并输出本节点填写的文本',
        ins: [{ k: 'text', label: '文本' }], out: { k: 'text', label: '文本' },
        fields: [
            { key: 'text', label: '文本内容（未连线时使用）', type: 'textarea', rows: 2, def: '你好，星月智能！' },
            { key: 'hint', label: '卡片描述标签（说明本节点展示什么）', type: 'text', def: '用于展示文本内容' }
        ]
    },
    image_display: {
        label: '图像显示', icon: 'fa-image', color: '#fb7185',
        desc: '接收上游 base64/data URI 图像并弹出展示（激活时若有图像自动弹出；也可点头部「图片」图标查看）；卡片仅展示描述标签；激活需时钟线接入，未连线时展示本节点填写的图像；输出透传图像数据',
        ins: [{ k: 'image', label: '图像(base64)' }], out: { k: 'image', label: '图像(base64)', kind: 'image' },
        fields: [
            { key: 'image', label: '图像 base64 / data URI（未连线时使用）', type: 'textarea', rows: 2, def: '' },
            { key: 'hint', label: '卡片描述标签（说明本节点展示什么）', type: 'text', def: '用于展示图像内容' }
        ]
    },
    image_confuse: {
        label: '图像混淆', icon: 'fa-shuffle', color: '#c084fc',
        desc: '混淆/解混淆/还原图像（本地处理）：按 Gilbert 空间填充曲线遍历像素 + 黄金比例偏移做环形置换，与该工具 lunar.image-confusion 同算法，可互相混淆/解混淆；输出处理后图像，运行收尾自动弹出展示；输出格式 png 为无损（利于逐像素还原），jpeg 为有损（体积小）',
        ins: [{ k: 'image', label: '图像(base64)' }], out: { k: 'image', label: '图像(base64)', kind: 'image' },
        fields: [
            { key: 'mode', label: '运行模式（confuse 混淆 / deconfuse 解混淆 / restore 还原原图）', type: 'select', options: ['confuse', 'deconfuse', 'restore'], def: 'confuse' },
            { key: 'format', label: '输出格式（png 无损 / jpeg 有损）', type: 'select', options: ['jpeg', 'png'], def: 'jpeg' },
            { key: 'quality', label: 'JPEG 质量(0.1-1，仅 jpeg 生效)', type: 'number', def: 0.95 },
            { key: 'image', label: '图像 base64 / data URI（未连线时使用）', type: 'textarea', rows: 2, def: '' }
        ]
    },
    video_keyframe: {
        label: '视频抽帧', icon: 'fa-film', color: '#fb923c',
        desc: '视频转GIF：拉取视频URL上传至琉璃 /keyframe 提取关键帧（每秒5帧+相似度去重，最长1小时），前端将关键帧序列量化合成 GIF 动图（中位切法调色板 + LZW 无损压缩）；输出 GIF base64（data URI），可连线到图像显示节点展示；<视频URL>可来自上游',
        ins: [{ k: 'url', label: '视频URL' }], out: { k: 'gif', label: 'GIF(base64)', kind: 'image' },
        fields: [
            { key: 'url', label: '视频URL（未连线时使用）', type: 'text', ph: 'https://... 或 /file/read/...', def: '' },
            { key: 'maxWidth', label: 'GIF 宽度上限(像素，等比缩放，16~1024)', type: 'number', def: 480 },
            { key: 'delayMs', label: '帧间隔(毫秒，按 1/100 秒取整)', type: 'number', def: 200 },
            { key: 'maxFrames', label: '最多帧数(0=不限制；GIF 体积随帧数增长)', type: 'number', def: 0 }
        ]
    },
    wait: {
        label: '同步等待', icon: 'fa-hourglass-half', color: '#94a3b8',
        desc: 'engine.sleep 语义：阻塞等待指定毫秒；无数据输入输出',
        ins: [], out: null, fields: [{ key: 'ms', label: '等待毫秒', type: 'number', def: 800 }]
    },
    stats: {
        label: '插件状态', icon: 'fa-cubes', color: '#f43f5e',
        desc: '查询引擎在线 + 已加载插件（含事件/导出/智能体包）；输出原始回执（含 ok/plugins 等完整字段），不做简化提取', ins: [], out: { k: 'rows', label: '插件列表' }, fields: []
    },
    and: { label: '与 AND', icon: 'fa-diagram-project', color: '#22c55e', gate: true, desc: '所有输入成功才放行', ins: port_ins4(), out: null, fields: [] },
    or: { label: '或 OR', icon: 'fa-share-nodes', color: '#a3e635', gate: true, desc: '任一路径成功即放行', ins: port_ins4(), out: null, fields: [] },
    not: { label: '非 NOT', icon: 'fa-right-left', color: '#facc15', gate: true, desc: '输入取反（单输入）', ins: port_ins4(), out: null, fields: [] },
    nand: { label: '与非 NAND', icon: 'fa-code-branch', color: '#eab308', gate: true, desc: '非(全部成功)', ins: port_ins4(), out: null, fields: [] },
    nor: { label: '或非 NOR', icon: 'fa-toggle-off', color: '#ca8a04', gate: true, desc: '非(任一成功)', ins: port_ins4(), out: null, fields: [] },
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
        desc: 'engine：经月华 system-asr（Qwen3-ASR）HTTP 接口识别 base64 音频为文本，需月华服务在线；wav/mp3/flac/ogg 直接识别，其他格式由琉璃端 ffmpeg 转 16k 单声道；<音频>可来自上游',
        ins: [{ k: 'audio', label: '音频(base64)' }], out: { k: 'text', label: '识别文本' },
        fields: [
            { key: 'audio', label: '音频 base64（wav/mp3/flac/ogg）', type: 'textarea', rows: 3, def: '' },
            { key: 'format', label: '源音频格式（非 wav/mp3/flac/ogg 时转码用）', type: 'select', options: ['wav', 'webm', 'ogg', 'mp4', 'm4a', 'weba'], def: 'wav' }
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
    audio_eq: {
        label: '音频均衡器', icon: 'fa-wave-square', color: '#38bdf8',
        desc: '三段均衡（本地处理）：低频低架 + 中频峰值 + 高频高架滤波器串联，各段可增益/衰减 ±24dB，输出 16bit PCM WAV（base64 data URI），可直接连线到扬声器播放或继续串联；wav 走内置解析，webm/opus、mp3、ogg、m4a 交由浏览器解码',
        ins: [{ k: 'audio', label: '音频(base64)' }], out: { k: 'audio', label: '音频(wav base64)', kind: 'audio' },
        fields: [
            { key: 'lowGain', label: '低频增益(dB，负数=衰减)', type: 'number', def: 0 },
            { key: 'lowFreq', label: '低频分频点(Hz，低架转折)', type: 'number', def: 250 },
            { key: 'midGain', label: '中频增益(dB，负数=衰减)', type: 'number', def: 0 },
            { key: 'midFreq', label: '中频中心频率(Hz，峰值)', type: 'number', def: 1000 },
            { key: 'midQ', label: '中频带宽 Q(越大越窄)', type: 'number', def: 1 },
            { key: 'highGain', label: '高频增益(dB，负数=衰减)', type: 'number', def: 0 },
            { key: 'highFreq', label: '高频分频点(Hz，高架转折)', type: 'number', def: 4000 },
            { key: 'outGain', label: '总输出增益(dB，补偿用)', type: 'number', def: 0 },
            { key: 'audio', label: '音频 base64 / data URI（未连线时使用）', type: 'textarea', rows: 2, def: '' }
        ]
    },
    composite: {
        label: '复合节点', icon: 'fa-object-group', color: '#a78bfa',
        desc: '自定义蓝图封装：把一段子流程打包为单个节点，降低复杂蓝图的搭建与阅读成本。由工具栏「封装」对 Ctrl+多选节点生成；外部入线→输入端口、外部出线→单输出端口；点「展开」还原为原始节点再编辑。参数面板可「保存到节点模块」收藏，之后可在左侧「我的复合」随时加入任意画布。注意：内部不建议使用时钟启动节点，内部连线不可成环',
        ins: [], out: { k: 'out', label: '输出' },
        fields: [
            { key: 'label', label: '复合节点名称（卡片标题）', type: 'text', def: '复合节点' },
            { key: 'hint', label: '卡片描述标签（说明内部子流程做什么）', type: 'text', def: '封装的子流程' }
        ]
    },
};
function port_ins4() { return [{ k: 'in1', label: '输入1' }, { k: 'in2', label: '输入2' }, { k: 'in3', label: '输入3' }, { k: 'in4', label: '输入4' }]; }

// ==== 节点元信息 / 连线判定 ====
function metaOf(type) { return NODES[type] || { label: type, icon: 'fa-cube', color: '#6d5ce7', ins: [], out: null, fields: [] }; }
function isGate(type) { return !!GATE_TYPES[type]; }
// 时钟线：连接目标「顺序」输入口，或目标为逻辑门（门的任意输入都参与执行）；决定执行链/门控
// 信号线：连接目标数据输入口，仅用于传入参数，不参与执行顺序判断
function isClockLink(l) {
    if (l.to.port === 'gate') return true;
    const to = nodeById(l.to.node);
    return !!to && isGate(to.type);
}
function clockLinksOf(id) { return viewOf(id).links.filter(l => l.to.node === id && isClockLink(l)); }