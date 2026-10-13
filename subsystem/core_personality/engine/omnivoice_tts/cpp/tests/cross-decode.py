#!/usr/bin/env python3
"""Cross decode probe : feed CPP mg-tokens to the Python audio decoder.

If decoder_pt(tokens_cpp) matches tts-cpp.wav, the C++ decoder is fine and
the divergence lives entirely in the LM/MaskGIT path. If it does not match,
the C++ audio decoder has a bug too.

We also feed PT tokens to the same Python decoder as a sanity reference,
which must reproduce tts-python.wav (modulo whatever post we still have).
"""

import os
import struct

# Strict F32 matmul on both sides. NVIDIA_TF32_OVERRIDE=0 forces full FP32
# mantissa in cuBLAS for both PyTorch and any C++ child via inheritance.
# Must be set BEFORE torch imports so the cuBLAS handle reads it on init.
os.environ["NVIDIA_TF32_OVERRIDE"] = "0"

import numpy as np
import soundfile as sf
import torch

# Belt and suspenders : disable PyTorch's own TF32 toggles too. Some code
# paths bypass NVIDIA_TF32_OVERRIDE through cudnn or torch internal flags.
torch.backends.cuda.matmul.allow_tf32 = False
torch.backends.cudnn.allow_tf32       = False

from omnivoice import OmniVoice
from omnivoice.utils.common import fix_random_seed

CKPT = "../checkpoints/OmniVoice"

def load_dump(path):
    raw = np.fromfile(path, dtype=np.uint8)
    nd = int(np.frombuffer(raw[0:4], dtype=np.int32)[0])
    sh = tuple(int(x) for x in np.frombuffer(raw[4:4 + 4 * nd], dtype=np.int32))
    body = np.frombuffer(raw[4 + 4 * nd:], dtype=np.float32)
    return body.reshape(sh)

def cos(a, b):
    a = a.astype(np.float64).ravel()
    b = b.astype(np.float64).ravel()
    n = min(len(a), len(b))
    a, b = a[:n], b[:n]
    d = float(np.linalg.norm(a) * np.linalg.norm(b))
    return float(np.dot(a, b) / d) if d > 1e-10 else 0.0

def stft_cos(a, b, win=2048, hop=512):
    # STFT magnitude cosine. Drops phase, so a constant time shift between
    # the two waveforms does not collapse the score. Mirrors the helper in
    # debug-{tts,clone}-cossim.py.
    a = a.astype(np.float64).ravel()
    b = b.astype(np.float64).ravel()
    n = min(len(a), len(b))
    a, b = a[:n], b[:n]
    window = np.hanning(win)
    frames = (n - win) // hop + 1
    if frames <= 0:
        return 0.0
    sa = np.zeros((frames, win // 2 + 1))
    sb = np.zeros((frames, win // 2 + 1))
    for i in range(frames):
        s = i * hop
        sa[i] = np.abs(np.fft.rfft(a[s:s + win] * window))
        sb[i] = np.abs(np.fft.rfft(b[s:s + win] * window))
    return cos(sa.ravel(), sb.ravel())

def decode_tokens(model, tokens_kt):
    """tokens_kt is [K, T] int. Returns float32 mono numpy array of samples."""
    device = next(model.parameters()).device
    t = torch.from_numpy(tokens_kt.astype(np.int64)).to(device)
    t = t.unsqueeze(0)
    with torch.no_grad():
        out = model.audio_tokenizer.decode(t)
    wav = getattr(out, "audio_values", out)
    if isinstance(wav, torch.Tensor):
        wav = wav.detach().to(torch.float32).cpu().numpy()
    if wav.ndim == 3:
        wav = wav[0, 0]
    elif wav.ndim == 2:
        wav = wav[0]
    return np.asarray(wav, dtype=np.float32)

def main():
    fix_random_seed(42)
    device = "cuda" if torch.cuda.is_available() else "cpu"
    model = OmniVoice.from_pretrained(
        CKPT,
        torch_dtype=torch.float32,
        attn_implementation="eager",
    ).to(device).eval()

    tok_cpp = load_dump("cpp/mg-tokens.bin").astype(np.int32)
    tok_pt  = load_dump("python/mg-tokens.bin").astype(np.int32)
    print(f"[Input] Tokens ggml: {tok_cpp.shape} python: {tok_pt.shape}")

    audio_pt_from_cpp = decode_tokens(model, tok_cpp)
    audio_pt_from_pt  = decode_tokens(model, tok_pt)

    sf.write("python/decode-of-cpp-tokens.wav", audio_pt_from_cpp, 24000, subtype="FLOAT")
    sf.write("python/decode-of-pt-tokens.wav",  audio_pt_from_pt,  24000, subtype="FLOAT")

    cpp_wav, _ = sf.read("cpp/tts-cpp.wav")
    pt_wav,  _ = sf.read("python/tts-python.wav")
    if cpp_wav.ndim > 1:
        cpp_wav = cpp_wav[:, 0]
    if pt_wav.ndim > 1:
        pt_wav = pt_wav[:, 0]
    cpp_wav = cpp_wav.astype(np.float32)
    pt_wav  = pt_wav.astype(np.float32)

    raw_cpp = load_dump("cpp/output-audio.bin").astype(np.float32)
    raw_pt  = load_dump("python/output-audio.bin").astype(np.float32)

    # Sanity: the Python decoder on Python tokens must reproduce the Python
    # raw audio exactly. Anything below 1.0 means the reference path itself
    # is unstable.
    print(f"[Sanity] PyDecoder(PyTokens) vs PyRaw: {cos(audio_pt_from_pt, raw_pt):.6f}")

    # Cross: the Python decoder fed the GGML tokens. If this matches the GGML
    # raw audio, the GGML decoder is bit equivalent and the divergence lives
    # entirely upstream in the LM and MaskGIT path.
    print(f"[Cross] PyDecoder(GgmlTokens) vs GgmlRaw: {cos(audio_pt_from_cpp, raw_cpp):.6f}")

    # Full pipeline parity. Both WAVs went through identical post processing
    # (fade_and_pad, silence trim, peak normalize), so frame aligned STFT
    # magnitude cosine is the right metric. Comparing raw vs WAV here would
    # collapse to ~0 because of the 2400 sample fade pad inserted at the
    # head, regardless of decoder quality.
    n = min(len(cpp_wav), len(pt_wav))
    print(f"[Pipeline] cpp_wav vs pt_wav stft_cos: {stft_cos(cpp_wav[:n], pt_wav[:n]):.6f}")

    n = min(tok_cpp.size, tok_pt.size)
    a = tok_cpp.ravel()[:n]
    b = tok_pt.ravel()[:n]
    print(f"[Cossim] Tokens exact: {100.0 * float((a == b).mean()):.2f}%")

if __name__ == "__main__":
    main()
