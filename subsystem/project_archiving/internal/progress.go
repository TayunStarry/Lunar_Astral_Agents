package internal

import (
	"bufio"   // 用于缓冲读取，提高效率
	"fmt"     // 用于格式化输出，打印错误信息
	"io"      // 提供基本的 IO 接口，用于读取数据
	"regexp"  // 用于正则表达式匹配，解析进度信息
	"strings" // 用于字符串操作，如过滤控制字符
	"time"    // 用于记录时间，计算更新间隔
	"unicode" // 用于判断字符是否为控制字符
)

// progressReader 结构体用于包装 io.Reader，实现进度读取功能
// 可以处理进度信息、文件处理信息和其他重要信息
type progressReader struct {
	reader      io.Reader // 底层的 io.Reader，用于读取数据
	isStderr    bool      // 标记是否从标准错误输出读取
	lastUpdate  time.Time // 记录上次更新进度信息的时间
	buffer      []byte    // 用于临时存储读取的字节数据
	lastPercent string    // 记录上次的百分比，避免重复输出
}

// read 方法从底层 reader 读取数据，并处理不同类型的信息
func (pr *progressReader) read() {
	// 使用 bufio.NewReader 包装底层 reader，提高读取效率
	reader := bufio.NewReader(pr.reader)
	// 编译正则表达式，用于匹配进度百分比信息
	progressRegex := regexp.MustCompile(`(\d{1,3})%`)
	// 编译正则表达式，用于匹配文件处理信息
	fileRegex := regexp.MustCompile(`([+-] )(.+)`)
	// 循环读取字节数据
	for {
		// 读取一个字节
		b, err := reader.ReadByte()
		if err != nil {
			// 如果不是 EOF 错误，则打印读取错误信息
			if err != io.EOF {
				fmt.Printf("读取错误: %v\n", err)
			}
			// 遇到错误或 EOF 时退出循环
			break
		}
		// 将读取的字节添加到缓冲区
		pr.buffer = append(pr.buffer, b)
		// 当遇到换行符或缓冲区长度超过 1000 时，处理缓冲区数据
		if b == '\n' || b == '\r' || len(pr.buffer) > 1000 {
			// 将缓冲区数据转换为字符串
			line := string(pr.buffer)
			// 清空缓冲区
			pr.buffer = nil
			// 过滤掉除换行符、回车符和制表符外的控制字符
			line = strings.Map(func(r rune) rune {
				if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
					return -1
				}
				return r
			}, line)
			// 去除字符串两端的空白字符
			line = strings.TrimSpace(line)
			// 如果处理后字符串为空，则跳过本次循环
			if line == "" {
				continue
			}
			// 检查是否匹配进度百分比信息
			if matches := progressRegex.FindStringSubmatch(line); len(matches) > 1 {
				// 提取百分比数值
				percent := matches[1]
				// 避免重复输出相同的百分比
				if percent == pr.lastPercent {
					continue
				}
				// 记录当前百分比，避免重复输出
				pr.lastPercent = percent
				// 获取当前时间
				now := time.Now()
				// 如果距离上次更新超过 1 秒，则输出进度信息（降低间隔让进度条更流畅）
				if now.Sub(pr.lastUpdate) > time.Second*1 {
					// 绘制进度条
					progressBar := pr.createProgressBar(percent)
					fmt.Printf("\r压缩进度: %s %3s%%", progressBar, percent)
					// 更新上次更新时间
					pr.lastUpdate = now
				}
				// 跳过后续处理
				continue
			}
			// 检查是否匹配文件处理信息 （仅在调试模式下）
			if matches := fileRegex.FindStringSubmatch(line); len(matches) > 2 && *SystemDevMode {
				// 提取操作信息
				operation := matches[1]
				// 提取文件名
				filename := matches[2]
				// 先换行再输出文件处理信息，避免干扰进度条
				fmt.Printf("\n%s 处理文件: %s\n", operation, filename)
				// 跳过后续处理
				continue
			}
			// 输出其他重要信息
			if strings.Contains(line, "Everything is Ok") || strings.Contains(line, "Creating archive") || strings.Contains(line, "Items to compress") {
				// 先换行再输出状态信息
				fmt.Printf("\n")
				// 输出 7z 状态信息
				fmt.Printf("7z状态: %s\n", line)
			}
		}
	}
}

// createProgressBar 方法用于根据传入的百分比字符串创建一个带颜色的进度条
// 参数 percent 是表示进度百分比的字符串
// 返回值为格式化后的进度条字符串
func (pr *progressReader) createProgressBar(percent string) string {
	// 定义进度条的总宽度为 30 个字符
	const width = 30
	// 初始化进度值为 0
	value := 0
	// 从百分比字符串中解析出整数进度值
	fmt.Sscanf(percent, "%d", &value)
	// 计算进度条中需要填充的字符数量
	filled := width * value / 100
	// 计算进度条中需要填充空白的字符数量
	empty := width - filled
	// 创建一个 strings.Builder 用于高效构建进度条字符串
	var bar strings.Builder
	// 向进度条添加左边界 [
	bar.WriteString("[")
	// 如果有需要填充的字符，则进行颜色处理和填充操作
	if filled > 0 {
		// 定义颜色代码变量
		var colorCode string
		// 根据进度值设置不同的颜色代码
		if value <= 30 {
			// 进度小于等于 30% 时使用红色
			colorCode = "\033[31m"
		} else if value <= 70 {
			// 进度在 30% 到 70% 之间时使用黄色
			colorCode = "\033[33m"
		} else {
			// 进度大于 70% 时使用绿色
			colorCode = "\033[32m"
		}
		// 定义颜色重置代码，用于恢复默认颜色
		resetCode := "\033[0m"
		// 向进度条添加颜色代码
		bar.WriteString(colorCode)
		// 向进度条添加填充字符 =，数量为 filled - 1
		bar.WriteString(strings.Repeat("=", filled-1))
		// 向进度条添加进度指示符 >
		bar.WriteString(">")
		// 向进度条添加颜色重置代码
		bar.WriteString(resetCode)
	}
	// 向进度条添加空白字符
	bar.WriteString(strings.Repeat(" ", empty))
	// 向进度条添加右边界 ]
	bar.WriteString("]")
	// 返回构建好的进度条字符串
	return bar.String()
}
