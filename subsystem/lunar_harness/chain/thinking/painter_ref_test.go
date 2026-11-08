package thinking

// painter_ref_test.go 参考资源解析的纯函数规则。

import (
	"strings"
	"testing"
)

// TestImageRelOf 本机图片路径归一化：只接受 multimedia/ 下的图片地址。
func TestImageRelOf(t *testing.T) {
	cases := map[string]string{
		"/file/read/multimedia/images/a.png":                    "multimedia/images/a.png",
		"D:\\data\\local_data\\multimedia\\images\\a.png":       "multimedia/images/a.png",
		"multimedia/images/a.png":                               "multimedia/images/a.png",
		"http://127.0.0.1:36789/file/read/multimedia/a.png":     "multimedia/a.png",
		"http://127.0.0.1:36789/file/read/multimedia/a.png?v=1": "multimedia/a.png",
		"audios/a.mp3": "",
		"":             "",
	}
	for raw, want := range cases {
		if got := imageRelOf(raw); got != want {
			t.Fatalf("imageRelOf(%q) = %q; want %q", raw, got, want)
		}
	}
}

// TestResolvePainterRefsRejectsNonImage 非图片句柄必须给出明确原因，不能静默当作无参考图。
func TestResolvePainterRefsRejectsNonImage(t *testing.T) {
	for _, raw := range []string{"@V.001", "@A.002"} {
		_, err := resolvePainterRefs(raw)
		if err == nil || !strings.Contains(err.Error(), "不是图片") {
			t.Fatalf("%s 应被拒绝并说明不是图片，实得 %v", raw, err)
		}
	}
}
