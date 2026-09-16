package StarLTP

// ==== LTP9-Flash 引擎接线（宿主注入入口） ====
// 宿主（crystal_astral）启动时调用 Bridge 完成引擎初始化与真实通道注入；
// 依赖宿主服务的能力（TTS/ASR 测试动作、/ws 回执推送）经 SetTestActionHook / SetDebugPush 注入。

// Bridge 初始化 LTP9-Flash 引擎并注入宿主真实通道（幂等）。
//   - agentInvoker：agent.synergy（Mini-LTP / Node-LTP）的真实通道；
//   - outbound：signal.all/target 与 event.publish 单向通报的接收函数。
func Bridge(agentInvoker func(appID, instruction string) (string, error), outbound func(topic string, payload any)) error {
	var rerr error
	if err := Init(); err != nil {
		rerr = err
	}
	SetAgentInvoker(agentInvoker)
	SetOutbound(outbound)
	return rerr
}
