#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""生成黄金测试向量 + 示例数字图片。

1. test/golden.json : 测试集第 0 张图经过网络每一层的输出 (JS 引擎对拍用)
2. assets/samples.js: 数字 0-9 各一张测试集图片 (网页"示例"按钮用)

用法:  cd cnndemo/tools && python3 gen_golden.py
"""
import base64
import json
import pathlib
import sys

import torch
import torch.nn.functional as F
from torchvision import datasets, transforms

sys.path.append(str(pathlib.Path(__file__).resolve().parents[2]))
from train_mnist import Net  # noqa: E402

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parent
MEAN, STD = 0.1307, 0.3081


def forward_trace(model, x01):
    """x01: [1,1,28,28] 原始 [0,1] 像素; 返回各阶段输出(均为 post-ReLU/池化后的可见量)."""
    x = (x01 - MEAN) / STD
    t = {}
    t["input_norm"] = x
    a = F.relu(model.conv1(x));  t["conv1"] = a
    a = model.pool(a);           t["pool1"] = a
    a = F.relu(model.conv2(a));  t["conv2"] = a
    a = model.pool(a);           t["pool2"] = a
    flat = torch.flatten(a, 1);  t["flat"] = flat
    h = F.relu(model.fc1(flat)); t["fc1"] = h
    logits = model.fc2(h);       t["logits"] = logits
    t["probs"] = F.softmax(logits, dim=1)
    return {k: v.squeeze(0).tolist() for k, v in t.items()}


def r6(x):
    if isinstance(x, dict):
        return {k: r6(v) for k, v in x.items()}
    if isinstance(x, list):
        return [r6(v) for v in x]
    return round(x, 6)


def main():
    sd = torch.load(HERE.parents[1] / "mnist_cnn_best.pth", map_location="cpu")
    model = Net(); model.load_state_dict(sd); model.eval()

    ds = datasets.MNIST(str(ROOT.parent / "data"), train=False,
                        transform=transforms.ToTensor())

    # ---- 黄金向量: 测试集第 0 张 ----
    x0, y0 = ds[0]
    golden = {"label": int(y0), **forward_trace(model, x0.unsqueeze(0))}
    gp = HERE.parent / "test" / "golden.json"
    gp.write_text(json.dumps(r6(golden)), encoding="utf-8")
    print(f"{gp} -> {gp.stat().st_size/1e3:.0f} KB (标签={y0})")

    # ---- 示例: 0-9 各第一张 ----
    samples = {}
    for idx in range(len(ds)):
        _, y = ds[idx]
        y = int(y)
        if y not in samples:
            img = (ds[idx][0][0] * 255).round().byte().tolist()
            samples[y] = img
        if len(samples) == 10:
            break
    from itertools import chain
    flat = chain.from_iterable(chain.from_iterable(samples[d] for d in range(10)))
    b64 = base64.b64encode(bytes(flat)).decode()
    js = ("/* 自动生成: tools/gen_golden.py <- MNIST testset, 勿手改 */\n"
          "window.CNN_SAMPLES_B64 = \"%s\";\n" % b64)
    sp = ROOT / "assets" / "samples.js"
    sp.write_text(js, encoding="utf-8")
    print(f"samples.js -> {sp.stat().st_size/1e3:.0f} KB (数字0-9各1张,8000B)")


if __name__ == "__main__":
    main()
