"""工具函数"""

import base64
import os
import time
from pathlib import Path


def encode_audio_to_base64(audio_path: str) -> str:
    """将音频文件编码为 Base64 字符串"""
    with open(audio_path, "rb") as f:
        audio_bytes = f.read()
    return base64.b64encode(audio_bytes).decode("utf-8")


def decode_base64_to_audio(base64_str: str, output_path: Path) -> Path:
    """将 Base64 字符串解码为音频文件"""
    audio_bytes = base64.b64decode(base64_str)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with open(output_path, "wb") as f:
        f.write(audio_bytes)
    return output_path


def generate_filename() -> str:
    """生成唯一文件名"""
    return f"audio_{int(time.time())}_{os.urandom(4).hex()}"


def generate_filename_from_base64(base64_str: str) -> str:
    """从 Base64 字符串生成合法的文件名（取前16个合法字符）"""
    valid_chars = set("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
    valid_chars_from_start = []
    for char in base64_str:
        if char in valid_chars:
            valid_chars_from_start.append(char)
            if len(valid_chars_from_start) == 16:
                break
    return "".join(valid_chars_from_start)


def find_port(start_port: int = 8000, max_port: int = 65535) -> int:
    """查找可用端口"""
    import socket
    for port in range(start_port, max_port + 1):
        try:
            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
                s.bind(("localhost", port))
                return port
        except OSError:
            continue
    raise RuntimeError("未找到可用端口")


LANGUAGE_MAP = {
    "zh": "chinese",
    "zh-cn": "chinese",
    "zh-hans": "chinese",
    "cn": "chinese",
    "en": "english",
    "en-us": "english",
    "en-gb": "english",
    "fr": "french",
    "de": "german",
    "it": "italian",
    "ja": "japanese",
    "jp": "japanese",
    "ko": "korean",
    "kr": "korean",
    "pt": "portuguese",
    "ru": "russian",
    "es": "spanish",
}


def map_language_code(language: str | None) -> str | None:
    """将语言代码映射为 Qwen TTS 支持的格式"""
    if language is None or language.lower() == "auto":
        return None
    language_lower = language.lower()
    return LANGUAGE_MAP.get(language_lower, language_lower)
