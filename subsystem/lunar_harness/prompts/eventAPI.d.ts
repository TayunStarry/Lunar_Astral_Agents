// ═══════════════════════════════════════════════════════════
// 事件订阅（event.*）与宿主事件契约
//
// 依赖 commonAPI.d.ts 的 EventLoad / ReturnResult / HandlerResult。
// ═══════════════════════════════════════════════════════════

/** 宿主即将发出的消息（before_sending_amessage 的负载项） */
interface OutgoingMessage {
	/** 消息类型（text / emoji 等，由前端按类型消费） */
	type: string
	/** 消息正文 */
	content: string
	/** 语音音频地址（可选） */
	audio?: string
	/** 是否为本轮高耗时任务的快速应答（可选） */
	auto?: boolean
}

/** 宿主事件契约：payload 为送入数据，return 为应回传的业务结果（空则走默认处理） */
interface EventContract {
	/** 读取消息前：改写即将交给宿主的消息（可清空、追加、移除、重排序） */
	before_reading_message: {
		payload: {
			/** 本轮即将被读取的未读消息 */
			messages: PostMessage[]
		}
		/** 替换后的消息列表；返回空数组等于清空本轮消息 */
		return: {
			messages?: PostMessage[]
		}
	}
	/** 消息发送前：改写、追加或取消即将发出的消息 */
	before_sending_amessage: {
		payload: {
			/** 即将发出的消息 */
			messages: OutgoingMessage[]
		}
		/** messages 替换待发内容（可多条）；cancel=true 取消本次发送 */
		return: {
			messages?: OutgoingMessage[]
			cancel?: boolean
		}
	}
	/** 生活开始前：可取消本次生活 */
	before_life_starts: {
		payload: {
			/** 是否由控制台强制触发 */
			forced: boolean
			/** 本轮目标视频数 */
			videosPerRound: number
			/** 每个视频理解的切片数 */
			slicesPerVideo: number
			/** 本轮可回应消息次数 */
			repliesPerRound: number
		}
		/** cancel=true 取消本次生活；reason 仅用于日志 */
		return: {
			cancel?: boolean
			reason?: string
		}
	}
	/** 生活结束前：纯触发通知，返回值不被使用 */
	before_life_ends: {
		payload: {
			/** 结束形态：finished=正常完成；failed=中途失败 */
			result: 'finished' | 'failed'
			/** 是否由控制台强制触发 */
			forced: boolean
			/** 本轮已看视频数 */
			videosWatched: number
			/** 本轮已用回应次数 */
			repliesUsed: number
			/** 失败原因（result=failed 时） */
			reason?: string
		}
		return: {}
	}
}

/** 宿主事件主题 */
type EventTopic = keyof EventContract

/** 事件处理 */
export const event: {
	/**
	 * 订阅某主题事件，注册一个回调
	 *
	 * @param topic 事件主题字符串
	 * @param handler 收到事件时的回调；同步返回业务结果（`{ return: ... }`）或控制结果
	 * @param priority 可选订阅优先级，仅支持非负整数（0 最高）；未设置则按时间顺序
	 * @returns 订阅 id，用于后续 unsubscribe 退订
	 */
	subscribe<T extends EventTopic>(topic: T, handler: (e: EventLoad<EventContract[T]['payload']>) => ReturnResult<EventContract[T]['return']> | HandlerResult, priority?: number): number
	subscribe(topic: string, handler: (e: EventLoad<Record<string, unknown>>) => HandlerResult, priority?: number): number
	/**
	 * 按订阅 id 退订某个事件
	 *
	 * @param topic 事件主题字符串
	 * @param id subscribe 返回的订阅 id
	 */
	unsubscribe(topic: string, id: number): void
}
