package broadcaster

// maintenance_test.go 维护态：播报链路不得在维护期间响应合成请求。

import (
	"net/http"
	"testing"
)

func TestPauseBlocksSynthesize(t *testing.T) {
	Pause()
	if !Paused() {
		t.Fatalf("Pause 后应处于维护态")
	}

	payload, status := Synthesize(TTSRequest{Text: "测试"})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("维护态合成应返回 503，实得 %d", status)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("维护态合成不应成功")
	}
	if msg, _ := payload["error"].(string); msg != errMaintenanceMessage {
		t.Fatalf("维护态错误信息应为维护提示，实得 %q", msg)
	}

	Resume()
	if Paused() {
		t.Fatalf("Resume 后不应仍处于维护态")
	}
}

func TestPauseIsIdempotent(t *testing.T) {
	Pause()
	Pause()
	if !Paused() {
		t.Fatalf("重复 Pause 后应处于维护态")
	}
	Resume()
	Resume()
	if Paused() {
		t.Fatalf("重复 Resume 后不应仍处于维护态")
	}
}
