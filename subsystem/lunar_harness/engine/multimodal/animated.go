package multimodal

// 动态图（GIF / APNG / 动态 WebP）检测与临时落盘
//
// 动态图没有独立的抽帧链路：它按「一个很短的影像片段」交给 watch.go 的
// 统一等距抽帧（与视频切片同一套规则），此处只负责格式识别与临时文件。

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/gif"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/netsource"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// maxAnimatedBytes 动态图最大输入字节数（与静态图处理保持一致的上限）
const maxAnimatedBytes = 50 * 1024 * 1024

// IsAnimatedImage 判断图片数据是否为动态图（多帧 GIF / APNG / 动态 WebP）
// 非动态图（含单帧 GIF）返回 false，由静态图链路处理
func IsAnimatedImage(imgData []byte) bool {
	if len(imgData) == 0 || len(imgData) > maxAnimatedBytes {
		return false
	}
	switch detectImageFormat(imgData) {
	case "gif":
		// GIF 帧数需完整解码统计，单帧与双帧 GIF 按静态图处理
		gifImg, err := gif.DecodeAll(bytes.NewReader(imgData))
		if err != nil {
			return false
		}
		return len(gifImg.Image) > 2
	case "png":
		return persistence.IsAnimatedPNG(imgData)
	case "webp":
		return persistence.IsAnimatedWebP(imgData)
	default:
		return false
	}
}

// writeTempImage 将图片数据写入带正确扩展名的临时文件，供 ffmpeg 按扩展名识别
func writeTempImage(imgData []byte, format string) (string, error) {
	ext := ".bin"
	switch format {
	case "gif":
		ext = ".gif"
	case "png":
		ext = ".png"
	case "webp":
		ext = ".webp"
	}
	tempFile, err := os.CreateTemp("", "lunar_animated_*"+ext)
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	if _, err := tempFile.Write(imgData); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	tempFile.Close()
	return tempFile.Name(), nil
}

// ImageSourceToBytes 按来源读取图片数据：支持 HTTP(S) URL、data:image;base64 URI 与本地文件路径
// 供适配器在 TypeScript 传入图片地址（而非字节）时使用
func ImageSourceToBytes(source string) ([]byte, error) {
	if source == "" {
		return nil, fmt.Errorf("图片地址为空")
	}
	switch {
	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		return downloadImageBytes(source)
	case strings.HasPrefix(source, "data:"):
		parts := strings.SplitN(source, ",", 2)
		if len(parts) != 2 || !strings.HasSuffix(parts[0], "base64") {
			return nil, fmt.Errorf("无效的 data URI 格式")
		}
		data, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("base64 解码失败: %w", err)
		}
		return data, nil
	default:
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("读取图片文件失败: %w", err)
		}
		return data, nil
	}
}

// imageSourceClient 图片下载客户端
//   - 60 秒超时：远端无响应时不得阻塞智能体主循环（图片体积远小于模型/媒体文件）
//   - 跳过 TLS 校验：与 lunar_goja 的 fetch 默认策略一致，兼容自签名图床
var imageSourceClient = netsource.NewHTTPClient(60*time.Second, true)

// downloadImageBytes 下载图片到内存，超过 maxAnimatedBytes 时中止
func downloadImageBytes(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	// 部分图床会拒绝无 UA 的请求
	req.Header.Set("User-Agent", "LunarAstral/1.0")

	resp, err := imageSourceClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载图片失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载图片失败，状态码 %d", resp.StatusCode)
	}

	// 多读 1 字节用于判断是否超限
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAnimatedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取图片数据失败: %w", err)
	}
	if len(data) > maxAnimatedBytes {
		return nil, fmt.Errorf("图片数据过大，超过 %dMB 限制", maxAnimatedBytes/1024/1024)
	}
	return data, nil
}
