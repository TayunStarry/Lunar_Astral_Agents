package ltpxmarket

import (
	"fmt"
	"sync"
)

// giteeAPIBase Gitee OpenAPI v5 基地址。
// 变量而非常量：单测把它指向本地 httptest 服务，验证列表顺序与截断行为。
var giteeAPIBase = "https://gitee.com/api/v5"

// assetPrefix 市场附件命名前缀（与 Gitee 为 Release 自动附加的源码归档区分，
// 对齐 updatepack 的 lunar_updatepack_ 前缀策略）
const assetPrefix = "ltpx_"

// assetExt 市场附件后缀（琉璃生态标准安装包格式：zip 内容 + .ltpx 后缀）
const assetExt = ".ltpx"

// bodyMarker Release body 的市场标记（市场列表过滤依据，避免混入更新包发布）
const bodyMarker = "[LTPX]"

// defaultTagLimit 标签筛选默认返回数量上限
const defaultTagLimit = 10

// logHookFn 日志钩子（TUI 激活时由宿主注入；未注入时回退控制台打印）
var (
	hookMu       sync.Mutex
	logHookFn    func(msg string)
	progressHook func(percent int)
)

// SetLogHook 注入日志钩子
func SetLogHook(f func(msg string)) {
	hookMu.Lock()
	defer hookMu.Unlock()
	logHookFn = f
}

// SetProgressHook 注入进度回调钩子（percent: 0-100；-1 表示未知进度）
func SetProgressHook(f func(percent int)) {
	hookMu.Lock()
	defer hookMu.Unlock()
	progressHook = f
}

// logf 统一日志出口
func logf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	hookMu.Lock()
	hook := logHookFn
	hookMu.Unlock()
	if hook != nil {
		hook(msg)
		return
	}
	fmt.Println(msg)
}

// reportProgress 上报进度（未注入钩子时静默跳过）
func reportProgress(percent int) {
	hookMu.Lock()
	hook := progressHook
	hookMu.Unlock()
	if hook != nil {
		hook(percent)
	}
}
