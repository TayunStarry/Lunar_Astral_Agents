// 游戏常量
const GAME_CONSTANTS = {
    BOARD_SIZE: 10,
    MAX_PIECES: 10,
    MAX_MOVES: 30,
    AI_TIMEOUT: 5000,
    PLAYERS: {
        HUMAN: 'X',
        AI: 'O'
    }
};

// 游戏状态
let gameState = {
    board: createEmptyBoard(),
    currentPlayer: GAME_CONSTANTS.PLAYERS.HUMAN,
    gameOver: false,
    winner: null, // null: 无, 'X'/'O': 赢家, 'draw': 平局
    moveHistory: [],
    lockedPositions: new Set(),
    isRequestPending: false,
    pendingTimeout: null
};

// DOM 元素
const DOM_ELEMENTS = {
    board: document.getElementById('board'),
    status: document.getElementById('game-status'),
    resetBtn: document.getElementById('reset-btn')
};

// 通信通道
const channel = new BroadcastChannel('ttt_ai_channel');

// 创建空棋盘
function createEmptyBoard() {
    return Array(GAME_CONSTANTS.BOARD_SIZE).fill().map(() => Array(GAME_CONSTANTS.BOARD_SIZE).fill(''));
}

// 初始化棋盘UI
function buildBoardUI() {
    DOM_ELEMENTS.board.innerHTML = '';

    for (let row = 0; row < GAME_CONSTANTS.BOARD_SIZE; row++) {
        for (let col = 0; col < GAME_CONSTANTS.BOARD_SIZE; col++) {
            const cell = document.createElement('div');
            cell.className = 'cell';
            cell.dataset.row = row;
            cell.dataset.col = col;
            cell.addEventListener('click', handleCellClick);
            DOM_ELEMENTS.board.appendChild(cell);
        }
    }
}

// 渲染棋盘
function renderBoard() {
    const cells = document.querySelectorAll('.cell');
    cells.forEach(cell => {
        const row = parseInt(cell.dataset.row);
        const col = parseInt(cell.dataset.col);
        const value = gameState.board[row][col];
        const positionKey = `${row}-${col}`;

        cell.textContent = value;

        // 更新样式类
        cell.classList.remove('X', 'O', 'occupied', 'locked');
        if (value === GAME_CONSTANTS.PLAYERS.HUMAN) {
            cell.classList.add('X', 'occupied');
        } else if (value === GAME_CONSTANTS.PLAYERS.AI) {
            cell.classList.add('O', 'occupied');
        } else if (gameState.lockedPositions.has(positionKey)) {
            cell.classList.add('locked');
        }
    });
}

// 检查游戏状态
function checkGameState() {
    // 检查横向连线
    for (let row = 0; row < GAME_CONSTANTS.BOARD_SIZE; row++) {
        if (checkLine(gameState.board[row])) {
            gameState.winner = gameState.board[row][0];
            gameState.gameOver = true;
            return;
        }
    }

    // 检查纵向连线
    for (let col = 0; col < GAME_CONSTANTS.BOARD_SIZE; col++) {
        const column = [];
        for (let row = 0; row < GAME_CONSTANTS.BOARD_SIZE; row++) {
            column.push(gameState.board[row][col]);
        }
        if (checkLine(column)) {
            gameState.winner = gameState.board[0][col];
            gameState.gameOver = true;
            return;
        }
    }

    // 检查主对角线连线 (左上到右下)
    const mainDiagonal = [];
    for (let i = 0; i < GAME_CONSTANTS.BOARD_SIZE; i++) {
        mainDiagonal.push(gameState.board[i][i]);
    }
    if (checkLine(mainDiagonal)) {
        gameState.winner = gameState.board[0][0];
        gameState.gameOver = true;
        return;
    }

    // 检查副对角线连线 (右上到左下)
    const antiDiagonal = [];
    for (let i = 0; i < GAME_CONSTANTS.BOARD_SIZE; i++) {
        antiDiagonal.push(gameState.board[i][GAME_CONSTANTS.BOARD_SIZE - 1 - i]);
    }
    if (checkLine(antiDiagonal)) {
        gameState.winner = gameState.board[0][GAME_CONSTANTS.BOARD_SIZE - 1];
        gameState.gameOver = true;
        return;
    }

    // 检查步数限制：30步内未胜利则AI胜利
    if (gameState.moveHistory.length >= GAME_CONSTANTS.MAX_MOVES && !gameState.gameOver) {
        gameState.winner = GAME_CONSTANTS.PLAYERS.AI;
        gameState.gameOver = true;
        return;
    }

    // 检查平局: 所有格子非空
    if (gameState.board.every(row => row.every(cell => cell !== ''))) {
        gameState.winner = 'draw';
        gameState.gameOver = true;
    }
}

// 检查一行是否全部相同且非空
function checkLine(line) {
    const firstCell = line[0];
    return firstCell && line.every(cell => cell === firstCell);
}

// 更新UI并广播状态
function updateUIAndBroadcast() {
    // 更新状态栏
    updateStatusBar();

    // 通过广播发送完整状态给控制台
    channel.postMessage({
        type: 'stateUpdate',
        board: JSON.parse(JSON.stringify(gameState.board)),
        currentPlayer: gameState.currentPlayer,
        gameOver: gameState.gameOver,
        winner: gameState.winner,
        history: JSON.parse(JSON.stringify(gameState.moveHistory)),
        lockedPositions: Array.from(gameState.lockedPositions),
        timestamp: Date.now()
    });
}

// 更新状态栏
function updateStatusBar() {
    if (gameState.gameOver) {
        switch (gameState.winner) {
            case GAME_CONSTANTS.PLAYERS.HUMAN:
                DOM_ELEMENTS.status.innerText = '🏆 你赢了！';
                break;
            case GAME_CONSTANTS.PLAYERS.AI:
                DOM_ELEMENTS.status.innerText = '🤖 AI 赢了';
                break;
            case 'draw':
                DOM_ELEMENTS.status.innerText = '🤝 平局';
                break;
            default:
                DOM_ELEMENTS.status.innerText = '⚡ 游戏结束';
        }
    } else {
        DOM_ELEMENTS.status.innerText = gameState.currentPlayer === GAME_CONSTANTS.PLAYERS.HUMAN
            ? '✨ 你的回合 (X)'
            : '🤖 AI 思考中 ...';
    }
}

// 清理挂起的AI请求
function clearPendingRequest() {
    if (gameState.pendingTimeout) {
        clearTimeout(gameState.pendingTimeout);
        gameState.pendingTimeout = null;
    }
    gameState.isRequestPending = false;
}

// 向AI控制台请求落子
function requestAIMove() {
    if (gameState.gameOver || gameState.currentPlayer !== GAME_CONSTANTS.PLAYERS.AI || gameState.isRequestPending) {
        return;
    }

    clearPendingRequest();
    gameState.isRequestPending = true;

    // 发送AI请求
    channel.postMessage({
        type: 'aiMoveRequest',
        board: JSON.parse(JSON.stringify(gameState.board)),
        player: GAME_CONSTANTS.PLAYERS.AI,
        history: JSON.parse(JSON.stringify(gameState.moveHistory)),
        lockedPositions: Array.from(gameState.lockedPositions),
        requestId: Date.now() + Math.random()
    });

    // 超时保护
    gameState.pendingTimeout = setTimeout(() => {
        if (gameState.isRequestPending) {
            console.warn('AI响应超时，请确保AI控制台已打开并启用自动响应');
            gameState.isRequestPending = false;
        }
    }, GAME_CONSTANTS.AI_TIMEOUT);
}

// 执行AI落子
function applyAIMove(row, col, message) {
    // 验证落子合法性
    if (!isValidMove(row, col, GAME_CONSTANTS.PLAYERS.AI)) {
        return false;
    }

    // 执行落子
    makeMove(row, col, GAME_CONSTANTS.PLAYERS.AI);

    // 检查游戏状态
    checkGameState();
    clearPendingRequest();

    // 切换玩家或结束游戏
    if (!gameState.gameOver) {
        gameState.currentPlayer = GAME_CONSTANTS.PLAYERS.HUMAN;
    }

    // 更新UI和广播
    renderBoard();
    updateUIAndBroadcast();

    // 显示AI消息
    if (message) {
        console.log('🤖 AI 消息:', message);
    }

    return true;
}

// 人类点击格子
function handleCellClick(e) {
    if (gameState.gameOver || gameState.currentPlayer !== GAME_CONSTANTS.PLAYERS.HUMAN) {
        return;
    }

    const row = parseInt(e.currentTarget.dataset.row);
    const col = parseInt(e.currentTarget.dataset.col);

    // 验证落子合法性
    if (!isValidMove(row, col, GAME_CONSTANTS.PLAYERS.HUMAN)) {
        return;
    }

    // 执行落子
    makeMove(row, col, GAME_CONSTANTS.PLAYERS.HUMAN);

    // 检查游戏状态
    checkGameState();

    // 切换到AI或结束游戏
    if (!gameState.gameOver) {
        gameState.currentPlayer = GAME_CONSTANTS.PLAYERS.AI;
        updateUIAndBroadcast();
        requestAIMove();
    } else {
        updateUIAndBroadcast();
    }

    // 重新渲染棋盘
    renderBoard();
}

// 验证落子是否合法
function isValidMove(row, col, player) {
    // 检查游戏是否结束
    if (gameState.gameOver) return false;

    // 检查是否是当前玩家的回合
    if (gameState.currentPlayer !== player) return false;

    // 检查位置是否在棋盘范围内
    if (row < 0 || row >= GAME_CONSTANTS.BOARD_SIZE || col < 0 || col >= GAME_CONSTANTS.BOARD_SIZE) {
        return false;
    }

    // 检查位置是否被占用
    if (gameState.board[row][col] !== '') return false;

    // 检查位置是否被锁定
    const positionKey = `${row}-${col}`;
    if (gameState.lockedPositions.has(positionKey)) return false;

    // 对于AI，检查是否有挂起的请求
    if (player === GAME_CONSTANTS.PLAYERS.AI && !gameState.isRequestPending) {
        return false;
    }

    return true;
}

// 执行落子
function makeMove(row, col, player) {
    // 放置棋子
    gameState.board[row][col] = player;

    // 记录落子历史
    gameState.moveHistory.push({ row, col, player });

    // 检查棋子数量限制
    checkPieceLimit(player);
}

// 检查棋子数量限制
function checkPieceLimit(player) {
    // 统计该玩家的棋子数量
    let count = 0;
    for (let row = 0; row < GAME_CONSTANTS.BOARD_SIZE; row++) {
        for (let col = 0; col < GAME_CONSTANTS.BOARD_SIZE; col++) {
            if (gameState.board[row][col] === player) {
                count++;
            }
        }
    }

    // 如果超过限制，移除最早放置的棋子
    if (count > GAME_CONSTANTS.MAX_PIECES) {
        removeEarliestPiece(player);
    }
}

// 移除最早放置的棋子
function removeEarliestPiece(player) {
    // 找到最早放置的该玩家的棋子
    const earliestMove = gameState.moveHistory.find(move => move.player === player);
    if (earliestMove) {
        // 移除该棋子
        gameState.board[earliestMove.row][earliestMove.col] = '';

        // 从历史记录中移除
        gameState.moveHistory = gameState.moveHistory.filter(move =>
            !(move.row === earliestMove.row && move.col === earliestMove.col && move.player === player)
        );

        console.log(`移除了${player}的最早棋子: (${earliestMove.row}, ${earliestMove.col})`);
    }
}

// 生成初始棋子
function generateInitialPieces() {
    // 清空棋盘
    gameState.board = createEmptyBoard();
    gameState.moveHistory = [];

    // 生成人类玩家的5个随机棋子
    placeRandomPieces(GAME_CONSTANTS.PLAYERS.HUMAN, 5);

    // 生成AI玩家的10个随机棋子
    placeRandomPieces(GAME_CONSTANTS.PLAYERS.AI, 10);
}

// 随机放置指定数量的棋子
function placeRandomPieces(player, count) {
    let placed = 0;
    while (placed < count) {
        const row = Math.floor(Math.random() * GAME_CONSTANTS.BOARD_SIZE);
        const col = Math.floor(Math.random() * GAME_CONSTANTS.BOARD_SIZE);

        if (gameState.board[row][col] === '') {
            gameState.board[row][col] = player;
            gameState.moveHistory.push({ row, col, player });
            placed++;
        }
    }
}

// 锁定10个随机空位
function lockRandomEmptyPositions() {
    // 清空锁定位置
    gameState.lockedPositions.clear();

    // 收集所有空位置
    const emptyPositions = [];
    for (let row = 0; row < GAME_CONSTANTS.BOARD_SIZE; row++) {
        for (let col = 0; col < GAME_CONSTANTS.BOARD_SIZE; col++) {
            if (gameState.board[row][col] === '') {
                emptyPositions.push({ row, col });
            }
        }
    }

    // 随机选择10个位置锁定
    const positionsToLock = Math.min(10, emptyPositions.length);
    for (let i = 0; i < positionsToLock; i++) {
        const randomIndex = Math.floor(Math.random() * emptyPositions.length);
        const { row, col } = emptyPositions.splice(randomIndex, 1)[0];
        gameState.lockedPositions.add(`${row}-${col}`);
    }

    console.log(`锁定了 ${gameState.lockedPositions.size} 个空位:`, Array.from(gameState.lockedPositions));
}

// 重置游戏
function resetGame() {
    // 生成初始棋子
    generateInitialPieces();

    // 锁定10个空位
    lockRandomEmptyPositions();

    // 重置游戏状态
    gameState.currentPlayer = GAME_CONSTANTS.PLAYERS.HUMAN;
    gameState.gameOver = false;
    gameState.winner = null;
    clearPendingRequest();

    // 更新UI
    renderBoard();
    updateUIAndBroadcast();
}

// 监听来自AI控制台的消息
channel.onmessage = (event) => {
    const data = event.data;
    if (!data || !data.type) return;

    // 处理AI移动响应
    if (data.type === 'aiMoveResponse') {
        if (data.row !== undefined && data.col !== undefined) {
            const row = parseInt(data.row);
            const col = parseInt(data.col);
            const message = data.message;
            applyAIMove(row, col, message);
        }
    }
};

// 页面关闭时释放channel
window.addEventListener('beforeunload', () => {
    channel.close();
});

// 初始化
function initializeGame() {
    buildBoardUI();
    resetGame();
    setupEventListeners();
}

// 设置事件监听器
function setupEventListeners() {
    DOM_ELEMENTS.resetBtn.addEventListener('click', resetGame);
}

// 初始化游戏
initializeGame();