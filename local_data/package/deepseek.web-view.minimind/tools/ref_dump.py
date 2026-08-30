#!/usr/bin/env python3
"""Dump reference tensors from HuggingFace transformers for JS parity checks."""
import json, os, sys
import numpy as np
import torch

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
MODEL_DIR = os.path.join(ROOT, "minimind-3")
OUT = os.path.join(ROOT, "tools", "ref_sample.json")

PROMPTS = [
    "北京是中国的",
    "The capital city of France is",
]

def main():
    from transformers import AutoModelForCausalLM, AutoTokenizer
    tok = AutoTokenizer.from_pretrained(MODEL_DIR)
    try:
        model = AutoModelForCausalLM.from_pretrained(MODEL_DIR, dtype=torch.float32,
                                                     attn_implementation="eager")
    except TypeError:
        model = AutoModelForCausalLM.from_pretrained(MODEL_DIR, torch_dtype=torch.float32,
                                                     attn_implementation="eager")
    model.eval()

    out = {"model_type": model.config.model_type, "prompts": []}
    with torch.no_grad():
        for prompt in PROMPTS:
            ids = tok(prompt, add_special_tokens=False)["input_ids"]
            # detokenize each prefix position token repr for UI cross-checks
            tok_repr = [tok.decode([i]) for i in ids]
            res = model(torch.tensor([ids]), output_hidden_states=True, output_attentions=True)
            logits = res.logits[0].float().numpy()           # [T, V]
            hiddens = [h[0].float().numpy() for h in res.hidden_states]  # L+1 x [T, d]
            # attentions: tuple(L) of [B, H, T, T]
            atts = [a[0].float().numpy() for a in res.attentions]
            probs_last = torch.softmax(res.logits[0, -1], dim=-1)
            topk = torch.topk(probs_last, 12)
            entry = {
                "prompt": prompt,
                "ids": ids,
                "tok_repr": tok_repr,
                "logits_last": np.round(logits[-1], 5).tolist(),
                "logits_pos0": np.round(logits[1] if len(ids) > 1 else logits[0], 5).tolist(),
                "topk_ids": topk.indices.tolist(),
                "topk_probs": np.round(topk.values.numpy(), 6).tolist(),
                # snapshot of residual stream after each block for a middle token
                "hidden_snapshots_tok2": [np.round(h[len(ids)//2], 5).tolist() for h in hiddens],
                # attention weights row for query=len//2+1, layer 2, head 5  (Qwen3 min layers=8 ok)
                "attn_row_L2H5_qmid": {
                    "layer": 2, "head": 5,
                    "query": len(ids)//2 + 1,
                    "weights": np.round(atts[2][5, len(ids)//2 + 1], 7).tolist(),
                },
            }
            out["prompts"].append(entry)

    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False)
    print("wrote", OUT)
    for p in out["prompts"]:
        print(p["prompt"], "ids=", p["ids"])
        print("  tokens:", p["tok_repr"])
        print("  top:", [(i, round(pr,4)) for i, pr in zip(p["topk_ids"][:5], p["topk_probs"][:5])])

if __name__ == "__main__":
    main()
