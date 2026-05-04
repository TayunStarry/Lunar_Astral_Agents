# llama.cpp 参数文档

## 一、通用参数

| 短参数   | 长参数                                   | 说明                              | 默认值              | 环境变量                       |
| -------- | ---------------------------------------- | --------------------------------- | ------------------- | ------------------------------ |
| `-h`     | `--help`, `--usage`                      | 打印使用说明并退出                | -                   | -                              |
| -        | `--version`                              | 显示版本和构建信息                | -                   | -                              |
| -        | `--completion-bash`                      | 打印可用于 bash 自动补全的脚本    | -                   | -                              |
| -        | `--verbose-prompt`                       | 在生成前打印详细提示              | false               | -                              |
| `-t`     | `--threads N`                            | 生成过程中使用的线程数            | -1                  | `LLAMA_ARG_THREADS`            |
| `-tb`    | `--threads-batch N`                      | 批处理和提示处理期间使用的线程数  | 同 `--threads`      | -                              |
| `-C`     | `--cpu-mask M`                           | CPU 亲和性掩码（十六进制）        | ""                  | -                              |
| `-Cr`    | `--cpu-range lo-hi`                      | 用于亲和性的 CPU 范围             | -                   | -                              |
| -        | `--cpu-strict <0\|1>`                    | 使用严格的 CPU 放置               | 0                   | -                              |
| -        | `--prio N`                               | 设置进程/线程优先级（-1 ~ 3）     | 0                   | -                              |
| -        | `--poll <0...100>`                       | 使用轮询级别等待工作              | 50                  | -                              |
| `-Cb`    | `--cpu-mask-batch M`                     | 批处理 CPU 亲和性掩码（十六进制） | 同 `--cpu-mask`     | -                              |
| `-Crb`   | `--cpu-range-batch lo-hi`                | 批处理 CPU 范围                   | -                   | -                              |
| -        | `--cpu-strict-batch <0\|1>`              | 批处理严格的 CPU 放置             | 同 `--cpu-strict`   | -                              |
| -        | `--prio-batch N`                         | 批处理进程/线程优先级             | 0                   | -                              |
| -        | `--poll-batch <0\|1>`                    | 批处理轮询等待工作                | 同 `--poll`         | -                              |
| `-c`     | `--ctx-size N`                           | 提示上下文大小                    | 4096                | `LLAMA_ARG_CTX_SIZE`           |
| `-n`     | `--predict`, `--n-predict N`             | 要预测的 token 数量               | -1                  | `LLAMA_ARG_N_PREDICT`          |
| `-b`     | `--batch-size N`                         | 逻辑最大批处理大小                | 2048                | `LLAMA_ARG_BATCH`              |
| `-ub`    | `--ubatch-size N`                        | 物理最大批处理大小                | 512                 | `LLAMA_ARG_UBATCH`             |
| -        | `--keep N`                               | 从初始提示中保留的 token 数量     | 0                   | -                              |
| -        | `--swa-full`                             | 使用全尺寸 SWA 缓存               | false               | `LLAMA_ARG_SWA_FULL`           |
| `-kvu`   | `--kv-unified`                           | 使用统一的 KV 缓冲区              | false               | `LLAMA_ARG_KV_SPLIT`           |
| `-fa`    | `--flash-attn`                           | 启用 Flash Attention              | 禁用                | `LLAMA_ARG_FLASH_ATTN`         |
| -        | `--no-perf`                              | 禁用内部性能计时                  | false               | `LLAMA_ARG_NO_PERF`            |
| `-e`     | `--escape`                               | 处理转义序列                      | true                | -                              |
| -        | `--no-escape`                            | 不处理转义序列                    | -                   | -                              |
| -        | `--rope-scaling {none,linear,yarn}`      | RoPE 频率缩放方法                 | 线性                | `LLAMA_ARG_ROPE_SCALING_TYPE`  |
| -        | `--rope-scale N`                         | RoPE 上下文缩放因子               | -                   | `LLAMA_ARG_ROPE_SCALE`         |
| -        | `--rope-freq-base N`                     | RoPE 基础频率                     | 从模型加载          | `LLAMA_ARG_ROPE_FREQ_BASE`     |
| -        | `--rope-freq-scale N`                    | RoPE 频率缩放因子                 | -                   | `LLAMA_ARG_ROPE_FREQ_SCALE`    |
| -        | `--yarn-orig-ctx N`                      | YaRN 原始上下文大小               | 0                   | `LLAMA_ARG_YARN_ORIG_CTX`      |
| -        | `--yarn-ext-factor N`                    | YaRN 外推混合因子                 | -1.0                | `LLAMA_ARG_YARN_EXT_FACTOR`    |
| -        | `--yarn-attn-factor N`                   | YaRN 注意力缩放因子               | 1.0                 | `LLAMA_ARG_YARN_ATTN_FACTOR`   |
| -        | `--yarn-beta-slow N`                     | YaRN 高校正维度                   | 1.0                 | `LLAMA_ARG_YARN_BETA_SLOW`     |
| -        | `--yarn-beta-fast N`                     | YaRN 低校正维度                   | 32.0                | `LLAMA_ARG_YARN_BETA_FAST`     |
| `-nkvo`  | `--no-kv-offload`                        | 禁用 KV 卸载                      | -                   | `LLAMA_ARG_NO_KV_OFFLOAD`      |
| `-nr`    | `--no-repack`                            | 禁用权重重新打包                  | -                   | `LLAMA_ARG_NO_REPACK`          |
| `-ctk`   | `--cache-type-k TYPE`                    | K 的 KV 缓存数据类型              | f16                 | `LLAMA_ARG_CACHE_TYPE_K`       |
| `-ctv`   | `--cache-type-v TYPE`                    | V 的 KV 缓存数据类型              | f16                 | `LLAMA_ARG_CACHE_TYPE_V`       |
| `-dt`    | `--defrag-thold N`                       | KV 缓存碎片整理阈值               | 0.1                 | `LLAMA_ARG_DEFRAG_THOLD`       |
| `-np`    | `--parallel N`                           | 并行解码的序列数                  | 1                   | `LLAMA_ARG_N_PARALLEL`         |
| -        | `--rpc SERVERS`                          | RPC 服务器列表                    | -                   | `LLAMA_ARG_RPC`                |
| -        | `--mlock`                                | 强制模型保存在 RAM 中             | -                   | `LLAMA_ARG_MLOCK`              |
| -        | `--no-mmap`                              | 不进行模型内存映射                | -                   | `LLAMA_ARG_NO_MMAP`            |
| -        | `--numa TYPE`                            | NUMA 优化设置                     | -                   | `LLAMA_ARG_NUMA`               |
| `-dev`   | `--device <dev1,dev2,..>`                | 卸载设备列表                      | -                   | `LLAMA_ARG_DEVICE`             |
| -        | `--list-devices`                         | 打印可用设备列表                  | -                   | -                              |
| `-ot`    | `--override-tensor <tensor=type>`        | 覆盖张量缓冲区类型                | -                   | -                              |
| `-cmoe`  | `--cpu-moe`                              | 将所有 MoE 权重保存在 CPU         | -                   | `LLAMA_ARG_CPU_MOE`            |
| `-ncmoe` | `--n-cpu-moe N`                          | 将前 N 层 MoE 权重保存在 CPU      | -                   | `LLAMA_ARG_N_CPU_MOE`          |
| `-ngl`   | `--gpu-layers N`                         | 存储在 VRAM 中的层数              | -                   | `LLAMA_ARG_N_GPU_LAYERS`       |
| `-sm`    | `--split-mode {none,layer,row}`          | 多 GPU 拆分模式                   | layer               | `LLAMA_ARG_SPLIT_MODE`         |
| `-ts`    | `--tensor-split N0,N1,...`               | GPU 模型比例拆分                  | -                   | `LLAMA_ARG_TENSOR_SPLIT`       |
| `-mg`    | `--main-gpu INDEX`                       | 主 GPU 索引                       | 0                   | `LLAMA_ARG_MAIN_GPU`           |
| -        | `--check-tensors`                        | 检查模型张量数据                  | false               | -                              |
| -        | `--override-kv KEY=TYPE:VALUE`           | 覆盖模型元数据                    | -                   | -                              |
| -        | `--no-op-offload`                        | 禁用主机张量操作卸载              | false               | -                              |
| -        | `--lora FNAME`                           | LoRA 适配器路径                   | -                   | -                              |
| -        | `--lora-scaled FNAME SCALE`              | 带缩放的 LoRA 适配器              | -                   | -                              |
| -        | `--control-vector FNAME`                 | 添加控制向量                      | -                   | -                              |
| -        | `--control-vector-scaled FNAME SCALE`    | 带缩放的控制向量                  | -                   | -                              |
| -        | `--control-vector-layer-range START END` | 控制向量层范围                    | -                   | -                              |
| `-m`     | `--model FNAME`                          | 模型路径                          | 默认路径            | `LLAMA_ARG_MODEL`              |
| `-mu`    | `--model-url MODEL_URL`                  | 模型下载 URL                      | -                   | `LLAMA_ARG_MODEL_URL`          |
| `-hf`    | `--hf-repo <user>/<model>[:quant]`       | Hugging Face 模型仓库             | -                   | `LLAMA_ARG_HF_REPO`            |
| `-hfd`   | `--hf-repo-draft <user>/<model>[:quant]` | 草稿模型 HF 仓库                  | -                   | `LLAMA_ARG_HFD_REPO`           |
| `-hff`   | `--hf-file FILE`                         | HF 模型文件                       | -                   | `LLAMA_ARG_HF_FILE`            |
| `-hfv`   | `--hf-repo-v <user>/<model>[:quant]`     | 声码器模型 HF 仓库                | -                   | `LLAMA_ARG_HF_REPO_V`          |
| `-hffv`  | `--hf-file-v FILE`                       | 声码器模型 HF 文件                | -                   | `LLAMA_ARG_HF_FILE_V`          |
| `-hft`   | `--hf-token TOKEN`                       | HF 访问令牌                       | 环境变量 `HF_TOKEN` | -                              |
| -        | `--log-disable`                          | 禁用日志                          | -                   | -                              |
| -        | `--log-file FNAME`                       | 记录到文件                        | -                   | -                              |
| -        | `--log-colors`                           | 启用彩色日志                      | -                   | `LLAMA_LOG_COLORS`             |
| `-v`     | `--verbose`                              | 详细日志（调试用）                | -                   | -                              |
| -        | `--offline`                              | 离线模式                          | -                   | `LLAMA_OFFLINE`                |
| `-lv`    | `--verbosity N`                          | 设置详细级别阈值                  | -                   | `LLAMA_LOG_VERBOSITY`          |
| -        | `--log-prefix`                           | 启用日志前缀                      | -                   | `LLAMA_LOG_PREFIX`             |
| -        | `--log-timestamps`                       | 启用时间戳                        | -                   | `LLAMA_LOG_TIMESTAMPS`         |
| `-ctkd`  | `--cache-type-k-draft TYPE`              | 草稿模型 K 缓存类型               | f16                 | `LLAMA_ARG_CACHE_TYPE_K_DRAFT` |
| `-ctvd`  | `--cache-type-v-draft TYPE`              | 草稿模型 V 缓存类型               | f16                 | `LLAMA_ARG_CACHE_TYPE_V_DRAFT` |

---

## 二、采样参数

| 短参数 | 长参数                           | 说明                    | 默认值                                                            | 环境变量 |
| ------ | -------------------------------- | ----------------------- | ----------------------------------------------------------------- | -------- |
| -      | `--samplers SAMPLERS`            | 使用的采样器顺序        | penalties;dry;top_n_sigma;top_k;typ_p;top_p;min_p;xtc;temperature | -        |
| `-s`   | `--seed SEED`                    | RNG 种子                | -1                                                                | -        |
| -      | `--sampling-seq SEQUENCE`        | 简化采样器序列          | edskypmxt                                                         | -        |
| -      | `--ignore-eos`                   | 忽略 EOS token          | -                                                                 | -        |
| -      | `--temp N`                       | 温度                    | 0.8                                                               | -        |
| -      | `--top-k N`                      | top-k 采样              | 40                                                                | -        |
| -      | `--top-p N`                      | top-p 采样              | 0.9                                                               | -        |
| -      | `--min-p N`                      | min-p 采样              | 0.1                                                               | -        |
| -      | `--xtc-probability N`            | xtc 概率                | 0.0                                                               | -        |
| -      | `--xtc-threshold N`              | xtc 阈值                | 0.1                                                               | -        |
| -      | `--typical N`                    | 局部典型采样            | 1.0                                                               | -        |
| -      | `--repeat-last-n N`              | 重复惩罚最后 n 个 token | 64                                                                | -        |
| -      | `--repeat-penalty N`             | 重复惩罚                | 1.0                                                               | -        |
| -      | `--presence-penalty N`           | 出现惩罚                | 0.0                                                               | -        |
| -      | `--frequency-penalty N`          | 频率惩罚                | 0.0                                                               | -        |
| -      | `--dry-multiplier N`             | DRY 采样乘数            | 0.0                                                               | -        |
| -      | `--dry-base N`                   | DRY 采样基值            | 1.75                                                              | -        |
| -      | `--dry-allowed-length N`         | DRY 允许长度            | 2                                                                 | -        |
| -      | `--dry-penalty-last-n N`         | DRY 惩罚最后 n 个 token | -1                                                                | -        |
| -      | `--dry-sequence-breaker STRING`  | DRY 序列中断符          | 默认                                                              | -        |
| -      | `--dynatemp-range N`             | 动态温度范围            | 0.0                                                               | -        |
| -      | `--dynatemp-exp N`               | 动态温度指数            | 1.0                                                               | -        |
| -      | `--mirostat N`                   | Mirostat 采样（0/1/2）  | 0                                                                 | -        |
| -      | `--mirostat-lr N`                | Mirostat 学习率         | 0.1                                                               | -        |
| -      | `--mirostat-ent N`               | Mirostat 目标熵         | 5.0                                                               | -        |
| `-l`   | `--logit-bias TOKEN_ID(+/-)BIAS` | 修改 token 出现可能性   | -                                                                 | -        |
| -      | `--grammar GRAMMAR`              | BNF 语法约束生成        | ''                                                                | -        |
| -      | `--grammar-file FNAME`           | 读取语法文件            | -                                                                 | -        |
| `-j`   | `--json-schema SCHEMA`           | JSON 模式约束           | -                                                                 | -        |
| `-jf`  | `--json-schema-file FILE`        | JSON 模式文件           | -                                                                 | -        |

---

## 三、示例特定参数

| 短参数    | 长参数                                     | 说明                                | 默认值     | 环境变量                         |
| --------- | ------------------------------------------ | ----------------------------------- | ---------- | -------------------------------- |
| -         | `--no-context-shift`                       | 禁用无限文本生成时的上下文移位      | 禁用       | `LLAMA_ARG_NO_CONTEXT_SHIFT`     |
| `-r`      | `--reverse-prompt PROMPT`                  | 在指定提示处停止生成                | -          | -                                |
| `-sp`     | `--special`                                | 启用特殊 token 输出                 | false      | -                                |
| -         | `--no-warmup`                              | 跳过空运行预热                      | -          | -                                |
| -         | `--spm-infill`                             | 使用后缀/前缀/中间模式填充          | 禁用       | -                                |
| -         | `--pooling {none,mean,cls,last,rank}`      | 嵌入池化类型                        | 模型默认   | `LLAMA_ARG_POOLING`              |
| `-cb`     | `--cont-batching`                          | 启用连续批处理                      | 启用       | `LLAMA_ARG_CONT_BATCHING`        |
| `-nocb`   | `--no-cont-batching`                       | 禁用连续批处理                      | -          | `LLAMA_ARG_NO_CONT_BATCHING`     |
| -         | `--mmproj FILE`                            | 多模态投影器文件路径                | -          | `LLAMA_ARG_MMPROJ`               |
| -         | `--mmproj-url URL`                         | 多模态投影器 URL                    | -          | `LLAMA_ARG_MMPROJ_URL`           |
| -         | `--no-mmproj`                              | 禁用多模态投影器                    | -          | `LLAMA_ARG_NO_MMPROJ`            |
| -         | `--no-mmproj-offload`                      | 不将投影器卸载到 GPU                | -          | `LLAMA_ARG_NO_MMPROJ_OFFLOAD`    |
| `-a`      | `--alias STRING`                           | 设置模型别名                        | -          | `LLAMA_ARG_ALIAS`                |
| -         | `--host HOST`                              | 监听地址                            | 127.0.0.1  | `LLAMA_ARG_HOST`                 |
| -         | `--port PORT`                              | 监听端口                            | 8080       | `LLAMA_ARG_PORT`                 |
| -         | `--path PATH`                              | 静态文件路径                        | ''         | `LLAMA_ARG_STATIC_PATH`          |
| -         | `--api-prefix PREFIX`                      | API 前缀路径                        | ''         | `LLAMA_ARG_API_PREFIX`           |
| -         | `--no-webui`                               | 禁用 Web UI                         | 启用       | `LLAMA_ARG_NO_WEBUI`             |
| -         | `--embedding`                              | 仅支持嵌入用例                      | 禁用       | `LLAMA_ARG_EMBEDDINGS`           |
| -         | `--reranking`                              | 启用重排序端点                      | 禁用       | `LLAMA_ARG_RERANKING`            |
| -         | `--api-key KEY`                            | API 密钥                            | 无         | `LLAMA_API_KEY`                  |
| -         | `--api-key-file FNAME`                     | API 密钥文件                        | 无         | -                                |
| -         | `--ssl-key-file FNAME`                     | SSL 私钥文件                        | -          | `LLAMA_ARG_SSL_KEY_FILE`         |
| -         | `--ssl-cert-file FNAME`                    | SSL 证书文件                        | -          | `LLAMA_ARG_SSL_CERT_FILE`        |
| -         | `--chat-template-kwargs STRING`            | 聊天模板附加参数                    | -          | `LLAMA_CHAT_TEMPLATE_KWARGS`     |
| `-to`     | `--timeout N`                              | 服务器超时（秒）                    | 600        | `LLAMA_ARG_TIMEOUT`              |
| -         | `--threads-http N`                         | HTTP 请求线程数                     | -1         | `LLAMA_ARG_THREADS_HTTP`         |
| -         | `--cache-reuse N`                          | 最小缓存重用块大小                  | 0          | `LLAMA_ARG_CACHE_REUSE`          |
| -         | `--metrics`                                | 启用 Prometheus 指标端点            | 禁用       | `LLAMA_ARG_ENDPOINT_METRICS`     |
| -         | `--slots`                                  | 启用插槽监控端点                    | 禁用       | `LLAMA_ARG_ENDPOINT_SLOTS`       |
| -         | `--props`                                  | 启用 POST /props 更改属性           | 禁用       | `LLAMA_ARG_ENDPOINT_PROPS`       |
| -         | `--no-slots`                               | 禁用插槽监控端点                    | -          | `LLAMA_ARG_NO_ENDPOINT_SLOTS`    |
| -         | `--slot-save-path PATH`                    | 插槽 KV 缓存保存路径                | 禁用       | -                                |
| -         | `--jinja`                                  | 使用 Jinja 模板                     | 禁用       | `LLAMA_ARG_JINJA`                |
| -         | `--reasoning-format FORMAT`                | 控制思想标签格式                    | auto       | `LLAMA_ARG_THINK`                |
| -         | `--reasoning-budget N`                     | 思考预算控制                        | -1         | `LLAMA_ARG_THINK_BUDGET`         |
| -         | `--chat-template JINJA_TEMPLATE`           | 自定义 Jinja 聊天模板               | 从模型获取 | `LLAMA_ARG_CHAT_TEMPLATE`        |
| -         | `--chat-template-file JINJA_TEMPLATE_FILE` | 自定义 Jinja 模板文件               | 从模型获取 | `LLAMA_ARG_CHAT_TEMPLATE_FILE`   |
| -         | `--no-prefill-assistant`                   | 不预填充助手消息                    | 启用       | `LLAMA_ARG_NO_PREFILL_ASSISTANT` |
| `-sps`    | `--slot-prompt-similarity SIMILARITY`      | 插槽提示相似度阈值                  | 0.50       | -                                |
| -         | `--lora-init-without-apply`                | 加载 LoRA 但不应用                  | 禁用       | -                                |
| `--draft` | `--draft-max N`                            | 推测解码草稿 token 数量             | 16         | `LLAMA_ARG_DRAFT_MAX`            |
| -         | `--draft-min N`                            | 最小草稿 token 数量                 | 0          | `LLAMA_ARG_DRAFT_MIN`            |
| -         | `--draft-p-min P`                          | 最小推测解码概率                    | 0.8        | `LLAMA_ARG_DRAFT_P_MIN`          |
| `-cd`     | `--ctx-size-draft N`                       | 草稿模型上下文大小                  | 0          | `LLAMA_ARG_CTX_SIZE_DRAFT`       |
| `-devd`   | `--device-draft <dev1,dev2,..>`            | 草稿模型卸载设备                    | -          | -                                |
| `-ngld`   | `--gpu-layers-draft N`                     | 草稿模型 VRAM 层数                  | -          | `LLAMA_ARG_N_GPU_LAYERS_DRAFT`   |
| `-md`     | `--model-draft FNAME`                      | 草稿模型路径                        | -          | `LLAMA_ARG_MODEL_DRAFT`          |
| -         | `--spec-replace TARGET DRAFT`              | 草稿模型字符串替换                  | -          | -                                |
| `-mv`     | `--model-vocoder FNAME`                    | 声码器模型路径                      | -          | -                                |
| -         | `--tts-use-guide-tokens`                   | TTS 使用引导 token                  | -          | -                                |
| -         | `--embd-bge-small-en-default`              | 使用默认 bge-small-en 模型          | -          | -                                |
| -         | `--embd-e5-small-en-default`               | 使用默认 e5-small 模型              | -          | -                                |
| -         | `--embd-gte-small-default`                 | 使用默认 gte-small 模型             | -          | -                                |
| -         | `--fim-qwen-1.5b-default`                  | 使用默认 Qwen 2.5 Coder 1.5B        | -          | -                                |
| -         | `--fim-qwen-3b-default`                    | 使用默认 Qwen 2.5 Coder 3B          | -          | -                                |
| -         | `--fim-qwen-7b-default`                    | 使用默认 Qwen 2.5 Coder 7B          | -          | -                                |
| -         | `--fim-qwen-7b-spec`                       | 使用 Qwen 2.5 Coder 7B + 0.5B 草稿  | -          | -                                |
| -         | `--fim-qwen-14b-spec`                      | 使用 Qwen 2.5 Coder 14B + 0.5B 草稿 | -          | -                                |
