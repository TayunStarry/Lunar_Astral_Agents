package WebLTP

// Web-LTP 网络搜索模拟测试（真实环境）：
// 模拟月华依次下发四条自然语言检索要求，走完整流水线（搜索 → 结果页滚动截图 →
// 依次进入前 N 个结果页 → 逐页摘要 → 回到搜索引擎页 → 汇编报告）。
// 配置读取真实的 local_data/lunar_config.json（web_search 字段），截图按配置落盘。

import (
	"LunarSubsystem/GeneralConfig"
	"strings"
	"testing"
)

func TestWebSearchSimulation(t *testing.T) {
	// 使用仓库真实的 local_data（lunar_config.json 的 web_search 字段 + 截图落盘目录）
	*GeneralConfig.LocalDir = `D:\Lunar_Astral_Agents\local_data`

	cases := []string{
		"原神最新卡池",
		"DeepSeek最新情报",
		"星月智能 月之华",
		"最终档案馆",
	}
	for _, q := range cases {
		t.Run(q, func(t *testing.T) {
			report, err := Run(q)
			if err != nil {
				t.Fatalf("Run 失败: %v", err)
			}
			if strings.TrimSpace(report) == "" {
				t.Fatal("报告为空")
			}
			t.Logf("报告:\n%s", report)
		})
	}
}
