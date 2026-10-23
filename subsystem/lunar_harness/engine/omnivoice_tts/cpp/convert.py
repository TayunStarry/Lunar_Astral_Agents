#!/usr/bin/env python3
# convert.py: safetensors to GGUF for OmniVoice (LM + audio tokenizer).
# Reads from checkpoints/OmniVoice/, writes 2 byte-perfect F32 GGUFs to models/.
# The source checkpoint is 100% F32 (k2-fsa/OmniVoice on Hugging Face),
# so this converter never downcasts : every tensor is written in its native
# source dtype and quantize.sh derives BF16 / Q8_0 from these F32 GGUFs.
#
# omnivoice-base-F32.gguf       Qwen3 0.6B + audio_embeddings + audio_heads + tokenizer
# omnivoice-tokenizer-F32.gguf  HuBERT + DAC encoder/decoder + RVQ + fc/fc2

import json
import os
import struct
import sys

import numpy as np
import gguf

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
CHECKPOINT_DIR = os.path.join(SCRIPT_DIR, "checkpoints", "OmniVoice")
OUTPUT_DIR = os.path.join(SCRIPT_DIR, "models")

def log(tag, msg):
    print("[%s] %s" % (tag, msg), file=sys.stderr, flush=True)

# safetensors header reader
def read_sf_header(path):
    with open(path, "rb") as f:
        n = struct.unpack("<Q", f.read(8))[0]
        meta = json.loads(f.read(n))
    meta.pop("__metadata__", None)
    return meta, 8 + n

# read a tensor from safetensors as a numpy view of the right dtype
def read_sf_tensor(sf_path, hdr_size, info):
    off0, off1 = info["data_offsets"]
    nbytes = off1 - off0
    with open(sf_path, "rb") as f:
        f.seek(hdr_size + off0)
        raw = f.read(nbytes)
    dtype_str = info["dtype"]
    shape = info["shape"]
    if dtype_str == "F32":
        return np.frombuffer(raw, dtype=np.float32).reshape(shape).copy()
    if dtype_str == "BF16":
        u16 = np.frombuffer(raw, dtype=np.uint16).reshape(shape)
        return (u16.astype(np.uint32) << 16).view(np.float32).copy()
    if dtype_str == "F16":
        return np.frombuffer(raw, dtype=np.float16).reshape(shape).astype(np.float32).copy()
    raise RuntimeError("unsupported dtype %s" % dtype_str)

# write a tensor to GGUF preserving the source dtype : F32 stays F32, BF16 stays
# BF16, F16 stays F16. No cast, no precision invented or destroyed. The OmniVoice
# checkpoint is integrally F32 in the reference release ; the BF16 / F16 branches
# stay as defensive code in case a future release switches dtype upstream.
def add_tensor_passthrough(w, name, sf_path, hdr_size, info):
    dtype_str = info["dtype"]
    shape = info["shape"]
    off0, off1 = info["data_offsets"]
    nbytes = off1 - off0
    with open(sf_path, "rb") as f:
        f.seek(hdr_size + off0)
        raw = f.read(nbytes)
    if dtype_str == "F32":
        arr = np.frombuffer(raw, dtype=np.float32).reshape(shape)
        w.add_tensor(name, arr)
        return nbytes
    if dtype_str == "BF16":
        arr = np.frombuffer(raw, dtype=np.uint16).reshape(shape)
        w.add_tensor(name, arr, raw_dtype=gguf.GGMLQuantizationType.BF16)
        return nbytes
    if dtype_str == "F16":
        arr = np.frombuffer(raw, dtype=np.float16).reshape(shape)
        w.add_tensor(name, arr)
        return nbytes
    raise RuntimeError("unsupported dtype %s for %s" % (dtype_str, name))

# BPE tokenizer: extract vocab + merges + added_tokens from tokenizer.json
def add_bpe_tokenizer(w, tokenizer_json_path, tag):
    with open(tokenizer_json_path, "r", encoding="utf-8") as f:
        tj = json.load(f)

    model = tj.get("model", {})
    if model.get("type") != "BPE":
        raise RuntimeError("expected BPE tokenizer, got %s" % model.get("type"))

    vocab_dict = model.get("vocab", {})
    added_tokens = tj.get("added_tokens", [])

    max_id = max(
        max(vocab_dict.values(), default=-1),
        max((a["id"] for a in added_tokens), default=-1),
    )
    tokens = [""] * (max_id + 1)
    for tok_str, tok_id in vocab_dict.items():
        if 0 <= tok_id <= max_id:
            tokens[tok_id] = tok_str
    for a in added_tokens:
        tokens[a["id"]] = a["content"]

    merges = []
    for m in model.get("merges", []):
        if isinstance(m, list):
            merges.append(" ".join(m))
        else:
            merges.append(m)

    w.add_tokenizer_model("gpt2")
    w.add_token_list(tokens)
    w.add_token_merges(merges)

    log(tag, "tokenizer: %d vocab, %d merges, %d added" %
        (len(tokens), len(merges), len(added_tokens)))

# Tensors to skip in model.safetensors (recomputable at runtime).
def is_skip_base_tensor(name):
    # codebook_layer_offsets = arange(num_audio_codebook) * audio_vocab_size,
    # trivially recomputed C++ side from the audio metadata.
    if name == "codebook_layer_offsets":
        return True
    return False

# omnivoice-base: LLM + audio_embeddings + audio_heads + tokenizer
def convert_base(ckpt_dir, output_path):
    tag = "BASE"
    cfg_path = os.path.join(ckpt_dir, "config.json")
    sf_path = os.path.join(ckpt_dir, "model.safetensors")
    tok_path = os.path.join(ckpt_dir, "tokenizer.json")

    with open(cfg_path, "r", encoding="utf-8") as f:
        cfg = json.load(f)
    llm_cfg = cfg["llm_config"]

    log(tag, "writing %s" % os.path.basename(output_path))
    w = gguf.GGUFWriter(output_path, "omnivoice-lm", use_temp_file=True)
    w.add_name("OmniVoice")

    # standard LLM metadata
    w.add_block_count(llm_cfg["num_hidden_layers"])
    w.add_embedding_length(llm_cfg["hidden_size"])
    w.add_feed_forward_length(llm_cfg["intermediate_size"])
    w.add_head_count(llm_cfg["num_attention_heads"])
    w.add_head_count_kv(llm_cfg["num_key_value_heads"])
    w.add_key_length(llm_cfg["head_dim"])
    w.add_value_length(llm_cfg["head_dim"])
    w.add_vocab_size(llm_cfg["vocab_size"])
    w.add_context_length(llm_cfg["max_position_embeddings"])
    w.add_layer_norm_rms_eps(llm_cfg["rms_norm_eps"])
    w.add_rope_freq_base(float(llm_cfg["rope_parameters"]["rope_theta"]))
    w.add_bool("omnivoice.tie_word_embeddings", bool(llm_cfg.get("tie_word_embeddings", False)))

    # audio IO (LLM-side custom)
    w.add_uint32("omnivoice.num_audio_codebook", cfg["num_audio_codebook"])
    w.add_uint32("omnivoice.audio_vocab_size", cfg["audio_vocab_size"])
    w.add_uint32("omnivoice.audio_mask_id", cfg["audio_mask_id"])
    w.add_array("omnivoice.audio_codebook_weights",
                [int(x) for x in cfg["audio_codebook_weights"]])

    # special token IDs from tokenizer.json (denoise / lang / instruct / text bracketing)
    with open(tok_path, "r", encoding="utf-8") as f:
        tj = json.load(f)
    special_map = {a["content"]: a["id"] for a in tj.get("added_tokens", [])}
    for tag_name in [
        "<|denoise|>",
        "<|lang_start|>", "<|lang_end|>",
        "<|instruct_start|>", "<|instruct_end|>",
        "<|text_start|>", "<|text_end|>",
    ]:
        if tag_name in special_map:
            key = "omnivoice.special." + tag_name.strip("<|>").replace("|", "_")
            w.add_uint32(key, special_map[tag_name])

    # tokenizer
    add_bpe_tokenizer(w, tok_path, tag)

    # tensors
    meta, hdr_size = read_sf_header(sf_path)
    n_tensors, n_bytes = 0, 0
    n_skip = 0
    for name in sorted(meta.keys()):
        if is_skip_base_tensor(name):
            n_skip += 1
            continue
        nb = add_tensor_passthrough(w, name, sf_path, hdr_size, meta[name])
        n_tensors += 1
        n_bytes += nb
    log(tag, "total: %d tensors, %.1f MB (%d skipped)" %
        (n_tensors, n_bytes / (1 << 20), n_skip))

    w.write_header_to_file()
    w.write_kv_data_to_file()
    w.write_tensors_to_file(progress=True)
    w.close()

    out_mb = os.path.getsize(output_path) / (1 << 20)
    log(tag, "wrote %.0f MB -> %s" % (out_mb, output_path))

# Tensors to skip in audio_tokenizer (training-only, not used at inference)
def is_skip_tokenizer_tensor(name):
    # decoder_semantic + fc1: auxiliary HuBERT-feature reconstruction loss
    if name.startswith("decoder_semantic.") or name.startswith("fc1."):
        return True
    # RVQ codebook EMA buffers (cluster_size, embed_avg, inited): training only
    if "quantizer.quantizers." in name:
        if name.endswith(".cluster_size") or name.endswith(".embed_avg") or name.endswith(".inited"):
            return True
    # parametrizations.weight.original{0,1}: replaced by folded weight_norm below
    if "parametrizations.weight.original" in name:
        return True
    return False

# Fold HuBERT pos_conv weight_norm: weight = v * g / ||v||_{dim=(0,1)}
# Convention: torch.nn.utils.weight_norm(conv, dim=2) for grouped conv1d.
# Verified bit-identical to torch._weight_norm(v, g, dim=2) up to FP32 noise.
def fold_pos_conv_weight_norm(meta, hdr_size, sf_path):
    g_name = "semantic_model.encoder.pos_conv_embed.conv.parametrizations.weight.original0"
    v_name = "semantic_model.encoder.pos_conv_embed.conv.parametrizations.weight.original1"
    target = "semantic_model.encoder.pos_conv_embed.conv.weight"

    g = read_sf_tensor(sf_path, hdr_size, meta[g_name])  # (1, 1, 128)
    v = read_sf_tensor(sf_path, hdr_size, meta[v_name])  # (768, 48, 128)
    norm = np.sqrt(np.sum(v * v, axis=(0, 1), keepdims=True))
    weight = v * g / norm
    return target, weight.astype(np.float32, copy=False)

# omnivoice-tokenizer: HuBERT + DAC + RVQ + fc/fc2
def convert_tokenizer(ckpt_dir, output_path):
    tag = "TOK"
    audio_dir = os.path.join(ckpt_dir, "audio_tokenizer")
    cfg_path = os.path.join(audio_dir, "config.json")
    sf_path = os.path.join(audio_dir, "model.safetensors")

    with open(cfg_path, "r", encoding="utf-8") as f:
        cfg = json.load(f)

    log(tag, "writing %s" % os.path.basename(output_path))
    w = gguf.GGUFWriter(output_path, "omnivoice-tokenizer", use_temp_file=True)
    w.add_name("OmniVoice-tokenizer")

    # global audio config
    w.add_uint32("omnivoice.sample_rate", cfg["sample_rate"])
    w.add_uint32("omnivoice.semantic_sample_rate", cfg["semantic_sample_rate"])
    w.add_uint32("omnivoice.downsample_factor", cfg["downsample_factor"])
    w.add_uint32("omnivoice.codebook_size", cfg["codebook_size"])
    w.add_uint32("omnivoice.codebook_dim", cfg["codebook_dim"])

    # acoustic (DAC) config
    ac = cfg["acoustic_model_config"]
    w.add_uint32("omnivoice.acoustic.encoder_hidden_size", ac["encoder_hidden_size"])
    w.add_uint32("omnivoice.acoustic.decoder_hidden_size", ac["decoder_hidden_size"])
    w.add_uint32("omnivoice.acoustic.hidden_size", ac["hidden_size"])
    w.add_uint32("omnivoice.acoustic.n_codebooks", ac["n_codebooks"])
    w.add_uint32("omnivoice.acoustic.hop_length", ac["hop_length"])
    w.add_array("omnivoice.acoustic.upsampling_ratios",
                [int(x) for x in ac["upsampling_ratios"]])
    w.add_array("omnivoice.acoustic.downsampling_ratios",
                [int(x) for x in ac["downsampling_ratios"]])

    # semantic (HuBERT) config
    sm = cfg["semantic_model_config"]
    w.add_uint32("omnivoice.semantic.hidden_size", sm["hidden_size"])
    w.add_uint32("omnivoice.semantic.intermediate_size", sm["intermediate_size"])
    w.add_uint32("omnivoice.semantic.num_attention_heads", sm["num_attention_heads"])
    w.add_uint32("omnivoice.semantic.num_hidden_layers", sm["num_hidden_layers"])
    w.add_uint32("omnivoice.semantic.num_feat_extract_layers", sm["num_feat_extract_layers"])
    w.add_array("omnivoice.semantic.conv_dim", [int(x) for x in sm["conv_dim"]])
    w.add_array("omnivoice.semantic.conv_kernel", [int(x) for x in sm["conv_kernel"]])
    w.add_array("omnivoice.semantic.conv_stride", [int(x) for x in sm["conv_stride"]])
    w.add_uint32("omnivoice.semantic.num_conv_pos_embeddings", sm["num_conv_pos_embeddings"])
    w.add_uint32("omnivoice.semantic.num_conv_pos_embedding_groups", sm["num_conv_pos_embedding_groups"])
    w.add_float32("omnivoice.semantic.layer_norm_eps", float(sm["layer_norm_eps"]))

    # tensors
    meta, hdr_size = read_sf_header(sf_path)

    # fold pos_conv weight_norm before iterating, so the folded weight replaces
    # both parametrizations entries in the output GGUF
    folded_name, folded_arr = fold_pos_conv_weight_norm(meta, hdr_size, sf_path)

    n_tensors, n_bytes = 0, 0
    n_skip = 0
    for name in sorted(meta.keys()):
        if is_skip_tokenizer_tensor(name):
            n_skip += 1
            continue
        nb = add_tensor_passthrough(w, name, sf_path, hdr_size, meta[name])
        n_tensors += 1
        n_bytes += nb

    # folded pos_conv weight is computed F32 from F32 source params, write F32
    w.add_tensor(folded_name, folded_arr)
    n_tensors += 1
    n_bytes += folded_arr.nbytes

    log(tag, "total: %d tensors, %.1f MB (%d skipped: training-only)" %
        (n_tensors, n_bytes / (1 << 20), n_skip))

    w.write_header_to_file()
    w.write_kv_data_to_file()
    w.write_tensors_to_file(progress=True)
    w.close()

    out_mb = os.path.getsize(output_path) / (1 << 20)
    log(tag, "wrote %.0f MB -> %s" % (out_mb, output_path))

def main():
    if not os.path.isdir(CHECKPOINT_DIR):
        log("GGUF", "checkpoints/OmniVoice not found, run checkpoints.sh first")
        sys.exit(1)

    os.makedirs(OUTPUT_DIR, exist_ok=True)

    base_path = os.path.join(OUTPUT_DIR, "omnivoice-base-F32.gguf")
    tok_path = os.path.join(OUTPUT_DIR, "omnivoice-tokenizer-F32.gguf")

    if os.path.exists(base_path):
        log("BASE", "skip: %s exists" % os.path.basename(base_path))
    else:
        convert_base(CHECKPOINT_DIR, base_path)

    if os.path.exists(tok_path):
        log("TOK", "skip: %s exists" % os.path.basename(tok_path))
    else:
        convert_tokenizer(CHECKPOINT_DIR, tok_path)

    log("GGUF", "done -> %s" % OUTPUT_DIR)

if __name__ == "__main__":
    main()
