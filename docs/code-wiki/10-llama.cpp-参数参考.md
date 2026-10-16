# 10 llama-server 文本推理参数手册

> [🏠 手册门户](README.md) ｜ [◀ 上一模块](09-LTPX协议-月华工具包.md) ｜ [下一模块 ▶](11-stable-diffusion.cpp-参数参考.md)

## 一、功能介绍

llama-server 是平台内置的大语言模型推理服务，负责承载对话、文本生成、代码补全、多模态（图像/视频理解）、嵌入与重排序等全部「语言类」推理任务：它以 GGUF 格式加载本地模型，常驻内存并通过 HTTP 接口对外提供生成能力，月华的对话链路与相关插件都依赖它产出回答。本手册是 llama-server 全部运行参数的速查手册，用于在你需要针对显存、速度、上下文或功能能力做调整时，快速查清每个参数的含义、默认值与推荐取值。参数实际读入形态与模型切换方式见 [08-构建运行与配置](08-构建运行与配置.md)，在平台中的角色见 [02-核心系统-钛宇-月华](02-核心系统-钛宇-月华.md)。

## 二、使用方法

本手册不是「必读」，多数情况下使用平台默认参数即可正常对话。只在出现下表场景时，才需要针对性地查阅并调整对应参数。

| 你想解决的问题 | 建议先看的参数分组 |
|----------------|--------------------|
| 显存不足或爆显存 | 性能与显存（层数卸载、KV 缓存类型、Flash Attention、`--fit` 系列） |
| 加载模型失败或加载很慢 | 模型来源与下载、性能与显存（`--mmap`/`--mlock`） |
| 上下文不够长、长文被截断 | 模型与上下文（`--ctx-size`、`--rope-scaling` 与 YaRN 系列） |
| 推理速度慢 | 性能与显存（线程数、批大小、层数卸载、推测解码） |
| 回答风格、重复度、随机性不满意 | 采样与解码 |
| 需要图像/视频理解 | 多模态 |
| 需要向量检索或重排序 | 嵌入与重排序 |
| 端口冲突或无法被外部访问 | 服务与并行 |
| 已启动多个实例、想查设备 | 帮助与信息（`--list-devices` 等） |

## 三、功能特性

| 特性 | 说明 |
|------|------|
| GGUF 模型加载 | 直接加载本机 GGUF 量化模型，支持多种权重与量化类型 |
| 多模态推理 | 通过多模态投影文件理解图像，并支持视频抽帧理解 |
| 嵌入与重排序 | 提供嵌入向量与重排序端点，供检索类任务使用 |
| 推测解码 | 用草稿模型或 ngram 预测加速生成，可在几乎不损失质量的前提下提速 |
| 量化 KV 缓存 | KV 缓存可用 f16、q8_0、q4_0 等类型，显著降低显存占用 |
| 采样与约束 | 提供温度、top-k/top-p、DRY、Mirostat 等采样，以及语法与 JSON Schema 约束生成 |
| 服务与并行 | 多插槽并行、连续批量、多 GPU 拆分、路由服务器多模型管理 |
| 聊天与推理模板 | 内置多款聊天模板，支持推理/思考标签的解析与预算控制 |
| 内存与卸载策略 | 支持 mmap、mlock、权重卸载、CPU/MoE 分层保留等显存优化手段 |
| 日志与可观测 | 分级日志、彩色输出、Prometheus 指标端点与插槽监控端点 |

## 四、运行参数

### 帮助与信息

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-h`, `--help`, `--usage` | 打印使用说明并退出 | — | 只用于自查参数 |
| `--version` | 显示版本与构建信息 | — | 排障先确认版本 |
| `--license` | 显示源代码许可证与依赖项 | — | 合规核查时使用 |
| `-cl`, `--cache-list` | 显示缓存中的模型列表 | — | 确认本地已缓存的模型 |
| `--completion-bash` | 打印自动补全脚本内容 | — | 一般无需使用 |
| `--list-devices` | 打印可用设备列表并退出 | — | 配置卸载设备前先执行它确认设备名 |

### 线程与 CPU

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-t`, `--threads N` | 生成时使用的 CPU 线程数（环境变量 `LLAMA_ARG_THREADS`） | -1 | 纯 CPU 推理时设为物理核心数附近 |
| `-tb`, `--threads-batch N` | 批量和提示处理时使用的线程数 | 同 `--threads` | 一般与 `--threads` 保持一致 |
| `-C`, `--cpu-mask M` | CPU 亲和性掩码，任意长度十六进制数 | 空 | 多路 CPU 绑定核心时使用 |
| `-Cr`, `--cpu-range lo-hi` | CPU 亲和性范围，与 `--cpu-mask` 互补 | — | 按核心区间绑定 |
| `--cpu-strict <0\|1>` | 是否使用严格的 CPU 放置 | 0 | 一般保持 0 |
| `--prio N` | 进程/线程优先级：low(-1)、normal(0)、medium(1)、high(2)、realtime(3) | 0 | 桌面交互可设 1 |
| `--poll <0...100>` | 等待工作的轮询级别，0 为不轮询 | 50 | 延迟敏感可适当调高 |
| `-Cb`, `--cpu-mask-batch M` | 批量处理的 CPU 亲和性掩码 | 同 `--cpu-mask` | 一般保持不变 |
| `-Crb`, `--cpu-range-batch lo-hi` | 批量处理的 CPU 亲和性范围 | — | 一般保持不变 |
| `--cpu-strict-batch <0\|1>` | 批量处理是否使用严格的 CPU 放置 | 同 `--cpu-strict` | 一般保持不变 |
| `--prio-batch N` | 批量处理进程/线程优先级 | 0 | 一般保持不变 |
| `--poll-batch <0\|1>` | 批量处理是否使用轮询等待工作 | 同 `--poll` | 一般保持不变 |
| `--numa TYPE` | 尝试在某些 NUMA 系统上有帮助的优化，取值 distribute、isolate、numactl | — | 多路 NUMA 机器可尝试 distribute |
| `--threads-http N` | 处理 HTTP 请求的线程数（环境变量 `LLAMA_ARG_THREADS_HTTP`） | -1 | 高并发时按需调高 |

### 模型与上下文

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-m`, `--model FNAME` | 要加载的模型路径（环境变量 `LLAMA_ARG_MODEL`） | — | 必填，指向 GGUF 文件 |
| `-c`, `--ctx-size N` | 提示上下文大小，0 表示从模型加载（环境变量 `LLAMA_ARG_CTX_SIZE`） | 0 | 长文场景按需调大，注意吃显存 |
| `-n`, `--predict`, `--n-predict N` | 要预测的 token 数量，-1 表示无限（环境变量 `LLAMA_ARG_N_PREDICT`） | -1 | 需要限制回答长度时设上限 |
| `-b`, `--batch-size N` | 逻辑最大批量大小（环境变量 `LLAMA_ARG_BATCH`） | 2048 | 一般保持默认 |
| `-ub`, `--ubatch-size N` | 物理最大批量大小（环境变量 `LLAMA_ARG_UBATCH`） | 512 | 提示处理慢可适当调大 |
| `--keep N` | 从初始提示中保留的 token 数量，-1 表示全部 | 0 | 一般保持默认 |
| `--swa-full` | 使用完整大小的 SWA 缓存（环境变量 `LLAMA_ARG_SWA_FULL`） | false | 短上下文模型无需开启 |
| `-fa`, `--flash-attn [on\|off\|auto]` | 设置 Flash Attention 使用（环境变量 `LLAMA_ARG_FLASH_ATTN`） | auto | 显存紧张时建议开启 |
| `--perf`, `--no-perf` | 是否启用内部性能计时（环境变量 `LLAMA_ARG_PERF`） | false | 排查性能问题时临时开启 |
| `-e`, `--escape`, `--no-escape` | 是否处理转义序列（`\n`、`\r`、`\t` 等） | true | 一般保持 true |
| `--rope-scaling {none,linear,yarn}` | RoPE 频率缩放方法（环境变量 `LLAMA_ARG_ROPE_SCALING_TYPE`） | linear（除非模型另有指定） | 扩展上下文时优先尝试 yarn |
| `--rope-scale N` | RoPE 上下文缩放因子，将上下文扩大 N 倍 | — | 扩展上下文时设置 |
| `--rope-freq-base N` | RoPE 基础频率，用于 NTK 感知缩放 | 从模型加载 | 一般保持默认 |
| `--rope-freq-scale N` | RoPE 频率缩放因子，将上下文扩大 1/N 倍 | — | 一般保持默认 |
| `--yarn-orig-ctx N` | YaRN 模型原始上下文大小，0 表示模型训练上下文 | 0 | 用 YaRN 时按模型设定 |
| `--yarn-ext-factor N` | YaRN 外推混合因子，0.0 表示完全插值 | -1.00 | 一般保持默认 |
| `--yarn-attn-factor N` | YaRN 缩放 sqrt(t) 或注意力幅度 | -1.00 | 一般保持默认 |
| `--yarn-beta-slow N` | YaRN 高校正维度或 alpha | -1.00 | 一般保持默认 |
| `--yarn-beta-fast N` | YaRN 低校正维度或 beta | -1.00 | 一般保持默认 |

### 性能与显存

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-ngl`, `--gpu-layers`, `--n-gpu-layers N` | 存储在 VRAM 中的最大层数，可为确切数字、auto 或 all（环境变量 `LLAMA_ARG_N_GPU_LAYERS`） | auto | 显存不足时降低层数 |
| `-sm`, `--split-mode {none,layer,row,tensor}` | 多 GPU 拆分方式：none 单卡、layer 按层流水线、row 按行并行、tensor 权重与 KV 并行 | layer | 多卡优先 layer |
| `-ts`, `--tensor-split N0,N1,...` | 卸载到每个 GPU 的模型比例，逗号分隔 | — | 多卡显存不等时手动分配 |
| `-mg`, `--main-gpu INDEX` | 主 GPU 序号（环境变量 `LLAMA_ARG_MAIN_GPU`） | 0 | 多卡时指定主卡 |
| `-dev`, `--device <dev1,dev2,..>` | 用于卸载的设备列表，none 表示不卸载（环境变量 `LLAMA_ARG_DEVICE`） | — | 先执行 `--list-devices` 查看设备名 |
| `-fit`, `--fit [on\|off]` | 是否调整未设置的参数以适应设备内存（环境变量 `LLAMA_ARG_FIT`） | on | 建议保持开启 |
| `-fitt`, `--fit-target MiB0,...` | `--fit` 的每个设备余量，单值广播到所有设备（环境变量 `LLAMA_ARG_FIT_TARGET`） | 1024 | 显存吃紧可调高余量 |
| `-fitc`, `--fit-ctx N` | `--fit` 可设置的最小 ctx 大小（环境变量 `LLAMA_ARG_FIT_CTX`） | 4096 | 保证自动适配后上下文不过小 |
| `-ot`, `--override-tensor <pattern>=<buffer>,...` | 覆盖张量缓冲区类型（环境变量 `LLAMA_ARG_OVERRIDE_TENSOR`） | — | 精细控制部分张量落盘位置 |
| `-cmoe`, `--cpu-moe` | 将所有混合专家（MoE）权重保留在 CPU（环境变量 `LLAMA_ARG_CPU_MOE`） | — | MoE 模型省显存利器 |
| `-ncmoe`, `--n-cpu-moe N` | 将前 N 层的 MoE 权重保留在 CPU（环境变量 `LLAMA_ARG_N_CPU_MOE`） | — | 按显存缺口逐层下调 |
| `-ctk`, `--cache-type-k TYPE` | K 的 KV 缓存数据类型（`LLAMA_ARG_CACHE_TYPE_K`） | f16 | 显存紧张可降为 q8_0 |
| `-ctv`, `--cache-type-v TYPE` | V 的 KV 缓存数据类型（`LLAMA_ARG_CACHE_TYPE_V`） | f16 | 显存紧张可降为 q8_0 |
| `-kvo`, `--kv-offload` / `-nkvo`, `--no-kv-offload` | 是否启用 KV 缓存卸载（环境变量 `LLAMA_ARG_KV_OFFLOAD`） | 启用 | 一般保持启用 |
| `--repack`, `-nr`, `--no-repack` | 是否启用权重重新打包（环境变量 `LLAMA_ARG_REPACK`） | 启用 | 一般保持启用 |
| `--no-host` | 绕过主机缓冲区，允许使用额外缓冲区（环境变量 `LLAMA_ARG_NO_HOST`） | — | 特定后端排障时使用 |
| `-dt`, `--defrag-thold N` | KV 缓存碎片整理阈值（已弃用，环境变量 `LLAMA_ARG_DEFRAG_THOLD`） | — | 已弃用，不建议使用 |
| `--rpc SERVERS` | 逗号分隔的 RPC 服务器列表（host:port，环境变量 `LLAMA_ARG_RPC`） | — | 跨机卸载算力时使用 |
| `--mlock` | 强制系统保持模型在内存中，而不是交换或压缩（环境变量 `LLAMA_ARG_MLOCK`） | — | 内存充足且需稳定延迟时开启 |
| `--mmap`, `--no-mmap` | 是否内存映射模型，禁用 mmap 加载较慢但可能减少页面置换（环境变量 `LLAMA_ARG_MMAP`） | 启用 | 一般保持启用 |
| `-dio`, `--direct-io`, `-ndio`, `--no-direct-io` | 使用 DirectIO（如果可用，环境变量 `LLAMA_ARG_DIO`） | 禁用 | 需要精确控制磁盘读取时开启 |
| `--check-tensors` | 检查模型张量数据是否包含无效值 | false | 模型疑似异常时开启排查 |
| `--override-kv KEY=TYPE:VALUE,...` | 高级选项，通过键覆盖模型元数据，类型为 int、float、bool、str | — | 仅高级排障使用 |
| `--op-offload`, `--no-op-offload` | 是否将主机张量操作卸载到设备 | true | 一般保持默认 |

### LoRA 与控制向量

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `--lora FNAME` | LoRA 适配器路径，逗号分隔可加载多个 | — | 需要风格或能力微调时加载 |
| `--lora-scaled FNAME:SCALE,...` | 带用户定义缩放的 LoRA 适配器路径 | — | 需要控制权重大小时使用 |
| `--control-vector FNAME` | 添加控制向量，逗号分隔可添加多个 | — | 高级用法 |
| `--control-vector-scaled FNAME:SCALE,...` | 添加带自定义缩放的控制向量 | — | 高级用法 |
| `--control-vector-layer-range START END` | 应用控制向量的层范围，含起止 | — | 高级用法 |

### 模型来源与下载

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-mu`, `--model-url MODEL_URL` | 模型下载 URL（环境变量 `LLAMA_ARG_MODEL_URL`） | 未使用 | 需要远程拉取时使用 |
| `-dr`, `--docker-repo [<repo>/]<model>[:quant]` | Docker Hub 模型仓库，repo 默认为 ai/，quant 默认为 :latest（`LLAMA_ARG_DOCKER_REPO`） | 未使用 | 从 Docker Hub 取模型 |
| `-hf`, `-hfr`, `--hf-repo <user>/<model>[:quant]` | Hugging Face 模型仓库，quant 默认为 Q4_K_M，缺失则回退仓库首个文件；可用时自动下载 mmproj（`LLAMA_ARG_HF_REPO`） | 未使用 | 从 HF 直接拉取模型 |
| `-hff`, `--hf-file FILE` | Hugging Face 模型文件，覆盖 `--hf-repo` 中的 quant（`LLAMA_ARG_HF_FILE`） | 未使用 | 需指定具体文件时使用 |
| `-hfv`, `-hfrv`, `--hf-repo-v <user>/<model>[:quant]` | 声码器模型的 Hugging Face 仓库（`LLAMA_ARG_HF_REPO_V`） | 未使用 | 音频声码器场景 |
| `-hffv`, `--hf-file-v FILE` | 声码器模型的 Hugging Face 文件（`LLAMA_ARG_HF_FILE_V`） | 未使用 | 音频声码器场景 |
| `-hft`, `--hf-token TOKEN` | Hugging Face 访问令牌（环境变量 `HF_TOKEN`） | 取自 `HF_TOKEN` | 私有仓库时使用 |

### 日志与离线

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `--log-disable` | 禁用日志 | — | 只需结果输出时使用 |
| `--log-file FNAME` | 日志输出到文件（环境变量 `LLAMA_LOG_FILE`） | — | 长时间运行建议落盘 |
| `--log-colors [on\|off\|auto]` | 设置彩色日志（环境变量 `LLAMA_LOG_COLORS`） | auto | 一般保持默认 |
| `-v`, `--verbose`, `--log-verbose` | 将详细程度设为无限，记录所有消息 | — | 仅调试使用 |
| `--offline` | 离线模式，强制使用缓存、阻止网络访问（环境变量 `LLAMA_OFFLINE`） | — | 无网或禁止联网时开启 |
| `-lv`, `--verbosity N` | 设置详细程度阈值，0 通用、1 错误、2 警告、3 信息、4 跟踪、5 调试（`LLAMA_LOG_VERBOSITY`） | 3 | 排障时临时调高 |
| `--log-prefix`, `--no-log-prefix` | 是否启用日志消息前缀（环境变量 `LLAMA_ARG_LOG_PREFIX`） | — | 一般保持默认 |
| `--log-timestamps`, `--no-log-timestamps` | 是否启用日志消息时间戳（`LLAMA_ARG_LOG_TIMESTAMPS`） | — | 排障时建议启用 |

### 采样与解码

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `--samplers SAMPLERS` | 用于生成的采样器，按顺序以分号分隔 | penalties;dry;top_n_sigma;top_k;typ_p;top_p;min_p;xtc;temperature | 一般保持默认 |
| `-s`, `--seed SEED` | RNG 种子，-1 表示随机 | -1 | 需要复现时设固定值 |
| `--sampler-seq`, `--sampling-seq SEQUENCE` | 采样器简写序列 | edskypmxt | 一般保持默认 |
| `--ignore-eos` | 忽略结束流 token 并继续生成（等价于对 EOS 施加负偏置） | — | 需强制持续输出时使用 |
| `--temp`, `--temperature N` | 温度 | 0.80 | 编码/事实类可调低，创意类调高 |
| `--top-k N` | top-k 采样，0 表示禁用（环境变量 `LLAMA_ARG_TOP_K`） | 40 | 一般保持默认 |
| `--top-p N` | top-p 采样，1.0 表示禁用 | 0.95 | 一般保持默认 |
| `--min-p N` | min-p 采样，0.0 表示禁用 | 0.05 | 抑制低质量长尾 |
| `--top-nsigma`, `--top-n-sigma N` | top-n-sigma 采样，-1.0 表示禁用 | -1.00 | 一般保持禁用 |
| `--xtc-probability N` | xtc 概率，0.0 表示禁用 | 0.00 | 创意场景可尝试开启 |
| `--xtc-threshold N` | xtc 阈值，1.0 表示禁用 | 0.10 | 与 xtc 概率配套 |
| `--typical`, `--typical-p N` | 局部典型采样参数 p，1.0 表示禁用 | 1.00 | 一般保持禁用 |
| `--repeat-last-n N` | 考虑惩罚的最后 n 个 token，0 禁用、-1 用满上下文 | 64 | 长文重复时按需调大 |
| `--repeat-penalty N` | 惩罚重复 token 序列，1.0 表示禁用 | 1.00 | 重复严重时调高 |
| `--presence-penalty N` | 重复 alpha 存在惩罚，0.0 表示禁用 | 0.00 | 一般保持默认 |
| `--frequency-penalty N` | 重复 alpha 频率惩罚，0.0 表示禁用 | 0.00 | 一般保持默认 |
| `--dry-multiplier N` | DRY 采样乘数，0.0 表示禁用 | 0.00 | 抑制复读时开启 |
| `--dry-base N` | DRY 采样基值 | 1.75 | 一般保持默认 |
| `--dry-allowed-length N` | DRY 采样允许长度 | 2 | 一般保持默认 |
| `--dry-penalty-last-n N` | DRY 对最后 n 个 token 的惩罚，0 禁用、-1 用满上下文 | -1 | 一般保持默认 |
| `--dry-sequence-breaker STRING` | 为 DRY 添加序列分隔符并清除默认分隔符（默认 `\n`、`:`、`"`、`*`），传 none 不使用分隔符 | — | 需要自定义断句时使用 |
| `--adaptive-target N` | adaptive-p：选择接近此概率的 token，负值禁用 | -1.00 | 一般保持禁用 |
| `--adaptive-decay N` | adaptive-p：目标随时间适应的衰减率，越低越灵敏 | 0.90 | 一般保持默认 |
| `--dynatemp-range N` | 动态温度范围，0.0 表示禁用 | 0.00 | 需要动态随机性时开启 |
| `--dynatemp-exp N` | 动态温度指数 | 1.00 | 一般保持默认 |
| `--mirostat N` | 使用 Mirostat 采样，0 禁用、1 为 Mirostat、2 为 Mirostat 2.0；启用时 Top K、Nucleus、局部典型采样器被忽略 | 0 | 需要稳定困惑度时启用 |
| `--mirostat-lr N` | Mirostat 学习率，参数 eta | 0.10 | 一般保持默认 |
| `--mirostat-ent N` | Mirostat 目标熵，参数 tau | 5.00 | 一般保持默认 |
| `-l`, `--logit-bias TOKEN_ID(+/-)BIAS` | 修改指定 token 在补全中出现的可能性，用 + 或 - 调整偏置 | — | 需要定向增强或抑制某个 token |
| `--grammar GRAMMAR` | 限制生成的 BNF 类语法 | — | 需要严格格式时使用 |
| `--grammar-file FNAME` | 从文件读取语法 | — | 语法较长时用文件 |
| `-j`, `--json-schema SCHEMA` | 用 JSON Schema 限制生成（`{}` 表示任意 JSON 对象） | — | 需要结构化输出时使用 |
| `-jf`, `--json-schema-file FILE` | 包含 JSON Schema 的文件以限制生成 | — | Schema 较长时用文件 |
| `-bs`, `--backend-sampling` | 启用后端采样（实验性，环境变量 `LLAMA_ARG_BACKEND_SAMPLING`） | 禁用 | 仅实验场景 |

### 投机解码

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `--spec-draft-type-k`, `-ctkd`, `--cache-type-k-draft TYPE` | 草稿模型 K 的 KV 缓存类型（环境变量 `LLAMA_ARG_SPEC_DRAFT_CACHE_TYPE_K`） | f16 | 显存紧张可降为 q8_0 |
| `--spec-draft-type-v`, `-ctvd`, `--cache-type-v-draft TYPE` | 草稿模型 V 的 KV 缓存类型（`LLAMA_ARG_SPEC_DRAFT_CACHE_TYPE_V`） | f16 | 显存紧张可降为 q8_0 |
| `--spec-draft-hf`, `-hfd`, `-hfrd`, `--hf-repo-draft <user>/<model>[:quant]` | 与 `--hf-repo` 相同，但用于草稿模型（`LLAMA_ARG_SPEC_DRAFT_HF_REPO`） | 未使用 | 从 HF 拉取草稿模型 |
| `--spec-draft-threads`, `-td`, `--threads-draft N` | 草稿模型生成线程数 | 同 `--threads` | 一般保持默认 |
| `--spec-draft-threads-batch`, `-tbd`, `--threads-batch-draft N` | 草稿模型批量和提示处理线程数 | 同 `--spec-draft-threads` | 一般保持默认 |
| `--spec-draft-cpu-mask`, `-Cd`, `--cpu-mask-draft M` | 草稿模型 CPU 亲和性掩码 | 同 `--cpu-mask` | 一般保持默认 |
| `--spec-draft-cpu-range`, `-Crd`, `--cpu-range-draft lo-hi` | 草稿模型 CPU 亲和性范围 | — | 一般保持默认 |
| `--spec-draft-cpu-strict`, `--cpu-strict-draft <0\|1>` | 草稿模型是否严格 CPU 放置 | 同 `--cpu-strict` | 一般保持默认 |
| `--spec-draft-prio`, `--prio-draft N` | 草稿进程/线程优先级：0 normal、1 medium、2 high、3 realtime | 0 | 一般保持默认 |
| `--spec-draft-poll`, `--poll-draft <0\|1>` | 草稿模型是否轮询等待工作 | 同 `--poll` | 一般保持默认 |
| `--spec-draft-cpu-mask-batch`, `-Cbd`, `--cpu-mask-batch-draft M` | 草稿模型批量处理 CPU 亲和性掩码 | 同 `--cpu-mask` | 一般保持默认 |
| `--spec-draft-cpu-strict-batch`, `--cpu-strict-batch-draft <0\|1>` | 草稿模型批量处理是否严格 CPU 放置 | `--cpu-strict-draft` | 一般保持默认 |
| `--spec-draft-prio-batch`, `--prio-batch-draft N` | 草稿批量处理优先级 | 0 | 一般保持默认 |
| `--spec-draft-poll-batch`, `--poll-batch-draft <0\|1>` | 草稿批量处理是否轮询等待工作 | `--poll-draft` | 一般保持默认 |
| `--spec-draft-override-tensor`, `-otd`, `--override-tensor-draft <pattern>=<buffer>,...` | 覆盖草稿模型张量缓冲区类型 | — | 高级用法 |
| `--spec-draft-cpu-moe`, `-cmoed`, `--cpu-moe-draft` | 将草稿模型所有 MoE 权重保留在 CPU（`LLAMA_ARG_SPEC_DRAFT_CPU_MOE`） | — | MoE 草稿模型省显存 |
| `--spec-draft-n-cpu-moe`, `--spec-draft-ncmoe`, `-ncmoed`, `--n-cpu-moe-draft N` | 将草稿模型前 N 层 MoE 权重保留在 CPU（`LLAMA_ARG_SPEC_DRAFT_N_CPU_MOE`） | — | 按显存缺口逐层下调 |
| `--spec-draft-n-max N` | 投机解码时草稿 token 数量（`LLAMA_ARG_SPEC_DRAFT_N_MAX`） | 3 | 命中率高可适当调大 |
| `--spec-draft-n-min N` | 投机解码时使用的最小草稿 token 数量（`LLAMA_ARG_SPEC_DRAFT_N_MIN`） | 0 | 一般保持默认 |
| `--spec-draft-p-split`, `--draft-p-split P` | 投机解码拆分概率（`LLAMA_ARG_SPEC_DRAFT_P_SPLIT`） | 0.10 | 一般保持默认 |
| `--spec-draft-p-min`, `--draft-p-min P` | 最小投机解码概率（贪婪，`LLAMA_ARG_SPEC_DRAFT_P_MIN`） | 0.00 | 一般保持默认 |
| `--spec-draft-backend-sampling`, `--no-spec-draft-backend-sampling` | 是否将草稿采样卸载到后端（`LLAMA_ARG_SPEC_DRAFT_BACKEND_SAMPLING`） | 启用 | 一般保持默认 |
| `--spec-draft-device`, `-devd`, `--device-draft <dev1,dev2,..>` | 用于卸载草稿模型的设备列表，none 表示不卸载 | — | 先执行 `--list-devices` 查看设备名 |
| `--spec-draft-ngl`, `-ngld`, `--gpu-layers-draft`, `--n-gpu-layers-draft N` | 草稿模型存储在 VRAM 的最大层数（`LLAMA_ARG_N_GPU_LAYERS_DRAFT`） | auto | 显存不足时降低 |
| `--spec-draft-model`, `-md`, `--model-draft FNAME` | 投机解码的草稿模型（环境变量 `LLAMA_ARG_SPEC_DRAFT_MODEL`） | 未使用 | 使用草稿模型加速时必填 |
| `--spec-type none,draft_simple,draft-eagle3,draft-mtp,ngram-simple,ngram-map-k,ngram-map-k4v,ngram-mod,ngram-cache` | 要使用的投机解码类型列表（环境变量 `LLAMA_ARG_SPEC_TYPE`） | none | 需要提速时按模型选择 |
| `--spec-ngram-mod-n-min N` | 基于 ngram 的投机解码使用的最小 ngram token 数 | 48 | 一般保持默认 |
| `--spec-ngram-mod-n-max N` | 基于 ngram 的投机解码使用的最大 ngram token 数 | 64 | 一般保持默认 |
| `--spec-ngram-mod-n-match N` | ngram-mod 查找长度 | 24 | 一般保持默认 |
| `--spec-ngram-simple-size-n N` | ngram-simple 的查找 n-gram 长度 | 12 | 一般保持默认 |
| `--spec-ngram-simple-size-m N` | ngram-simple 的草稿 m-gram 长度 | 48 | 一般保持默认 |
| `--spec-ngram-simple-min-hits N` | ngram-simple 的最小命中数 | 1 | 一般保持默认 |
| `--spec-ngram-map-k-size-n N` | ngram-map-k 的查找 n-gram 长度 | 12 | 一般保持默认 |
| `--spec-ngram-map-k-size-m N` | ngram-map-k 的草稿 m-gram 长度 | 48 | 一般保持默认 |
| `--spec-ngram-map-k-min-hits N` | ngram-map-k 的最小命中数 | 1 | 一般保持默认 |
| `--spec-ngram-map-k4v-size-n N` | ngram-map-k4v 的查找 n-gram 长度 | 12 | 一般保持默认 |
| `--spec-ngram-map-k4v-size-m N` | ngram-map-k4v 的草稿 m-gram 长度 | 48 | 一般保持默认 |
| `--spec-ngram-map-k4v-min-hits N` | ngram-map-k4v 的最小命中数 | 1 | 一般保持默认 |
| `--draft`, `--draft-n`, `--draft-max N` | 已移除，改用 `--spec-draft-n-max` 或 `--spec-ngram-mod-n-max`（`LLAMA_ARG_DRAFT_MAX`） | — | 请勿使用 |
| `--draft-min`, `--draft-n-min N` | 已移除，改用 `--spec-draft-n-min` 或 `--spec-ngram-mod-n-min`（`LLAMA_ARG_DRAFT_MIN`） | — | 请勿使用 |
| `--spec-ngram-size-n N` | 已移除，改用相应的 `--spec-ngram-*-size-n` 或 `--spec-ngram-mod-n-match` | — | 请勿使用 |
| `--spec-ngram-size-m N` | 已移除，改用相应的 `--spec-ngram-*-size-m` | — | 请勿使用 |
| `--spec-ngram-min-hits N` | 已移除，改用相应的 `--spec-ngram-*-min-hits` | — | 请勿使用 |

### 多模态

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-mm`, `--mmproj FILE` | 多模态投影文件路径；使用 `-hf` 时可省略（环境变量 `LLAMA_ARG_MMPROJ`） | — | 图片理解场景必须提供 |
| `-mmu`, `--mmproj-url URL` | 多模态投影文件 URL（环境变量 `LLAMA_ARG_MMPROJ_URL`） | — | 远程获取投影文件时使用 |
| `--mmproj-auto`, `--no-mmproj`, `--no-mmproj-auto` | 是否使用可用的多模态投影文件，配合 `-hf` 时有用（`LLAMA_ARG_MMPROJ_AUTO`） | 启用 | 一般保持启用 |
| `--mmproj-offload`, `--no-mmproj-offload` | 是否启用多模态投影的 GPU 卸载（`LLAMA_ARG_MMPROJ_OFFLOAD`） | 启用 | 显存足够建议开启加速 |
| `--image-min-tokens N` | 每张图像可采用的最小 token 数量，仅用于动态分辨率视觉模型（`LLAMA_ARG_IMAGE_MIN_TOKENS`） | 从模型读取 | 低分辨率图可调低省算力 |
| `--image-max-tokens N` | 每张图像可采用的最大 token 数量，仅用于动态分辨率视觉模型（`LLAMA_ARG_IMAGE_MAX_TOKENS`） | 从模型读取 | 高清图可调高提升细节 |
| `--media-path PATH` | 加载本地媒体文件的目录，文件可用相对路径访问 | 禁用 | 需要读取本地图片/视频时设置；`lunar_config.json` 的 `multimodal.video_input` 设为 `frames` 时视频改由本地抽帧提交，本项不参与该链路 |
| `--video-fps N` | 视频抽帧的目标帧率（环境变量 `LLAMA_ARG_VIDEO_FPS`） | 4.0 | 画面变化快可适当调高 |
| `--video-timestamp-interval N` | 视频文本时间戳间隔，单位毫秒（`LLAMA_ARG_VIDEO_TIMESTAMP_INTERVAL`） | 5000 | 一般保持默认 |
| `--video-ffmpeg-dir DIR` | ffmpeg 与 ffprobe 可执行文件所在目录（`LLAMA_ARG_VIDEO_FFMPEG_DIR`） | 搜索 PATH | 未装入 PATH 时指定 |

### 嵌入与重排序

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `--pooling {none,mean,cls,last,rank}` | 嵌入的池化类型，未指定时使用模型默认（环境变量 `LLAMA_ARG_POOLING`） | 模型默认 | 嵌入模型按官方推荐设置 |
| `--embd-normalize N` | 嵌入的归一化，-1 无、0 最大绝对 int16、1 出租车、2 欧几里得、>2 为 p-范数 | 2 | 一般保持默认 |
| `--embedding`, `--embeddings` | 限制仅支持嵌入用例，仅与专用嵌入模型一起使用（`LLAMA_ARG_EMBEDDINGS`） | 禁用 | 纯嵌入服务时开启 |
| `--rerank`, `--reranking` | 在服务器上启用重排序端点（环境变量 `LLAMA_ARG_RERANKING`） | 禁用 | 需要重排序能力时开启 |

### 服务与并行

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-np`, `--parallel N` | 服务器插槽数量，-1 表示自动（环境变量 `LLAMA_ARG_N_PARALLEL`） | -1 | 并发高时按需调大 |
| `-cb`, `--cont-batching`, `-nocb`, `--no-cont-batching` | 是否启用连续批量（又称动态批量，`LLAMA_ARG_CONT_BATCHING`） | 启用 | 一般保持启用 |
| `-a`, `--alias STRING` | 设置模型名称别名，逗号分隔，供 API 使用（`LLAMA_ARG_ALIAS`） | — | 多模型路由时便于识别 |
| `--tags STRING` | 设置模型标签，逗号分隔（信息性，不用于路由，`LLAMA_ARG_TAGS`） | — | 仅用于标注 |
| `--host HOST` | 监听的 IP 地址，地址以 .sock 结尾则绑定为 UNIX 套接字（`LLAMA_ARG_HOST`） | 127.0.0.1 | 需要外部访问时改为监听地址，注意安全 |
| `--port PORT` | 监听的端口（环境变量 `LLAMA_ARG_PORT`） | 8080 | 端口冲突时更换 |
| `--reuse-port` | 允许多个套接字绑定到同一端口（环境变量 `LLAMA_ARG_REUSE_PORT`） | 禁用 | 特定部署场景使用 |
| `--path PATH` | 提供静态文件的路径（环境变量 `LLAMA_ARG_STATIC_PATH`） | 空 | 提供自定义前端时使用 |
| `--api-prefix PREFIX` | 服务器提供服务的 API 前缀路径，不带尾部斜杠（`LLAMA_ARG_API_PREFIX`） | 空 | 反向代理分流时使用 |
| `--ui-config JSON` | 提供默认 UI 设置的 JSON（环境变量 `LLAMA_ARG_UI_CONFIG`） | — | 定制 UI 默认值 |
| `--ui-config-file PATH` | 提供默认 UI 设置的 JSON 文件（`LLAMA_ARG_UI_CONFIG_FILE`） | — | 定制 UI 默认值 |
| `--ui-mcp-proxy`, `--no-ui-mcp-proxy` | 是否启用 MCP CORS 代理，实验性，勿在不受信任环境开启（`LLAMA_ARG_UI_MCP_PROXY`） | 禁用 | 谨慎使用 |
| `--tools TOOL1,TOOL2,...` | 是否启用内置工具，指定 all 启用全部；可用工具包含 read_file、file_glob_search、grep_search、exec_shell_command、write_file、edit_file、apply_diff、get_datetime（`LLAMA_ARG_TOOLS`） | 无工具 | 实验性，勿在不受信任环境开启 |
| `--ui`, `--no-ui` | 是否启用 Web UI（环境变量 `LLAMA_ARG_UI`） | 启用 | 一般保持启用 |
| `--api-key KEY` | 用于认证的 API 密钥，可提供多个以逗号分隔（`LLAMA_API_KEY`） | 无 | 对外提供服务时务必设置 |
| `--api-key-file FNAME` | 包含 API 密钥的文件路径 | 无 | 密钥不宜明文写参数时使用 |
| `--ssl-key-file FNAME` | PEM 编码的 SSL 私钥文件路径（`LLAMA_ARG_SSL_KEY_FILE`） | — | 需要 HTTPS 时配置 |
| `--ssl-cert-file FNAME` | PEM 编码的 SSL 证书文件路径（`LLAMA_ARG_SSL_CERT_FILE`） | — | 需要 HTTPS 时配置 |
| `--chat-template-kwargs STRING` | 为 JSON 模板解析器设置额外参数，须为合法 JSON 对象字符串（`LLAMA_CHAT_TEMPLATE_KWARGS`） | — | 高级模板定制 |
| `-to`, `--timeout N` | 服务器读/写超时时间（秒，环境变量 `LLAMA_ARG_TIMEOUT`） | 600 | 长回答场景可调大 |
| `--cache-prompt`, `--no-cache-prompt` | 是否启用提示缓存（环境变量 `LLAMA_ARG_CACHE_PROMPT`） | 启用 | 多轮对话建议保持启用 |
| `--cache-reuse N` | 尝试通过 KV 偏移从缓存重用的最小块大小，需启用提示缓存（`LLAMA_ARG_CACHE_REUSE`） | 0 | 复用重复前缀时可设置 |
| `--metrics` | 启用与 Prometheus 兼容的指标端点（`LLAMA_ARG_ENDPOINT_METRICS`） | 禁用 | 监控接入时启用 |
| `--props` | 启用通过属性端点更改全局属性（`LLAMA_ARG_ENDPOINT_PROPS`） | 禁用 | 需要动态改属性时启用 |
| `--slots`, `--no-slots` | 暴露插槽监控端点（环境变量 `LLAMA_ARG_ENDPOINT_SLOTS`） | 启用 | 观察并发占用 |
| `--slot-save-path PATH` | 保存插槽 KV 缓存的路径 | 禁用 | 需要持久化对话状态时设置 |
| `-sps`, `--slot-prompt-similarity SIMILARITY` | 请求提示需与插槽提示匹配多少才复用该插槽，0.0 表示禁用 | 0.10 | 多轮会话复用时可微调 |
| `--lora-init-without-apply` | 加载 LoRA 适配器但不立即应用，稍后通过适配器端点应用 | 禁用 | 需要运行时切换 LoRA 时使用 |
| `--sleep-idle-seconds SECONDS` | 服务器空闲多少秒后进入休眠 | -1（禁用） | 省电或释放显存时可设置 |

### 路由服务器

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `--models-dir PATH` | 包含路由服务器模型的目录（环境变量 `LLAMA_ARG_MODELS_DIR`） | 禁用 | 多模型托管时设置 |
| `--models-preset PATH` | 包含路由服务器模型预设的 INI 文件路径（`LLAMA_ARG_MODELS_PRESET`） | 禁用 | 多模型托管时设置 |
| `--models-max N` | 同时加载的最大模型数量，0 表示无限制（`LLAMA_ARG_MODELS_MAX`） | 4 | 按显存容量调整 |
| `--models-autoload`, `--no-models-autoload` | 是否自动加载模型（`LLAMA_ARG_MODELS_AUTOLOAD`） | 启用 | 一般保持启用 |

### 聊天模板与推理

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `--jinja`, `--no-jinja` | 是否使用 jinja 模板引擎进行聊天（环境变量 `LLAMA_ARG_JINJA`） | 启用 | 一般保持启用 |
| `--reasoning-format FORMAT` | 控制思考标签的提取与返回格式，可取 none、deepseek、deepseek-legacy（`LLAMA_ARG_THINK`） | auto | 需要单独读取推理内容时用 deepseek |
| `-rea`, `--reasoning [on\|off\|auto]` | 聊天中是否使用推理/思考（环境变量 `LLAMA_ARG_REASONING`） | auto（从模板检测） | 推理模型建议 auto |
| `--reasoning-budget N` | 思考的 token 预算，-1 无限制、0 立即结束、N>0 为预算（`LLAMA_ARG_THINK_BUDGET`） | -1 | 限制思考长度时设置 |
| `--reasoning-budget-message MESSAGE` | 思考预算耗尽时注入到思考结束标签之前的消息（`LLAMA_ARG_THINK_BUDGET_MESSAGE`） | 无 | 需要提示收尾时设置 |
| `--chat-template JINJA_TEMPLATE` | 设置自定义 jinja 聊天模板，指定后缀/前缀时禁用模板（`LLAMA_ARG_CHAT_TEMPLATE`） | 从模型元数据获取 | 模型内置模板不兼容时改用内置模板名 |
| `--chat-template-file JINJA_TEMPLATE_FILE` | 设置自定义 jinja 聊天模板文件（`LLAMA_ARG_CHAT_TEMPLATE_FILE`） | 从模型元数据获取 | 模板较长时用文件 |
| `--skip-chat-parsing`, `--no-skip-chat-parsing` | 强制使用纯内容解析器，模型将所有内容输出到 content 部分（`LLAMA_ARG_SKIP_CHAT_PARSING`） | 禁用 | 需要自行解析时开启 |
| `--prefill-assistant`, `--no-prefill-assistant` | 最后一条消息为 assistant 消息时是否预填充其响应（`LLAMA_ARG_PREFILL_ASSISTANT`） | 启用预填充 | 一般保持默认 |

### 音频与预置模型

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-mv`, `--model-vocoder FNAME` | 用于音频生成的声码器模型 | 未使用 | 音频生成场景使用 |
| `--tts-use-guide-tokens` | 使用引导 token 以提高 TTS 单词召回率 | — | 语音合成场景可开启 |
| `--embd-gemma-default` | 使用默认 EmbeddingGemma 模型（可能从互联网下载权重） | — | 快速启用嵌入能力 |
| `--fim-qwen-1.5b-default` | 使用默认 Qwen 2.5 Coder 1.5B（可能从互联网下载权重） | — | 代码补全轻量档 |
| `--fim-qwen-3b-default` | 使用默认 Qwen 2.5 Coder 3B（可能从互联网下载权重） | — | 代码补全中量档 |
| `--fim-qwen-7b-default` | 使用默认 Qwen 2.5 Coder 7B（可能从互联网下载权重） | — | 代码补全高质档 |
| `--fim-qwen-7b-spec` | 使用 Qwen 2.5 Coder 7B + 0.5B 草稿模型进行投机解码（可能从互联网下载权重） | — | 需加速的代码补全 |
| `--fim-qwen-14b-spec` | 使用 Qwen 2.5 Coder 14B + 0.5B 草稿模型进行投机解码（可能从互联网下载权重） | — | 需加速的代码补全 |
| `--fim-qwen-30b-default` | 使用默认 Qwen 3 Coder 30B A3B Instruct（可能从互联网下载权重） | — | 大模型代码补全 |
| `--gpt-oss-20b-default` | 使用 gpt-oss-20b（可能从互联网下载权重） | — | 通用对话轻量档 |
| `--gpt-oss-120b-default` | 使用 gpt-oss-120b（可能从互联网下载权重） | — | 通用对话高质档 |
| `--vision-gemma-4b-default` | 使用 Gemma 3 4B QAT（可能从互联网下载权重） | — | 多模态轻量档 |
| `--vision-gemma-12b-default` | 使用 Gemma 3 12B QAT（可能从互联网下载权重） | — | 多模态高质档 |
| `--spec-default` | 启用默认投机解码配置 | — | 快速启用途加速 |

### 缓存与检查点

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-lcs`, `--lookup-cache-static FNAME` | 用于查找解码的静态查找缓存路径（不随生成更新） | — | 高级用法 |
| `-lcd`, `--lookup-cache-dynamic FNAME` | 用于查找解码的动态查找缓存路径（随生成更新） | — | 高级用法 |
| `-ctxcp`, `--ctx-checkpoints`, `--swa-checkpoints N` | 每个插槽创建的最大上下文检查点数量（`LLAMA_ARG_CTX_CHECKPOINTS`） | 32 | 一般保持默认 |
| `-cpent`, `--checkpoint-every-n-tokens N` | 预填充期间每 n 个 token 创建一个检查点，-1 禁用（`LLAMA_ARG_CHECKPOINT_EVERY_NT`） | 8192 | 一般保持默认 |
| `-cram`, `--cache-ram N` | 设置最大缓存大小（MiB），-1 无限制、0 禁用（`LLAMA_ARG_CACHE_RAM`） | 8192 | 内存充足可调大 |
| `-kvu`, `--kv-unified`, `-no-kvu`, `--no-kv-unified` | 使用所有序列共享的单一统一 KV 缓冲区（`LLAMA_ARG_KV_UNIFIED`） | 插槽数量自动时启用 | 一般保持默认 |
| `--cache-idle-slots`, `--no-cache-idle-slots` | 在新任务上保存和清除空闲插槽，需统一 KV 与 cache-ram（`LLAMA_ARG_CACHE_IDLE_SLOTS`） | 启用 | 多轮会话建议保持启用 |
| `--context-shift`, `--no-context-shift` | 是否在无限文本生成中使用上下文偏移（`LLAMA_ARG_CONTEXT_SHIFT`） | 禁用 | 超长对话可考虑开启 |
| `-r`, `--reverse-prompt PROMPT` | 在指定提示处停止生成并在交互模式中返回控制 | — | 交互式场景使用 |
| `-sp`, `--special` | 启用特殊 token 输出 | false | 调试或自定义解析时开启 |
| `--warmup`, `--no-warmup` | 是否使用空运行进行预热 | 启用 | 一般保持启用以降低首包延迟 |
| `--spm-infill` | 使用 Suffix/Prefix/Middle 模式进行填充，而非 Prefix/Suffix/Middle | 禁用 | 特定模型偏好该顺序时开启 |

## 五、常见排故

| 现象 | 可能原因 | 处理办法 |
|------|----------|----------|
| 启动或运行时报显存不足 | 层数卸载过多、上下文过大、批量过大 | 降低 `-ngl`，调小 `-c` 与 `-b`，开启 `-fa`，将 `-ctk`/`-ctv` 降为 q8_0，MoE 模型启用 `-cmoe` 或 `-ncmoe` |
| 加载模型失败 | 模型路径错误、文件损坏、格式不支持 | 核对 `-m` 路径与 GGUF 格式，用 `--check-tensors` 检查张量有效性 |
| 加载模型很慢 | mmap 被禁用或磁盘较慢 | 启用 `--mmap`，必要时用 `--mlock` 常驻内存 |
| 长文被截断或上下文溢出 | 上下文大小不足、RoPE 未扩展 | 调大 `-c`，配合 `--rope-scaling` 与 YaRN 系列参数扩展上下文 |
| 推理速度慢 | 线程数不合理、层数未充分卸载到 GPU、提示处理慢 | 调整 `-t`，提高 `-ngl`，调大 `-ub`，开启推测解码（`--spec-type` 与草稿模型） |
| 回答重复或风格异常 | 采样参数不合适 | 调高 `--repeat-penalty`，开启 DRY（`--dry-multiplier`），调整 `--temp`、`--top-p`、`--min-p` |
| 端口冲突或外部无法访问 | 端口被占用、监听地址受限 | 更换 `--port`，将 `--host` 由 127.0.0.1 改为对外监听地址 |
| 多模态图片不生效 | 投影文件缺失或未加载 | 提供 `-mm`，或用 `-hf` 配合 `--mmproj-auto`；必要时设置 `--mmproj-offload` |
| 需要图像/视频理解但报错 | 媒体路径未配置或抽帧工具缺失 | 设置 `--media-path`，确认 `--video-ffmpeg-dir` 指向有效 ffmpeg 目录 |
| 插件或外部客户端连接被拒 | 未设置 API 密钥或密钥不匹配 | 通过 `--api-key` 设置密钥，并确保客户端携带一致密钥 |

## 六、引用跳转

- [02-核心系统-钛宇-月华](02-核心系统-钛宇-月华.md)
- [08-构建运行与配置](08-构建运行与配置.md)
- [12-图像生成参数策略](12-图像生成参数策略.md)
