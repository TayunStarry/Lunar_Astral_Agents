"""Qwen TTS 推理后端"""

import os
import time
from typing import Literal, Any, TypeAlias
from pathlib import Path

import torch
import soundfile as sf
from modelscope import snapshot_download
from qwen_tts import Qwen3TTSModel

from ..logger import get_logger
from ..utils import generate_filename

logger = get_logger()

OUTPUT_PATH = Path(__file__).parent.parent / "outputs"
OUTPUT_PATH.mkdir(parents=True, exist_ok=True)

AttnImpl: TypeAlias = Literal["eager", "sdpa", "flash_attention_2"]


class OutOfMemoryError(Exception):
    """内存不足异常"""
    pass


class QwenTTSBackend:
    """Qwen TTS 推理后端"""

    def __init__(self):
        self.model_name = None
        self.model = None

    def unload_model(self):
        """卸载模型"""
        logger.info("卸载 %s 模型", self.model_name)
        try:
            del self.model
        except NameError:
            pass
        if torch.cuda.is_available():
            torch.cuda.empty_cache()
        self.model_name = None
        self.model = None
        logger.info("卸载模型完成")

    def load_model(
        self,
        model_name: str | None = None,
        device_map: str | None = "auto",
        dtype: torch.dtype | None = torch.bfloat16,
        attn_implementation: AttnImpl | None = None,
        api_type: Literal["huggingface", "modelscope", "local"] | None = "modelscope",
    ) -> None:
        """加载 Qwen TTS 模型"""
        if self.model is not None and self.model_name == model_name:
            logger.info("Qwen TTS 模型 %s 已加载", self.model_name)
            return

        if self.model_name is None and model_name is None:
            raise ValueError("未指定 Qwen TTS 模型名称")

        if self.model_name is not None and model_name is not None and self.model_name != model_name:
            logger.info("切换 Qwen TTS 模型: %s -> %s", self.model_name, model_name)
            self.unload_model()
            self.model_name = model_name
        else:
            self.model_name = model_name
            logger.info("加载 Qwen TTS 模型: %s", self.model_name)

        if api_type == "local":
            if Path(model_name).exists():
                logger.info("从本地路径加载模型: %s", model_name)
                model_path = model_name
            else:
                raise FileNotFoundError(f"模型路径 '{model_name}' 不存在")
        elif api_type == "modelscope":
            logger.info("加载 %s 模型", self.model_name)
            try:
                model_path = snapshot_download(
                    repo_id=model_name,
                    repo_type="model",
                    local_files_only=True,
                )
                logger.info("从本地缓存加载 %s", self.model_name)
            except Exception:
                logger.info("本地未找到模型, 从 ModelScope 下载 %s 中", self.model_name)
                model_path = snapshot_download(
                    repo_id=model_name,
                    repo_type="model",
                    local_files_only=False,
                )
                logger.info("%s 模型从 ModelScope 下载完成", self.model_name)
        else:
            raise ValueError(f"未知的 API 类型: {api_type}")

        try:
            self.model = Qwen3TTSModel.from_pretrained(
                pretrained_model_name_or_path=model_path,
                device_map=device_map,
                dtype=dtype,
                attn_implementation=attn_implementation,
            )
        except RuntimeError as e:
            if "out of memory" in str(e).lower():
                logger.error("加载 Qwen TTS 模型 %s 时发生内存不足: %s", self.model_name, e)
                tmp_name = self.model_name
                self.unload_model()
                raise OutOfMemoryError(f"加载 Qwen TTS 模型 {tmp_name} 时发生内存不足: {e}") from e
            raise

        logger.info("%s 模型加载完成", self.model_name)

    def generate_voice_clone(
        self,
        text: str,
        language: str | None = None,
        ref_audio: Path | None = None,
        ref_text: str | None = None,
        x_vector_only_mode: bool | None = False,
        do_sample: bool | None = True,
        top_k: int | None = 50,
        top_p: float | None = 1.0,
        temperature: float | None = 0.9,
        repetition_penalty: float | None = 1.05,
        subtalker_dosample: bool | None = True,
        subtalker_top_k: int | None = 50,
        subtalker_top_p: float | None = 1.0,
        subtalker_temperature: float | None = 0.9,
        max_new_tokens: int | None = 1024,
    ) -> Path:
        """使用提示词生成音频, 并利用一段音频克隆声音"""
        kwargs = {
            "text": text,
            "language": language,
            "ref_audio": str(ref_audio),
            "ref_text": ref_text,
            "x_vector_only_mode": x_vector_only_mode,
            "do_sample": do_sample,
            "top_k": top_k,
            "top_p": top_p,
            "temperature": temperature,
            "repetition_penalty": repetition_penalty,
            "subtalker_dosample": subtalker_dosample,
            "subtalker_top_k": subtalker_top_k,
            "subtalker_top_p": subtalker_top_p,
            "subtalker_temperature": subtalker_temperature,
            "max_new_tokens": max_new_tokens,
        }

        if not x_vector_only_mode and (ref_text is None or ref_text.strip() == ""):
            raise ValueError("当 `x_vector_only_mode=False` 时, 需要提供参考音频对应的参考文本")

        try:
            logger.info("生成音频中")
            start_time = time.perf_counter()
            wavs, sr = self.model.generate_voice_clone(**kwargs)
            logger.info("音频生成完成, 耗时: %.2fs", (time.perf_counter() - start_time))
        except RuntimeError as e:
            if "out of memory" in str(e).lower():
                logger.error("调用 Qwen TTS 模型 %s 进行音频合成时发生内存不足: %s", self.model_name, e)
                self.unload_model()
                raise OutOfMemoryError(f"音频合成时发生内存不足: {e}") from e
            raise

        return self.save_audio(wavs[0], sr)

    def save_audio(self, wav: Any, samplerate: int) -> Path:
        """将音频数据保存为音频文件"""
        save_path = OUTPUT_PATH / f"{generate_filename()}.wav"
        save_path.parent.mkdir(parents=True, exist_ok=True)
        sf.write(save_path, wav, samplerate)
        logger.info("音频文件保存到 '%s'", save_path)
        return save_path
