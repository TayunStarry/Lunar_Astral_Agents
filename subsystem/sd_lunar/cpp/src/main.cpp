#include "stable-diffusion.h"

#define STB_IMAGE_IMPLEMENTATION
#include "stb_image.h"
#define STB_IMAGE_WRITE_IMPLEMENTATION
#include "stb_image_write.h"

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>
#include <vector>

static void print_usage(const char* prog) {
    fprintf(stderr, "Usage: %s [options]\n", prog);
    fprintf(stderr, "\nOptions:\n");
    fprintf(stderr, "  -M, --model <path>          Diffusion model path (required)\n");
    fprintf(stderr, "  --vae <path>                VAE model path (required)\n");
    fprintf(stderr, "  --clip_l <path>             CLIP-L model path\n");
    fprintf(stderr, "  --clip_g <path>             CLIP-G model path\n");
    fprintf(stderr, "  --t5xxl <path>              T5-XXL model path\n");
    fprintf(stderr, "  --llm <path>                LLM prompt refinement model path\n");
    fprintf(stderr, "  --llm_vision <path>         LLM vision model path\n");
    fprintf(stderr, "  -p, --prompt <text>         Positive prompt (required)\n");
    fprintf(stderr, "  -n, --negative <text>       Negative prompt\n");
    fprintf(stderr, "  -W, --width <int>           Output width (default: 512)\n");
    fprintf(stderr, "  -H, --height <int>          Output height (default: 512)\n");
    fprintf(stderr, "  -s, --steps <int>           Sampling steps (default: 20)\n");
    fprintf(stderr, "  --cfg-scale <float>         CFG scale (default: 7.0)\n");
    fprintf(stderr, "  --seed <int>                Random seed (default: 42)\n");
    fprintf(stderr, "  -b, --batch <int>           Batch count (default: 1)\n");
    fprintf(stderr, "  --strength <float>          Denoising strength for img2img (default: 0.75)\n");
    fprintf(stderr, "  --init-img <path>           Initial image path for img2img\n");
    fprintf(stderr, "  -o, --output <path>         Output image path (required)\n");
    fprintf(stderr, "  --sampler <method>          Sampling method (default: euler_a)\n");
    fprintf(stderr, "  --scheduler <name>          Scheduler (default: karras)\n");
    fprintf(stderr, "  -t, --threads <int>         Thread count (default: 8)\n");
    fprintf(stderr, "  --flash-attn                Enable flash attention\n");
    fprintf(stderr, "  --diffusion-fa              Enable diffusion flash attention\n");
    fprintf(stderr, "  --vae-tiling                Enable VAE tiling\n");
    fprintf(stderr, "  --type <type>               Weight type (default: f16)\n");
    fprintf(stderr, "  -h, --help                  Show this help\n");
    fprintf(stderr, "\n");
}

static bool load_image(const char* path, sd_image_t* out_img) {
    int w, h, c;
    unsigned char* data = stbi_load(path, &w, &h, &c, 3);
    if (!data) {
        fprintf(stderr, "Failed to load image: %s\n", path);
        return false;
    }
    out_img->width   = (uint32_t)w;
    out_img->height  = (uint32_t)h;
    out_img->channel = 3;
    out_img->data    = (uint8_t*)malloc(w * h * 3);
    memcpy(out_img->data, data, w * h * 3);
    stbi_image_free(data);
    return true;
}

static bool save_image(const sd_image_t* images, int count, const char* output_path) {
    for (int i = 0; i < count; i++) {
        char path[1024];
        if (count == 1) {
            snprintf(path, sizeof(path), "%s", output_path);
        } else {
            const char* ext = strrchr(output_path, '.');
            if (ext) {
                std::string base(output_path, ext - output_path);
                snprintf(path, sizeof(path), "%s_%d%s", base.c_str(), i, ext);
            } else {
                snprintf(path, sizeof(path), "%s_%d.png", output_path, i);
            }
        }

        int ret = stbi_write_png(path,
                                 (int)images[i].width,
                                 (int)images[i].height,
                                 (int)images[i].channel,
                                 images[i].data,
                                 0);
        if (!ret) {
            fprintf(stderr, "Failed to write output image: %s\n", path);
            return false;
        }
        printf("Saved: %s\n", path);
    }
    return true;
}

static void safe_free(void* ptr) {
    if (ptr) free(ptr);
}

static bool str_eq(const char* a, const char* b) {
    return a && b && strcmp(a, b) == 0;
}

int main(int argc, char** argv) {
    if (argc < 2) {
        print_usage(argv[0]);
        return 1;
    }

    const char* model_path        = nullptr;
    const char* vae_path          = nullptr;
    const char* clip_l_path       = nullptr;
    const char* clip_g_path       = nullptr;
    const char* t5xxl_path        = nullptr;
    const char* llm_path          = nullptr;
    const char* llm_vision_path   = nullptr;
    const char* prompt            = nullptr;
    const char* negative_prompt   = nullptr;
    const char* output_path       = nullptr;
    const char* init_img_path     = nullptr;
    const char* sampler           = "euler_a";
    const char* scheduler         = "karras";
    const char* wtype_str         = "f16";

    int width          = 512;
    int height         = 512;
    int steps          = 20;
    int batch_count    = 1;
    int threads        = 8;
    float cfg_scale    = 7.0f;
    float strength     = 0.75f;
    int64_t seed       = 42;
    bool flash_attn    = false;
    bool diffusion_fa  = false;
    bool vae_tiling    = false;

    for (int i = 1; i < argc; i++) {
        if (str_eq(argv[i], "-h") || str_eq(argv[i], "--help")) {
            print_usage(argv[0]);
            return 0;
        } else if (str_eq(argv[i], "-M") || str_eq(argv[i], "--model")) {
            model_path = argv[++i];
        } else if (str_eq(argv[i], "--vae")) {
            vae_path = argv[++i];
        } else if (str_eq(argv[i], "--clip_l")) {
            clip_l_path = argv[++i];
        } else if (str_eq(argv[i], "--clip_g")) {
            clip_g_path = argv[++i];
        } else if (str_eq(argv[i], "--t5xxl")) {
            t5xxl_path = argv[++i];
        } else if (str_eq(argv[i], "--llm")) {
            llm_path = argv[++i];
        } else if (str_eq(argv[i], "--llm_vision")) {
            llm_vision_path = argv[++i];
        } else if (str_eq(argv[i], "-p") || str_eq(argv[i], "--prompt")) {
            prompt = argv[++i];
        } else if (str_eq(argv[i], "-n") || str_eq(argv[i], "--negative")) {
            negative_prompt = argv[++i];
        } else if (str_eq(argv[i], "-W") || str_eq(argv[i], "--width")) {
            width = atoi(argv[++i]);
        } else if (str_eq(argv[i], "-H") || str_eq(argv[i], "--height")) {
            height = atoi(argv[++i]);
        } else if (str_eq(argv[i], "-s") || str_eq(argv[i], "--steps")) {
            steps = atoi(argv[++i]);
        } else if (str_eq(argv[i], "--cfg-scale")) {
            cfg_scale = (float)atof(argv[++i]);
        } else if (str_eq(argv[i], "--seed")) {
            seed = atoll(argv[++i]);
        } else if (str_eq(argv[i], "-b") || str_eq(argv[i], "--batch")) {
            batch_count = atoi(argv[++i]);
        } else if (str_eq(argv[i], "--strength")) {
            strength = (float)atof(argv[++i]);
        } else if (str_eq(argv[i], "--init-img")) {
            init_img_path = argv[++i];
        } else if (str_eq(argv[i], "-o") || str_eq(argv[i], "--output")) {
            output_path = argv[++i];
        } else if (str_eq(argv[i], "--sampler")) {
            sampler = argv[++i];
        } else if (str_eq(argv[i], "--scheduler")) {
            scheduler = argv[++i];
        } else if (str_eq(argv[i], "-t") || str_eq(argv[i], "--threads")) {
            threads = atoi(argv[++i]);
        } else if (str_eq(argv[i], "--flash-attn")) {
            flash_attn = true;
        } else if (str_eq(argv[i], "--diffusion-fa")) {
            diffusion_fa = true;
        } else if (str_eq(argv[i], "--vae-tiling")) {
            vae_tiling = true;
        } else if (str_eq(argv[i], "--type")) {
            wtype_str = argv[++i];
        } else {
            fprintf(stderr, "Unknown argument: %s\n", argv[i]);
            print_usage(argv[0]);
            return 1;
        }
    }

    if (!model_path || !vae_path || !prompt || !output_path) {
        fprintf(stderr, "Error: --model, --vae, --prompt, and --output are required\n\n");
        print_usage(argv[0]);
        return 1;
    }

    sd_ctx_params_t ctx_params;
    sd_ctx_params_init(&ctx_params);
    ctx_params.diffusion_model_path = model_path;
    ctx_params.vae_path             = vae_path;
    if (clip_l_path) ctx_params.clip_l_path = clip_l_path;
    if (clip_g_path) ctx_params.clip_g_path = clip_g_path;
    if (t5xxl_path) ctx_params.t5xxl_path = t5xxl_path;
    if (llm_path) ctx_params.llm_path = llm_path;
    if (llm_vision_path) ctx_params.llm_vision_path = llm_vision_path;
    ctx_params.n_threads    = threads;
    ctx_params.wtype        = str_to_sd_type(wtype_str);
    ctx_params.flash_attn   = flash_attn;
    ctx_params.diffusion_flash_attn = diffusion_fa;
    ctx_params.offload_params_to_cpu = false;
    ctx_params.enable_mmap  = true;
    ctx_params.free_params_immediately = false;

    sd_ctx_t* ctx = new_sd_ctx(&ctx_params);
    if (!ctx) {
        fprintf(stderr, "Error: Failed to create SD context\n");
        return 1;
    }

    sd_img_gen_params_t gen_params;
    sd_img_gen_params_init(&gen_params);
    gen_params.prompt          = prompt;
    gen_params.negative_prompt = negative_prompt ? negative_prompt : "";
    gen_params.width           = width;
    gen_params.height          = height;
    gen_params.batch_count     = batch_count;
    gen_params.seed            = seed;
    gen_params.strength        = strength;
    gen_params.sample_params.sample_steps = steps;
    gen_params.sample_params.guidance.txt_cfg = cfg_scale;
    gen_params.sample_params.sample_method = str_to_sample_method(sampler);
    gen_params.sample_params.scheduler     = str_to_scheduler(scheduler);

    if (vae_tiling) {
        gen_params.vae_tiling_params.enabled     = true;
        gen_params.vae_tiling_params.tile_size_x = 256;
        gen_params.vae_tiling_params.tile_size_y = 256;
    }

    sd_image_t init_image = {};
    if (init_img_path) {
        if (!load_image(init_img_path, &init_image)) {
            free_sd_ctx(ctx);
            return 1;
        }
        gen_params.init_image = init_image;
    }

    sd_image_t* results = generate_image(ctx, &gen_params);
    if (!results) {
        fprintf(stderr, "Error: Image generation failed\n");
        free_sd_ctx(ctx);
        safe_free(init_image.data);
        return 1;
    }

    bool saved = save_image(results, batch_count, output_path);

    for (int i = 0; i < batch_count; i++) {
        safe_free(results[i].data);
    }
    safe_free(results);
    safe_free(init_image.data);
    free_sd_ctx(ctx);

    return saved ? 0 : 1;
}