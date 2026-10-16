import { GlobalConfig } from '../config/global';
import { PostMessage } from '../config/model';
import { dialogueRole, memorizerRole } from './roles/roles';

// ===== 律令指令机制 =====
// 未读消息中出现 <指令> 格式文本时，若命中已定义的律令，则跳过本轮 AI 应答：
// 触发对应的默认应答文本（仍走 TTS 流程），并将携带律令指令的消息从未读消息中剔除，
// 未读消息中的其他内容保留到下一周期再解析。

/** 律令指令处理函数（message 为携带该指令的原始未读消息） */
type DecreeHandler = (message: PostMessage) => void;

/** 推送律令默认应答：文本 + TTS 合成（合成失败时仅推送文本） */
function pushDecreeResponse(text: string): void {
    const [audio, err] = tts(text);
    pushContext('text', text, (err || !audio) ? '' : audio);
    console.log(`[律令] 默认应答: ${text}`);
}

/** 律令指令注册表：指令标签（不含尖括号）→ 处理函数 */
const decreeRegistry: Record<string, DecreeHandler> = {
    /** <全记住>：立刻将对话者的全部上下文消息推入记忆缓冲并立即写入记忆库（省去排队） */
    '全记住': () => {
        GlobalConfig.unreadRecords.push(...dialogueRole.messages);
        memorizerRole.persistUnreadRecords();
        pushDecreeResponse('全部记住啦~');
    },
};

/** 提取消息中的文本内容（多模态消息仅取文本项） */
function extractDecreeText(message: PostMessage): string {
    if (typeof message.content === 'string') return message.content;
    return message.content
        .filter((c: any) => c.type === 'text')
        .map((c: any) => c.text)
        .join('\n');
}

/** 匹配消息文本中命中的已知律令指令（按注册表顺序去重） */
function matchKnownDecrees(text: string): string[] {
    const found: string[] = [];
    for (const tag of Object.keys(decreeRegistry)) {
        if (text.includes(`<${tag}>`)) found.push(tag);
    }
    return found;
}

/**
 * 处理未读消息中的律令指令。
 * 命中任意律令时：执行对应默认动作与默认应答（含 TTS），并从 unreadContext 剔除
 * 携带律令的消息（未读消息中的其他内容保留到下一周期解析）。
 *
 * @returns 是否命中律令指令（命中时应跳过本轮 AI 应答）
 */
export function processDecrees(): boolean {
    let hit = false;
    const remaining: PostMessage[] = [];
    for (const message of GlobalConfig.unreadContext) {
        const decrees = matchKnownDecrees(extractDecreeText(message));
        if (decrees.length === 0) {
            remaining.push(message);
            continue;
        }
        hit = true;
        for (const tag of decrees) {
            try {
                decreeRegistry[tag](message);
            }
            catch (error) {
                console.error(`[律令] 指令 <${tag}> 执行失败:`, error);
            }
        }
    }
    GlobalConfig.unreadContext = remaining;
    return hit;
}
