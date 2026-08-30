# CNN-VIS · 手写数字识别推理终端

在浏览器里直观演示卷积神经网络 (CNN) 识别手写数字的**全过程**。
模型为 PyTorch 训练的 `../mnist_cnn_best.pth`（测试集准确率 99.31%），
权重已导出为本地数据文件，由纯 JavaScript 前向传播引擎执行——
**零外部依赖、可完全离线运行**（双击 index.html 即可，或用任意静态服务器）。

## 运行

```bash
cd cnndemo
python3 -m http.server 8610
# 浏览器打开 http://127.0.0.1:8610
```

或者直接双击 `index.html`（权重以 base64 内嵌，无 fetch/CORS 问题）。

## 功能

| 区域 | 内容 |
|---|---|
| 手写输入区 | 28×28 逻辑网格手绘板，MNIST 规范自动居中（可关），回车即推理 |
| 3D 总览 | 自研正交投影渲染器：输入→32 张特征图→池化→64 张→展平→FC→概率条，可拖拽旋转/缩放/点击联动 |
| STG00 | 预处理：(x−0.1307)/0.3081 标准化 |
| STG01 | 卷积①动画：真实训练出的卷积核逐格滑动，实时显示 PATCH×KERNEL=乘积、求和+bias、ReLU |
| STG02/04 | 最大池化前后对比（红线=2×2 分组） |
| STG03 | 卷积②（3×3×32 立体核）64 张特征图 |
| STG05 | 展平色带（通道优先顺序），高亮所选通道 |
| STG06/07 | FC1 128 维激活柱状图；Dropout 训练期模拟（推理时旁路说明） |
| STG08 | Softmax 概率分布 + 温度 T 滑杆演示 |

## 目录结构

```
cnndemo/
├── index.html            # 页面结构
├── assets/
│   ├── style.css         # 工业软件风格 (HMI)
│   ├── engine.js         # 纯 JS 推理引擎 (conv/pool/fc/softmax)
│   ├── viz3d.js          # 零依赖正交投影 3D 渲染器
│   ├── app.js            # 交互与各阶段可视化
│   ├── model_data.js     # 自动生成: base64 权重 (勿手改)
│   └── samples.js        # 自动生成: MNIST 示例数字 0-9
├── tools/
│   ├── export_weights.py # pth -> model_data.js
│   └── gen_golden.py     # 黄金对拍向量 + 示例图片
└── test/
    ├── parity.mjs        # node 对拍测试: JS 引擎 vs PyTorch
    └── golden.json       # PyTorch 各层基准输出
```

## 正确性保证

`node test/parity.mjs` 将 MNIST 测试集第 0 张图分别送入
PyTorch 模型与本 JS 引擎，逐层比对：

```
✓ conv1   最大绝对误差 = 1.3e-6      ✓ fc1     2.2e-6
✓ pool1             1.2e-6           ✓ logits  2.1e-6
✓ conv2             2.5e-6           ✓ probs   4.9e-7
== 全部通过: JS 引擎与 PyTorch 数值一致 ==  (预测均为 7)
```

## 再生成数据文件

```bash
python3 tools/export_weights.py   # 重新导出 ../mnist_cnn_best.pth
python3 tools/gen_golden.py       # 重新生成示例与黄金向量
node test/parity.mjs              # 复验一致性
```

## 为什么不用 TensorFlow.js？

演示需要暴露**每一层的中间激活量**做可视化，且要求离线可用、与训练权重
严格一致。TF.js 运行时无法直接拆出中间结果且需联网 CDN，因此改为
等价的纯 JS 实现 + 数值对拍验证（见上）。若需迁移 TF.js，
`tools/export_weights.py` 的分段布局可直接映射到 `tf.Variable` 权重。
