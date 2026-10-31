package channel

// ==== Gitee 接口错误：分类与人话提示 ====
//
// 两条通道（更新包 / LTPX 市场）共用同一仓库与同一套接口，因此错误处理也共用：
//
//   - Gitee 对**未认证**请求的额度极低（按 IP 计），一旦打满就返回
//     `403 Forbidden (Rate Limit Exceeded)`。用户实测「应用市场与更新包列表都拉不到」
//     正是这个原因——配置里没填令牌，所有列表请求都是匿名额度。
//   - 因此错误必须分类：限流要给出**可操作**的建议（去填令牌 / 稍后重试），
//     而不是把 `HTTP 403: 403 Forbidden (Rate Limit Exceeded)` 原样糊在界面上。

import (
	"errors"
	"fmt"
	"strings"
)

// APIError Gitee 接口返回的错误（非 200）
type APIError struct {
	Status int    // HTTP 状态码
	Path   string // 请求路径（不含令牌）
	Body   string // 响应体摘要（截断）
}

func (e *APIError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("HTTP %d: %s（%s）", e.Status, e.Body, e.Path)
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body)
}

// RateLimited 是否为「额度用尽」类错误
func (e *APIError) RateLimited() bool {
	if e.Status != 403 && e.Status != 429 {
		return false
	}
	body := strings.ToLower(e.Body)
	return strings.Contains(body, "rate limit") ||
		strings.Contains(body, "too many requests") ||
		strings.Contains(body, "超过") ||
		strings.Contains(body, "限制")
}

// AsAPIError 提取接口错误（非接口错误返回 nil）
func AsAPIError(err error) *APIError {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return nil
}

// IsRateLimited 判断错误是否为接口限流
func IsRateLimited(err error) bool {
	if apiErr := AsAPIError(err); apiErr != nil {
		return apiErr.RateLimited()
	}
	return false
}

// FriendlyAPIError 把接口错误转成「用户看得懂、且能照做」的提示；非接口错误原样返回。
//
// hasToken 表示本次请求是否带了令牌——这决定建议是「去填令牌」还是「等额度恢复」。
func FriendlyAPIError(err error, hasToken bool) string {
	if err == nil {
		return ""
	}
	apiErr := AsAPIError(err)
	if apiErr == nil {
		return err.Error()
	}
	switch {
	case apiErr.RateLimited() && !hasToken:
		return "Gitee 接口限流：未配置令牌时只能使用匿名额度（按 IP 计，很快耗尽）。" +
			"请在 local_data/release.json 的 gitee_token 填入 Gitee 令牌，" +
			"或在发布/安装流程中按提示输入令牌（输入后本次会话的列表请求都会带上它）；随后重试。"
	case apiErr.RateLimited():
		return "Gitee 接口限流：当前令牌的调用额度已用尽，请稍后重试。"
	case apiErr.Status == 401:
		return "Gitee 鉴权失败：令牌无效或已过期，请检查 local_data/release.json 的 gitee_token。"
	case apiErr.Status == 403:
		return "Gitee 拒绝访问：令牌权限不足（需要仓库的读写权限）。"
	case apiErr.Status == 404:
		return "Gitee 未找到目标：请核对仓库 owner/repo 与发布 tag 是否正确。"
	case apiErr.Status >= 500:
		return "Gitee 服务暂时不可用，请稍后重试。"
	default:
		return apiErr.Error()
	}
}
