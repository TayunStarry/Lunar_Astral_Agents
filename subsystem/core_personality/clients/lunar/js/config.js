// ============================================================
//  星月智能 · 消息终端 — 常量配置 / DOM 引用 / 全局状态
// ============================================================

// ---------- 常量配置 ----------
// WebSocket 连接同源地址：HTTP 直连时为后端 36789，HTTPS 代理时为代理端口（由代理隧道转发至后端）
// channel=message-terminal 为该终端声明的接入频道（供控制台在线探针识别，且该终端不订阅引擎总线）
const WS_PROTOCOL = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const WS_URL = `${WS_PROTOCOL}//${window.location.host}/ws?channel=message-terminal`;
const MAX_RECONNECT_ATTEMPTS = 3;
const RECONNECT_BASE_DELAY = 1500;
const ASSISTANT_NAME = '月华';
const DEFAULT_USER_NAME = '子幽';   // 配置未定义用户名时的默认用户名（气泡名与发送前缀共用）
const MESSAGE_FILE_PATH = 'database/messages.json'; // 聊天记录缓存文件（由思维链写入，前端仅刷新时读取）
const MAX_CACHED_MESSAGES = 48; // 与思维链侧 MAX_CACHE_MESSAGES 保持一致

// ---------- DOM 引用 ----------
const messageArea = document.getElementById('messageArea');
const emptyState = document.getElementById('emptyState');
const messageInput = document.getElementById('messageInput');
const sendButton = document.getElementById('sendButton');
const toastContainer = document.getElementById('toastContainer');
const dragOverlay = document.getElementById('dragOverlay');
const themeToggle = document.getElementById('themeToggle');
const attachBtn = document.getElementById('attachBtn');
const fileInput = document.getElementById('fileInput');
const voiceBtn = document.getElementById('voiceBtn');
const tabBar = document.getElementById('tabBar');
const searchInput = document.getElementById('searchInput');
const searchCount = document.getElementById('searchCount');
const searchPrev = document.getElementById('searchPrev');
const searchNext = document.getElementById('searchNext');
const searchClear = document.getElementById('searchClear');
const pendingAttachments = document.getElementById('pendingAttachments');
const fileRefChips = document.getElementById('fileRefChips');
const voiceToggleBtn = document.getElementById('voiceToggleBtn');

// ---------- 截图 DOM 引用 ----------
const captureBtn = document.getElementById('captureBtn');
const captureModal = document.getElementById('captureModal');
const capturePreviewImg = document.getElementById('capturePreviewImg');
const captureToSendBtn = document.getElementById('captureToSendBtn');
const captureToDrawboardBtn = document.getElementById('captureToDrawboardBtn');
const captureCloseBtn = document.getElementById('captureCloseBtn');

// ---------- 画板 DOM 引用 ----------
const openDrawboardBtn = document.getElementById('openDrawboardBtn');
const drawboardOverlay = document.getElementById('drawboardOverlay');
const importBgBtn = document.getElementById('importBgBtn');
const bgFileInput = document.getElementById('bgFileInput');
const clearDrawBtn = document.getElementById('clearDrawBtn');
const closeDrawboardBtn = document.getElementById('closeDrawboardBtn');
const undoDrawBtn = document.getElementById('undoDrawBtn');
const drawboardCanvasWrap = document.getElementById('drawboardCanvasWrap');
const drawboardBg = document.getElementById('drawboardBg');
const drawboardLayer = document.getElementById('drawboardLayer');
const drawboardPreview = document.getElementById('drawboardPreview');
const drawboardInput = document.getElementById('drawboardInput');
const drawboardSendBtn = document.getElementById('drawboardSendBtn');

// ---------- 滚动控制 DOM 引用 ----------
const cornerContent = document.getElementById('cornerContent');
const cornerToggle = document.getElementById('cornerToggle');
const scrollTopBtn = document.getElementById('scrollTopBtn');
const scrollBottomBtn = document.getElementById('scrollBottomBtn');
const jumpUserBtn = document.getElementById('jumpUserBtn');
const userJumpPanel = document.getElementById('userJumpPanel');
const userJumpList = document.getElementById('userJumpList');

// ---------- 状态变量 ----------
let ws = null;
let reconnectAttempts = 0;
let reconnectTimer = null;
let manualClose = false;
let backendConnected = false;
let isDarkMode = false;
let currentTab = 'all';
let messages = [];            // 会话内消息对象列表（页面刷新时从思维链缓存文件载入）
let dragCounter = 0;
let searchQuery = '';
let searchMatches = [];
let currentMatchIndex = -1;
let mermaidInitialized = false;
let pendingFiles = [];        // 待发送附件（悬浮气泡）
let referencedFiles = [];     // 已加载到输入框的资源句柄（@D.003 等规范句柄）
let isSending = false;        // 是否正在发送
let autoPlayVoice = true;     // 收到语音消息时是否自动播放
let characterRenderer = 'bedrock'; // 角色渲染器（bedrock=基岩版模型 / live2d=Live2D 引擎层），由 lunar_config.json 的 system.character_renderer 决定
let backendUserName = DEFAULT_USER_NAME; // 用户气泡名与发送给月华的前缀共用的用户名（来自配置）
let captureFile = null;       // 当前截图的 File 对象
let capturePreviewUrl = null; // 当前截图预览 blob URL
