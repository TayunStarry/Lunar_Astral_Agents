#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把 PyTorch 训练好的权重导出为浏览器可用的 JS 数据文件 (base64 Float32)。

用法:  cd cnndemo/tools && python3 export_weights.py
输出:  ../assets/model_data.js   -> window.CNN_WEIGHTS_B64 = "<base64>"
"""
import base64
import json
import pathlib
import sys

import torch

sys.path.append(str(pathlib.Path(__file__).resolve().parents[2]))
from train_mnist import Net  # noqa: E402

HERE = pathlib.Path(__file__).resolve().parent
ASSETS = HERE.parent / "assets"

# 与 engine.js 中 LAYOUT 严格一致
LAYOUT = [
    ("conv1.weight", [32, 1, 3, 3]),
    ("conv1.bias",   [32]),
    ("conv2.weight", [64, 32, 3, 3]),
    ("conv2.bias",   [64]),
    ("fc1.weight",   [128, 3136]),
    ("fc1.bias",     [128]),
    ("fc2.weight",   [10, 128]),
    ("fc2.bias",     [10]),
]


def main():
    sd = torch.load(HERE.parents[1] / "mnist_cnn_best.pth", map_location="cpu")
    model = Net()
    model.load_state_dict(sd)
    model.eval()

    blobs, total = [], 0
    for name, shape in LAYOUT:
        t = sd[name].detach().cpu().contiguous().float()
        assert list(t.shape) == shape, f"{name}: {list(t.shape)} != {shape}"
        blobs.append(t.numpy().tobytes())
        n = t.numel()
        print(f"  {name:14s} {str(shape):18s} {n:>8,} 参数")
        total += n
    raw = b"".join(blobs)
    b64 = base64.b64encode(raw).decode()
    js = (
        "/* 自动生成: tools/export_weights.py <- mnist_cnn_best.pth, 勿手改 */\n"
        f"window.CNN_WEIGHTS_META = {json.dumps({'params': total, 'source': 'mnist_cnn_best.pth'})};\n"
        f'window.CNN_WEIGHTS_B64 = "{b64}";\n'
    )
    ASSETS.mkdir(exist_ok=True)
    out = ASSETS / "model_data.js"
    out.write_text(js, encoding="utf-8")
    print(f"共 {total:,} 参数, {len(raw):,} 字节 -> {out} ({out.stat().st_size/1e6:.2f} MB)")


if __name__ == "__main__":
    main()
