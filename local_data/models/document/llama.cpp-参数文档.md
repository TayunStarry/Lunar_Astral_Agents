# llama.cpp 服务器 (llama-server) 参数说明

## 通用参数

| 参数 | 说明 |
|------|------|
| `-h`, `--help`, `--usage` | 打印用法并退出 |
| `--version` | 显示版本和构建信息 |
| `--license` | 显示源代码许可证及依赖项 |
| `-cl`, `--cache-list` | 显示缓存中的模型列表 |
| `--completion-bash` | 打印可用于 bash 的自动补全脚本 |
| `-t`, `--threads N` | 生成时使用的 CPU 线程数（默认：`-1`，自动）。环境变量：`LLAMA_ARG_THREADS` |
| `-tb`, `--threads-batch N` | 批处理和提示词处理时使用的线程数（默认与 `--threads` 相同） |
| `-C`, `--cpu-mask M` | CPU 亲和性掩码（任意长度十六进制），与 `--cpu-range` 互补（默认：`""`） |
| `-Cr`, `--cpu-range lo-hi` | CPU 亲和性范围，与 `--cpu-mask` 互补 |
| `--cpu-strict <0\|1>` | 是否使用严格的 CPU 放置（默认：`0`） |
| `--prio N` | 设置进程/线程优先级：`-1`（低），`0`（正常），`1`（中），`2`（高），`3`（实时）（默认：`0`） |
| `--poll <0...100>` | 轮询等待工作的级别（`0` 不轮询，默认：`50`） |
| `-Cb`, `--cpu-mask-batch M` | 批处理 CPU 亲和性掩码（默认与 `--cpu-mask` 相同） |
| `-Crb`, `--cpu-range-batch lo-hi` | 批处理 CPU 亲和性范围，与 `--cpu-mask-batch` 互补 |
| `--cpu-strict-batch <0\|1>` | 批处理是否使用严格 CPU 放置（默认与 `--cpu-strict` 相同） |
| `--prio-batch N` | 批处理优先级：`0`（正常），`1`（中），`2`（高），`3`（实时）（默认：`0`） |
| `--poll-batch <0\|1>` | 批处理是否使用轮询等待（默认与 `--poll` 相同） |
| `-c`, `--ctx-size N` | 提示词上下文大小（默认：`0`，即从模型读取）。环境变量：`LLAMA_ARG_CTX_SIZE` |
| `-n`, `--predict`, `--n-predict N` | 预测的 token 数量（默认：`-1`，即无限）。环境变量：`LLAMA_ARG_N_PREDICT` |
| `-b`, `--batch-size N` | 逻辑最大批处理大小（默认：`2048`）。环境变量：`LLAMA_ARG_BATCH` |
| `-ub`, `--ubatch-size N` | 物理最大批处理大小（默认：`512`）。环境变量：`LLAMA_ARG_UBATCH` |
| `--keep N` | 从初始提示词中保留的 token 数（默认：`0`，`-1` 表示全部） |
| `--swa-full` | 使用全尺寸 SWA 缓存（默认：false）。环境变量：`LLAMA_ARG_SWA_FULL` |
| `-fa`, `--flash-attn [on\|off\|auto]` | Flash Attention 使用设置（默认：`auto`）。环境变量：`LLAMA_ARG_FLASH_ATTN` |
| `--perf`, `--no-perf` | 是否启用内部 libllama 性能计时（默认：false）。环境变量：`LLAMA_ARG_PERF` |
| `-e`, `--escape`, `--no-escape` | 是否处理转义序列（`\n`, `\r`, `\t`, `\'`, `\"`, `\\`）（默认：`true`） |

## RoPE 缩放参数

| 参数 | 说明 |
|------|------|
| `--rope-scaling {none,linear,yarn}` | RoPE 频率缩放方法，默认由模型指定（`linear`）。环境变量：`LLAMA_ARG_ROPE_SCALING_TYPE` |
| `--rope-scale N` | RoPE 上下文缩放因子，将上下文扩展 N 倍。环境变量：`LLAMA_ARG_ROPE_SCALE` |
| `--rope-freq-base N` | RoPE 基础频率，用于 NTK-aware 缩放（默认从模型读取）。环境变量：`LLAMA_ARG_ROPE_FREQ_BASE` |
| `--rope-freq-scale N` | RoPE 频率缩放因子，将上下文按 1/N 扩展。环境变量：`LLAMA_ARG_ROPE_FREQ_SCALE` |
| `--yarn-orig-ctx N` | YaRN：模型的原始上下文大小（默认：`0` = 模型训练上下文大小）。环境变量：`LLAMA_ARG_YARN_ORIG_CTX` |
| `--yarn-ext-factor N` | YaRN：外推混合因子（默认：`-1.00`，`0.0` 表示完全内插）。环境变量：`LLAMA_ARG_YARN_EXT_FACTOR` |
| `--yarn-attn-factor N` | YaRN：缩放 sqrt(t) 或注意力幅度（默认：`-1.00`）。环境变量：`LLAMA_ARG_YARN_ATTN_FACTOR` |
| `--yarn-beta-slow N` | YaRN：高修正维度/alpha（默认：`-1.00`）。环境变量：`LLAMA_ARG_YARN_BETA_SLOW` |
| `--yarn-beta-fast N` | YaRN：低修正维度/beta（默认：`-1.00`）。环境变量：`LLAMA_ARG_YARN_BETA_FAST` |

## 内存与卸载参数

| 参数 | 说明 |
|------|------|
| `-kvo`, `--kv-offload`, `-nkvo`, `--no-kv-offload` | 是否启用 KV 缓存卸载（默认：启用）。环境变量：`LLAMA_ARG_KV_OFFLOAD` |
| `--repack`, `-nr`, `--no-repack` | 是否启用权重重新打包（默认：启用）。环境变量：`LLAMA_ARG_REPACK` |
| `--no-host` | 绕过主机缓冲区，允许使用额外缓冲区。环境变量：`LLAMA_ARG_NO_HOST` |
| `-ctk`, `--cache-type-k TYPE` | K 的 KV 缓存数据类型。可选：`f32, f16, bf16, q8_0, q4_0, q4_1, iq4_nl, q5_0, q5_1`（默认：`f16`）。环境变量：`LLAMA_ARG_CACHE_TYPE_K` |
| `-ctv`, `--cache-type-v TYPE` | V 的 KV 缓存数据类型。可选同上（默认：`f16`）。环境变量：`LLAMA_ARG_CACHE_TYPE_V` |
| `-dt`, `--defrag-thold N` | KV 缓存碎片整理阈值（已弃用）。环境变量：`LLAMA_ARG_DEFRAG_THOLD` |
| `--rpc SERVERS` | 逗号分隔的 RPC 服务器列表（`host:port`）。环境变量：`LLAMA_ARG_RPC` |
| `--mlock` | 强制将模型保留在 RAM 中，防止交换或压缩。环境变量：`LLAMA_ARG_MLOCK` |
| `--mmap`, `--no-mmap` | 是否使用内存映射加载模型（默认：启用）。禁用时加载较慢，但若未使用 mlock 可减少页面换出。环境变量：`LLAMA_ARG_MMAP` |
| `-dio`, `--direct-io`, `-ndio`, `--no-direct-io` | 是否使用 DirectIO（如果可用）（默认：禁用）。环境变量：`LLAMA_ARG_DIO` |
| `--numa TYPE` | 针对某些 NUMA 系统的优化尝试。`distribute`：均匀分配到所有节点；`isolate`：仅在启动所在节点的 CPU 上生成线程；`numactl`：使用 numactl 提供的 CPU 映射。若之前未使用，建议先清除系统页面缓存。参见 https://github.com/ggml-org/llama.cpp/issues/1437 。环境变量：`LLAMA_ARG_NUMA` |

## GPU 相关参数

| 参数 | 说明 |
|------|------|
| `-dev`, `--device <dev1,dev2,..>` | 逗号分隔的设备列表，用于卸载（`none` 表示不卸载）。使用 `--list-devices` 查看可用设备。环境变量：`LLAMA_ARG_DEVICE` |
| `--list-devices` | 打印可用设备列表并退出 |
| `-ot`, `--override-tensor <tensor name pattern>=<buffer type>,...` | 覆盖张量缓冲区类型。环境变量：`LLAMA_ARG_OVERRIDE_TENSOR` |
| `-cmoe`, `--cpu-moe` | 将所有专家混合（MoE）权重保留在 CPU。环境变量：`LLAMA_ARG_CPU_MOE` |
| `-ncmoe`, `--n-cpu-moe N` | 将前 N 层的 MoE 权重保留在 CPU。环境变量：`LLAMA_ARG_N_CPU_MOE` |
| `-ngl`, `--gpu-layers`, `--n-gpu-layers N` | 最多存储在 VRAM 中的层数，可指定具体数字、`auto` 或 `all`（默认：`auto`）。环境变量：`LLAMA_ARG_N_GPU_LAYERS` |
| `-sm`, `--split-mode {none,layer,row,tensor}` | 多 GPU 模型分割方式：`none`（仅使用一个 GPU），`layer`（默认，按层和 KV 分割，流水线），`row`（按行分割权重，并行），`tensor`（分割权重和 KV，并行，实验性）。环境变量：`LLAMA_ARG_SPLIT_MODE` |
| `-ts`, `--tensor-split N0,N1,N2,...` | 分配给每个 GPU 的模型比例，逗号分隔，例如 `3,1`。环境变量：`LLAMA_ARG_TENSOR_SPLIT` |
| `-mg`, `--main-gpu INDEX` | 在 `split-mode=none` 时使用的 GPU；在 `split-mode=row` 时用于中间结果和 KV 的 GPU（默认：`0`）。环境变量：`LLAMA_ARG_MAIN_GPU` |
| `-fit`, `--fit [on\|off]` | 是否自动调整未设置的参数以适应设备内存（默认：`on`）。环境变量：`LLAMA_ARG_FIT` |
| `-fitt`, `--fit-target MiB0,MiB1,MiB2,...` | `--fit` 的每个设备目标余量（MiB），逗号分隔，单个值会应用到所有设备（默认：`1024`）。环境变量：`LLAMA_ARG_FIT_TARGET` |
| `-fitc`, `--fit-ctx N` | `--fit` 可设置的最小上下文大小（默认：`4096`）。环境变量：`LLAMA_ARG_FIT_CTX` |
| `--check-tensors` | 检查模型张量数据是否有无效值（默认：false） |
| `--override-kv KEY=TYPE:VALUE,...` | 覆盖模型元数据键。类型：`int`, `float`, `bool`, `str`。示例：`--override-kv tokenizer.ggml.add_bos_token=bool:false,...` |
| `--op-offload`, `--no-op-offload` | 是否将主机张量操作卸载到设备（默认：true） |

## LoRA 与控制向量

| 参数 | 说明 |
|------|------|
| `--lora FNAME` | LoRA 适配器路径（多个用逗号分隔） |
| `--lora-scaled FNAME:SCALE,...` | 带有自定义缩放的 LoRA 适配器（格式：`FNAME:SCALE,...`） |
| `--control-vector FNAME` | 添加控制向量（多个用逗号分隔） |
| `--control-vector-scaled FNAME:SCALE,...` | 添加带自定义缩放的控制向量（格式：`FNAME:SCALE,...`） |
| `--control-vector-layer-range START END` | 控制向量应用的层范围，起始和结束均包含 |

## 模型加载参数

| 参数 | 说明 |
|------|------|
| `-m`, `--model FNAME` | 模型路径。环境变量：`LLAMA_ARG_MODEL` |
| `-mu`, `--model-url MODEL_URL` | 模型下载 URL（默认：未使用）。环境变量：`LLAMA_ARG_MODEL_URL` |
| `-dr`, `--docker-repo [<repo>/]<model>[:quant]` | Docker Hub 模型仓库。`repo` 可选，默认为 `ai/`；`quant` 可选，默认为 `:latest`。例如 `gemma3`。环境变量：`LLAMA_ARG_DOCKER_REPO` |
| `-hf`, `-hfr`, `--hf-repo <user>/<model>[:quant]` | Hugging Face 模型仓库；`quant` 可选，不区分大小写，默认 `Q4_K_M`，若不存在则使用仓库第一个文件。若存在 mmproj 文件也会自动下载，可添加 `--no-mmproj` 禁用。示例：`ggml-org/GLM-4.7-Flash-GGUF:Q4_K_M`。环境变量：`LLAMA_ARG_HF_REPO` |
| `-hff`, `--hf-file FILE` | Hugging Face 模型文件。若指定，将覆盖 `--hf-repo` 中的量化版本。环境变量：`LLAMA_ARG_HF_FILE` |
| `-hfv`, `-hfrv`, `--hf-repo-v <user>/<model>[:quant]` | 声码器模型的 Hugging Face 仓库。环境变量：`LLAMA_ARG_HF_REPO_V` |
| `-hffv`, `--hf-file-v FILE` | 声码器模型的 Hugging Face 文件。环境变量：`LLAMA_ARG_HF_FILE_V` |
| `-hft`, `--hf-token TOKEN` | Hugging Face 访问令牌（默认从 `HF_TOKEN` 环境变量读取） |

## 日志参数

| 参数 | 说明 |
|------|------|
| `--log-disable` | 禁用日志 |
| `--log-file FNAME` | 日志输出到文件。环境变量：`LLAMA_LOG_FILE` |
| `--log-colors [on\|off\|auto]` | 设置彩色日志（默认：`auto`，输出到终端时启用）。环境变量：`LLAMA_LOG_COLORS` |
| `-v`, `--verbose`, `--log-verbose` | 将日志详细级别设为无限（记录所有消息，用于调试） |
| `--offline` | 离线模式：强制使用缓存，阻止网络访问。环境变量：`LLAMA_OFFLINE` |
| `-lv`, `--verbosity`, `--log-verbosity N` | 设置详细程度阈值，高于该值的消息将被忽略。`0`：通用输出，`1`：错误，`2`：警告，`3`：信息（默认），`4`：调试。环境变量：`LLAMA_LOG_VERBOSITY` |
| `--log-prefix` | 在日志消息中显示前缀。环境变量：`LLAMA_LOG_PREFIX` |
| `--log-timestamps` | 在日志消息中显示时间戳。环境变量：`LLAMA_LOG_TIMESTAMPS` |

## 推测解码相关参数（草稿模型 KV 缓存类型）

| 参数 | 说明 |
|------|------|
| `--spec-draft-type-k`, `-ctkd`, `--cache-type-k-draft TYPE` | 草稿模型的 K 缓存数据类型，选项同主模型（默认：`f16`）。环境变量：`LLAMA_ARG_SPEC_DRAFT_CACHE_TYPE_K` |
| `--spec-draft-type-v`, `-ctvd`, `--cache-type-v-draft TYPE` | 草稿模型的 V 缓存数据类型（默认：`f16`）。环境变量：`LLAMA_ARG_SPEC_DRAFT_CACHE_TYPE_V` |

## 采样参数

| 参数 | 说明 |
|------|------|
| `--samplers SAMPLERS` | 生成时使用的采样器，按顺序，以 `;` 分隔（默认：`penalties;dry;top_n_sigma;top_k;typ_p;top_p;min_p;xtc;temperature`） |
| `-s`, `--seed SEED` | RNG 种子（默认：`-1`，随机种子） |
| `--sampler-seq`, `--sampling-seq SEQUENCE` | 简化采样序列（默认：`edskypmxt`） |
| `--ignore-eos` | 忽略结束符并继续生成（隐式包含 `--logit-bias EOS-inf`） |
| `--temp`, `--temperature N` | 温度（默认：`0.80`） |
| `--top-k N` | top-k 采样（默认：`40`，`0` 禁用）。环境变量：`LLAMA_ARG_TOP_K` |
| `--top-p N` | top-p（核）采样（默认：`0.95`，`1.0` 禁用） |
| `--min-p N` | min-p 采样（默认：`0.05`，`0.0` 禁用） |
| `--top-nsigma`, `--top-n-sigma N` | top-n-sigma 采样（默认：`-1.00`，`-1.0` 禁用） |
| `--xtc-probability N` | xtc 概率（默认：`0.00`，`0.0` 禁用） |
| `--xtc-threshold N` | xtc 阈值（默认：`0.10`，`1.0` 禁用） |
| `--typical`, `--typical-p N` | 局部典型采样参数 p（默认：`1.00`，`1.0` 禁用） |
| `--repeat-last-n N` | 惩罚时考虑的最后 n 个 token（默认：`64`，`0` 禁用，`-1` 为上下文大小） |
| `--repeat-penalty N` | 重复序列惩罚（默认：`1.00`，`1.0` 禁用） |
| `--presence-penalty N` | 重复 alpha 存在惩罚（默认：`0.00`，`0.0` 禁用） |
| `--frequency-penalty N` | 重复 alpha 频率惩罚（默认：`0.00`，`0.0` 禁用） |
| `--dry-multiplier N` | DRY 采样乘数（默认：`0.00`，`0.0` 禁用） |
| `--dry-base N` | DRY 采样基值（默认：`1.75`） |
| `--dry-allowed-length N` | DRY 允许长度（默认：`2`） |
| `--dry-penalty-last-n N` | DRY 惩罚最后 n 个 token（默认：`-1`，`0` 禁用，`-1` 为上下文大小） |
| `--dry-sequence-breaker STRING` | DRY 序列断点，设置后会清除默认断点（`'\n'`, `':'`, `'"'`, `'*'`）；使用 `"none"` 则不使用任何断点 |
| `--adaptive-target N` | adaptive-p：选择接近此概率的 token（有效范围 0.0 到 1.0；负值禁用）（默认：`-1.00`）。更多信息见 [PR #17927](https://github.com/ggml-org/llama.cpp/pull/17927) |
| `--adaptive-decay N` | adaptive-p：目标适应衰减率，值越低反应越快，越高越稳定（有效范围 0.0 到 0.99，默认：`0.90`） |
| `--dynatemp-range N` | 动态温度范围（默认：`0.00`，`0.0` 禁用） |
| `--dynatemp-exp N` | 动态温度指数（默认：`1.00`） |
| `--mirostat N` | 使用 Mirostat 采样。若使用，Top K、Nucleus 和 Locally Typical 采样器将被忽略。（`0` 禁用，`1` Mirostat，`2` Mirostat 2.0） |
| `--mirostat-lr N` | Mirostat 学习率 eta（默认：`0.10`） |
| `--mirostat-ent N` | Mirostat 目标熵 tau（默认：`5.00`） |
| `-l`, `--logit-bias TOKEN_ID(+/-)BIAS` | 修改 token 出现的可能性。例如 `--logit-bias 15043+1` 增加 `' Hello'` 的概率，`-1` 降低 |
| `--grammar GRAMMAR` | BNF 语法约束生成（参见 `grammars/` 目录示例） |
| `--grammar-file FNAME` | 从文件读取语法 |
| `-j`, `--json-schema SCHEMA` | JSON Schema 约束生成（https://json-schema.org/），如 `{}` 表示任意 JSON 对象。若含外部 `$refs`，请使用 `--grammar` + `example/json_schema_to_grammar.py` |
| `-jf`, `--json-schema-file FILE` | 包含 JSON Schema 的文件 |
| `-bs`, `--backend-sampling` | 启用后端采样（实验性）（默认：禁用）。环境变量：`LLAMA_ARG_BACKEND_SAMPLING` |

## 推测解码（Speculative Decoding）参数

| 参数 | 说明 |
|------|------|
| `--spec-draft-hf`, `-hfd`, `-hfrd`, `--hf-repo-draft <user>/<model>[:quant]` | 同 `--hf-repo`，但用于草稿模型。环境变量：`LLAMA_ARG_SPEC_DRAFT_HF_REPO` |
| `--spec-draft-threads`, `-td`, `--threads-draft N` | 草稿模型生成线程数（默认同 `--threads`） |
| `--spec-draft-threads-batch`, `-tbd`, `--threads-batch-draft N` | 草稿模型批处理线程数（默认同 `--threads-draft`） |
| `--spec-draft-cpu-mask`, `-Cd`, `--cpu-mask-draft M` | 草稿模型 CPU 亲和性掩码（默认同 `--cpu-mask`） |
| `--spec-draft-cpu-range`, `-Crd`, `--cpu-range-draft lo-hi` | 草稿模型 CPU 亲和性范围 |
| `--spec-draft-cpu-strict`, `--cpu-strict-draft <0\|1>` | 草稿模型是否使用严格 CPU 放置（默认同 `--cpu-strict`） |
| `--spec-draft-prio`, `--prio-draft N` | 草稿模型优先级：`0` 正常，`1` 中，`2` 高，`3` 实时（默认：`0`） |
| `--spec-draft-poll`, `--poll-draft <0\|1>` | 草稿模型是否使用轮询等待（默认同 `--poll`） |
| `--spec-draft-cpu-mask-batch`, `-Cbd`, `--cpu-mask-batch-draft M` | 草稿模型批处理 CPU 亲和性掩码 |
| `--spec-draft-cpu-strict-batch`, `--cpu-strict-batch-draft <0\|1>` | 草稿模型批处理严格 CPU 放置（默认同 `--cpu-strict-draft`） |
| `--spec-draft-prio-batch`, `--prio-batch-draft N` | 草稿模型批处理优先级（默认：`0`） |
| `--spec-draft-poll-batch`, `--poll-batch-draft <0\|1>` | 草稿模型批处理轮询等待（默认同 `--poll-draft`） |
| `--spec-draft-override-tensor`, `-otd`, `--override-tensor-draft <tensor name pattern>=<buffer type>,...` | 覆盖草稿模型张量缓冲区类型 |
| `--spec-draft-cpu-moe`, `-cmoed`, `--cpu-moe-draft` | 将草稿模型的 MoE 权重保留在 CPU。环境变量：`LLAMA_ARG_SPEC_DRAFT_CPU_MOE` |
| `--spec-draft-n-cpu-moe`, `--spec-draft-ncmoe`, `-ncmoed`, `--n-cpu-moe-draft N` | 将草稿模型前 N 层的 MoE 权重保留在 CPU。环境变量：`LLAMA_ARG_SPEC_DRAFT_N_CPU_MOE` |
| `--spec-draft-n-max N` | 推测解码最大草稿 token 数（默认：`16`）。环境变量：`LLAMA_ARG_SPEC_DRAFT_N_MAX` |
| `--spec-draft-n-min N` | 推测解码最小草稿 token 数（默认：`0`）。环境变量：`LLAMA_ARG_SPEC_DRAFT_N_MIN` |
| `--spec-draft-p-split`, `--draft-p-split P` | 推测解码分割概率（默认：`0.10`）。环境变量：`LLAMA_ARG_SPEC_DRAFT_P_SPLIT` |
| `--spec-draft-p-min`, `--draft-p-min P` | 最小推测解码概率（贪心）（默认：`0.75`）。环境变量：`LLAMA_ARG_SPEC_DRAFT_P_MIN` |
| `--spec-draft-ctx-size`, `-cd`, `--ctx-size-draft N` | 草稿模型提示词上下文大小（默认：`0`，从模型读取）。环境变量：`LLAMA_ARG_SPEC_DRAFT_CTX_SIZE` |
| `--spec-draft-device`, `-devd`, `--device-draft <dev1,dev2,..>` | 草稿模型卸载设备列表。环境变量：`LLAMA_ARG_SPEC_DRAFT_DEVICE` |
| `--spec-draft-ngl`, `-ngld`, `--gpu-layers-draft`, `--n-gpu-layers-draft N` | 草稿模型存储在 VRAM 的最大层数（默认：`auto`）。环境变量：`LLAMA_ARG_N_GPU_LAYERS_DRAFT` |
| `--spec-draft-model`, `-md`, `--model-draft FNAME` | 草稿模型文件路径。环境变量：`LLAMA_ARG_SPEC_DRAFT_MODEL` |
| `--spec-draft-replace`, `--spec-replace TARGET DRAFT` | 如果草稿模型与主模型不兼容，将 TARGET 字符串翻译为 DRAFT |
| `--spec-type [none\|ngram-cache\|ngram-simple\|ngram-map-k\|ngram-map-k4v\|ngram-mod]` | 无草稿模型时使用的推测解码类型（默认：`none`）。环境变量：`LLAMA_ARG_SPEC_TYPE` |
| `--spec-ngram-mod-n-min N` | ngram-based 推测解码的最小 ngram token 数（默认：`48`） |
| `--spec-ngram-mod-n-max N` | ngram-based 推测解码的最大 ngram token 数（默认：`64`） |
| `--spec-ngram-mod-n-match N` | ngram-mod 查找长度（默认：`24`） |
| `--spec-ngram-simple-size-n N` | ngram-simple 推测解码的 n-gram 大小 N（默认：`12`） |
| `--spec-ngram-simple-size-m N` | ngram-simple 推测解码的 m-gram 大小 M（默认：`48`） |
| `--spec-ngram-simple-min-hits N` | ngram-simple 推测解码的最小命中数（默认：`1`） |
| `--spec-ngram-map-k-size-n N` | ngram-map-k 推测解码的 n-gram 大小 N（默认：`12`） |
| `--spec-ngram-map-k-size-m N` | ngram-map-k 推测解码的 m-gram 大小 M（默认：`48`） |
| `--spec-ngram-map-k-min-hits N` | ngram-map-k 推测解码的最小命中数（默认：`1`） |
| `--spec-ngram-map-k4v-size-n N` | ngram-map-k4v 推测解码的 n-gram 大小 N（默认：`12`） |
| `--spec-ngram-map-k4v-size-m N` | ngram-map-k4v 推测解码的 m-gram 大小 M（默认：`48`） |
| `--spec-ngram-map-k4v-min-hits N` | ngram-map-k4v 推测解码的最小命中数（默认：`1`） |
| `--draft`, `--draft-n`, `--draft-max N` | 已移除，请使用 `--spec-draft-n-max` 或 `--spec-ngram-mod-n-max`。环境变量：`LLAMA_ARG_DRAFT_MAX` |
| `--draft-min`, `--draft-n-min N` | 已移除，请使用 `--spec-draft-n-min` 或 `--spec-ngram-mod-n-min`。环境变量：`LLAMA_ARG_DRAFT_MIN` |
| `--spec-ngram-size-n N` | 已移除，请使用对应的 `--spec-ngram-*-size-n` 或 `--spec-ngram-mod-n-match` |
| `--spec-ngram-size-m N` | 已移除，请使用对应的 `--spec-ngram-*-size-m` |
| `--spec-ngram-min-hits N` | 已移除，请使用对应的 `--spec-ngram-*-min-hits` |

## 特定功能参数

### 查找缓存与上下文检查点

| 参数 | 说明 |
|------|------|
| `-lcs`, `--lookup-cache-static FNAME` | 静态查找缓存路径（生成时不更新） |
| `-lcd`, `--lookup-cache-dynamic FNAME` | 动态查找缓存路径（生成时更新） |
| `-ctxcp`, `--ctx-checkpoints`, `--swa-checkpoints N` | 每个槽最多创建的上下文检查点数量（默认：`32`）。环境变量：`LLAMA_ARG_CTX_CHECKPOINTS` |
| `-cpent`, `--checkpoint-every-n-tokens N` | 预填充期间每 N 个 token 创建一个检查点，`-1` 禁用（默认：`8192`）。环境变量：`LLAMA_ARG_CHECKPOINT_EVERY_NT` |
| `-cram`, `--cache-ram N` | 设置最大缓存大小（MiB），`-1` 无限制，`0` 禁用。环境变量：`LLAMA_ARG_CACHE_RAM` |
| `-kvu`, `--kv-unified`, `-no-kvu`, `--no-kv-unified` | 使用统一 KV 缓冲区（默认：槽数自动时启用）。环境变量：`LLAMA_ARG_KV_UNIFIED` |
| `--cache-idle-slots`, `--no-cache-idle-slots` | 新任务时保存并清除空闲槽（默认：启用，需统一 KV 和 cache-ram）。环境变量：`LLAMA_ARG_CACHE_IDLE_SLOTS` |
| `--context-shift`, `--no-context-shift` | 无限文本生成时是否使用上下文移位（默认：禁用）。环境变量：`LLAMA_ARG_CONTEXT_SHIFT` |

### 服务器与推理设置

| 参数 | 说明 |
|------|------|
| `-r`, `--reverse-prompt PROMPT` | 交互模式下在遇到 PROMPT 时暂停生成 |
| `-sp`, `--special` | 输出特殊 token（默认：false） |
| `--warmup`, `--no-warmup` | 是否执行空运行预热（默认：启用） |
| `--spm-infill` | 使用 Suffix/Prefix/Middle 填充模式（某些模型偏好） |
| `--pooling {none,mean,cls,last,rank}` | 嵌入的池化类型，未指定时使用模型默认。环境变量：`LLAMA_ARG_POOLING` |
| `-np`, `--parallel N` | 服务器槽数量（默认：`-1`，自动）。环境变量：`LLAMA_ARG_N_PARALLEL` |
| `-cb`, `--cont-batching`, `-nocb`, `--no-cont-batching` | 是否启用连续批处理（默认：启用）。环境变量：`LLAMA_ARG_CONT_BATCHING` |

### 多模态参数

| 参数 | 说明 |
|------|------|
| `-mm`, `--mmproj FILE` | 多模态投影文件路径。若使用 `-hf`，可省略。环境变量：`LLAMA_ARG_MMPROJ` |
| `-mmu`, `--mmproj-url URL` | 多模态投影文件 URL。环境变量：`LLAMA_ARG_MMPROJ_URL` |
| `--mmproj-auto`, `--no-mmproj`, `--no-mmproj-auto` | 是否使用多模态投影文件（如果可用），与 `-hf` 一起使用时有效（默认：启用）。环境变量：`LLAMA_ARG_MMPROJ_AUTO` |
| `--mmproj-offload`, `--no-mmproj-offload` | 是否启用多模态投影 GPU 卸载（默认：启用）。环境变量：`LLAMA_ARG_MMPROJ_OFFLOAD` |
| `--image-min-tokens N` | 每张图像最少占用 token 数（动态分辨率视觉模型，默认从模型读取）。环境变量：`LLAMA_ARG_IMAGE_MIN_TOKENS` |
| `--image-max-tokens N` | 每张图像最多占用 token 数（默认从模型读取）。环境变量：`LLAMA_ARG_IMAGE_MAX_TOKENS` |

### 服务器与 API 配置

| 参数 | 说明 |
|------|------|
| `-a`, `--alias STRING` | 模型别名，逗号分隔（供 API 使用）。环境变量：`LLAMA_ARG_ALIAS` |
| `--tags STRING` | 模型标签，逗号分隔（仅信息，不用于路由）。环境变量：`LLAMA_ARG_TAGS` |
| `--host HOST` | 监听的 IP 地址，或以 `.sock` 结尾绑定 UNIX socket（默认：`127.0.0.1`）。环境变量：`LLAMA_ARG_HOST` |
| `--port PORT` | 监听端口（默认：`8080`）。环境变量：`LLAMA_ARG_PORT` |
| `--reuse-port` | 允许多个 socket 绑定同一端口（默认：禁用）。环境变量：`LLAMA_ARG_REUSE_PORT` |
| `--path PATH` | 提供静态文件服务的路径（默认：空）。环境变量：`LLAMA_ARG_STATIC_PATH` |
| `--api-prefix PREFIX` | API 前缀路径，不含尾部斜杠（默认：空）。环境变量：`LLAMA_ARG_API_PREFIX` |
| `--webui-config JSON` | 提供默认 WebUI 设置的 JSON（覆盖 WebUI 默认值）。环境变量：`LLAMA_ARG_WEBUI_CONFIG` |
| `--webui-config-file PATH` | 提供默认 WebUI 设置的 JSON 文件。环境变量：`LLAMA_ARG_WEBUI_CONFIG_FILE` |
| `--webui-mcp-proxy`, `--no-webui-mcp-proxy` | 实验性：是否启用 MCP CORS 代理（请勿在不受信任环境启用，默认：禁用）。环境变量：`LLAMA_ARG_WEBUI_MCP_PROXY` |
| `--tools TOOL1,TOOL2,...` | 实验性：启用内置 AI 代理工具（请勿在不受信任环境启用）。指定 `"all"` 启用全部。可用工具：`read_file`, `file_glob_search`, `grep_search`, `exec_shell_command`, `write_file`, `edit_file`, `apply_diff`。环境变量：`LLAMA_ARG_TOOLS` |
| `--webui`, `--no-webui` | 是否启用 Web UI（默认：启用）。环境变量：`LLAMA_ARG_WEBUI` |
| `--embedding`, `--embeddings` | 限制仅支持嵌入用例；仅用于专用嵌入模型（默认：禁用）。环境变量：`LLAMA_ARG_EMBEDDINGS` |
| `--rerank`, `--reranking` | 启用服务器上的重排序端点（默认：禁用）。环境变量：`LLAMA_ARG_RERANKING` |
| `--api-key KEY` | API 密钥，多个用逗号分隔（默认：无）。环境变量：`LLAMA_API_KEY` |
| `--api-key-file FNAME` | 包含 API 密钥的文件路径 |
| `--ssl-key-file FNAME` | PEM 编码 SSL 私钥文件。环境变量：`LLAMA_ARG_SSL_KEY_FILE` |
| `--ssl-cert-file FNAME` | PEM 编码 SSL 证书文件。环境变量：`LLAMA_ARG_SSL_CERT_FILE` |

### 聊天模板与推理行为

| 参数 | 说明 |
|------|------|
| `--chat-template-kwargs STRING` | 为 JSON 模板解析器设置额外参数，必须是有效 JSON 对象字符串，例如 `'{"key1":"value1","key2":"value2"}'`。环境变量：`LLAMA_CHAT_TEMPLATE_KWARGS` |
| `-to`, `--timeout N` | 服务器读写超时（秒）（默认：`600`）。环境变量：`LLAMA_ARG_TIMEOUT` |
| `--threads-http N` | 处理 HTTP 请求的线程数（默认：`-1`）。环境变量：`LLAMA_ARG_THREADS_HTTP` |
| `--cache-prompt`, `--no-cache-prompt` | 是否启用提示词缓存（默认：启用）。环境变量：`LLAMA_ARG_CACHE_PROMPT` |
| `--cache-reuse N` | 尝试通过 KV 移位重用缓存的最小块大小，需启用提示词缓存（默认：`0`）。参见 [卡片](https://ggml.ai/f0.png)。环境变量：`LLAMA_ARG_CACHE_REUSE` |
| `--metrics` | 启用 Prometheus 兼容的指标端点（默认：禁用）。环境变量：`LLAMA_ARG_ENDPOINT_METRICS` |
| `--props` | 允许通过 POST /props 更改全局属性（默认：禁用）。环境变量：`LLAMA_ARG_ENDPOINT_PROPS` |
| `--slots`, `--no-slots` | 是否暴露槽监控端点（默认：启用）。环境变量：`LLAMA_ARG_ENDPOINT_SLOTS` |
| `--slot-save-path PATH` | 槽 KV 缓存保存路径（默认：禁用） |
| `--media-path PATH` | 加载本地媒体文件的目录；可通过 `file://` URL 使用相对路径访问（默认：禁用） |

### 路由服务器（多模型）参数

| 参数 | 说明 |
|------|------|
| `--models-dir PATH` | 包含路由服务器模型的目录（默认：禁用）。环境变量：`LLAMA_ARG_MODELS_DIR` |
| `--models-preset PATH` | 包含路由服务器模型预设的 INI 文件路径（默认：禁用）。环境变量：`LLAMA_ARG_MODELS_PRESET` |
| `--models-max N` | 路由服务器同时加载的最大模型数（默认：`4`，`0` 无限制）。环境变量：`LLAMA_ARG_MODELS_MAX` |
| `--models-autoload`, `--no-models-autoload` | 路由服务器是否自动加载模型（默认：启用）。环境变量：`LLAMA_ARG_MODELS_AUTOLOAD` |

### Jinja 模板与推理格式

| 参数 | 说明 |
|------|------|
| `--jinja`, `--no-jinja` | 是否使用 Jinja 模板引擎进行聊天（默认：启用）。环境变量：`LLAMA_ARG_JINJA` |
| `--reasoning-format FORMAT` | 控制思考标签的处理方式：`none`（留在 `message.content` 中），`deepseek`（放入 `message.reasoning_content`），`deepseek-legacy`（保留 `<think>` 标签在 `content` 并填充 `reasoning_content`）。默认：`auto`。环境变量：`LLAMA_ARG_THINK` |
| `-rea`, `--reasoning [on\|off\|auto]` | 聊天中是否使用推理/思考（`auto` 从模板检测）。环境变量：`LLAMA_ARG_REASONING` |
| `--reasoning-budget N` | 思考 token 预算：`-1` 无限制，`0` 立即结束，`N>0` 为预算值（默认：`-1`）。环境变量：`LLAMA_ARG_THINK_BUDGET` |
| `--reasoning-budget-message MESSAGE` | 预算耗尽时在结束思考标签前注入的消息（默认：无）。环境变量：`LLAMA_ARG_THINK_BUDGET_MESSAGE` |
| `--chat-template JINJA_TEMPLATE` | 自定义 Jinja 聊天模板（默认：来自模型元数据）。若指定前缀/后缀，模板将被禁用。仅接受常用模板（除非之前设置 `--jinja`）。内置模板列表：`bailing`, `bailing-think`, `bailing2`, `chatglm3`, `chatglm4`, `chatml`, `command-r`, `deepseek`, `deepseek-ocr`, `deepseek2`, `deepseek3`, `exaone-moe`, `exaone3`, `exaone4`, `falcon3`, `gemma`, `gigachat`, `glmedge`, `gpt-oss`, `granite`, `granite-4.0`, `grok-2`, `hunyuan-dense`, `hunyuan-moe`, `hunyuan-ocr`, `kimi-k2`, `llama2`, `llama2-sys`, `llama2-sys-bos`, `llama2-sys-strip`, `llama3`, `llama4`, `megrez`, `minicpm`, `mistral-v1`, `mistral-v3`, `mistral-v3-tekken`, `mistral-v7`, `mistral-v7-tekken`, `monarch`, `openchat`, `orion`, `pangu-embedded`, `phi3`, `phi4`, `rwkv-world`, `seed_oss`, `smolvlm`, `solar-open`, `vicuna`, `vicuna-orca`, `yandex`, `zephyr`。环境变量：`LLAMA_ARG_CHAT_TEMPLATE` |
| `--chat-template-file JINJA_TEMPLATE_FILE` | 从文件加载自定义 Jinja 聊天模板（同上模板列表）。环境变量：`LLAMA_ARG_CHAT_TEMPLATE_FILE` |
| `--skip-chat-parsing`, `--no-skip-chat-parsing` | 强制使用纯内容解析器，即使指定 Jinja 模板；模型将在内容部分输出所有内容，包括推理和/或工具调用（默认：禁用）。环境变量：`LLAMA_ARG_SKIP_CHAT_PARSING` |
| `--prefill-assistant`, `--no-prefill-assistant` | 如果最后一条消息是助手消息，是否预填充助手回复（默认：启用）。设置后，最后一条助手消息将作为完整消息，不进行预填充。环境变量：`LLAMA_ARG_PREFILL_ASSISTANT` |
| `-sps`, `--slot-prompt-similarity SIMILARITY` | 请求的提示与槽提示必须匹配多少才能使用该槽（默认：`0.10`，`0.0` 禁用） |

### 其他杂项

| 参数 | 说明 |
|------|------|
| `--lora-init-without-apply` | 加载 LoRA 适配器但不立即应用（之后通过 POST /lora-adapters 应用）（默认：禁用） |
| `--sleep-idle-seconds SECONDS` | 空闲指定秒数后服务器进入睡眠（默认：`-1`，禁用） |
| `-mv`, `--model-vocoder FNAME` | 用于音频生成的声码器模型路径 |
| `--tts-use-guide-tokens` | 使用引导 token 提高 TTS 单词召回率 |

### 预设默认模型

以下参数可直接启用特定预配置模型（可能从网络下载权重）：

| 参数 | 说明 |
|------|------|
| `--embd-gemma-default` | 使用默认 EmbeddingGemma 模型 |
| `--fim-qwen-1.5b-default` | 使用默认 Qwen 2.5 Coder 1.5B |
| `--fim-qwen-3b-default` | 使用默认 Qwen 2.5 Coder 3B |
| `--fim-qwen-7b-default` | 使用默认 Qwen 2.5 Coder 7B |
| `--fim-qwen-7b-spec` | 使用 Qwen 2.5 Coder 7B + 0.5B 草稿推测解码 |
| `--fim-qwen-14b-spec` | 使用 Qwen 2.5 Coder 14B + 0.5B 草稿推测解码 |
| `--fim-qwen-30b-default` | 使用默认 Qwen 3 Coder 30B A3B Instruct |
| `--gpt-oss-20b-default` | 使用 gpt-oss-20b |
| `--gpt-oss-120b-default` | 使用 gpt-oss-120b |
| `--vision-gemma-4b-default` | 使用 Gemma 3 4B QAT 视觉模型 |
| `--vision-gemma-12b-default` | 使用 Gemma 3 12B QAT 视觉模型 |
| `--spec-default` | 启用默认推测解码配置 |