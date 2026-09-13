package llama

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"LunarSubsystem/GeneralConfig"
)

// TestQueryFreeVRAMReal 真实调用 nvidia-smi 验证显存检测链路（无 NVIDIA 环境跳过）
func TestQueryFreeVRAMReal(t *testing.T) {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		if _, err := os.Stat(`C:\Windows\System32\nvidia-smi.exe`); err != nil {
			t.Skip("本机未找到 nvidia-smi, 跳过真实显存检测")
		}
	}
	freeMiB, err := queryFreeVRAMMiB()
	if err != nil {
		t.Fatalf("查询空闲显存失败: %v", err)
	}
	if freeMiB <= 0 {
		t.Fatalf("空闲显存应当为正数, 实际 %d MiB", freeMiB)
	}
	t.Logf("当前空闲显存: %d MiB", freeMiB)
}

// TestParseFreeVRAMMiB 验证 nvidia-smi 输出解析：多卡取最大值，无法识别的行跳过
func TestParseFreeVRAMMiB(t *testing.T) {
	cases := []struct {
		name    string
		output  string
		want    int
		wantErr bool
	}{
		{"单卡纯数字", "8192\n", 8192, false},
		{"多卡取最大", "2048\n12288\n", 12288, false},
		{"带MiB单位", "8192 MiB\n", 8192, false},
		{"异常行跳过", "[N/A]\n4096\n", 4096, false},
		{"全部无法解析", "[N/A]\n\n", 0, true},
		{"空输出", "", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseFreeVRAMMiB(c.output)
			if c.wantErr {
				if err == nil {
					t.Fatalf("期望解析失败, 实际返回 %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("期望解析成功, 实际错误: %v", err)
			}
			if got != c.want {
				t.Fatalf("期望 %d MiB, 实际 %d MiB", c.want, got)
			}
		})
	}
}

// TestIsCPUOnlyModel 验证纯 CPU 模型（不占显存）判定
func TestIsCPUOnlyModel(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"GPU默认全量加载", []string{"--ctx-size", "20480", "--model", "a.gguf"}, false},
		{"主模型层数为0", []string{"--model", "a.gguf", "--n-gpu-layers", "0"}, true},
		{"短参数层数为0", []string{"-ngl", "0"}, true},
		{"设备为none", []string{"--device", "none"}, true},
		{"层数非0", []string{"--n-gpu-layers", "99"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isCPUOnlyModel(c.args); got != c.want {
				t.Fatalf("期望 %v, 实际 %v", c.want, got)
			}
		})
	}
}

// TestUnloadLoadedGPUModels 验证模型列表获取与卸载调用：只卸载已加载且占用显存的模型
func TestUnloadLoadedGPUModels(t *testing.T) {
	var unloaded []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/models" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "gpu-model", "status": map[string]any{"value": "loaded", "args": []string{"--model", "big.gguf"}}},
					{"id": "cpu-model", "status": map[string]any{"value": "loaded", "args": []string{"--n-gpu-layers", "0"}}},
					{"id": "already-unloaded", "status": map[string]any{"value": "unloaded"}},
				},
			})
		case r.URL.Path == "/models/unload" && r.Method == http.MethodPost:
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("解析卸载请求失败: %v", err)
			}
			unloaded = append(unloaded, body["model"])
			json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port

	ids, err := unloadLoadedGPUModels(port)
	if err != nil {
		t.Fatalf("卸载失败: %v", err)
	}
	if len(ids) != 1 || ids[0] != "gpu-model" {
		t.Fatalf("期望返回卸载列表 [gpu-model], 实际 %v", ids)
	}
	if len(unloaded) != 1 || unloaded[0] != "gpu-model" {
		t.Fatalf("期望卸载请求只发给 [gpu-model], 实际 %v", unloaded)
	}
}

// TestVRAMGuardHandler 验证显存守卫端点：显存充足时不触发，低于阈值时卸载已加载的 GPU 模型
func TestVRAMGuardHandler(t *testing.T) {
	// 端点依赖 nvidia-smi 查询真实显存，无该环境时跳过
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		if _, err := os.Stat(`C:\Windows\System32\nvidia-smi.exe`); err != nil {
			t.Skip("本机未找到 nvidia-smi, 跳过端点测试")
		}
	}

	// 模拟月华路由服务器：一个 GPU 模型 + 一个纯 CPU 模型
	var unloadCalls []string
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/models" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "system-multimodal", "status": map[string]any{"value": "loaded", "args": []string{"--model", "big.gguf"}}},
					{"id": "system-embedding", "status": map[string]any{"value": "loaded", "args": []string{"--n-gpu-layers", "0"}}},
				},
			})
		case r.URL.Path == "/models/unload" && r.Method == http.MethodPost:
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("解析卸载请求失败: %v", err)
			}
			unloadCalls = append(unloadCalls, body["model"])
			json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer router.Close()

	originalPort := *GeneralConfig.ModelPort
	*GeneralConfig.ModelPort = router.Listener.Addr().(*net.TCPAddr).Port
	defer func() { *GeneralConfig.ModelPort = originalPort }()

	callGuard := func(threshold int) map[string]any {
		req := httptest.NewRequest(http.MethodPost, "/vram/guard", strings.NewReader(fmt.Sprintf(`{"threshold_mib": %d}`, threshold)))
		rec := httptest.NewRecorder()
		VRAMGuardHandler(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("期望 200, 实际 %d: %s", rec.Code, rec.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		return result
	}

	// 阈值 1 MiB: 真实空闲显存必然高于该值，不应触发卸载
	if result := callGuard(1); result["triggered"] != false {
		t.Fatalf("显存充足时不应触发卸载: %v", result)
	}
	if len(unloadCalls) != 0 {
		t.Fatalf("显存充足时不应发起卸载请求: %v", unloadCalls)
	}

	// 阈值极大: 必然低于阈值，应触发卸载且跳过纯 CPU 模型
	if result := callGuard(1 << 40); result["triggered"] != true {
		t.Fatalf("显存不足时应触发卸载: %v", result)
	}
	if len(unloadCalls) != 1 || unloadCalls[0] != "system-multimodal" {
		t.Fatalf("期望只卸载 [system-multimodal], 实际 %v", unloadCalls)
	}
}

// TestFetchRouterModels 验证从路由服务器解析模型列表
func TestFetchRouterModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "system-multimodal", "status": map[string]any{"value": "loaded", "args": []string{"--model", "a.gguf"}}},
			},
		})
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port

	models, err := fetchRouterModels(port)
	if err != nil {
		t.Fatalf("获取模型列表失败: %v", err)
	}
	if len(models) != 1 || models[0].ID != "system-multimodal" || models[0].Status.Value != "loaded" {
		t.Fatalf("模型列表解析结果不符: %+v", models)
	}
}

// TestUnloadRouterModelError 验证卸载接口非 200 时返回错误
func TestUnloadRouterModelError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"model is not running"}}`))
	}))
	defer server.Close()

	port, _ := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	if err := unloadRouterModel(port, "none"); err == nil {
		t.Fatal("期望卸载失败时返回错误, 实际为 nil")
	}
}
