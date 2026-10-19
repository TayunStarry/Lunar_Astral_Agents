// 图片格式转换核心逻辑
package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"

	persistence "CorePersonality/engine/persistence"
	"LunarSubsystem/GeneralConfig"

	"github.com/chai2010/webp"
)

// ConvertImage 执行图片格式转换
// 支持 png / jpeg / webp 三种目标格式，jpeg/webp 使用 quality 控制编码质量
//
// 文件读取与写入统一经 persistence（core_personality 内文件操作的唯一实现），
// 写入时强制落在调用方指定路径（MustStore），不触发内容哈希复用。
func ConvertImage(inputPath, outputPath, targetFormat string, quality int) error {
	inputRel, err := relativeToLocal(inputPath)
	if err != nil {
		return fmt.Errorf("源文件不在本地目录内: %v", err)
	}
	reader, _, _, err := persistence.ReadFile(inputRel)
	if err != nil {
		return fmt.Errorf("无法打开源文件: %v", err)
	}
	defer reader.Close()

	img, _, err := image.Decode(reader)
	if err != nil {
		return fmt.Errorf("无法解码图片: %v", err)
	}

	var buf bytes.Buffer
	switch targetFormat {
	case "png":
		err = png.Encode(&buf, img)
	case "jpeg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	case "webp":
		err = webp.Encode(&buf, img, &webp.Options{Quality: float32(quality)})
	default:
		return fmt.Errorf("不支持的目标格式: %s", targetFormat)
	}
	if err != nil {
		return fmt.Errorf("编码失败: %v", err)
	}

	outputRel, err := relativeToLocal(outputPath)
	if err != nil {
		return fmt.Errorf("输出路径不在本地目录内: %v", err)
	}
	if _, err := persistence.SaveFileEx(outputRel, persistence.SaveOptions{Overwrite: true, MustStore: true}, &buf); err != nil {
		return fmt.Errorf("无法创建输出文件: %v", err)
	}
	return nil
}

// DeleteLocalFile 删除 LocalDir 内的文件，经 persistence 同步清理句柄
func DeleteLocalFile(abs string) error {
	rel, err := relativeToLocal(abs)
	if err != nil {
		return err
	}
	_, err = persistence.DeleteFile(rel)
	return err
}

// relativeToLocal 把绝对路径转换为相对 LocalDir 的斜杠路径
func relativeToLocal(abs string) (string, error) {
	localDir := filepath.Clean(*GeneralConfig.LocalDir)
	rel, err := filepath.Rel(localDir, filepath.Clean(abs))
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("路径越界: %s", abs)
	}
	return rel, nil
}
