# llama.cpp 参数文档（中文翻译）

## 通用参数 (Common Params)

| 参数 | 说明 |
|------|------|
| `-h`, `--help`, `--usage` | 打印使用说明并退出 |
| `--version` | 显示版本和构建信息 |
| `--license` | 显示源代码许可证和依赖项 |
| `-cl`, `--cache-list` | 显示缓存中的模型列表 |
| `--completion-bash` | 打印 llama.cpp 的 bash 自动补全脚本 |
| `-t`, `--threads N` | 生成时使用的 CPU 线程数（默认：-1）<br>环境变量：`LLAMA_ARG_THREADS` |
| `-tb`, `--threads-batch N` | 批量和提示处理时使用的线程数（默认：同 `--threads`） |
| `-C`, `--cpu-mask M` | CPU 亲和性掩码：任意长度的十六进制数。与 `cpu-range` 互补（默认：""） |
| `-Cr`, `--cpu-range lo-hi` | CPU 亲和性范围。与 `--cpu-mask` 互补 |
| `--cpu-strict <0\|1>` | 使用严格的 CPU 放置（默认：0） |
| `--prio N` | 设置进程/线程优先级：low(-1), normal(0), medium(1), high(2), realtime(3)（默认：0） |
| `--poll <0...100>` | 使用轮询级别等待工作（0 - 无轮询，默认：50） |
| `-Cb`, `--cpu-mask-batch M` | 批量处理的 CPU 亲和性掩码（默认：同 `--cpu-mask`） |
| `-Crb`, `--cpu-range-batch lo-hi` | 批量处理的 CPU 亲和性范围 |
| `--cpu-strict-batch <0\|1>` | 批量处理使用严格的 CPU 放置（默认：同 `--cpu-strict`） |
| `--prio-batch N` | 批量处理进程/线程优先级（默认：0） |
| `--poll-batch <0\|1>` | 批量处理使用轮询等待工作（默认：同 `--poll`） |
| `-c`, `--ctx-size N` | 提示上下文大小（默认：0，0 = 从模型加载）<br>环境变量：`LLAMA_ARG_CTX_SIZE` |
| `-n`, `--predict`, `--n-predict N` | 要预测的 token 数量（默认：-1，-1 = 无限）<br>环境变量：`LLAMA_ARG_N_PREDICT` |
| `-b`, `--batch-size N` | 逻辑最大批量大小（默认：2048）<br>环境变量：`LLAMA_ARG_BATCH` |
| `-ub`, `--ubatch-size N` | 物理最大批量大小（默认：512）<br>环境变量：`LLAMA_ARG_UBATCH` |
| `--keep N` | 从初始提示中保留的 token 数量（默认：0，-1 = 全部） |
| `--swa-full` | 使用完整大小的 SWA 缓存（默认：false）<br>环境变量：`LLAMA_ARG_SWA_FULL` |
| `-fa`, `--flash-attn [on\|off\|auto]` | 设置 Flash Attention 使用（'on', 'off', 或 'auto'，默认：'auto'）<br>环境变量：`LLAMA_ARG_FLASH_ATTN` |
| `--perf`, `--no-perf` | 是否启用内部 libllama 性能计时（默认：false）<br>环境变量：`LLAMA_ARG_PERF` |
| `-e`, `--escape`, `--no-escape` | 是否处理转义序列（\n, \r, \t, \', \", \\）（默认：true） |
| `--rope-scaling {none,linear,yarn}` | RoPE 频率缩放方法，默认 linear，除非模型另有指定<br>环境变量：`LLAMA_ARG_ROPE_SCALING_TYPE` |
| `--rope-scale N` | RoPE 上下文缩放因子，将上下文扩大 N 倍<br>环境变量：`LLAMA_ARG_ROPE_SCALE` |
| `--rope-freq-base N` | RoPE 基础频率，用于 NTK 感知缩放（默认：从模型加载）<br>环境变量：`LLAMA_ARG_ROPE_FREQ_BASE` |
| `--rope-freq-scale N` | RoPE 频率缩放因子，将上下文扩大 1/N 倍<br>环境变量：`LLAMA_ARG_ROPE_FREQ_SCALE` |
| `--yarn-orig-ctx N` | YaRN：模型原始上下文大小（默认：0 = 模型训练上下文大小）<br>环境变量：`LLAMA_ARG_YARN_ORIG_CTX` |
| `--yarn-ext-factor N` | YaRN：外推混合因子（默认：-1.00，0.0 = 完全插值）<br>环境变量：`LLAMA_ARG_YARN_EXT_FACTOR` |
| `--yarn-attn-factor N` | YaRN：缩放 sqrt(t) 或注意力幅度（默认：-1.00）<br>环境变量：`LLAMA_ARG_YARN_ATTN_FACTOR` |
| `--yarn-beta-slow N` | YaRN：高校正维度或 alpha（默认：-1.00）<br>环境变量：`LLAMA_ARG_YARN_BETA_SLOW` |
| `--yarn-beta-fast N` | YaRN：低校正维度或 beta（默认：-1.00）<br>环境变量：`LLAMA_ARG_YARN_BETA_FAST` |
| `-kvo`, `--kv-offload`, `-nkvo`, `--no-kv-offload` | 是否启用 KV 缓存卸载（默认：启用）<br>环境变量：`LLAMA_ARG_KV_OFFLOAD` |
| `--repack`, `-nr`, `--no-repack` | 是否启用权重重新打包（默认：启用）<br>环境变量：`LLAMA_ARG_REPACK` |
| `--no-host` | 绕过主机缓冲区，允许使用额外缓冲区<br>环境变量：`LLAMA_ARG_NO_HOST` |
| `-ctk`, `--cache-type-k TYPE` | K 的 KV 缓存数据类型<br>允许值：f32, f16, bf16, q8_0, q4_0, q4_1, iq4_nl, q5_0, q5_1<br>（默认：f16）<br>环境变量：`LLAMA_ARG_CACHE_TYPE_K` |
| `-ctv`, `--cache-type-v TYPE` | V 的 KV 缓存数据类型<br>允许值：f32, f16, bf16, q8_0, q4_0, q4_1, iq4_nl, q5_0, q5_1<br>（默认：f16）<br>环境变量：`LLAMA_ARG_CACHE_TYPE_V` |
| `-dt`, `--defrag-thold N` | KV 缓存碎片整理阈值（已弃用）<br>环境变量：`LLAMA_ARG_DEFRAG_THOLD` |
| `--rpc SERVERS` | 逗号分隔的 RPC 服务器列表（host:port）<br>环境变量：`LLAMA_ARG_RPC` |
| `--mlock` | 强制系统保持模型在内存中，而不是交换或压缩<br>环境变量：`LLAMA_ARG_MLOCK` |
| `--mmap`, `--no-mmap` | 是否内存映射模型（如果禁用 mmap，加载较慢但如果不使用 mlock 可能减少页面置换）（默认：启用）<br>环境变量：`LLAMA_ARG_MMAP` |
| `-dio`, `--direct-io`, `-ndio`, `--no-direct-io` | 使用 DirectIO（如果可用）（默认：禁用）<br>环境变量：`LLAMA_ARG_DIO` |
| `--numa TYPE` | 尝试在某些 NUMA 系统上有帮助的优化<br>- distribute：在所有节点上均匀分布执行<br>- isolate：仅在开始执行的节点上的 CPU 上生成线程<br>- numactl：使用 numactl 提供的 CPU 映射<br>如果之前未运行过此命令，建议在使用前清除系统页面缓存<br>环境变量：`LLAMA_ARG_NUMA` |
| `-dev`, `--device <dev1,dev2,..>` | 用于卸载的设备逗号分隔列表（none = 不卸载）<br>使用 `--list-devices` 查看可用设备列表<br>环境变量：`LLAMA_ARG_DEVICE` |
| `--list-devices` | 打印可用设备列表并退出 |
| `-ot`, `--override-tensor <tensor name pattern>=<buffer type>,...` | 覆盖张量缓冲区类型<br>环境变量：`LLAMA_ARG_OVERRIDE_TENSOR` |
| `-cmoe`, `--cpu-moe` | 将所有混合专家（MoE）权重保留在 CPU 中<br>环境变量：`LLAMA_ARG_CPU_MOE` |
| `-ncmoe`, `--n-cpu-moe N` | 将前 N 层的混合专家（MoE）权重保留在 CPU 中<br>环境变量：`LLAMA_ARG_N_CPU_MOE` |
| `-ngl`, `--gpu-layers`, `--n-gpu-layers N` | 存储在 VRAM 中的最大层数，可以是确切数字、'auto' 或 'all'（默认：auto）<br>环境变量：`LLAMA_ARG_N_GPU_LAYERS` |
| `-sm`, `--split-mode {none,layer,row,tensor}` | 如何在多个 GPU 之间拆分模型：<br>- none：仅使用一个 GPU<br>- layer（默认）：在 GPU 之间拆分层和 KV（流水线）<br>- row：按行在 GPU 之间拆分权重（并行化）<br>- tensor：在 GPU 之间拆分权重和 KV（并行化，实验性）<br>环境变量：`LLAMA_ARG_SPLIT_MODE` |
| `-ts`, `--tensor-split N0,N1,N2,...` | 卸载到每个 GPU 的模型比例，逗号分隔的比例列表，例如 3,1<br>环境变量：`LLAMA_ARG_TENSOR_SPLIT` |
| `-mg`, `--main-gpu INDEX` | 用于模型的 GPU（split-mode = none），或用于中间结果和 KV 的 GPU（split-mode = row）（默认：0）<br>环境变量：`LLAMA_ARG_MAIN_GPU` |
| `-fit`, `--fit [on\|off]` | 是否调整未设置的参数以适应设备内存（'on' 或 'off'，默认：'on'）<br>环境变量：`LLAMA_ARG_FIT` |
| `-fitt`, `--fit-target MiB0,MiB1,MiB2,...` | `--fit` 的目标每个设备余量，逗号分隔的值列表，单个值广播到所有设备，默认：1024<br>环境变量：`LLAMA_ARG_FIT_TARGET` |
| `-fitc`, `--fit-ctx N` | `--fit` 选项可以设置的最小 ctx 大小，默认：4096<br>环境变量：`LLAMA_ARG_FIT_CTX` |
| `--check-tensors` | 检查模型张量数据是否包含无效值（默认：false） |
| `--override-kv KEY=TYPE:VALUE,...` | 高级选项，通过键覆盖模型元数据。要指定多个覆盖，使用逗号分隔的值。<br>类型：int, float, bool, str。示例：`--override-kv tokenizer.ggml.add_bos_token=bool:false,tokenizer.ggml.add_eos_token=bool:false` |
| `--op-offload`, `--no-op-offload` | 是否将主机张量操作卸载到设备（默认：true） |
| `--lora FNAME` | LoRA 适配器路径（使用逗号分隔值加载多个适配器） |
| `--lora-scaled FNAME:SCALE,...` | 带用户定义缩放的 LoRA 适配器路径（格式：FNAME:SCALE,...）<br>注意：使用逗号分隔的值 |
| `--control-vector FNAME` | 添加控制向量<br>注意：使用逗号分隔值添加多个控制向量 |
| `--control-vector-scaled FNAME:SCALE,...` | 添加带用户定义缩放 SCALE 的控制向量<br>注意：使用逗号分隔的值（格式：FNAME:SCALE,...） |
| `--control-vector-layer-range START END` | 应用控制向量的层范围，包含起止 |
| `-m`, `--model FNAME` | 要加载的模型路径<br>环境变量：`LLAMA_ARG_MODEL` |
| `-mu`, `--model-url MODEL_URL` | 模型下载 URL（默认：未使用）<br>环境变量：`LLAMA_ARG_MODEL_URL` |
| `-dr`, `--docker-repo [<repo>/]<model>[:quant]` | Docker Hub 模型仓库。repo 是可选的，默认为 ai/。quant 是可选的，默认为 :latest。<br>示例：gemma3<br>（默认：未使用）<br>环境变量：`LLAMA_ARG_DOCKER_REPO` |
| `-hf`, `-hfr`, `--hf-repo <user>/<model>[:quant]` | Hugging Face 模型仓库；quant 是可选的，不区分大小写，默认为 Q4_K_M，如果 Q4_K_M 不存在则回退到仓库中的第一个文件。<br>如果可用，也会自动下载 mmproj。要禁用，添加 `--no-mmproj`<br>示例：ggml-org/GLM-4.7-Flash-GGUF:Q4_K_M<br>（默认：未使用）<br>环境变量：`LLAMA_ARG_HF_REPO` |
| `-hff`, `--hf-file FILE` | Hugging Face 模型文件。如果指定，将覆盖 `--hf-repo` 中的 quant（默认：未使用）<br>环境变量：`LLAMA_ARG_HF_FILE` |
| `-hfv`, `-hfrv`, `--hf-repo-v <user>/<model>[:quant]` | 声码器模型的 Hugging Face 模型仓库（默认：未使用）<br>环境变量：`LLAMA_ARG_HF_REPO_V` |
| `-hffv`, `--hf-file-v FILE` | 声码器模型的 Hugging Face 模型文件（默认：未使用）<br>环境变量：`LLAMA_ARG_HF_FILE_V` |
| `-hft`, `--hf-token TOKEN` | Hugging Face 访问令牌（默认：来自 HF_TOKEN 环境变量的值）<br>环境变量：`HF_TOKEN` |
| `--log-disable` | 禁用日志 |
| `--log-file FNAME` | 日志输出到文件<br>环境变量：`LLAMA_LOG_FILE` |
| `--log-colors [on\|off\|auto]` | 设置彩色日志（'on', 'off', 或 'auto'，默认：'auto'）<br>'auto' 在输出到终端时启用颜色<br>环境变量：`LLAMA_LOG_COLORS` |
| `-v`, `--verbose`, `--log-verbose` | 将详细程度设置为无限（即记录所有消息，用于调试） |
| `--offline` | 离线模式：强制使用缓存，防止网络访问<br>环境变量：`LLAMA_OFFLINE` |
| `-lv`, `--verbosity`, `--log-verbosity N` | 设置详细程度阈值。具有更高详细程度的消息将被忽略。值：<br>- 0：通用输出<br>- 1：错误<br>- 2：警告<br>- 3：信息<br>- 4：跟踪（更多信息）<br>- 5：调试<br>（默认：3）<br>环境变量：`LLAMA_LOG_VERBOSITY` |
| `--log-prefix`, `--no-log-prefix` | 启用日志消息中的前缀<br>环境变量：`LLAMA_ARG_LOG_PREFIX` |
| `--log-timestamps`, `--no-log-timestamps` | 启用日志消息中的时间戳<br>环境变量：`LLAMA_ARG_LOG_TIMESTAMPS` |
| `--spec-draft-type-k`, `-ctkd`, `--cache-type-k-draft TYPE` | 草稿模型 K 的 KV 缓存数据类型<br>允许值：f32, f16, bf16, q8_0, q4_0, q4_1, iq4_nl, q5_0, q5_1<br>（默认：f16）<br>环境变量：`LLAMA_ARG_SPEC_DRAFT_CACHE_TYPE_K` |
| `--spec-draft-type-v`, `-ctvd`, `--cache-type-v-draft TYPE` | 草稿模型 V 的 KV 缓存数据类型<br>允许值：f32, f16, bf16, q8_0, q4_0, q4_1, iq4_nl, q5_0, q5_1<br>（默认：f16）<br>环境变量：`LLAMA_ARG_SPEC_DRAFT_CACHE_TYPE_V` |

---

## 采样参数 (Sampling Params)

| 参数 | 说明 |
|------|------|
| `--samplers SAMPLERS` | 用于生成的采样器，按顺序排列，用 ';' 分隔<br>（默认：penalties;dry;top_n_sigma;top_k;typ_p;top_p;min_p;xtc;temperature） |
| `-s`, `--seed SEED` | RNG 种子（默认：-1，-1 使用随机种子） |
| `--sampler-seq`, `--sampling-seq SEQUENCE` | 简化序列，用于将使用的采样器（默认：edskypmxt） |
| `--ignore-eos` | 忽略结束流 token 并继续生成（意味着 `--logit-bias EOS-inf`） |
| `--temp`, `--temperature N` | 温度（默认：0.80） |
| `--top-k N` | top-k 采样（默认：40，0 = 禁用）<br>环境变量：`LLAMA_ARG_TOP_K` |
| `--top-p N` | top-p 采样（默认：0.95，1.0 = 禁用） |
| `--min-p N` | min-p 采样（默认：0.05，0.0 = 禁用） |
| `--top-nsigma`, `--top-n-sigma N` | top-n-sigma 采样（默认：-1.00，-1.0 = 禁用） |
| `--xtc-probability N` | xtc 概率（默认：0.00，0.0 = 禁用） |
| `--xtc-threshold N` | xtc 阈值（默认：0.10，1.0 = 禁用） |
| `--typical`, `--typical-p N` | 局部典型采样，参数 p（默认：1.00，1.0 = 禁用） |
| `--repeat-last-n N` | 考虑惩罚的最后 n 个 token（默认：64，0 = 禁用，-1 = ctx_size） |
| `--repeat-penalty N` | 惩罚重复 token 序列（默认：1.00，1.0 = 禁用） |
| `--presence-penalty N` | 重复 alpha 存在惩罚（默认：0.00，0.0 = 禁用） |
| `--frequency-penalty N` | 重复 alpha 频率惩罚（默认：0.00，0.0 = 禁用） |
| `--dry-multiplier N` | 设置 DRY 采样乘数（默认：0.00，0.0 = 禁用） |
| `--dry-base N` | 设置 DRY 采样基值（默认：1.75） |
| `--dry-allowed-length N` | 设置 DRY 采样允许长度（默认：2） |
| `--dry-penalty-last-n N` | 设置 DRY 对最后 n 个 token 的惩罚（默认：-1，0 = 禁用，-1 = 上下文大小） |
| `--dry-sequence-breaker STRING` | 为 DRY 采样添加序列分隔符，清除默认分隔符（'\n', ':', '"', '*'）；使用 "none" 不使用任何序列分隔符 |
| `--adaptive-target N` | adaptive-p：选择接近此概率的 token（有效范围 0.0 到 1.0；负值 = 禁用）（默认：-1.00） |
| `--adaptive-decay N` | adaptive-p：目标随时间适应的衰减率。较低的值更具反应性，较高的值更稳定。（有效范围 0.0 到 0.99）（默认：0.90） |
| `--dynatemp-range N` | 动态温度范围（默认：0.00，0.0 = 禁用） |
| `--dynatemp-exp N` | 动态温度指数（默认：1.00） |
| `--mirostat N` | 使用 Mirostat 采样。<br>如果使用，Top K、Nucleus 和局部典型采样器将被忽略。<br>（默认：0，0 = 禁用，1 = Mirostat，2 = Mirostat 2.0） |
| `--mirostat-lr N` | Mirostat 学习率，参数 eta（默认：0.10） |
| `--mirostat-ent N` | Mirostat 目标熵，参数 tau（默认：5.00） |
| `-l`, `--logit-bias TOKEN_ID(+/-)BIAS` | 修改 token 在补全中出现的可能性，例如 `--logit-bias 15043+1` 增加 token ' Hello' 的可能性，或 `--logit-bias 15043-1` 减少 token ' Hello' 的可能性 |
| `--grammar GRAMMAR` | 限制生成的 BNF 类语法（参见 grammars/ 目录中的示例） |
| `--grammar-file FNAME` | 从文件读取语法 |
| `-j`, `--json-schema SCHEMA` | JSON schema 限制生成（https://json-schema.org/），例如 `{}` 用于任何 JSON 对象<br>对于带有外部 $refs 的 schema，使用 `--grammar` + example/json_schema_to_grammar.py |
| `-jf`, `--json-schema-file FILE` | 包含 JSON schema 的文件以限制生成（https://json-schema.org/），例如 `{}` 用于任何 JSON 对象<br>对于带有外部 $refs 的 schema，使用 `--grammar` + example/json_schema_to_grammar.py |
| `-bs`, `--backend-sampling` | 启用后端采样（实验性）（默认：禁用）<br>环境变量：`LLAMA_ARG_BACKEND_SAMPLING` |

---

## 投机解码参数 (Speculative Params)

| 参数 | 说明 |
|------|------|
| `--spec-draft-hf`, `-hfd`, `-hfrd`, `--hf-repo-draft <user>/<model>[:quant]` | 与 `--hf-repo` 相同，但用于草稿模型（默认：未使用）<br>环境变量：`LLAMA_ARG_SPEC_DRAFT_HF_REPO` |
| `--spec-draft-threads`, `-td`, `--threads-draft N` | 生成时使用的线程数（默认：同 `--threads`） |
| `--spec-draft-threads-batch`, `-tbd`, `--threads-batch-draft N` | 批量和提示处理时使用的线程数（默认：同 `--threads-draft`） |
| `--spec-draft-cpu-mask`, `-Cd`, `--cpu-mask-draft M` | 草稿模型 CPU 亲和性掩码。与 cpu-range-draft 互补（默认：同 `--cpu-mask`） |
| `--spec-draft-cpu-range`, `-Crd`, `--cpu-range-draft lo-hi` | CPU 亲和性范围。与 `--cpu-mask-draft` 互补 |
| `--spec-draft-cpu-strict`, `--cpu-strict-draft <0\|1>` | 草稿模型使用严格的 CPU 放置（默认：同 `--cpu-strict`） |
| `--spec-draft-prio`, `--prio-draft N` | 设置草稿进程/线程优先级：0-normal, 1-medium, 2-high, 3-realtime（默认：0） |
| `--spec-draft-poll`, `--poll-draft <0\|1>` | 使用轮询等待草稿模型工作（默认：同 `--poll`） |
| `--spec-draft-cpu-mask-batch`, `-Cbd`, `--cpu-mask-batch-draft M` | 草稿模型批量处理的 CPU 亲和性掩码（默认：同 `--cpu-mask`） |
| `--spec-draft-cpu-strict-batch`, `--cpu-strict-batch-draft <0\|1>` | 草稿模型批量处理使用严格的 CPU 放置（默认：`--cpu-strict-draft`） |
| `--spec-draft-prio-batch`, `--prio-batch-draft N` | 设置草稿批量处理进程/线程优先级（默认：0） |
| `--spec-draft-poll-batch`, `--poll-batch-draft <0\|1>` | 使用轮询等待草稿模型批量处理工作（默认：`--poll-draft`） |
| `--spec-draft-override-tensor`, `-otd`, `--override-tensor-draft <tensor name pattern>=<buffer type>,...` | 覆盖草稿模型张量缓冲区类型 |
| `--spec-draft-cpu-moe`, `-cmoed`, `--cpu-moe-draft` | 将草稿模型的所有混合专家（MoE）权重保留在 CPU 中<br>环境变量：`LLAMA_ARG_SPEC_DRAFT_CPU_MOE` |
| `--spec-draft-n-cpu-moe`, `--spec-draft-ncmoe`, `-ncmoed`, `--n-cpu-moe-draft N` | 将草稿模型的前 N 层混合专家（MoE）权重保留在 CPU 中<br>环境变量：`LLAMA_ARG_SPEC_DRAFT_N_CPU_MOE` |
| `--spec-draft-n-max N` | 投机解码时草稿 token 的数量（默认：3）<br>环境变量：`LLAMA_ARG_SPEC_DRAFT_N_MAX` |
| `--spec-draft-n-min N` | 投机解码时使用的最小草稿 token 数量（默认：0）<br>环境变量：`LLAMA_ARG_SPEC_DRAFT_N_MIN` |
| `--spec-draft-p-split`, `--draft-p-split P` | 投机解码拆分概率（默认：0.10）<br>环境变量：`LLAMA_ARG_SPEC_DRAFT_P_SPLIT` |
| `--spec-draft-p-min`, `--draft-p-min P` | 最小投机解码概率（贪婪）（默认：0.00）<br>环境变量：`LLAMA_ARG_SPEC_DRAFT_P_MIN` |
| `--spec-draft-backend-sampling`, `--no-spec-draft-backend-sampling` | 将草稿采样卸载到后端（默认：启用）<br>环境变量：`LLAMA_ARG_SPEC_DRAFT_BACKEND_SAMPLING` |
| `--spec-draft-device`, `-devd`, `--device-draft <dev1,dev2,..>` | 用于卸载草稿模型的设备逗号分隔列表（none = 不卸载）<br>使用 `--list-devices` 查看可用设备 |
| `--spec-draft-ngl`, `-ngld`, `--gpu-layers-draft`, `--n-gpu-layers-draft N` | 存储在 VRAM 中的草稿模型最大层数，可以是确切数字、'auto' 或 'all'（默认：auto）<br>环境变量：`LLAMA_ARG_N_GPU_LAYERS_DRAFT` |
| `--spec-draft-model`, `-md`, `--model-draft FNAME` | 投机解码的草稿模型（默认：未使用）<br>环境变量：`LLAMA_ARG_SPEC_DRAFT_MODEL` |
| `--spec-type none,draft_simple,draft-eagle3,draft-mtp,ngram-simple,ngram-map-k,ngram-map-k4v,ngram-mod,ngram-cache` | 要使用的投机解码类型逗号分隔列表（默认：none）<br>环境变量：`LLAMA_ARG_SPEC_TYPE` |
| `--spec-ngram-mod-n-min N` | 基于 ngram 的投机解码使用的最小 ngram token 数量（默认：48） |
| `--spec-ngram-mod-n-max N` | 基于 ngram 的投机解码使用的最大 ngram token 数量（默认：64） |
| `--spec-ngram-mod-n-match N` | ngram-mod 查找长度（默认：24） |
| `--spec-ngram-simple-size-n N` | ngram-simple 投机解码的 ngram 大小 N，查找 n-gram 的长度（默认：12） |
| `--spec-ngram-simple-size-m N` | ngram-simple 投机解码的 ngram 大小 M，草稿 m-gram 的长度（默认：48） |
| `--spec-ngram-simple-min-hits N` | ngram-simple 投机解码的最小命中数（默认：1） |
| `--spec-ngram-map-k-size-n N` | ngram-map-k 投机解码的 ngram 大小 N，查找 n-gram 的长度（默认：12） |
| `--spec-ngram-map-k-size-m N` | ngram-map-k 投机解码的 ngram 大小 M，草稿 m-gram 的长度（默认：48） |
| `--spec-ngram-map-k-min-hits N` | ngram-map-k 投机解码的最小命中数（默认：1） |
| `--spec-ngram-map-k4v-size-n N` | ngram-map-k4v 投机解码的 ngram 大小 N，查找 n-gram 的长度（默认：12） |
| `--spec-ngram-map-k4v-size-m N` | ngram-map-k4v 投机解码的 ngram 大小 M，草稿 m-gram 的长度（默认：48） |
| `--spec-ngram-map-k4v-min-hits N` | ngram-map-k4v 投机解码的最小命中数（默认：1） |
| `--draft`, `--draft-n`, `--draft-max N` | 此参数已被移除。使用 `--spec-draft-n-max` 或 `--spec-ngram-mod-n-max`<br>环境变量：`LLAMA_ARG_DRAFT_MAX` |
| `--draft-min`, `--draft-n-min N` | 此参数已被移除。使用 `--spec-draft-n-min` 或 `--spec-ngram-mod-n-min`<br>环境变量：`LLAMA_ARG_DRAFT_MIN` |
| `--spec-ngram-size-n N` | 此参数已被移除。使用相应的 `--spec-ngram-*-size-n` 或 `--spec-ngram-mod-n-match` |
| `--spec-ngram-size-m N` | 此参数已被移除。使用相应的 `--spec-ngram-*-size-m` |
| `--spec-ngram-min-hits N` | 此参数已被移除。使用相应的 `--spec-ngram-*-min-hits` |

---

## 示例特定参数 (Example-Specific Params)

| 参数 | 说明 |
|------|------|
| `-lcs`, `--lookup-cache-static FNAME` | 用于查找解码的静态查找缓存路径（不随生成更新） |
| `-lcd`, `--lookup-cache-dynamic FNAME` | 用于查找解码的动态查找缓存路径（随生成更新） |
| `-ctxcp`, `--ctx-checkpoints`, `--swa-checkpoints N` | 每个插槽创建的最大上下文检查点数量（默认：32）<br>环境变量：`LLAMA_ARG_CTX_CHECKPOINTS` |
| `-cpent`, `--checkpoint-every-n-tokens N` | 在预填充（处理）期间每 n 个 token 创建一个检查点，-1 禁用（默认：8192）<br>环境变量：`LLAMA_ARG_CHECKPOINT_EVERY_NT` |
| `-cram`, `--cache-ram N` | 设置最大缓存大小（MiB）（默认：8192，-1 = 无限制，0 = 禁用）<br>环境变量：`LLAMA_ARG_CACHE_RAM` |
| `-kvu`, `--kv-unified`, `-no-kvu`, `--no-kv-unified` | 使用所有序列共享的单一统一 KV 缓冲区（默认：如果插槽数量自动则启用）<br>环境变量：`LLAMA_ARG_KV_UNIFIED` |
| `--cache-idle-slots`, `--no-cache-idle-slots` | 在新任务上保存和清除空闲插槽（默认：启用，需要统一 KV 和 cache-ram）<br>环境变量：`LLAMA_ARG_CACHE_IDLE_SLOTS` |
| `--context-shift`, `--no-context-shift` | 是否在无限文本生成中使用上下文偏移（默认：禁用）<br>环境变量：`LLAMA_ARG_CONTEXT_SHIFT` |
| `-r`, `--reverse-prompt PROMPT` | 在 PROMPT 处停止生成，在交互模式中返回控制 |
| `-sp`, `--special` | 启用特殊 token 输出（默认：false） |
| `--warmup`, `--no-warmup` | 是否使用空运行进行预热（默认：启用） |
| `--spm-infill` | 使用 Suffix/Prefix/Middle 模式进行填充（而不是 Prefix/Suffix/Middle），因为某些模型更喜欢这种模式（默认：禁用） |
| `--pooling {none,mean,cls,last,rank}` | 嵌入的池化类型，使用模型默认（如果未指定）<br>环境变量：`LLAMA_ARG_POOLING` |
| `-np`, `--parallel N` | 服务器插槽数量（默认：-1，-1 = 自动）<br>环境变量：`LLAMA_ARG_N_PARALLEL` |
| `-cb`, `--cont-batching`, `-nocb`, `--no-cont-batching` | 是否启用连续批量（又称动态批量）（默认：启用）<br>环境变量：`LLAMA_ARG_CONT_BATCHING` |
| `-mm`, `--mmproj FILE` | 多模态投影文件路径。参见 tools/mtmd/README.md<br>注意：如果使用 `-hf`，此参数可以省略<br>环境变量：`LLAMA_ARG_MMPROJ` |
| `-mmu`, `--mmproj-url URL` | 多模态投影文件 URL。参见 tools/mtmd/README.md<br>环境变量：`LLAMA_ARG_MMPROJ_URL` |
| `--mmproj-auto`, `--no-mmproj`, `--no-mmproj-auto` | 是否使用多模态投影文件（如果可用），使用 `-hf` 时有用（默认：启用）<br>环境变量：`LLAMA_ARG_MMPROJ_AUTO` |
| `--mmproj-offload`, `--no-mmproj-offload` | 是否启用多模态投影的 GPU 卸载（默认：启用）<br>环境变量：`LLAMA_ARG_MMPROJ_OFFLOAD` |
| `--image-min-tokens N` | 每个图像可以采用的最小 token 数量，仅用于具有动态分辨率的视觉模型（默认：从模型读取）<br>环境变量：`LLAMA_ARG_IMAGE_MIN_TOKENS` |
| `--image-max-tokens N` | 每个图像可以采用的最大 token 数量，仅用于具有动态分辨率的视觉模型（默认：从模型读取）<br>环境变量：`LLAMA_ARG_IMAGE_MAX_TOKENS` |
| `-a`, `--alias STRING` | 设置模型名称别名，逗号分隔（供 API 使用）<br>环境变量：`LLAMA_ARG_ALIAS` |
| `--tags STRING` | 设置模型标签，逗号分隔（信息性，不用于路由）<br>环境变量：`LLAMA_ARG_TAGS` |
| `--embd-normalize N` | 嵌入的归一化（默认：2）（-1=无，0=最大绝对 int16，1=出租车，2=欧几里得，>2=p-范数） |
| `--host HOST` | 监听的 IP 地址，或如果地址以 .sock 结尾则绑定到 UNIX 套接字（默认：127.0.0.1）<br>环境变量：`LLAMA_ARG_HOST` |
| `--port PORT` | 监听的端口（默认：8080）<br>环境变量：`LLAMA_ARG_PORT` |
| `--reuse-port` | 允许多个套接字绑定到同一端口（默认：禁用）<br>环境变量：`LLAMA_ARG_REUSE_PORT` |
| `--path PATH` | 提供静态文件的路径（默认：空）<br>环境变量：`LLAMA_ARG_STATIC_PATH` |
| `--api-prefix PREFIX` | 服务器提供服务的 API 前缀路径，不带尾部斜杠（默认：空）<br>环境变量：`LLAMA_ARG_API_PREFIX` |
| `--webui-config JSON` | [已弃用：使用 `--ui-config`] 提供默认 WebUI 设置的 JSON（覆盖 WebUI 默认值）<br>环境变量：`LLAMA_ARG_WEBUI_CONFIG` |
| `--ui-config JSON` | 提供默认 UI 设置的 JSON（覆盖 UI 默认值）<br>环境变量：`LLAMA_ARG_UI_CONFIG` |
| `--webui-config-file PATH` | [已弃用：使用 `--ui-config-file`] 提供默认 WebUI 设置的 JSON 文件（覆盖 WebUI 默认值）<br>环境变量：`LLAMA_ARG_WEBUI_CONFIG_FILE` |
| `--ui-config-file PATH` | 提供默认 UI 设置的 JSON 文件（覆盖 UI 默认值）<br>环境变量：`LLAMA_ARG_UI_CONFIG_FILE` |
| `--webui-mcp-proxy`, `--no-webui-mcp-proxy` | [已弃用：使用 `--ui-mcp-proxy/--no-ui-mcp-proxy`] 实验性：是否启用 MCP CORS 代理<br>环境变量：`LLAMA_ARG_WEBUI_MCP_PROXY` |
| `--ui-mcp-proxy`, `--no-ui-mcp-proxy` | 实验性：是否启用 MCP CORS 代理 - 不要在不受信任的环境中启用（默认：禁用）<br>环境变量：`LLAMA_ARG_UI_MCP_PROXY` |
| `--tools TOOL1,TOOL2,...` | 实验性：是否为 AI 代理启用内置工具 - 不要在不受信任的环境中启用（默认：无工具）<br>指定 "all" 启用所有工具<br>可用工具：read_file, file_glob_search, grep_search, exec_shell_command, write_file, edit_file, apply_diff, get_datetime<br>环境变量：`LLAMA_ARG_TOOLS` |
| `--webui`, `--no-webui` | [已弃用：使用 `--ui/--no-ui`] 是否启用 Web UI<br>环境变量：`LLAMA_ARG_WEBUI` |
| `--ui`, `--no-ui` | 是否启用 Web UI（默认：启用）<br>环境变量：`LLAMA_ARG_UI` |
| `--embedding`, `--embeddings` | 限制仅支持嵌入用例；仅与专用嵌入模型一起使用（默认：禁用）<br>环境变量：`LLAMA_ARG_EMBEDDINGS` |
| `--rerank`, `--reranking` | 在服务器上启用重排序端点（默认：禁用）<br>环境变量：`LLAMA_ARG_RERANKING` |
| `--api-key KEY` | 用于认证的 API 密钥，可以提供多个密钥作为逗号分隔列表（默认：无）<br>环境变量：`LLAMA_API_KEY` |
| `--api-key-file FNAME` | 包含 API 密钥的文件路径（默认：无） |
| `--ssl-key-file FNAME` | PEM 编码的 SSL 私钥文件路径<br>环境变量：`LLAMA_ARG_SSL_KEY_FILE` |
| `--ssl-cert-file FNAME` | PEM 编码的 SSL 证书文件路径<br>环境变量：`LLAMA_ARG_SSL_CERT_FILE` |
| `--chat-template-kwargs STRING` | 为 JSON 模板解析器设置额外的参数，必须是有效的 JSON 对象字符串，例如 `'{"key1":"value1","key2":"value2"}'`<br>环境变量：`LLAMA_CHAT_TEMPLATE_KWARGS` |
| `-to`, `--timeout N` | 服务器读/写超时时间（秒）（默认：600）<br>环境变量：`LLAMA_ARG_TIMEOUT` |
| `--threads-http N` | 用于处理 HTTP 请求的线程数（默认：-1）<br>环境变量：`LLAMA_ARG_THREADS_HTTP` |
| `--cache-prompt`, `--no-cache-prompt` | 是否启用提示缓存（默认：启用）<br>环境变量：`LLAMA_ARG_CACHE_PROMPT` |
| `--cache-reuse N` | 尝试通过 KV 偏移从缓存中重用的最小块大小，需要启用提示缓存（默认：0）<br>环境变量：`LLAMA_ARG_CACHE_REUSE` |
| `--metrics` | 启用与 Prometheus 兼容的指标端点（默认：禁用）<br>环境变量：`LLAMA_ARG_ENDPOINT_METRICS` |
| `--props` | 启用通过 POST /props 更改全局属性（默认：禁用）<br>环境变量：`LLAMA_ARG_ENDPOINT_PROPS` |
| `--slots`, `--no-slots` | 暴露插槽监控端点（默认：启用）<br>环境变量：`LLAMA_ARG_ENDPOINT_SLOTS` |
| `--slot-save-path PATH` | 保存插槽 KV 缓存的路径（默认：禁用） |
| `--media-path PATH` | 用于加载本地媒体文件的目录；文件可以使用 file:// URL 通过相对路径访问（默认：禁用） |
| `--models-dir PATH` | 包含路由服务器模型的目录（默认：禁用）<br>环境变量：`LLAMA_ARG_MODELS_DIR` |
| `--models-preset PATH` | 包含路由服务器模型预设的 INI 文件路径（默认：禁用）<br>环境变量：`LLAMA_ARG_MODELS_PRESET` |
| `--models-max N` | 对于路由服务器，同时加载的最大模型数量（默认：4，0 = 无限制）<br>环境变量：`LLAMA_ARG_MODELS_MAX` |
| `--models-autoload`, `--no-models-autoload` | 对于路由服务器，是否自动加载模型（默认：启用）<br>环境变量：`LLAMA_ARG_MODELS_AUTOLOAD` |
| `--jinja`, `--no-jinja` | 是否使用 jinja 模板引擎进行聊天（默认：启用）<br>环境变量：`LLAMA_ARG_JINJA` |
| `--reasoning-format FORMAT` | 控制是否允许和/或从响应中提取思考标签，以及它们以哪种格式返回；其中之一：<br>- none：将思考保留在 `message.content` 中不解析<br>- deepseek：将思考放在 `message.reasoning_content` 中<br>- deepseek-legacy：在 `message.content` 中保留 `<think>` 标签，同时填充 `message.reasoning_content`<br>（默认：auto）<br>环境变量：`LLAMA_ARG_THINK` |
| `-rea`, `--reasoning [on\|off\|auto]` | 在聊天中使用推理/思考（'on', 'off', 或 'auto'，默认：'auto'（从模板检测））<br>环境变量：`LLAMA_ARG_REASONING` |
| `--reasoning-budget N` | 思考的 token 预算：-1 表示无限制，0 表示立即结束，N>0 表示 token 预算（默认：-1）<br>环境变量：`LLAMA_ARG_THINK_BUDGET` |
| `--reasoning-budget-message MESSAGE` | 当思考预算耗尽时注入到思考结束标签之前的消息（默认：无）<br>环境变量：`LLAMA_ARG_THINK_BUDGET_MESSAGE` |
| `--chat-template JINJA_TEMPLATE` | 设置自定义 jinja 聊天模板（默认：从模型元数据获取模板）<br>如果指定了后缀/前缀，将禁用模板<br>仅接受常用的模板（除非在此标志之前设置 `--jinja`）：<br>内置模板列表：bailing, bailing-think, bailing2, chatglm3, chatglm4, chatml, command-r, deepseek, deepseek-ocr, deepseek2, deepseek3, exaone-moe, exaone3, exaone4, falcon3, gemma, gigachat, glmedge, gpt-oss, granite, granite-4.0, grok-2, hunyuan-dense, hunyuan-moe, hunyuan-vl, kimi-k2, llama2, llama2-sys, llama2-sys-bos, llama2-sys-strip, llama3, llama4, megrez, minicpm, mistral-v1, mistral-v3, mistral-v3-tekken, mistral-v7, mistral-v7-tekken, monarch, openchat, orion, pangu-embedded, phi3, phi4, rwkv-world, seed_oss, smolvlm, solar-open, vicuna, vicuna-orca, yandex, zephyr<br>环境变量：`LLAMA_ARG_CHAT_TEMPLATE` |
| `--chat-template-file JINJA_TEMPLATE_FILE` | 设置自定义 jinja 聊天模板文件（默认：从模型元数据获取模板）<br>如果指定了后缀/前缀，将禁用模板<br>仅接受常用的模板（除非在此标志之前设置 `--jinja`）：<br>内置模板列表同上<br>环境变量：`LLAMA_ARG_CHAT_TEMPLATE_FILE` |
| `--skip-chat-parsing`, `--no-skip-chat-parsing` | 强制使用纯内容解析器，即使指定了 Jinja 模板；模型将输出所有内容到 content 部分，包括任何推理和/或工具调用（默认：禁用）<br>环境变量：`LLAMA_ARG_SKIP_CHAT_PARSING` |
| `--prefill-assistant`, `--no-prefill-assistant` | 如果最后一条消息是 assistant 消息，是否预填充 assistant 的响应（默认：启用预填充）<br>当设置此标志时，如果最后一条消息是 assistant 消息，则将其视为完整消息而不是预填充<br>环境变量：`LLAMA_ARG_PREFILL_ASSISTANT` |
| `-sps`, `--slot-prompt-similarity SIMILARITY` | 请求的提示必须与插槽的提示匹配多少才能使用该插槽（默认：0.10，0.0 = 禁用） |
| `--lora-init-without-apply` | 加载 LoRA 适配器而不应用它们（稍后通过 POST /lora-adapters 应用）（默认：禁用） |
| `--sleep-idle-seconds SECONDS` | 服务器在空闲多少秒后进入休眠（默认：-1；-1 = 禁用） |
| `-mv`, `--model-vocoder FNAME` | 用于音频生成的声码器模型（默认：未使用） |
| `--tts-use-guide-tokens` | 使用引导 token 以提高 TTS 单词召回率 |
| `--embd-gemma-default` | 使用默认 EmbeddingGemma 模型（注意：可以从互联网下载权重） |
| `--fim-qwen-1.5b-default` | 使用默认 Qwen 2.5 Coder 1.5B（注意：可以从互联网下载权重） |
| `--fim-qwen-3b-default` | 使用默认 Qwen 2.5 Coder 3B（注意：可以从互联网下载权重） |
| `--fim-qwen-7b-default` | 使用默认 Qwen 2.5 Coder 7B（注意：可以从互联网下载权重） |
| `--fim-qwen-7b-spec` | 使用 Qwen 2.5 Coder 7B + 0.5B 草稿模型进行投机解码（注意：可以从互联网下载权重） |
| `--fim-qwen-14b-spec` | 使用 Qwen 2.5 Coder 14B + 0.5B 草稿模型进行投机解码（注意：可以从互联网下载权重） |
| `--fim-qwen-30b-default` | 使用默认 Qwen 3 Coder 30B A3B Instruct（注意：可以从互联网下载权重） |
| `--gpt-oss-20b-default` | 使用 gpt-oss-20b（注意：可以从互联网下载权重） |
| `--gpt-oss-120b-default` | 使用 gpt-oss-120b（注意：可以从互联网下载权重） |
| `--vision-gemma-4b-default` | 使用 Gemma 3 4B QAT（注意：可以从互联网下载权重） |
| `--vision-gemma-12b-default` | 使用 Gemma 3 12B QAT（注意：可以从互联网下载权重） |
| `--spec-default` | 启用默认投机解码配置 |
