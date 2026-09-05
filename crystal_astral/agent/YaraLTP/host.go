package YaraLTP

// ==== 包级入口：Init / Close / Run ====

import (
	"encoding/json"
	"fmt"
	"time"

	"LunarSubsystem/LoggerGeneral"
)

// initDone 保证 Init 幂等。
var initDone bool

// Init 初始化 LTP3 引擎：扫描包目录并加载全部 LTP3 插件，启动对账循环。
// 由 crystal_astral 在 WebSocket 集线器就绪后、注入发送函数后调用。
func Init() error {
	if initDone {
		return nil
	}
	Engine = newEngine()
	Engine.LoadAll()
	Engine.startReconcile()
	initDone = true
	LoggerGeneral.Info(ServiceName, "LTP3 引擎已初始化，插件数: %d", Engine.pluginCount())
	return nil
}

// Close 关闭引擎：停止对账循环并卸载全部插件。
func Close() {
	if Engine != nil {
		Engine.stopReconcile()
		Engine.shutdown()
	}
	initDone = false
}

// Run 处理一条来自月华的指令（经 /ltpx/call 的内置 yara_ltp 工具调用）。
// 将指令文本作为一条聊天消息，路由到默认钩子点，交由所有订阅的插件订阅器处理。
func Run(instruction string) (resultText string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("LTP3 引擎内部异常: %v", r)
		}
	}()
	if Engine == nil {
		return "", fmt.Errorf("LTP3 引擎未初始化")
	}
	msg := &YaraMessage{
		ID:        fmt.Sprintf("yara-%d", time.Now().UnixNano()),
		Content:   instruction,
		Timestamp: time.Now().UnixMilli(),
		Platform:  "crystal_astral",
	}
	outs, summary := Engine.DispatchHook(defaultHookTopic, msg, nil, "")
	b, _ := json.Marshal(map[string]any{
		"hook":      defaultHookTopic,
		"subscribed": summary.Subscribed,
		"errored":   summary.Errored,
		"allow_continue": summary.AllowContinue,
		"results":   outs,
	})
	return string(b), nil
}