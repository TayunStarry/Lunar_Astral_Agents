package app

// LTP9 授权签发的宿主装配：把引擎侧的签发能力注入控制台的「LTP9 授权签发」屏。
//
// 为什么由 app 层做转换而不让控制台直接依赖引擎包：控制台是通用 UI 子系统，不认识
// CorePersonality 的引擎；引擎也不应认识 TUI 的展示结构。中间这一层只做结构映射，
// 签发规则本身留在引擎侧（与运行时校验共用同一份实现）。

import (
	StarLTP "CorePersonality/engine/ltp9"
	"LunarSubsystem/TuiSystem"
)

// WireTUILTP9AuthHooks 注入 LTP9 授权能力（须在 TuiSystem.Run() 之前调用）：
//   - inspect：读取某包的授权现状（不修改文件）；
//   - sign：依据脚本现有引用的权限签发授权（覆盖写 permissions.key）。
func WireTUILTP9AuthHooks() {
	TuiSystem.SetLTP9AuthHooks(
		func(dir string) (TuiSystem.LTP9AuthStatus, error) {
			auth, err := StarLTP.InspectPackageAuth(dir)
			return toTuiLTP9Auth(auth), err
		},
		func(dir string) (TuiSystem.LTP9AuthStatus, error) {
			auth, err := StarLTP.SignPackageAuth(dir)
			return toTuiLTP9Auth(auth), err
		},
	)
}

// toTuiLTP9Auth 引擎侧授权现状 → 控制台展示结构
func toTuiLTP9Auth(auth StarLTP.PackageAuth) TuiSystem.LTP9AuthStatus {
	return TuiSystem.LTP9AuthStatus{
		HasScript: auth.HasScript,
		HasKey:    auth.HasKey,
		Signed:    auth.Signed,
		Granted:   auth.Granted,
		Required:  auth.Required,
		Reasons:   auth.Reasons,
		Hash:      auth.Hash,
	}
}
