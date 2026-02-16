import * as EntryAPI from './code';

// 类型定义
type BoardState = string[][];

type Move = {
    row: number;
    col: number;
    player: string;
};

type AIMoveRequest = {
    type: 'aiMoveRequest';
    board: BoardState;
    history?: Move[];
    lockedPositions?: string[];
};

type AIMoveResponse = {
    type: 'aiMoveResponse';
    row: number;
    col: number;
    timestamp: number;
    message?: string;
};

type StateUpdate = {
    type: 'stateUpdate';
    board: BoardState;
    currentPlayer: string;
    gameOver: boolean;
    winner: string | null;
    history?: Move[];
    lockedPositions?: string[];
};

/**
 * 游戏 AI 管理器
 * 负责处理广播消息，与 AI 交互，发送落子响应
 */
class GameAIManager {
    private channel: BroadcastChannel;
    private lastKnownBoard: BoardState | null = null;
    private lastKnownHistory: Move[] = [];
    private lastKnownLockedPositions: Set<string> = new Set();
    private isProcessing: boolean = false;

    constructor() {
        // 初始化广播频道
        this.channel = new BroadcastChannel('ttt_ai_channel');
        this.setupEventListeners();
    }

    /**
     * 设置事件监听器
     */
    private setupEventListeners(): void {
        // 监听广播消息
        this.channel.onmessage = async (event) => {
            try {
                const data = event.data;
                if (!data || !data.type) return;

                // 处理棋盘状态更新
                if (data.type === 'stateUpdate') {
                    this.handleStateUpdate(data as StateUpdate);
                }

                // 处理 AI 移动请求
                if (data.type === 'aiMoveRequest') {
                    await this.handleAIMoveRequest(data as AIMoveRequest);
                }
            } catch (error) {
                console.error('处理广播消息时出错:', error);
                EntryAPI.showSystemMessage(`处理游戏消息时出错: ${error}`, 'error');
            }
        };

        // 页面关闭时释放资源
        window.addEventListener('beforeunload', () => {
            this.channel.close();
        });
    }

    /**
     * 处理棋盘状态更新
     * @param update 状态更新消息
     */
    private handleStateUpdate(update: StateUpdate): void {
        if (update.board) {
            // 深拷贝棋盘状态，避免引用问题
            this.lastKnownBoard = JSON.parse(JSON.stringify(update.board));
        }
        if (update.history) {
            // 保存历史记录，最多保留10步
            this.lastKnownHistory = update.history.slice(-10);
        }
        if (update.lockedPositions) {
            // 保存锁定位置
            this.lastKnownLockedPositions = new Set(update.lockedPositions);
        }
    }

    /**
     * 处理 AI 移动请求
     * @param request AI 移动请求
     */
    private async handleAIMoveRequest(request: AIMoveRequest): Promise<void> {
        // 避免并发处理多个请求
        if (this.isProcessing) {
            console.warn('正在处理另一个 AI 移动请求，跳过当前请求');
            return;
        }

        try {
            this.isProcessing = true;
            const { board, history, lockedPositions } = request;

            // 验证棋盘数据
            if (!board || !Array.isArray(board) || board.length !== 10 || board[0].length !== 10) {
                console.error('无效的棋盘数据:', board);
                EntryAPI.showSystemMessage('无效的棋盘数据', 'error');
                return;
            }

            // 检查棋盘是否已满
            if (this.isBoardFull(board)) {
                console.warn('棋盘已满，无法落子');
                return;
            }

            // 更新锁定位置信息
            if (lockedPositions) {
                this.lastKnownLockedPositions = new Set(lockedPositions);
            }

            // 生成优化的提示词
            const prompt = this.generateOptimizedPrompt(board, history || this.lastKnownHistory);

            // 构建消息对象
            const messages: EntryAPI.PostMessage[] = [
                {
                    role: 'system',
                    content: '你是一个"星空五子棋"游戏的 AI 助手，是一位精通棋类策略的高手。你擅长分析棋盘局势，具有强烈的进攻性和防守意识。游戏规则：棋盘为10x10，先连成10子者胜（横向、纵向或对角线完整连线），双方各有10颗棋子，超过10颗时最早放置的棋子会随机消失。你的目标是：1. 积极进攻，尽快形成10子连线；2. 严密防守，阻止对手形成10子连线；3. 制定战略，控制棋盘关键点和整行整列；4. 根据局势选择是否添加一条消息与玩家交流，展现你的棋艺和自信。'
                },
                {
                    role: 'user',
                    content: prompt
                }
            ];

            // 调用 AI 服务
            console.log('发送请求给 AI...');
            const aiResponse = await this.getAIResponse(messages);

            // 解析 AI 响应
            const move = this.parseAIResponse(aiResponse);

            // 验证落子位置
            if (move && this.isValidMove(board, move.row, move.col)) {
                // 发送落子响应
                this.sendMoveResponse(move.row, move.col, move.message);
                console.log(`🤖 AI 决策: 落子 (${move.row}, ${move.col})`);
                if (move.message) {
                    console.log(`🤖 AI 消息: ${move.message}`);
                    // 播放 AI 消息
                    if (EntryAPI.RandomFloat(0, 100) > 75) EntryAPI.playSpeechModel(move.message);
                }
            } else {
                // 如果 AI 响应无效，使用备用策略
                console.warn('AI 响应无效，使用备用策略');
                const fallbackMove = this.getFallbackMove(board);
                if (fallbackMove) {
                    this.sendMoveResponse(fallbackMove.row, fallbackMove.col);
                    console.log(`🤖 备用策略: 落子 (${fallbackMove.row}, ${fallbackMove.col})`);
                }
            }
        } catch (error) {
            console.error('处理 AI 移动请求时出错:', error);
            EntryAPI.showSystemMessage(`处理 AI 移动请求时出错: ${error}`, 'error');

            // 出错时使用备用策略
            if (request.board) {
                const fallbackMove = this.getFallbackMove(request.board);
                if (fallbackMove) {
                    this.sendMoveResponse(fallbackMove.row, fallbackMove.col);
                    console.log(`🤖 出错备用: 落子 (${fallbackMove.row}, ${fallbackMove.col})`);
                }
            }
        } finally {
            this.isProcessing = false;
        }
    }

    /**
     * 生成优化的提示词
     * @param board 棋盘状态
     * @param history 历史记录
     * @returns 优化后的提示词
     */
    private generateOptimizedPrompt(board: BoardState, history: Move[]): string {
        // 将棋盘转换为更直观的格式
        const boardStr = board.map((row, rIndex) => {
            return `${rIndex}: ${row.map((cell, cIndex) => cell || '-').join(' | ')}`;
        }).join('\n');

        // 生成历史记录字符串
        const historyStr = history.length > 0 ?
            `## 历史记录\n${history.map((move, index) =>
                `步骤 ${index + 1}: ${move.player} 在 (${move.row}, ${move.col}) 落子`
            ).join('\n')}\n\n` : '';

        return `# 星空五子棋游戏分析\n\n## 当前棋盘状态\n${boardStr}\n\n${historyStr}## 任务\n作为星空五子棋 AI，请分析当前棋盘状态，使用 O 棋子给出下一步最佳落子位置。\n\n## 游戏规则\n1. 棋盘为 10x10，行和列范围为 0-9\n2. 先连成10子者胜（横向、纵向或对角线完整连线）\n3. 双方各有 10 颗棋子，超过 10 颗时最早放置的棋子会随机消失\n4. 落子位置必须为空（即当前为 '' 或 '-'）\n5. 注意：部分空位可能被锁定，无法落子，落子前请确保位置有效\n\n## 策略指导\n1. **进攻优先**：寻找机会形成自己的10子连线，控制整行、整列或整条对角线\n2. **严密防守**：当对手有形成10子连线的趋势时，必须立即阻挡\n3. **控制关键点**：抢占棋盘中心和边缘的关键位置，控制局势\n4. **形成攻势**：创造多个进攻点，让对手顾此失彼\n5. **分析历史**：参考历史记录，了解对手的落子习惯和策略\n\n## 严格要求\n1. **必须以纯 JSON 格式返回结果**，不要添加任何额外的文字、解释或说明\n2. **JSON 必须包含 row 和 col 字段**，分别表示行和列，值为 0-9 之间的整数\n3. **可选包含 message 字段**，表示你想对玩家说的话，必须是字符串类型\n4. **确保 JSON 格式正确**，可以被标准 JSON.parse() 方法直接解析\n5. **只返回 JSON 对象**，不要包含任何其他内容\n6. **落子位置必须有效**：确保位置为空且未被锁定\n\n## 正确示例输出\n{"row": 0, "col": 0, "message": "我要开始进攻了！"}\n\n## 错误示例（不要这样做）\n// 我认为最佳落子位置是...\n{"row": 0, "col": 0}\n\n或者\n\n{"row": 0, "col": 0}\n// 这是我的选择`;
    }

    /**
     * 调用 AI 服务获取响应
     * @param messages 消息数组
     * @returns AI 响应内容
     */
    private async getAIResponse(messages: EntryAPI.PostMessage[]): Promise<string> {
        try {
            // 调用多模态 API
            const response = await new EntryAPI.MultimodalRequest(
                messages,
                false,
                false,
                false
            ).response;

            const chatAnswer = await response.json();

            // 提取 AI 响应内容
            if (chatAnswer && chatAnswer.choices && chatAnswer.choices.length > 0) {
                return chatAnswer.choices[0].message.content;
            }

            throw new Error('未收到 AI 响应');
        } catch (error) {
            console.error('调用 AI 服务时出错:', error);
            throw error;
        }
    }

    /**
     * 解析 AI 响应
     * @param response AI 响应内容
     * @returns 落子位置或 null
     */
    private parseAIResponse(response: string): { row: number; col: number; message?: string } | null {
        try {
            console.log('AI 原始响应:', response);

            // 清理响应内容，去除首尾空白
            const cleanResponse = response.trim();

            // 尝试直接解析 JSON
            try {
                const move = JSON.parse(cleanResponse);
                console.log('直接解析 JSON 成功:', move);
                if (this.isValidMoveObject(move)) {
                    return move;
                } else {
                    console.error('JSON 格式正确但内容无效:', move);
                }
            } catch (e) {
                console.log('直接解析 JSON 失败，尝试提取 JSON 部分:', e.message);
                // 如果直接解析失败，尝试提取 JSON 部分
                // 改进的正则表达式，尝试匹配完整的 JSON 对象
                const jsonMatch = cleanResponse.match(/\{[\s\S]*\}/);
                if (jsonMatch) {
                    try {
                        const move = JSON.parse(jsonMatch[0]);
                        console.log('提取并解析 JSON 成功:', move);
                        if (this.isValidMoveObject(move)) {
                            return move;
                        } else {
                            console.error('提取的 JSON 内容无效:', move);
                        }
                    } catch (innerError) {
                        console.error('提取 JSON 后解析失败:', innerError);
                    }
                } else {
                    console.error('未找到 JSON 部分');
                }
            }

            console.error('无法解析 AI 响应:', cleanResponse);
            return null;
        } catch (error) {
            console.error('解析 AI 响应时出错:', error);
            return null;
        }
    }

    /**
     * 验证落子对象是否有效
     * @param move 落子对象
     * @returns 是否有效
     */
    private isValidMoveObject(move: any): move is { row: number; col: number; message?: string } {
        return (
            typeof move === 'object' &&
            move !== null &&
            typeof move.row === 'number' &&
            typeof move.col === 'number' &&
            move.row >= 0 &&
            move.row <= 9 &&
            move.col >= 0 &&
            move.col <= 9 &&
            (typeof move.message === 'undefined' || typeof move.message === 'string')
        );
    }

    /**
     * 验证落子位置是否有效
     * @param board 棋盘状态
     * @param row 行
     * @param col 列
     * @returns 是否有效
     */
    private isValidMove(board: BoardState, row: number, col: number): boolean {
        // 检查位置是否为空且未被锁定
        const positionKey = `${row}-${col}`;
        return board[row][col] === '' && !this.lastKnownLockedPositions.has(positionKey);
    }

    /**
     * 检查棋盘是否已满
     * @param board 棋盘状态
     * @returns 是否已满
     */
    private isBoardFull(board: BoardState): boolean {
        for (let row = 0; row < 10; row++) {
            for (let col = 0; col < 10; col++) {
                if (board[row][col] === '') {
                    return false;
                }
            }
        }
        return true;
    }

    /**
     * 获取备用落子位置（随机选择空位置）
     * @param board 棋盘状态
     * @returns 落子位置或 null
     */
    private getFallbackMove(board: BoardState): { row: number; col: number } | null {
        const emptyCells: { row: number; col: number }[] = [];

        for (let row = 0; row < 10; row++) {
            for (let col = 0; col < 10; col++) {
                const positionKey = `${row}-${col}`;
                if (board[row][col] === '' && !this.lastKnownLockedPositions.has(positionKey)) {
                    emptyCells.push({ row, col });
                }
            }
        }

        if (emptyCells.length === 0) {
            return null;
        }

        const randomIndex = Math.floor(Math.random() * emptyCells.length);
        return emptyCells[randomIndex];
    }

    /**
     * 发送落子响应
     * @param row 行
     * @param col 列
     * @param message AI 消息
     */
    private sendMoveResponse(row: number, col: number, message?: string): void {
        const response: AIMoveResponse = {
            type: 'aiMoveResponse',
            row: row,
            col: col,
            timestamp: Date.now(),
            message: message
        };

        console.log('发送落子响应:', response);
        this.channel.postMessage(response);
    }
}

// 导出初始化函数
export function initializeGameAI(): void {
    console.log('游戏 AI 管理器初始化完成');
    new GameAIManager()
}

// 立即初始化
initializeGameAI();
