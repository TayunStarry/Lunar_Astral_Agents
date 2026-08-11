package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"LunarSubsystem/lunar_chromedp"
)

// TestResult 单次查询的测试结果
type TestResult struct {
	Query        string
	Success      bool
	Error        string
	FromMemory   bool
	SearchRounds int
	SourceCount  int
	AnswerLen    int
	Duration     time.Duration
}

func main() {
	fmt.Println("============================================")
	fmt.Println("  搜索智能体 — 集成测试 v2")
	fmt.Println("============================================")

	// 配置：模型配置已迁移至 lunar_config.json，此处仅设置记忆库目录和上下文控制
	config := lunar_chromedp.DefaultSearchConfig()
	config.MaxContextTokens = 16384

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

	// 结果收集
	results := make([]TestResult, 0, len(testQueries))
	totalStart := time.Now()

	for i, query := range testQueries {
		fmt.Printf("\n============================================\n")
		fmt.Printf("  测试 %d/%d: %s\n", i+1, len(testQueries), query)
		fmt.Printf("============================================\n")

		startTime := time.Now()

		report, err := lunar_chromedp.Search(query)
		elapsed := time.Since(startTime)

		tr := TestResult{
			Query:    query,
			Duration: elapsed,
		}

		if err != nil {
			fmt.Printf("\n[错误] 搜索失败: %v\n", err)
			tr.Success = false
			tr.Error = err.Error()
			results = append(results, tr)
			continue
		}

		tr.Success = true
		tr.FromMemory = report.FromMemory
		tr.SearchRounds = report.SearchRounds
		tr.SourceCount = len(report.UsedSources)
		tr.AnswerLen = len([]rune(report.Answer))

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

		results = append(results, tr)
	}

	totalElapsed := time.Since(totalStart)

	// ========================================
	// 汇总报告
	// ========================================
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║           测 试 汇 总 报 告              ║")
	fmt.Println("╚══════════════════════════════════════════╝")

	successCount := 0
	failCount := 0
	memoryCount := 0
	var totalAnswerLen int
	var totalSources int
	var totalRounds int
	var maxDuration time.Duration
	var maxDurationQuery string

	for _, r := range results {
		if r.Success {
			successCount++
			if r.FromMemory {
				memoryCount++
			}
			totalAnswerLen += r.AnswerLen
			totalSources += r.SourceCount
			totalRounds += r.SearchRounds
			if r.Duration > maxDuration {
				maxDuration = r.Duration
				maxDurationQuery = r.Query
			}
		} else {
			failCount++
		}
	}

	fmt.Printf("\n📊 基本统计\n")
	fmt.Printf("  总查询数:     %d\n", len(results))
	fmt.Printf("  成功:         %d  ✓\n", successCount)
	fmt.Printf("  失败:         %d  ✗\n", failCount)
	fmt.Printf("  总耗时:       %s\n", totalElapsed.Round(time.Millisecond))

	if successCount > 0 {
		avgDuration := totalElapsed / time.Duration(successCount)
		fmt.Printf("  平均耗时:     %s/查询\n", avgDuration.Round(time.Millisecond))
		fmt.Printf("  最长耗时:     %s (%s)\n", maxDuration.Round(time.Millisecond), truncateStr(maxDurationQuery, 40))
		fmt.Printf("  记忆命中:     %d 次\n", memoryCount)
		fmt.Printf("  平均搜索轮数: %.1f\n", float64(totalRounds)/float64(successCount))
		fmt.Printf("  平均来源数:   %.1f\n", float64(totalSources)/float64(successCount))
		fmt.Printf("  平均答案长度: %d 字\n", totalAnswerLen/successCount)
	}

	fmt.Println("\n📋 详细结果")
	fmt.Printf("  %-4s %-6s %-6s %-6s %-8s %-45s\n", "序号", "状态", "来源", "轮数", "耗时", "查询")
	fmt.Println("  " + strings.Repeat("-", 80))
	for i, r := range results {
		status := "✓"
		source := "网络"
		if !r.Success {
			status = "✗"
			source = "失败"
		} else if r.FromMemory {
			source = "记忆"
		}
		fmt.Printf("  %-4d %-6s %-6s %-6d %-8s %-45s\n",
			i+1, status, source, r.SearchRounds,
			r.Duration.Round(time.Millisecond).String(),
			truncateStr(r.Query, 42))
	}

	// 失败详情
	if failCount > 0 {
		fmt.Println("\n⚠️  失败详情")
		for i, r := range results {
			if !r.Success {
				fmt.Printf("  [%d] %s\n", i+1, r.Query)
				fmt.Printf("      错误: %s\n", truncateStr(r.Error, 120))
			}
		}
	}

	fmt.Println("\n============================================")
	fmt.Println("  全部测试完成")
	fmt.Println("============================================")
}

// truncateStr 截断字符串到指定长度（中文友好）
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
