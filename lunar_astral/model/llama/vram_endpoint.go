package llama

import (
	"encoding/json"
	"fmt"
	"net/http"

	"LunarSubsystem/GeneralConfig"
)

// defaultGuardThresholdMiB 显存守卫端点的默认阈值（MiB），请求未指定时生效
const defaultGuardThresholdMiB = 1024

// VRAMGuardHandler 显存守卫端点：可用显存低于阈值（MiB）时，卸载路由服务器上已加载的 GPU 模型。
// 卸载会一并释放 KV 缓存，模型在下次被使用时按需自动重新加载。
//
// 请求体可选：{"threshold_mib": 1024}
// 响应：{"free_mib": 8867, "threshold_mib": 1024, "triggered": true, "unloaded": ["system-multimodal"]}
func VRAMGuardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "VRAM守卫 → 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析阈值，缺省或非法时使用默认值
	threshold := defaultGuardThresholdMiB
	var req struct {
		ThresholdMiB int `json:"threshold_mib"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.ThresholdMiB > 0 {
		threshold = req.ThresholdMiB
	}

	freeMiB, err := queryFreeVRAMMiB()
	if err != nil {
		http.Error(w, fmt.Sprintf("VRAM守卫 → 查询可用显存失败: %v", err), http.StatusServiceUnavailable)
		return
	}

	// 显存充足时直接返回，不做任何卸载
	response := map[string]any{
		"free_mib":      freeMiB,
		"threshold_mib": threshold,
		"triggered":     false,
		"unloaded":      []string{},
	}
	if freeMiB >= threshold {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	unloaded, err := unloadLoadedGPUModels(*GeneralConfig.ModelPort)
	if err != nil {
		http.Error(w, fmt.Sprintf("VRAM守卫 → 卸载模型失败: %v", err), http.StatusServiceUnavailable)
		return
	}
	response["triggered"] = true
	response["unloaded"] = unloaded

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
