// 图片格式转换核心逻辑
package module

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"

	"github.com/chai2010/webp"
)

// ConvertImage 执行图片格式转换
// 支持 png / jpeg / webp 三种目标格式，jpeg/webp 使用 quality 控制编码质量
func ConvertImage(inputPath, outputPath, targetFormat string, quality int) error {
	// 打开源文件
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("无法打开源文件: %v", err)
	}
	defer inputFile.Close()

	// 解码图片
	img, _, err := image.Decode(inputFile)
	if err != nil {
		return fmt.Errorf("无法解码图片: %v", err)
	}

	// 创建输出文件
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("无法创建输出文件: %v", err)
	}
	defer outputFile.Close()

	// 根据目标格式编码
	switch targetFormat {
	case "png":
		err = png.Encode(outputFile, img)
	case "jpeg":
		err = jpeg.Encode(outputFile, img, &jpeg.Options{Quality: quality})
	case "webp":
		err = webp.Encode(outputFile, img, &webp.Options{Quality: float32(quality)})
	default:
		return fmt.Errorf("不支持的目标格式: %s", targetFormat)
	}

	if err != nil {
		return fmt.Errorf("编码失败: %v", err)
	}

	return nil
}
