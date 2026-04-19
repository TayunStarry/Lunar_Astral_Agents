package component
// 导入必要的包
import (
	"fmt"  // 用于格式化输入输出
	"time" // 提供时间相关的功能
	"io"   // 提供输入输出接口
)

// formatDuration 格式化时间间隔
// 参数:
// d - 要格式化的时间间隔
// 返回值: 格式化后的时间字符串，格式为 "X小时 Y分钟 ZZ.XX秒"
func formatDuration(d time.Duration) string {
	// 计算小时数
	hours := int(d.Hours())
	// 计算分钟数
	minutes := int(d.Minutes()) % 60
	// 计算剩余的秒数
	seconds := d.Seconds() - float64(hours*3600) - float64(minutes*60)
	// 按照指定格式返回格式化后的时间字符串
	return fmt.Sprintf("%d小时 %d分钟 %.2f秒", hours, minutes, seconds)
}

// readOutput 改进的输出读取函数，创建 progressReader 实例并调用其 read 方法
func readOutput(reader io.Reader, isStderr bool) {
	// 创建 progressReader 实例
	pr := &progressReader{
		reader:   reader,
		isStderr: isStderr,
	}
	// 调用 read 方法开始读取和处理数据
	pr.read()
	// 在读取完成后输出换行，确保后续输出在新行开始
	fmt.Println()
}
