package module

import (
	"strings"
	"testing"
)

// resetCaptureState 重置截图频率限制状态，避免测试间相互影响
func resetCaptureState() {
	ScreenshotMutex.Lock()
	LastCapture = 0
	ScreenshotMutex.Unlock()
}

// TestNormalizeFormat 验证图片格式归一化
func TestNormalizeFormat(t *testing.T) {
	cases := map[string]string{
		"png":  "png",
		"PNG":  "png",
		"jpg":  "jpeg",
		"jpeg": "jpeg",
		"JPG":  "jpeg",
		"bmp":  "png", // 未知格式兜底为 png
		"":     "png",
	}
	for input, want := range cases {
		if got := normalizeFormat(input); got != want {
			t.Errorf("normalizeFormat(%q) = %q, 期望 %q", input, got, want)
		}
	}
}

// TestCaptureInvalidMode 验证不支持的截图模式返回明确错误
func TestCaptureInvalidMode(t *testing.T) {
	resetCaptureState()
	_, err := Capture(CaptureRequest{Mode: "invalid"})
	if err == nil || !strings.Contains(err.Error(), "不支持的截图模式") {
		t.Fatalf("期望返回「不支持的截图模式」错误，实际: %v", err)
	}
}

// TestCaptureRegionValidation 验证区域宽高校验
func TestCaptureRegionValidation(t *testing.T) {
	resetCaptureState()
	_, err := captureRegion(CaptureRequest{RegionW: 0, RegionH: 100})
	if err == nil || !strings.Contains(err.Error(), "区域宽高必须大于 0") {
		t.Fatalf("期望返回「区域宽高必须大于 0」错误，实际: %v", err)
	}
}

// TestCaptureDisplayValidation 验证显示器索引校验
func TestCaptureDisplayValidation(t *testing.T) {
	resetCaptureState()
	_, err := captureDisplay(9999)
	if err == nil || !strings.Contains(err.Error(), "无效的显示器索引") {
		t.Fatalf("期望返回「无效的显示器索引」错误，实际: %v", err)
	}
}
