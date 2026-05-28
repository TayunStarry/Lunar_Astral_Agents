package cgo

/*
#cgo CPPFLAGS: -I${SRCDIR}/../native/include
#cgo CPPFLAGS: -I${SRCDIR}/../native/ggml/include
#cgo LDFLAGS: -L${SRCDIR}/../native/build/bin/Release -lstable-diffusion
#cgo LDFLAGS: -L${SRCDIR}/../native/build/ggml/src -lggml
#cgo LDFLAGS: -L${SRCDIR}/../native/build/ggml/src -lggml-cpu
#cgo LDFLAGS: -L${SRCDIR}/../native/build/ggml/src -lggml-vulkan

#include <stdlib.h>
#include <stdint.h>
#include <stdbool.h>

enum rng_type_t {
	RNG_STD_DEFAULT,
	RNG_CUDA,
	RNG_CPU,
	RNG_COUNT
};

enum sample_method_t {
	EULER,
	EULER_A,
	HEUN,
	DPM2,
	DPMPP2S_A,
	DPMPP2M,
	DPMPP2Mv2,
	IPNDM,
	IPNDM_V,
	LCM,
	DDIM_TRAILING,
	TCD,
	RES_MULTISTEP,
	RES_2S,
	ER_SDE,
	EULER_CFG_PP,
	EULER_A_CFG_PP,
	EULER_GE,
	SAMPLE_COUNT
};

enum scheduler_t {
	SCHED_DISCRETE,
	SCHED_KARRAS,
	SCHED_EXPONENTIAL,
	SCHED_AYS,
	SCHED_GITS,
	SCHED_SGM_UNIFORM,
	SCHED_SIMPLE,
	SCHED_SMOOTHSTEP,
	SCHED_KL_OPTIMAL,
	SCHED_LCM,
	SCHED_BONG_TANGENT,
	SCHED_LTX2,
	SCHED_COUNT
};

enum sd_type_t {
	SD_F32  = 0,
	SD_F16  = 1,
	SD_Q4_0 = 2,
	SD_Q4_1 = 3,
	SD_Q5_0 = 6,
	SD_Q5_1 = 7,
	SD_Q8_0 = 8,
	SD_Q8_1 = 9,
	SD_Q2_K = 10,
	SD_Q3_K = 11,
	SD_Q4_K = 12,
	SD_Q5_K = 13,
	SD_Q6_K = 14,
	SD_Q8_K = 15,
	SD_IQ2_XXS = 16,
	SD_IQ2_XS  = 17,
	SD_IQ3_XXS = 18,
	SD_IQ1_S   = 19,
	SD_IQ4_NL  = 20,
	SD_IQ3_S   = 21,
	SD_IQ2_S   = 22,
	SD_IQ4_XS  = 23,
	SD_I8      = 24,
	SD_I16     = 25,
	SD_I32     = 26,
	SD_I64     = 27,
	SD_F64     = 28,
	SD_IQ1_M   = 29,
	SD_BF16    = 30,
	SD_TQ1_0 = 34,
	SD_TQ2_0 = 35,
	SD_MXFP4 = 39,
	SD_NVFP4 = 40,
	SD_Q1_0  = 41,
	SD_COUNT = 42,
};

typedef struct {
	uint32_t width;
	uint32_t height;
	uint32_t channel;
	uint8_t* data;
} sd_image_t;

typedef struct {
	bool enabled;
	bool temporal_tiling;
	int tile_size_x;
	int tile_size_y;
	float target_overlap;
	float rel_size_x;
	float rel_size_y;
	const char* extra_tiling_args;
} sd_tiling_params_t;

typedef struct {
	int* layers;
	size_t layer_count;
	float layer_start;
	float layer_end;
	float scale;
} sd_slg_params_t;

typedef struct {
	float txt_cfg;
	float img_cfg;
	float distilled_guidance;
	sd_slg_params_t slg;
} sd_guidance_params_t;

typedef struct {
	sd_guidance_params_t guidance;
	enum scheduler_t scheduler;
	enum sample_method_t sample_method;
	int sample_steps;
	float eta;
	int shifted_timestep;
	float* custom_sigmas;
	int custom_sigmas_count;
	float flow_shift;
	const char* extra_sample_args;
} sd_sample_params_t;

typedef struct {
	bool is_high_noise;
	float multiplier;
	const char* path;
} sd_lora_t;

typedef struct {
	const sd_lora_t* loras;
	uint32_t lora_count;
	const char* prompt;
	const char* negative_prompt;
	int clip_skip;
	sd_image_t init_image;
	sd_image_t* ref_images;
	int ref_images_count;
	bool auto_resize_ref_image;
	bool increase_ref_index;
	sd_image_t mask_image;
	int width;
	int height;
	sd_sample_params_t sample_params;
	float strength;
	int64_t seed;
	int batch_count;
	sd_image_t control_image;
	float control_strength;
	sd_tiling_params_t vae_tiling_params;
} sd_img_gen_params_t;

typedef struct {
	const char* model_path;
	const char* clip_l_path;
	const char* clip_g_path;
	const char* t5xxl_path;
	const char* diffusion_model_path;
	const char* vae_path;
	const char* taesd_path;
	const char* tensor_type_rules;
	bool vae_decode_only;
	bool free_params_immediately;
	int n_threads;
	enum sd_type_t wtype;
	enum rng_type_t rng_type;
	bool enable_mmap;
	bool keep_clip_on_cpu;
	bool keep_vae_on_cpu;
	bool flash_attn;
	bool diffusion_flash_attn;
	const char* backend;
} sd_ctx_params_t;

typedef struct sd_ctx_t sd_ctx_t;

extern void sd_ctx_params_init(sd_ctx_params_t* params);
extern sd_ctx_t* new_sd_ctx(const sd_ctx_params_t* params);
extern void free_sd_ctx(sd_ctx_t* ctx);
extern void sd_sample_params_init(sd_sample_params_t* params);
extern void sd_img_gen_params_init(sd_img_gen_params_t* params);
extern sd_image_t* generate_image(sd_ctx_t* ctx, const sd_img_gen_params_t* params);
extern const char* sd_get_system_info(void);
extern int32_t sd_get_num_physical_cores(void);
*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"
)

type SampleMethod int

const (
	SampleEuler     SampleMethod = 0
	SampleEulerA    SampleMethod = 1
	SampleHeun      SampleMethod = 2
	SampleDPM2      SampleMethod = 3
	SampleDPMPP2SA  SampleMethod = 4
	SampleDPMPP2M   SampleMethod = 5
	SampleDPMPP2Mv2 SampleMethod = 6
	SampleIPNDM     SampleMethod = 7
	SampleIPNDMV    SampleMethod = 8
	SampleLCM       SampleMethod = 9
	SampleDDIM      SampleMethod = 10
	SampleTCD       SampleMethod = 11
)

type Scheduler int

const (
	SchedDiscrete    Scheduler = 0
	SchedKarras      Scheduler = 1
	SchedExponential Scheduler = 2
	SchedAYS         Scheduler = 3
	SchedSimple      Scheduler = 7
)

type SDType int

const (
	SDTypeF32  SDType = 0
	SDTypeF16  SDType = 1
	SDTypeQ4_0 SDType = 2
	SDTypeQ4_1 SDType = 3
	SDTypeQ5_0 SDType = 6
	SDTypeQ5_1 SDType = 7
	SDTypeQ8_0 SDType = 8
	SDTypeQ8_1 SDType = 9
	SDTypeQ4_K SDType = 12
	SDTypeQ5_K SDType = 13
	SDTypeQ6_K SDType = 14
	SDTypeQ8_K SDType = 15
)

type RNGType int

const (
	RNGStdDefault RNGType = 0
	RNGCUDA       RNGType = 1
	RNGCPU        RNGType = 2
)

type SDImage struct {
	Width   uint32
	Height  uint32
	Channel uint32
	Data    []byte
}

type CtxParams struct {
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
	WType              SDType
	RNGType            RNGType
	EnableMmap         bool
	KeepClipOnCPU      bool
	KeepVaeOnCPU       bool
	FlashAttn          bool
	DiffusionFlashAttn bool
	Backend            string
}

type SampleParams struct {
	Guidance      float32
	Scheduler     Scheduler
	SampleMethod  SampleMethod
	SampleSteps   int
	Eta           float32
	FlowShift     float32
}

type ImgGenParams struct {
	Prompt         string
	NegativePrompt string
	ClipSkip       int
	Width          int
	Height         int
	SampleParams   SampleParams
	Strength       float32
	Seed           int64
	BatchCount     int
	InitImage      *SDImage
}

type SDContext struct {
	mu  sync.Mutex
	ptr *C.sd_ctx_t
}

var (
	globalCtx     *SDContext
	globalCtxOnce sync.Once
	globalCtxMu   sync.Mutex
)

func InitGlobalContext(params CtxParams) error {
	var initErr error
	globalCtxOnce.Do(func() {
		ctx, err := NewContext(params)
		if err != nil {
			initErr = err
			return
		}
		globalCtx = ctx
	})
	return initErr
}

func GetGlobalContext() *SDContext {
	globalCtxMu.Lock()
	defer globalCtxMu.Unlock()
	return globalCtx
}

func NewContext(params CtxParams) (*SDContext, error) {
	var cParams C.sd_ctx_params_t
	C.sd_ctx_params_init(&cParams)

	if params.ModelPath != "" {
		cModelPath := C.CString(params.ModelPath)
		defer C.free(unsafe.Pointer(cModelPath))
		cParams.model_path = cModelPath
	}

	if params.ClipLPath != "" {
		cClipLPath := C.CString(params.ClipLPath)
		defer C.free(unsafe.Pointer(cClipLPath))
		cParams.clip_l_path = cClipLPath
	}

	if params.ClipGPath != "" {
		cClipGPath := C.CString(params.ClipGPath)
		defer C.free(unsafe.Pointer(cClipGPath))
		cParams.clip_g_path = cClipGPath
	}

	if params.T5xxlPath != "" {
		cT5Path := C.CString(params.T5xxlPath)
		defer C.free(unsafe.Pointer(cT5Path))
		cParams.t5xxl_path = cT5Path
	}

	if params.DiffusionModelPath != "" {
		cDiffPath := C.CString(params.DiffusionModelPath)
		defer C.free(unsafe.Pointer(cDiffPath))
		cParams.diffusion_model_path = cDiffPath
	}

	if params.VaePath != "" {
		cVaePath := C.CString(params.VaePath)
		defer C.free(unsafe.Pointer(cVaePath))
		cParams.vae_path = cVaePath
	}

	if params.TaeSDPath != "" {
		cTaePath := C.CString(params.TaeSDPath)
		defer C.free(unsafe.Pointer(cTaePath))
		cParams.taesd_path = cTaePath
	}

	if params.TensorTypeRules != "" {
		cRules := C.CString(params.TensorTypeRules)
		defer C.free(unsafe.Pointer(cRules))
		cParams.tensor_type_rules = cRules
	}

	if params.Backend != "" {
		cBackend := C.CString(params.Backend)
		defer C.free(unsafe.Pointer(cBackend))
		cParams.backend = cBackend
	}

	cParams.vae_decode_only = C.bool(params.VaeDecodeOnly)
	cParams.free_params_immediately = C.bool(params.FreeParamsImmed)
	cParams.n_threads = C.int(params.NThreads)
	cParams.wtype = C.enum_sd_type_t(params.WType)
	cParams.rng_type = C.enum_rng_type_t(params.RNGType)
	cParams.enable_mmap = C.bool(params.EnableMmap)
	cParams.keep_clip_on_cpu = C.bool(params.KeepClipOnCPU)
	cParams.keep_vae_on_cpu = C.bool(params.KeepVaeOnCPU)
	cParams.flash_attn = C.bool(params.FlashAttn)
	cParams.diffusion_flash_attn = C.bool(params.DiffusionFlashAttn)

	cCtx := C.new_sd_ctx(&cParams)
	if cCtx == nil {
		return nil, fmt.Errorf("创建SD上下文失败，请检查模型路径是否正确")
	}

	return &SDContext{ptr: cCtx}, nil
}

func (c *SDContext) Release() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ptr != nil {
		C.free_sd_ctx(c.ptr)
		c.ptr = nil
	}
}

func (c *SDContext) GenerateImage(params ImgGenParams) ([]SDImage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ptr == nil {
		return nil, fmt.Errorf("SD上下文未初始化")
	}

	if params.Prompt == "" {
		return nil, fmt.Errorf("提示词不能为空")
	}

	if params.Width <= 0 || params.Width > 4096 || params.Height <= 0 || params.Height > 4096 {
		return nil, fmt.Errorf("图像尺寸无效: %dx%d，合法范围: 64~4096", params.Width, params.Height)
	}

	if params.SampleParams.SampleSteps <= 0 || params.SampleParams.SampleSteps > 150 {
		return nil, fmt.Errorf("采样步数无效: %d，合法范围: 1~150", params.SampleParams.SampleSteps)
	}

	if params.BatchCount <= 0 || params.BatchCount > 32 {
		return nil, fmt.Errorf("批量生成数量无效: %d，合法范围: 1~32", params.BatchCount)
	}

	var cParams C.sd_img_gen_params_t
	C.sd_img_gen_params_init(&cParams)

	cPrompt := C.CString(params.Prompt)
	defer C.free(unsafe.Pointer(cPrompt))
	cParams.prompt = cPrompt

	if params.NegativePrompt != "" {
		cNegPrompt := C.CString(params.NegativePrompt)
		defer C.free(unsafe.Pointer(cNegPrompt))
		cParams.negative_prompt = cNegPrompt
	}

	cParams.clip_skip = C.int(params.ClipSkip)
	cParams.width = C.int(params.Width)
	cParams.height = C.int(params.Height)
	cParams.seed = C.int64_t(params.Seed)
	cParams.batch_count = C.int(params.BatchCount)
	cParams.strength = C.float(params.Strength)

	cParams.sample_params.sample_steps = C.int(params.SampleParams.SampleSteps)
	cParams.sample_params.sample_method = C.enum_sample_method_t(params.SampleParams.SampleMethod)
	cParams.sample_params.scheduler = C.enum_scheduler_t(params.SampleParams.Scheduler)
	cParams.sample_params.guidance.txt_cfg = C.float(params.SampleParams.Guidance)
	cParams.sample_params.eta = C.float(params.SampleParams.Eta)
	cParams.sample_params.flow_shift = C.float(params.SampleParams.FlowShift)

	var cInitData unsafe.Pointer
	if params.InitImage != nil {
		cParams.init_image.width = C.uint32_t(params.InitImage.Width)
		cParams.init_image.height = C.uint32_t(params.InitImage.Height)
		cParams.init_image.channel = C.uint32_t(params.InitImage.Channel)
		if len(params.InitImage.Data) > 0 {
			cInitData = C.CBytes(params.InitImage.Data)
			cParams.init_image.data = (*C.uint8_t)(cInitData)
		}
	}
	defer func() {
		if cInitData != nil {
			C.free(cInitData)
		}
	}()

	cResult := C.generate_image(c.ptr, &cParams)
	if cResult == nil {
		return nil, fmt.Errorf("图像生成失败")
	}

	var images []SDImage
	for i := 0; i < params.BatchCount; i++ {
		imgPtr := (*C.sd_image_t)(unsafe.Pointer(uintptr(unsafe.Pointer(cResult)) + uintptr(i)*unsafe.Sizeof(*cResult)))
		if imgPtr.data == nil {
			continue
		}
		dataSize := imgPtr.width * imgPtr.height * imgPtr.channel
		img := SDImage{
			Width:   uint32(imgPtr.width),
			Height:  uint32(imgPtr.height),
			Channel: uint32(imgPtr.channel),
			Data:    C.GoBytes(unsafe.Pointer(imgPtr.data), C.int(dataSize)),
		}
		images = append(images, img)
	}

	C.free(unsafe.Pointer(cResult))

	return images, nil
}

func ReleaseGlobalContext() {
	if globalCtx != nil {
		globalCtx.Release()
		globalCtx = nil
	}
	globalCtxOnce = sync.Once{}
}

func GetSystemInfo() string {
	cInfo := C.sd_get_system_info()
	if cInfo == nil {
		return "未知"
	}
	return C.GoString(cInfo)
}

func GetPhysicalCores() int {
	return int(C.sd_get_num_physical_cores())
}