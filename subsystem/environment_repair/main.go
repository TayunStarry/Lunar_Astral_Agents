package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	config "LunarSubsystem/GeneralConfig"
)

func main() {
	printBanner()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		printMenu()
		fmt.Print("\n请输入选项编号 (1-5): ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		switch input {
		case "1":
			runResourceRepair()
		case "2":
			runPortRelease()
		case "3":
			runHTTPSProxy()
		case "4":
			runPackageArchive()
		case "5":
			fmt.Println("\n感谢使用「星月智能 · 环境修复工具」，再见！")
			return
		default:
			fmt.Println("无效选项，请重新输入。")
		}
	}
	_ = scanner.Err()
}

func printBanner() {
	fmt.Println(strings.Repeat("═", 56))
	fmt.Println("  ✦  星月智能 · 环境修复工具  ✦")
	fmt.Println("  Environment Repair Tool")
	fmt.Println(strings.Repeat("═", 56))
}

func printMenu() {
	fmt.Println()
	fmt.Println("  ┌──────────────────────────────────────────────────┐")
	fmt.Println("  │ [1] 资源补全修复                                  │")
	fmt.Println("  │     从内嵌资源中释放缺失的 local_data 文件          │")
	fmt.Println("  │     涵盖 audios/、images/、package/ 等目录         │")
	fmt.Println("  │     仅补全缺失文件，不覆盖已有文件                   │")
	fmt.Println("  ├──────────────────────────────────────────────────┤")
	fmt.Println("  │ [2] 端口占用释放                                  │")
	fmt.Println("  │     扫描指定端口范围，终止占用端口的进程             │")
	fmt.Println("  │     支持自定义端口范围，自动验证释放结果             │")
	fmt.Println("  ├──────────────────────────────────────────────────┤")
	fmt.Println("  │ [3] HTTPS 代理服务                                │")
	fmt.Println("  │     启动 HTTPS → HTTP 反向代理，解密转发请求        │")
	fmt.Println("  │     自动生成 TLS 证书，终端显示访问链接              │")
	fmt.Println("  ├──────────────────────────────────────────────────┤")
	fmt.Println("  │ [4] 分卷打包归档                                  │")
	fmt.Println("  │     将项目文件打包为 7z 分卷压缩包                   │")
	fmt.Println("  │     支持自定义配置文件，指定包含/排除路径             │")
	fmt.Println("  ├──────────────────────────────────────────────────┤")
	fmt.Println("  │ [5] 退出程序                                      │")
	fmt.Println("  └──────────────────────────────────────────────────┘")
}

func runResourceRepair() {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 48))
	fmt.Println("  [1] 资源补全修复")
	fmt.Println(strings.Repeat("─", 48))
	fmt.Println()
	fmt.Println("正在检查 local_data 目录资源完整性...")

	if err := EnsureLocalData(); err != nil {
		fmt.Printf("\n✗ 资源修复失败: %v\n", err)
	} else {
		fmt.Println("\n资源修复流程完成。")
	}
}

func runPortRelease() {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 48))
	fmt.Println("  [2] 端口占用释放")
	fmt.Println(strings.Repeat("─", 48))
	fmt.Println()

	portRange := promptPortRange()
	fmt.Printf("\n将扫描端口范围: %d - %d\n", portRange.Start, portRange.End)

	if err := ExecutePortRelease(portRange); err != nil {
		fmt.Printf("\n✗ 端口释放失败: %v\n", err)
	} else {
		fmt.Println("\n端口释放流程完成。")
	}
}

func runHTTPSProxy() {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 48))
	fmt.Println("  [3] HTTPS 代理服务")
	fmt.Println(strings.Repeat("─", 48))
	fmt.Println()

	RunHTTPSProxy()
}

func promptPortRange() PortRange {
	scanner := bufio.NewScanner(os.Stdin)

	// 默认端口范围（与 config 子系统默认值一致）
	defaultStart := *config.MinPort
	defaultEnd := *config.MaxPort

	fmt.Printf("请输入起始端口 [默认 %d]: ", defaultStart)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input != "" {
			if v, err := strconv.Atoi(input); err == nil && v > 0 {
				defaultStart = v
			} else {
				fmt.Printf("  输入无效，使用默认起始端口: %d\n", defaultStart)
			}
		}
	}

	fmt.Printf("请输入结束端口 [默认 %d]: ", defaultEnd)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input != "" {
			if v, err := strconv.Atoi(input); err == nil && v > 0 {
				if v < defaultStart {
					fmt.Printf("  结束端口不能小于起始端口，使用默认结束端口: %d\n", defaultEnd)
				} else {
					defaultEnd = v
				}
			} else {
				fmt.Printf("  输入无效，使用默认结束端口: %d\n", defaultEnd)
			}
		}
	}

	_ = scanner.Err()
	return PortRange{Start: defaultStart, End: defaultEnd}
}
