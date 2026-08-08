package main

import (
	"fmt"
	"os"
	"time"

	"lunar_chromedp"
)

func main() {
	fmt.Println("============================================")
	fmt.Println("  搜索智能体 — 集成测试")
	fmt.Println("============================================")

	// 配置：使用本地 36789 端口的模型服务
	config := lunar_chromedp.DefaultSearchConfig()
	config.MultimodalURL = "http://127.0.0.1:36789/v1"
	config.MultimodalName = "system-multimodal"
	config.EmbeddingURL = "http://127.0.0.1:36789/v1"
	config.EmbeddingName = "system-embedding"
	config.MaxContextTokens = 16384

	// 显式指定使用 Edge 浏览器（自动检测，省去 Chromium 下载）
	// 已在 variable.go 中添加 edgePaths 自动检测，也会尝试 Chrome

	fmt.Println("\n[初始化] 正在连接模型服务器并启动浏览器...")
	if err := lunar_chromedp.InitSearch(config); err != nil {
		fmt.Printf("\n[错误] 初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 测试查询列表
	testQueries := []string{
		"查询一下钛宇星光阁是谁",
		"查询一下我的世界基岩版的彼岸幻梦模组",
		"查询一下钛宇星光阁编写的《最终档案馆》这部小说的情报",
		"查询一下钛宇星光阁的月华的相关情报",
		"查询一下原神最新卡池信息",
		"查询终末地卡池信息",
		"查询明日方舟卡池信息",
		"查询崩坏星穹铁道卡池信息",
		"查询我的世界基岩版最新版本更新信息",
		"查询Qwen模型的最新进展",
		"查询DeepSeek模型的最新进展",
	}

	for i, query := range testQueries {
		fmt.Printf("\n============================================\n")
		fmt.Printf("  测试 %d/%d: %s\n", i+1, len(testQueries), query)
		fmt.Printf("============================================\n")

		startTime := time.Now()

		report, err := lunar_chromedp.Search(query)
		if err != nil {
			fmt.Printf("\n[错误] 搜索失败: %v\n", err)
			continue
		}

		elapsed := time.Since(startTime)

		fmt.Printf("\n============================================\n")
		fmt.Printf("  搜索结果\n")
		fmt.Printf("============================================\n")
		fmt.Printf("查询: %s\n", report.Query)
		fmt.Printf("来源: ")
		if report.FromMemory {
			fmt.Printf("记忆库\n")
		} else {
			fmt.Printf("网络搜索\n")
		}
		fmt.Printf("搜索轮数: %d\n", report.SearchRounds)
		fmt.Printf("耗时: %s\n", elapsed.Round(time.Millisecond))
		fmt.Printf("引用来源数: %d\n", len(report.UsedSources))
		for j, src := range report.UsedSources {
			fmt.Printf("  [%d] %s\n", j+1, src)
		}
		fmt.Printf("\n--- 答案 ---\n%s\n", report.Answer)
		fmt.Printf("--- 结束 ---\n")
	}

	fmt.Println("\n============================================")
	fmt.Println("  全部测试完成")
	fmt.Println("============================================")
}
