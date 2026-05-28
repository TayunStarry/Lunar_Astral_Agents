package engine

import (
	"errors"
	"fmt"
	sdcgo "sd_lunar/cgo"
	"strings"
)

var ErrNotReady = errors.New("SD引擎未就绪")

type EngineParams struct {
	ModelPath          string
	ClipLPath          string
	ClipGPath          string
	T5xxlPath          string
	DiffusionModelPath string
	VaePath            string
	TaeSDPath          string
	TensorTypeRules    string
	VaeDecodeOnly      bool
	FreeParamsImmed    bool
	NThreads           int
	WType              string
	RNGType            string
	EnableMmap         bool
	KeepClipOnCPU      bool
	KeepVaeOnCPU       bool
	FlashAttn          bool
	DiffusionFlashAttn bool
	Backend            string
}

type GenerationConfig struct {
	Prompt         string
	NegativePrompt string
	ClipSkip       int
	Width          int
	Height         int
	Steps          int
	Sampler        string
	Scheduler      string
	CFGScale       float32
	Seed           int64
	BatchCount     int
	Strength       float32
}

type GeneratedImage struct {
	Width   uint32
	Height  uint32
	Channel uint32
	Data    []byte
	Seed    int64
}

type Engine struct {
	ctx    *sdcgo.SDContext
	params EngineParams
	ready  bool
}

var defaultEngine *Engine

func (e *Engine) IsReady() bool {
	return e != nil && e.ready
}

func Init(params EngineParams) error {
	cgoParams := sdcgo.CtxParams{
		ModelPath:          params.ModelPath,
		ClipLPath:          params.ClipLPath,
		ClipGPath:          params.ClipGPath,
		T5xxlPath:          params.T5xxlPath,
		DiffusionModelPath: params.DiffusionModelPath,
		VaePath:            params.VaePath,
		TaeSDPath:          params.TaeSDPath,
		TensorTypeRules:    params.TensorTypeRules,
		VaeDecodeOnly:      params.VaeDecodeOnly,
		FreeParamsImmed:    params.FreeParamsImmed,
		NThreads:           params.NThreads,
		WType:              parseSDType(params.WType),
		RNGType:            parseRNGType(params.RNGType),
		EnableMmap:         params.EnableMmap,
		KeepClipOnCPU:      params.KeepClipOnCPU,
		KeepVaeOnCPU:       params.KeepVaeOnCPU,
		FlashAttn:          params.FlashAttn,
		DiffusionFlashAttn: params.DiffusionFlashAttn,
		Backend:            params.Backend,
	}

	ctx, err := sdcgo.NewContext(cgoParams)
	if err != nil {
		return fmt.Errorf("初始化SD引擎失败: %w", err)
	}

	defaultEngine = &Engine{
		ctx:    ctx,
		params: params,
		ready:  true,
	}

	return nil
}

func GetEngine() *Engine {
	return defaultEngine
}

func Release() {
	if defaultEngine != nil && defaultEngine.ctx != nil {
		defaultEngine.ctx.Release()
		defaultEngine.ready = false
	}
	defaultEngine = nil
}

func (e *Engine) GenerateTextToImage(cfg GenerationConfig) (*GeneratedImage, error) {
	if !e.IsReady() {
		return nil, fmt.Errorf("SD引擎未初始化")
	}

	genParams := sdcgo.ImgGenParams{
		Prompt:         cfg.Prompt,
		NegativePrompt: cfg.NegativePrompt,
		ClipSkip:       cfg.ClipSkip,
		Width:          cfg.Width,
		Height:         cfg.Height,
		Seed:           cfg.Seed,
		BatchCount:     1,
		SampleParams: sdcgo.SampleParams{
			Guidance:     cfg.CFGScale,
			Scheduler:    parseScheduler(cfg.Scheduler),
			SampleMethod: parseSampler(cfg.Sampler),
			SampleSteps:  cfg.Steps,
		},
	}

	images, err := e.ctx.GenerateImage(genParams)
	if err != nil {
		return nil, fmt.Errorf("文生图失败: %w", err)
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("生成结果为空")
	}

	return &GeneratedImage{
		Width:   images[0].Width,
		Height:  images[0].Height,
		Channel: images[0].Channel,
		Data:    images[0].Data,
		Seed:    cfg.Seed,
	}, nil
}

func (e *Engine) GenerateImageToImage(cfg GenerationConfig, refImageData []byte, refWidth, refHeight, refChannels uint32) (*GeneratedImage, error) {
	if !e.IsReady() {
		return nil, fmt.Errorf("SD引擎未初始化")
	}

	initImage := &sdcgo.SDImage{
		Width:   refWidth,
		Height:  refHeight,
		Channel: refChannels,
		Data:    refImageData,
	}

	genParams := sdcgo.ImgGenParams{
		Prompt:         cfg.Prompt,
		NegativePrompt: cfg.NegativePrompt,
		ClipSkip:       cfg.ClipSkip,
		Width:          cfg.Width,
		Height:         cfg.Height,
		Seed:           cfg.Seed,
		BatchCount:     1,
		Strength:       cfg.Strength,
		InitImage:      initImage,
		SampleParams: sdcgo.SampleParams{
			Guidance:     cfg.CFGScale,
			Scheduler:    parseScheduler(cfg.Scheduler),
			SampleMethod: parseSampler(cfg.Sampler),
			SampleSteps:  cfg.Steps,
		},
	}

	images, err := e.ctx.GenerateImage(genParams)
	if err != nil {
		return nil, fmt.Errorf("图生图失败: %w", err)
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("生成结果为空")
	}

	return &GeneratedImage{
		Width:   images[0].Width,
		Height:  images[0].Height,
		Channel: images[0].Channel,
		Data:    images[0].Data,
		Seed:    cfg.Seed,
	}, nil
}

func parseSDType(t string) sdcgo.SDType {
	switch strings.ToLower(t) {
	case "f16", "float16", "":
		return sdcgo.SDTypeF16
	case "f32", "float32":
		return sdcgo.SDTypeF32
	case "q4_0":
		return sdcgo.SDTypeQ4_0
	case "q4_1":
		return sdcgo.SDTypeQ4_1
	case "q5_0":
		return sdcgo.SDTypeQ5_0
	case "q5_1":
		return sdcgo.SDTypeQ5_1
	case "q8_0":
		return sdcgo.SDTypeQ8_0
	case "q4_k":
		return sdcgo.SDTypeQ4_K
	case "q5_k":
		return sdcgo.SDTypeQ5_K
	case "q6_k":
		return sdcgo.SDTypeQ6_K
	case "q8_k":
		return sdcgo.SDTypeQ8_K
	default:
		return sdcgo.SDTypeF16
	}
}

func parseRNGType(t string) sdcgo.RNGType {
	switch strings.ToLower(t) {
	case "cuda":
		return sdcgo.RNGCUDA
	case "cpu":
		return sdcgo.RNGCPU
	default:
		return sdcgo.RNGStdDefault
	}
}

func parseSampler(s string) sdcgo.SampleMethod {
	switch strings.ToLower(s) {
	case "euler":
		return sdcgo.SampleEuler
	case "euler_a", "euler a":
		return sdcgo.SampleEulerA
	case "heun":
		return sdcgo.SampleHeun
	case "dpm2":
		return sdcgo.SampleDPM2
	case "dpm++2s_a", "dpmpp_2s_a":
		return sdcgo.SampleDPMPP2SA
	case "dpm++2m", "dpmpp_2m":
		return sdcgo.SampleDPMPP2M
	case "dpm++2mv2", "dpmpp_2m_v2":
		return sdcgo.SampleDPMPP2Mv2
	case "ipndm":
		return sdcgo.SampleIPNDM
	case "ipndm_v":
		return sdcgo.SampleIPNDMV
	case "lcm":
		return sdcgo.SampleLCM
	case "ddim":
		return sdcgo.SampleDDIM
	case "tcd":
		return sdcgo.SampleTCD
	default:
		return sdcgo.SampleEulerA
	}
}

func parseScheduler(s string) sdcgo.Scheduler {
	switch strings.ToLower(s) {
	case "discrete", "":
		return sdcgo.SchedDiscrete
	case "karras":
		return sdcgo.SchedKarras
	case "exponential":
		return sdcgo.SchedExponential
	case "ays":
		return sdcgo.SchedAYS
	case "simple":
		return sdcgo.SchedSimple
	default:
		return sdcgo.SchedDiscrete
	}
}
