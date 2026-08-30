# Transformer 推理原理分析仪（minimind-3 · 浏览器端）

一个**完全在浏览器本地运行**的 Transformer 大语言模型推理可视化系统：加载真实的
`minimind-3`（Qwen3ForCausalLM，≈63.9M 参数）权重，从分词、嵌入、RoPE、多头/GQA
注意力、SwiGLU 前馈到 logits 采样，**每一步计算都可观察、可干预、可验证**。

## 快速开始

**方式 A —— 直接双击 index.html（零交互，推荐）：**
权重已由 `tools/gen_weights_js.py` 生成为普通 JS 代码
（`js/weights/w_00.js … w_14.js`，base64 分块 + config/tokenizer 对象），
页面检测到内嵌权重后直接使用 —— 无需服务器、无需选择任何文件。

**方式 B —— HTTP 服务：**

```bash
./start.sh            # 等价于 python3 -m http.server 8000
# 打开 http://localhost:8000/
```

加载策略为三级自动回退：**内嵌 JS 权重 → HTTP fetch minimind-3/ → file:// 文件夹授权**。
若删除 js/weights/ 目录，则自动退回后两种方式（行为与旧版一致）。

> 说明：
> - file:// 下 ES Module 会被 CORS 拦截，页面在 file:// 时自动改用预打包的
>   `js/app.bundle.js`（经典脚本）；HTTP 下则直接运行 js/ 源码。
> - 修改模型权重或 js/ 源码后，双击模式需要重新生成：
>   `python3 tools/gen_weights_js.py`
>   `npx esbuild js/main.js --bundle --format=iife --outfile=js/app.bundle.js`
> - 内嵌模式下页面内存峰值约 +300MB（base64 文本 163MB → 二进制 122MB → FP32 张量 255MB 依次产生并回收）。
> - 推荐桌面 Chrome/Edge（WebGL 加速）；无 GPU 时自动回退 CPU/纯 JS 内核。

## 功能地图

| 区域 | 功能 |
|---|---|
| MODULE01–03（左栏） | 输入模式（Chat 模板 / Base 续写）、单步 +1 Token、自动播放、温度 / Top-P / Top-K / 重复惩罚 / 间隔、预设用例库 |
| MODULE04 | Token 序列条：左键选 Query、右键/Shift 选 Key |
| MODULE05 | Transformer 前向流水线图（可点击跳转对应面板） |
| 页签① | **成对注意力演算台**：任选层/头/两个 token → 96 维逐维点积柱状图 → 除以 √96 → 整行 softmax 分布 → 按 w·v 加权求和堆叠，全链路真实数字 |
| 页签② | **注意力矩阵热力图**：每层每头的下三角权重矩阵，悬停读数、单击联动的演算台，支持 8 头平铺 |
| 页签③ | **QKV 散点观测**：任意 token 的 Q/K/V/嵌入/残差流向量投影到任意两维，支持 PCA 主成分（可直观看到 RoPE 旋转轨迹） |
| 页签④ | **FFN/SwiGLU 视图**：gate/up/silu⊙up 三组 2432 维激活谱实时重算 |
| MODULE06–07（右栏） | 下一 Token 候选表（raw 概率 / 截断后概率 / 累计 / 熵 / PPL / 全词表分布曲线）、生成流、运行日志 |
| 顶栏「交叉验证」 | 当前 TF 后端 vs 纯 JS 内核 logits 一致性自检 |

## 数值可信度

工程内置与 HuggingFace `transformers` 的自动化对照（`npm run parity`，需本地
python + transformers）：

- 分词：62/62 对抗用例逐一 token 级一致
- logits：`max|Δ| ≈ 4e-5`（FP16 权重噪声量级），Top-5 一致
- 注意力权重：`max|Δ| ≈ 1e-6`，行和恒为 1
- 逐层残差流 / 最终 RMSNorm：全部 ≤3e-3
- KV Cache 分步解码 vs 整段前向：logits **Δ=0**

## 目录结构

```
index.html          主应用
index.html          主应用（内嵌《Transformer 原理详解》长文，顶栏按钮打开）
css/style.css       工业控制台风格样式
js/st.js            safetensors 解析（FP16/BF16 → FP32）
js/tok.js           字节级 BPE 分词器（与 HF 完全对齐）
js/mat.js           计算后端：TensorFlow.js(WebGL) + 纯 JS 双路径
js/model.js         Qwen3 前向（RoPE/逐头Norm/GQA/KV Cache/捕获中间量）
js/engine.js        会话/采样策略/自动播放
js/viz.js, ui.js    Canvas 可视化与交互接线
js/vendor/tf.min.js TensorFlow.js 4.22（本地内置，离线可用）
minimind-3/         模型权重与配置（model.safetensors 等）
tools/              parity 数值对照、CDP 端到端测试、参考数据生成
```

## 说明

- 上下文限制默认 **512**，可调至 2048（KV Cache / RoPE 按 2048 预分配）；EOS `<|im_end|>` 自动停止。
- 可视化捕获默认开启，配套**滑动窗口**（默认 128 token，超出丢弃更早捕获）与**满上下文内存预估警示**（>150MB 黄色 / >400MB 红色）；全部在 Module02「资源」区调整，原理见内嵌文档 §7.4。
- Chat 模式使用 Qwen 官方模板（含空 `<think>` 块，与 HF `apply_chat_template` 一致）。
- 纯文本续写模式下小模型易复读 —— 这正是演示温度/Top-P/重复惩罚效果的素材。
