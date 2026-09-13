package llama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
)

func init() {
	// 向图像生成模块注册画图前显存守卫：可用显存不足时先卸载本地模型释放显存
	GeneralConfig.ImageVRAMGuardHook = EnsureDiffusionVRAM
}

// routerModel 路由服务器 /models 端点返回的单个模型信息
type routerModel struct {
	ID     string `json:"id"`
	Status struct {
		Value string   `json:"value"` // unloaded / loaded / loading
		Args  []string `json:"args"`  // 模型实例的启动参数，用于判断是否占用显存
	} `json:"status"`
}

// EnsureDiffusionVRAM 画图前的显存守卫：可用显存低于阈值时，卸载路由服务器上已加载的 GPU 模型。
// 卸载后的模型在下次被使用时（如收到聊天请求）会由路由服务器按需自动重新加载，无需手动恢复。
// 返回守卫结束后的可用显存（MiB，0 表示未知），供画图模块决定是否进一步降低显存占用。
func EnsureDiffusionVRAM() (int, error) {
	if !*GeneralConfig.ImageVRAMGuard || *GeneralConfig.ImageVRAMGuardMiB <= 0 {
		return 0, nil
	}
	threshold := *GeneralConfig.ImageVRAMGuardMiB

	freeMiB, err := queryFreeVRAMMiB()
	if err != nil {
		// 检测不到显存（无 NVIDIA 显卡或缺少 nvidia-smi）时跳过守卫，不阻断画图
		return 0, fmt.Errorf("检测可用显存失败, 跳过模型卸载: %w", err)
	}
	if freeMiB >= threshold {
		return freeMiB, nil
	}

	LoggerGeneral.Info("LlamaProxy", "可用显存 %d MiB 低于阈值 %d MiB, 正在卸载已加载的模型...", freeMiB, threshold)

	unloaded, err := unloadLoadedGPUModels(*GeneralConfig.ModelPort)
	if err != nil {
		return freeMiB, fmt.Errorf("卸载模型失败: %w", err)
	}
	if len(unloaded) == 0 {
		return freeMiB, nil
	}

	// 等待显存真正释放；超时后仍然继续，把显存不足的风险留给画图引擎自行处理
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		free, err := queryFreeVRAMMiB()
		if err == nil {
			freeMiB = free
			if free >= threshold {
				LoggerGeneral.Info("LlamaProxy", "显存已释放, 当前可用 %d MiB", free)
				return freeMiB, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	LoggerGeneral.Warn("LlamaProxy", "等待显存释放超时, 继续执行画图任务")
	return freeMiB, nil
}

// unloadLoadedGPUModels 卸载路由服务器上已加载且占用显存的模型，返回成功卸载的模型 ID 列表
func unloadLoadedGPUModels(port int) ([]string, error) {
	models, err := fetchRouterModels(port)
	if err != nil {
		return nil, err
	}

	unloaded := []string{}
	for _, m := range models {
		// 未加载的模型无需处理；纯 CPU 模型不占显存，卸载只会徒增重载开销
		if m.Status.Value == "unloaded" || isCPUOnlyModel(m.Status.Args) {
			continue
		}
		if err := unloadRouterModel(port, m.ID); err != nil {
			LoggerGeneral.Warn("LlamaProxy", "卸载模型[%s]失败: %v", m.ID, err)
			continue
		}
		LoggerGeneral.Info("LlamaProxy", "已卸载模型[%s]释放显存", m.ID)
		unloaded = append(unloaded, m.ID)
	}
	return unloaded, nil
}

// fetchRouterModels 获取路由服务器的模型列表及加载状态
func fetchRouterModels(port int) ([]routerModel, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://localhost:%d/models", port), nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求模型列表失败: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data []routerModel `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析模型列表失败: %w", err)
	}
	return result.Data, nil
}

// unloadRouterModel 调用路由服务器卸载接口，释放指定模型的显存占用
func unloadRouterModel(port int, modelID string) error {
	body, err := json.Marshal(map[string]string{"model": modelID})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://localhost:%d/models/unload", port), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求卸载接口失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("卸载接口返回状态码 %d", resp.StatusCode)
	}
	return nil
}

// queryFreeVRAMMiB 通过 nvidia-smi 查询空闲显存，多卡时取空闲最多的一张
func queryFreeVRAMMiB() (int, error) {
	smiPath, err := exec.LookPath("nvidia-smi")
	if err != nil {
		// Windows 下 nvidia-smi 随驱动安装在 System32，个别环境 PATH 未包含该目录
		smiPath = filepath.Join(os.Getenv("SystemRoot"), "System32", "nvidia-smi.exe")
		if _, err := os.Stat(smiPath); err != nil {
			return 0, fmt.Errorf("未找到 nvidia-smi")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, smiPath, "--query-gpu=memory.free", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return 0, fmt.Errorf("nvidia-smi 查询失败: %w", err)
	}
	return parseFreeVRAMMiB(string(output))
}

// parseFreeVRAMMiB 解析 nvidia-smi 的 CSV 输出，返回各显卡空闲显存中的最大值（MiB）
func parseFreeVRAMMiB(output string) (int, error) {
	maxMiB := -1
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		// nounits 下为纯数字，个别驱动输出形如 "8192 MiB"，无法识别的行（如 [N/A]）跳过
		mib, err := strconv.Atoi(strings.TrimSuffix(fields[0], "MiB"))
		if err != nil {
			continue
		}
		if mib > maxMiB {
			maxMiB = mib
		}
	}
	if maxMiB < 0 {
		return 0, fmt.Errorf("nvidia-smi 输出无法解析: %q", strings.TrimSpace(output))
	}
	return maxMiB, nil
}

// isCPUOnlyModel 根据模型实例的启动参数判断模型是否完全不占用显存
func isCPUOnlyModel(args []string) bool {
	for i, arg := range args {
		switch arg {
		// 主模型 GPU 层数为 0 或设备设为 none 时全部驻留内存
		case "--n-gpu-layers", "-ngl", "--gpu-layers":
			if i+1 < len(args) && args[i+1] == "0" {
				return true
			}
		case "--device":
			if i+1 < len(args) && strings.EqualFold(args[i+1], "none") {
				return true
			}
		}
	}
	return false
}
