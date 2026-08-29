// ============================================================
//  星月智能 · 消息终端 — 常量配置 / DOM 引用 / 全局状态
// ============================================================

// ---------- 常量配置 ----------
const WS_PROTOCOL = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const WS_URL = `${WS_PROTOCOL}//${window.location.hostname}:36789/ws`;
const MAX_RECONNECT_ATTEMPTS = 3;
const RECONNECT_BASE_DELAY = 1500;
const USER_NAME = '你';
const ASSISTANT_NAME = '月华';
const MESSAGES_FILE_PATH = 'database/messages.json';
const MAX_PERSISTED_MESSAGES = 200;

// ---------- DOM 引用 ----------
const messageArea = document.getElementById('messageArea');
const emptyState = document.getElementById('emptyState');
const messageInput = document.getElementById('messageInput');
const sendButton = document.getElementById('sendButton');
const toastContainer = document.getElementById('toastContainer');
const dragOverlay = document.getElementById('dragOverlay');
const themeToggle = document.getElementById('themeToggle');
const clearBtn = document.getElementById('clearBtn');
const attachBtn = document.getElementById('attachBtn');
const fileInput = document.getElementById('fileInput');
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
const topControls = document.querySelector('.top-controls');
const scrollControls = document.getElementById('scrollControls');
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
let messages = [];            // 持久化的消息对象列表
let dragCounter = 0;
let searchQuery = '';
let searchMatches = [];
let currentMatchIndex = -1;
let saveTimer = null;
let mermaidInitialized = false;
let pendingFiles = [];        // 待发送附件（悬浮气泡）
let referencedFiles = [];     // 已加载到输入框的文件引用（[#文件名.ext]:）
let isSending = false;        // 是否正在发送
let autoPlayVoice = true;     // 收到语音消息时是否自动播放
let captureFile = null;       // 当前截图的 File 对象
let capturePreviewUrl = null; // 当前截图预览 blob URL
