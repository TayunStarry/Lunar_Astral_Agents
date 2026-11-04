package config

// painter_test.go 绘图配置拆分（general.json → painter.json）的回归测试：
// 1) 仓库内两份配置文件均可解析，且 general.json 不再含 models / diffusion 节；
// 2) painter.json 的扁平结构与代码期望一致，读出的值与拆分前完全等价；
// 3) 方案名不硬编码、缺省字段保持内建默认值、未知键与旧嵌套节被忽略。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// repoRoot 返回仓库根目录（测试文件位于 subsystem/lunar_harness/engine/config）。
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件路径")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

// TestGeneralConfigFileShape 校验 general.json 只保留非绘图配置节。
func TestGeneralConfigFileShape(t *testing.T) {
	path := filepath.Join(repoRoot(t), "local_data", "config", "general.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		t.Fatalf("解析 %s 失败: %v", path, err)
	}
	for _, key := range []string{"models", "diffusion"} {
		if _, ok := sections[key]; ok {
			t.Fatalf("general.json 不应再包含 %s 节（应迁移到 painter.json）", key)
		}
	}
	for _, key := range []string{"system", "agent", "tts", "web_search"} {
		if _, ok := sections[key]; !ok {
			t.Fatalf("general.json 缺少 %s 节", key)
		}
	}
	// 用代码实际结构解析，确保剩余字段仍能正确映射。
	if err := json.Unmarshal(raw, &ModelConfig{}); err != nil {
		t.Fatalf("general.json 无法映射到 ModelConfig: %v", err)
	}
}

// TestPainterConfigFileShape 校验 painter.json 扁平结构与拆分前的取值完全一致。
func TestPainterConfigFileShape(t *testing.T) {
	path := filepath.Join(repoRoot(t), "local_data", "config", "painter.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	var cfg PainterConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("解析 %s 失败: %v", path, err)
	}
	if cfg.DefaultScheme != "quality" {
		t.Fatalf("default_scheme 应为 quality，实得 %q", cfg.DefaultScheme)
	}
	if len(cfg.Schemes) != 2 {
		t.Fatalf("顶层应含 turbo / quality 两套方案，实得 %d 套", len(cfg.Schemes))
	}
	turbo, quality := cfg.Schemes["turbo"], cfg.Schemes["quality"]
	if turbo.Steps != 8 || quality.Steps != 40 {
		t.Fatalf("方案步数不一致: turbo=%d quality=%d", turbo.Steps, quality.Steps)
	}
	if turbo.DiffusionModel != "./local_data/models/painter/qwen-image-2.1-turbo-Q4_K_M.gguf" {
		t.Fatalf("turbo 扩散模型路径不一致: %q", turbo.DiffusionModel)
	}
	if quality.DiffusionModel != "./local_data/models/painter/qwen-image-2.1-Q4_K_M.gguf" {
		t.Fatalf("quality 扩散模型路径不一致: %q", quality.DiffusionModel)
	}
	// 顶层扩散参数逐项对照拆分前 general.json 的 diffusion 内容。
	d := cfg.Diffusion
	if !d.AllowDiffusion || d.ImageVRAMGuard == nil || !*d.ImageVRAMGuard || d.ImageVRAMGuardMiB == nil || *d.ImageVRAMGuardMiB != 8192 {
		t.Fatalf("显存守卫配置不一致: %+v", d)
	}
	if d.SDOffloadToCPU == nil || *d.SDOffloadToCPU != "always" || d.SDTextEncoderOnCPU == nil || !*d.SDTextEncoderOnCPU {
		t.Fatalf("sd.cpp 内存策略不一致: %+v", d)
	}
	if d.SDVaeTiling == nil || *d.SDVaeTiling != "off" || d.SDVaeTileOverlap == nil || *d.SDVaeTileOverlap != 0 {
		t.Fatalf("VAE 分片配置不一致: %+v", d)
	}
	if d.SDMaxVRAM != nil {
		t.Fatalf("sd_max_vram 应为 null（自动），实得 %#v", *d.SDMaxVRAM)
	}
	if d.SDVaeTileSize != nil {
		t.Fatalf("sd_vae_tile_size 应为 null（引擎默认），实得 %#v", *d.SDVaeTileSize)
	}
	if d.SDSampler == nil || *d.SDSampler != "auto" || d.SDScheduler == nil || *d.SDScheduler != "auto" {
		t.Fatalf("采样策略不一致: %+v", d)
	}
	if d.SDSteps == nil || *d.SDSteps != 40 || d.SDImageSize == nil || *d.SDImageSize != 1024 {
		t.Fatalf("兜底步数/边长不一致: %+v", d)
	}
}

// TestApplyPainterConfig 校验扁平格式的解析与应用：
// 方案名任意、未知标量键与旧嵌套节被忽略、缺省字段保持默认。
func TestApplyPainterConfig(t *testing.T) {
	// 先清理方案表，避免测试顺序影响；结束后恢复全局变量与空方案表。
	SetDiffusionSchemes("", nil)
	old := []any{
		DiffusionModel, VariationalModel, PromptAnalysisModel, PromptMmprojModel, RealESRGANModel,
		AllowDiffusion, ImageVRAMGuard, ImageVRAMGuardMiB, SDOffloadToCPU, SDTextEncoderOnCPU,
		SDVaeTiling, SDVaeTileSize, SDVaeTileOverlap, SDVaeBackend, SDVaeConvDirect,
		SDMaxVRAM, SDSampler, SDScheduler, SDFlowShift, SDFlashAttention,
		SDSteps, SDImageSize, DiffusionPixelBudgetMP, DiffusionImg2ImgCFG,
	}
	t.Cleanup(func() {
		SetDiffusionSchemes("", nil)
		DiffusionModel, VariationalModel = old[0].(string), old[1].(string)
		PromptAnalysisModel, PromptMmprojModel = old[2].(string), old[3].(string)
		RealESRGANModel = old[4].(string)
		AllowDiffusion, ImageVRAMGuard = old[5].(bool), old[6].(bool)
		ImageVRAMGuardMiB = old[7].(int)
		SDOffloadToCPU, SDTextEncoderOnCPU = old[8].(string), old[9].(bool)
		SDVaeTiling, SDVaeTileSize = old[10].(string), old[11].(string)
		SDVaeTileOverlap, SDVaeBackend = old[12].(float64), old[13].(string)
		SDVaeConvDirect = old[14].(bool)
		SDMaxVRAM, SDSampler, SDScheduler = old[15].(string), old[16].(string), old[17].(string)
		SDFlowShift, SDFlashAttention = old[18].(float64), old[19].(bool)
		SDSteps, SDImageSize = old[20].(int), old[21].(int)
		DiffusionPixelBudgetMP, DiffusionImg2ImgCFG = old[22].(float64), old[23].(float64)
	})

	dir := t.TempDir()
	if cfg, ok, err := LoadPainterConfigFile(dir); err != nil || ok || cfg != nil {
		t.Fatalf("缺失文件应返回 nil/false/nil，实得 cfg=%v ok=%v err=%v", cfg, ok, err)
	}

	// 方案名使用非 turbo / quality 的自定义键，证明方案集合不硬编码。
	raw := `{
		"default_scheme": "custom",
		"custom": {"diffusion_model": "custom.gguf", "variational_model": "vae.bin", "steps": 8},
		"legacy": {"diffusion_model": "legacy.gguf", "steps": 12},
		"allow_diffusion": false,
		"sd_steps": 20,
		"unknown_scalar": 1,
		"models": {"schemes": {"ghost": {"diffusion_model": "ghost.gguf"}}}
	}`
	if err := os.WriteFile(filepath.Join(dir, PainterConfigFile), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, ok, err := LoadPainterConfigFile(dir)
	if err != nil || !ok || cfg == nil {
		t.Fatalf("读取 painter.json 失败: ok=%v err=%v", ok, err)
	}
	if got := len(cfg.Schemes); got != 2 {
		t.Fatalf("应识别 2 套顶层方案（custom / legacy），实得 %d 套", got)
	}
	applyPainterConfig(cfg)

	if DiffusionModel != "custom.gguf" || VariationalModel != "vae.bin" {
		t.Fatalf("默认方案未生效: diffusion=%q vae=%q", DiffusionModel, VariationalModel)
	}
	if AllowDiffusion {
		t.Fatal("allow_diffusion=false 应关闭扩散生成")
	}
	if SDSteps != 20 {
		t.Fatalf("sd_steps 应覆盖为 20，实得 %d", SDSteps)
	}
	if got := ResolveDiffusionScheme(); got.Name != "custom" || got.Steps != 8 {
		t.Fatalf("默认方案应为 custom（steps=8），实得 name=%q steps=%d", got.Name, got.Steps)
	}
}
