package resource

// verify.go 强制类型校验：声明类型必须与本地字节一致。
//
// 只校验，不推断：入站信封已经由发送侧声明类型，这里仅核对字节魔数与
// 声明 MIME 是否互相矛盾；任何不一致都判定该信封无效。

import (
	"fmt"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/multimodal"
)

// AbsPathOfBlob 把内容仓库相对路径还原为本地绝对路径
func AbsPathOfBlob(blobPath string) string {
	blobPath = strings.TrimSpace(blobPath)
	if blobPath == "" {
		return ""
	}
	return filepath.Join(config.LocalDir, filepath.FromSlash(blobPath))
}

// VerifyStored 校验本地实体与声明的类型码 / MIME 是否一致。
func VerifyStored(kind, abs, declaredMIME string) error {
	head, err := readHead(abs)
	if err != nil {
		return fmt.Errorf("读取内容失败：%v", err)
	}
	info, sniffed := sniffMagic(head)
	if !sniffed {
		if kind == KindDocument {
			return nil
		}
		return fmt.Errorf("内容字节无法识别为 %s", kindLabel(kind))
	}
	switch kind {
	case KindImage:
		if info.Kind != KindImage || isAnimatedMagic(head, info) {
			return fmt.Errorf("声明为静态图，实际字节为 %s", info.MIME)
		}
	case KindAnimated:
		if info.Kind != KindImage || !isAnimatedMagic(head, info) {
			return fmt.Errorf("声明为动图，实际字节为 %s", info.MIME)
		}
	case KindVideo:
		if info.Kind != KindVideo {
			return fmt.Errorf("声明为视频，实际字节为 %s", info.MIME)
		}
	case KindAudio:
		if info.Kind != KindAudio {
			return fmt.Errorf("声明为音频，实际字节为 %s", info.MIME)
		}
	case KindDocument:
		return nil
	default:
		return fmt.Errorf("未知声明类型 %q", kind)
	}
	if err := verifyMIMEFamily(info.MIME, declaredMIME); err != nil {
		return err
	}
	return nil
}

// isAnimatedMagic 按文件头判定图片是否具备动画能力
func isAnimatedMagic(head []byte, info magicInfo) bool {
	switch info.Ext {
	case ".gif", ".apng":
		return true
	case ".webp":
		return multimodal.IsAnimatedImage(head)
	default:
		return false
	}
}

// verifyMIMEFamily 声明 MIME 必须与实际字节 MIME 一致（允许同义写法）；未声明时跳过
func verifyMIMEFamily(actual, declared string) error {
	declared = strings.ToLower(strings.TrimSpace(strings.SplitN(declared, ";", 2)[0]))
	if declared == "" {
		return nil
	}
	actual = strings.ToLower(strings.TrimSpace(strings.SplitN(actual, ";", 2)[0]))
	if actual != "" && normalizeVerifyMIME(actual) != normalizeVerifyMIME(declared) {
		return fmt.Errorf("声明 MIME %q 与实际字节 %q 不一致", declared, actual)
	}
	return nil
}

// normalizeVerifyMIME 合并常见同义 MIME，其余按原值精确比较
func normalizeVerifyMIME(mimeType string) string {
	switch mimeType {
	case "image/jpg":
		return "image/jpeg"
	case "audio/mp3":
		return "audio/mpeg"
	case "video/mov":
		return "video/quicktime"
	default:
		return mimeType
	}
}

func kindLabel(kind string) string {
	switch kind {
	case KindImage:
		return "静态图"
	case KindAnimated:
		return "动图"
	case KindVideo:
		return "视频"
	case KindAudio:
		return "音频"
	case KindDocument:
		return "文件"
	case KindURL:
		return "网页"
	default:
		return kind
	}
}
