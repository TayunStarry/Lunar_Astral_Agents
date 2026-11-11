//go:build windows

package desktop

// host.go 族共享的纯工具函数与确定性辅助。

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	desktopos "LunarSubsystem/LunarHarness/engine/desktop"
)

// Package desktop 桌面操纵：多成员协作的桌面自动化智能体族。
//
// 编纂与规划、启动、执行与核对分布在 desktop 子包中：
//   - plan    桌面规划：自然语言 → 结构化任务 → 下一步决策
//   - perceive 桌面感知：读取 UIA 树与截图
//   - execute 桌面执行：所有会改变桌面状态的动作
//
// 提示词统一放在 prompts/desktop*.md。

// ExtractJSON 从模型输出文本中提取第一组大括号包裹的 JSON 片段。
func ExtractJSON(s string) []byte {
	clean := strings.TrimSpace(s)
	start := strings.IndexByte(clean, '{')
	end := strings.LastIndexByte(clean, '}')
	if start < 0 || end <= start {
		return []byte(s)
	}
	return []byte(clean[start : end+1])
}

// Base64Encode 将字节数据编码为标准 base64 字符串（用于内嵌图片）。
func Base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// EnsureTargetForeground 确保目标应用窗口在前台；无法激活时如实失败，
// 避免后续阶段对着错误窗口（控制台/资源管理器）继续操作。
// 首次启动的应用可能较慢，这里最多等待约 8 秒；等待期间响应 ctx 取消。
func EnsureTargetForeground(ctx context.Context, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		if err := ctx.Err(); err != nil {
			return ErrCanceled
		}
		if desktopos.ForegroundMatches(target) {
			return nil
		}
		activated, _ := CallDesktop(ctx, 3*time.Second, "激活目标窗口", func() (string, error) {
			return desktopos.ActivateLaunchByName(target), nil
		})
		if activated != "" && desktopos.ForegroundMatches(target) {
			Trace("【编排】目标窗口已置前: %s", activated)
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("目标窗口「%s」不在前台且无法激活", target)
		}
		select {
		case <-ctx.Done():
			return ErrCanceled
		case <-time.After(500 * time.Millisecond):
		}
	}
}
