// Package protocolwire 把 engine/config 注入 protocol 层，解除 protocol 对 engine 的反向依赖。
//
// 只做装配：protocol 声明提供者接口，本包在显式 Wire 时注册读取 config 的实现。
// 不依赖 init 隐式顺序，由 app.Assemble 在启动早期显式调用；测试可重复调用（幂等）。
package protocolwire

import (
	"sync"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/loopguard"
)

var once sync.Once

// Wire 注册 protocol 层所需的配置提供者；多次调用只生效一次。
func Wire() {
	once.Do(func() {
		agentbase.SetModelEndpointProvider(func() agentbase.ModelEndpoint {
			return agentbase.ModelEndpoint{
				URL:   config.AgentMultimodalURL,
				Model: config.AgentMultimodalModel,
			}
		})
		loopguard.SetDefaultOptionsProvider(func() loopguard.Options {
			return loopguard.Options{
				NodeRunes:    config.LoopNodeRunes,
				SegmentRunes: config.LoopSegmentRunes,
				Similarity:   config.LoopSimilarity,
			}
		})
	})
}
