"""Qwen TTS Voice Clone API"""

import os
import signal
import time
import traceback
from pathlib import Path
from threading import Lock

import torch
import uvicorn
from fastapi import FastAPI, HTTPException
from contextlib import asynccontextmanager

from .models import VoiceCloneRequest, TTSResponse, ModelsResponse, ModelInfo
from .utils import encode_audio_to_base64, decode_base64_to_audio, map_language_code, generate_filename_from_base64
from .backend.qwen_backend import QwenTTSBackend, OutOfMemoryError

OUTPUT_PATH = Path(__file__).parent / "outputs"
OUTPUT_PATH.mkdir(parents=True, exist_ok=True)
TEMPLATE_AUDIO_PATH = Path(__file__).parent / "lunar-template.wav"


def cleanup_outputs():
    """清理 outputs 文件夹中的所有文件（不删除模板文件）"""
    try:
        if OUTPUT_PATH.exists():
            for item in OUTPUT_PATH.rglob("*"):
                if item.is_file():
                    item.unlink()
                    print(f"清理文件: {item}")
            print("outputs 文件夹清理完成")
    except Exception as e:
        print(f"清理 outputs 文件夹时发生错误: {e}")


def cleanup_handler(signum, frame):
    """信号处理函数 - 在收到关闭信号时执行清理"""
    print(f"\n收到信号 {signum}，开始清理...")
    cleanup_outputs()
    print("清理完成，退出程序")
    os._exit(0)


class VoiceCloneAPI:
    """声音克隆 API"""

    def __init__(self, preload_model: str = "Qwen/Qwen3-TTS-12Hz-0.6B-Base"):
        @asynccontextmanager
        async def lifespan(app: FastAPI):
            """FastAPI 生命周期管理"""
            print("启动清理信号监听...")
            signal.signal(signal.SIGINT, cleanup_handler)
            signal.signal(signal.SIGTERM, cleanup_handler)
            print(f"预加载 Qwen3-TTS-12Hz-0.6B-Base 模型以占用显存...")
            self.preload_model(preload_model)
            yield
            print("关闭前执行清理...")
            self.backend.unload_model()
            cleanup_outputs()
            print("清理完成")

        self.app = FastAPI(
            title="Qwen TTS Voice Clone API",
            description="Qwen TTS 音频克隆 API 接口",
            version="1.0.0",
            docs_url="/docs",
            redoc_url="/redoc",
            lifespan=lifespan,
        )
        self.queue_lock = Lock()
        self.backend = QwenTTSBackend()
        self.cache = {}  # 缓存: 文本 -> 音频文件路径
        self._setup_routes()

    def preload_model(self, model_name: str = "Qwen/Qwen3-TTS-12Hz-0.6B-Base"):
        """预加载模型以占用显存"""
        try:
            self.backend.load_model(
                model_name=model_name,
                api_type="modelscope",
                device_map="auto",
                dtype=torch.bfloat16,
                attn_implementation=None,
            )
            print(f"模型 {model_name} 预加载完成，显存已占用")
        except Exception as e:
            print(f"预加载模型时发生错误: {e}")
            raise

    def _setup_routes(self):
        """设置 API 路由"""
        self.app.post(
            "/qwen_tts/voice-clone",
            response_model=TTSResponse,
            summary="声音克隆",
            description="使用参考音频进行声音克隆并生成语音",
        )(self.voice_clone)

        self.app.get(
            "/qwen_tts/models",
            response_model=ModelsResponse,
            summary="获取模型列表",
            description="获取可用的 Qwen TTS 模型列表",
        )(self.list_models)

    def list_models(self) -> ModelsResponse:
        """获取可用模型列表"""
        models = [
            ModelInfo(
                id="Qwen/Qwen3-TTS-12Hz-0.6B-Base",
                object="model",
                created=int(time.time()),
                owned_by="Qwen",
            ),
        ]
        return ModelsResponse(data=models)

    def voice_clone(self, req: VoiceCloneRequest) -> TTSResponse:
        """声音克隆 API"""
        try:
            start_time = time.perf_counter()

            ref_audio_base64 = req.ref_audio_base64
            use_template = not ref_audio_base64 or not ref_audio_base64.strip()

            if use_template:
                if not TEMPLATE_AUDIO_PATH.exists():
                    raise FileNotFoundError(f"模板音频文件不存在: {TEMPLATE_AUDIO_PATH}")
                ref_audio_path = TEMPLATE_AUDIO_PATH
            else:
                filename = generate_filename_from_base64(ref_audio_base64)
                temp_audio_path = OUTPUT_PATH / "temp" / f"{filename}.wav"

                if temp_audio_path.exists():
                    ref_audio_path = temp_audio_path
                else:
                    ref_audio_path = decode_base64_to_audio(ref_audio_base64, temp_audio_path)

            with self.queue_lock:
                if req.text in self.cache:
                    cached_path = self.cache[req.text]
                    if cached_path.exists():
                        audio_base64 = encode_audio_to_base64(str(cached_path))
                        elapsed_time = time.perf_counter() - start_time
                        return TTSResponse(
                            audio_base64=audio_base64,
                            info=f"音频来自缓存, 耗时: {elapsed_time:.2f}s",
                        )
                    else:
                        del self.cache[req.text]

                self.backend.load_model(
                    model_name=req.model_name,
                    api_type="modelscope",
                    device_map="auto",
                    dtype=torch.bfloat16,
                    attn_implementation=None,
                )

                actual_language = map_language_code(req.language)

                output_path = self.backend.generate_voice_clone(
                    text=req.text,
                    language=actual_language,
                    ref_audio=ref_audio_path,
                    ref_text=req.ref_text,
                    x_vector_only_mode=(req.ref_text is None or req.ref_text.strip() == ""),
                    do_sample=req.do_sample,
                    top_k=req.top_k,
                    top_p=req.top_p,
                    temperature=req.temperature,
                    repetition_penalty=req.repetition_penalty,
                    subtalker_dosample=req.subtalker_dosample,
                    subtalker_top_k=req.subtalker_top_k,
                    subtalker_top_p=req.subtalker_top_p,
                    subtalker_temperature=req.subtalker_temperature,
                    max_new_tokens=req.max_new_tokens,
                )

                self.cache[req.text] = output_path

            audio_base64 = encode_audio_to_base64(str(output_path))
            elapsed_time = time.perf_counter() - start_time

            return TTSResponse(
                audio_base64=audio_base64,
                info=f"音频生成成功, 耗时: {elapsed_time:.2f}s",
            )

        except OutOfMemoryError as e:
            traceback.print_exc()
            raise HTTPException(status_code=500, detail=f"生成音频所需的显存不足: {str(e)}") from e
        except Exception as e:
            traceback.print_exc()
            raise HTTPException(status_code=500, detail=f"生成失败: {str(e)}") from e

    def run(self, host: str = "0.0.0.0", port: int = 8000):
        """启动 API 服务器"""
        uvicorn.run(self.app, host=host, port=port)
