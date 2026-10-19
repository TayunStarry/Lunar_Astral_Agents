package multimodal

import (
	"strings"
	"testing"

	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
)

// 覆盖：VAE 解码路径的决策（三档退让：GPU 整帧 → GPU + direct conv → CPU）。
//
// 回归背景（用户实测）：1024×1536 出图时 sd.cpp 报
// `wan_vae segment 1/1 (graph) failed during weight preparation` 与
// `vae decode compute failed`，随后引擎自行「retrying with spatial tiling」，
// 画面下巴处留下 17×16 px 的硬边纯白方块（右边界正落在 512px 的分片线上）。
// 因此决策必须保证：**放不下就退到 direct conv 或 CPU，而不是让引擎降级成空间分片**。
func TestResolveVAEDecode(t *testing.T) {
	cases := []struct {
		name      string
		w, h      int
		freeMiB   int
		mode      string
		allowConv bool
		wantCPU   bool
		wantConv  bool
	}{
		{"本次故障：1024×1536 在 12GB 卡上整帧放不下 → direct conv",
			1024, 1536, 10849, "auto", true, false, true},
		{"关掉 direct conv 时退到 CPU（慢但没有接缝）",
			1024, 1536, 10849, "auto", false, true, false},
		{"1024×1024 在 12GB 卡上仍走整帧 GPU（最快）",
			1024, 1024, 10849, "auto", true, false, false},
		{"同尺寸但 24GB 卡放得下 → 整帧 GPU",
			1024, 1536, 23000, "auto", true, false, false},
		{"可用显存未知时不擅自改后端",
			1024, 1536, 0, "auto", true, false, false},
		{"显式 cpu 一律 CPU",
			1024, 1024, 23000, "cpu", true, true, false},
		{"显式 gpu 一律 GPU（direct conv 由开关决定）",
			1024, 1536, 10849, "gpu", true, false, true},
		{"超大图连 direct conv 都放不下 → CPU",
			2048, 2048, 10849, "auto", true, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := resolveVAEDecode(tc.w, tc.h, tc.freeMiB, tc.mode, tc.allowConv)
			if plan.onCPU != tc.wantCPU || plan.convDirect != tc.wantConv {
				t.Fatalf("落点不符：onCPU=%v convDirect=%v，期望 onCPU=%v convDirect=%v（依据: %s）",
					plan.onCPU, plan.convDirect, tc.wantCPU, tc.wantConv, plan.why)
			}
			if plan.why == "" {
				t.Fatal("判定依据不能为空：它就是白块问题唯一可查的现场记录")
			}
		})
	}
}

// 覆盖：显存估算的量级与单调性——系数标定自实测（1024×1536 整帧需 12475–15689 MiB）。
func TestEstimateVAEDecodeMiB(t *testing.T) {
	got := estimateVAEDecodeMiB(1024, 1536)
	if got < 12000 || got > 16000 {
		t.Fatalf("1024×1536 整帧估算 %d MiB，应落在实测区间 12475-15689 附近", got)
	}
	if direct := estimateVAEDecodeDirectMiB(1024, 1536); direct >= got {
		t.Fatalf("direct conv 的估算应低于普通卷积: %d >= %d", direct, got)
	}
	if small, big := estimateVAEDecodeMiB(1024, 1024), estimateVAEDecodeMiB(1536, 1536); small >= big {
		t.Fatalf("估算未随面积增长: %d >= %d", small, big)
	}
	if zero := estimateVAEDecodeMiB(0, 0); zero != vaeDecodeFixedMiB {
		t.Fatalf("缺失尺寸时应只返回固定开销 %d，实得 %d", vaeDecodeFixedMiB, zero)
	}
}

// 覆盖：sd-vae-backend 的取值归一（大小写、空格、未知值）
func TestVAEBackendMode(t *testing.T) {
	cases := map[string]string{
		"cpu": "cpu", " CPU ": "cpu",
		"gpu": "gpu", "GPU": "gpu",
		"auto": "auto", "": "auto", "whatever": "auto",
	}
	for in, want := range cases {
		saved := *GeneralConfig.SDVaeBackend
		*GeneralConfig.SDVaeBackend = in
		if got := vaeBackendMode(); got != want {
			t.Fatalf("vaeBackendMode(%q)=%q，期望 %q", in, got, want)
		}
		*GeneralConfig.SDVaeBackend = saved
	}
}

// 覆盖：引擎自行降级为空间分片时，日志必须翻出一条可读的警告（且只报一次）。
//
// 为什么值得测：这条降级是**引擎内部**行为，调用方没有开关能关掉它；
// 用户看到的只有画面上的白块，若日志里也没有告警，排查就只能靠猜。
func TestEngineLogWarnsOnVAETilingFallback(t *testing.T) {
	type record struct{ level, module, sub, msg string }
	var got []record
	LoggerGeneral.SetLogHook(func(level, module, sub, msg string) {
		got = append(got, record{level, module, sub, msg})
	})
	defer LoggerGeneral.SetLogHook(nil)
	// 必须注入引擎钩子：没有钩子时 writer 直接直写终端流，走不到 emit 的检测逻辑
	SetEngineLogHook(func(stream, line string) {})
	defer SetEngineLogHook(nil)

	w := newEngineLogWriter("stdout", nil)
	// 引擎同一次失败会连打两行同义日志：只应提示一次
	w.Write([]byte("[WARN ] backend_fit.cpp:506 - VAE decode ran out of memory; retrying with spatial tiling\n"))
	w.Write([]byte("[WARN ] backend_fit.cpp:506 - VAE decode ran out of memory; retrying with spatial tiling\n"))

	warns := 0
	for _, r := range got {
		if r.level != "WARN" {
			continue
		}
		warns++
		if !strings.Contains(r.msg, "空间分片") || !strings.Contains(r.msg, "纯色方块") {
			t.Fatalf("警告内容应点明分片与纯色方块: %q", r.msg)
		}
	}
	if warns != 1 {
		t.Fatalf("同一引擎进程应只报一次降级警告，实得 %d 次", warns)
	}
}

// 覆盖：正常的引擎日志不触发降级告警（避免把「tiling」相关字样误判成降级）
func TestEngineLogNoWarnOnNormalLines(t *testing.T) {
	var warned bool
	LoggerGeneral.SetLogHook(func(level, _, _, _ string) {
		if level == "WARN" {
			warned = true
		}
	})
	defer LoggerGeneral.SetLogHook(nil)
	SetEngineLogHook(func(stream, line string) {})
	defer SetEngineLogHook(nil)

	w := newEngineLogWriter("stdout", nil)
	w.Write([]byte("[INFO ] vae.hpp:280 - vae decode start\n"))
	w.Write([]byte("[INFO ] sampling 1/40 completed\n"))
	if warned {
		t.Fatal("正常日志不应触发降级告警")
	}
}
