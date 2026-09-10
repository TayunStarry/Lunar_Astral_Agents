/**
 * 琉璃（LTPX）事件交互原语
 * 依赖隔离：本模块仅依赖全局注入的 ltpInteractEvent（由 Go 适配器注册），不 import 任何其他
 * TypeScript 模块，从而避免角色文件（roles/*）引入本模块时与 capabilities/ltpx.ts（其依赖 roles）
 * 形成循环依赖。
 */

/** 事件交互结果：订阅器经 `return` 字段回传的业务结果 */
export interface EventFeedback<R = any> {
    /** 插件回传的业务结果；琉璃离线 / 无插件订阅 / 未回传时为 undefined */
    return?: R;
}

/**
 * 在每个「xx事件发生前」触发点调用：把原始负载同步推送到琉璃，经 LTP9 插件处理。
 *
 * 约定：订阅器以 `{ return: 业务结果 }` 回传本次事件的业务结果。琉璃在线且确有订阅器回传时，
 * 返回对象的 `return` 即该业务结果；否则（琉璃离线 / 无插件订阅 / 未回传 / 异常）`return`
 * 为 undefined，调用方回退本地默认处理。
 *
 * @param topic 事件主题（如 `message_received_before`）
 * @param payload 事件负载对象（推送给琉璃的原始数据）
 * @returns 形如 `{ return }` 的结果对象；业务结果置于 `return` 字段
 */
export function interactEvent<R = any>(topic: string, payload: any): EventFeedback<R> {
    try {
        const resultJSON = ltpInteractEvent(topic, JSON.stringify(payload ?? {}));
        if (!resultJSON) return {};
        const res = JSON.parse(resultJSON);
        // 琉璃在线且订阅器确实回传了业务结果 → 采用该结果
        if (res && res.online && res.returned && res.return !== null && res.return !== undefined) {
            return { return: res.return as R };
        }
    } catch (e) {
        console.error('LTPX 事件交互失败:', e);
    }
    return {};
}
