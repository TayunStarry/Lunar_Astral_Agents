package WebLTP

// Web-LTP 网络搜索模拟测试（真实环境）：
// 模拟月华依次下发五条自然语言检索要求，走完整流水线（引擎降级链 → 结果页滚动截图 →
// 依次进入前 N 个结果页 → 缓存命中复用 SQL 摘要 / 未命中实时访问并摘要 → 回到搜索引擎页 → 汇编报告）。
// 配置读取仓库根 local_data/lunar_config.json 的 web_search 字段，截图按配置落盘；
// 仓库根以 subsystem/build_common.ps1 为标记向上定位，不硬编码绝对路径。

import (
	"LunarSubsystem/GeneralConfig"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findRepoRoot 从当前目录向上查找仓库根（以 subsystem/build_common.ps1 为标记）
func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "subsystem", "build_common.ps1")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func TestWebSearchSimulation(t *testing.T) {
	root := findRepoRoot()
	if root == "" {
		t.Skip("未找到仓库根目录（缺少 subsystem/build_common.ps1），跳过真实环境测试")
	}
	*GeneralConfig.LocalDir = filepath.Join(root, "local_data")

	cases := []string{
		"原神最新卡池",
		"DeepSeek最新情报",
		"星月智能 月之华",
		"最终档案馆",
		"彼岸幻梦模组",
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
