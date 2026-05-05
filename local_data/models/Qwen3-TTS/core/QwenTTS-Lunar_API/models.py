"""API 数据模型定义"""

from typing import Optional

from pydantic import BaseModel, Field


class VoiceCloneRequest(BaseModel):
    """声音克隆请求模型"""

    model_name: str = Field(description="模型名称")
    text: str = Field(description="待合成文本")
    language: Optional[str] = Field(default=None, description="语言标识")
    ref_audio_base64: Optional[str] = Field(default=None, description="参考音频文件的 Base64 编码（不提供则使用 lunar-template.wav）")
    ref_text: Optional[str] = Field(default=None, description="参考音频文本描述")
    segment_gen: bool = Field(default=False, description="是否分段生成")
    do_sample: Optional[bool] = Field(default=True, description="是否使用采样")
    top_k: Optional[int] = Field(default=50, description="Top-k 采样参数")
    top_p: Optional[float] = Field(default=1.0, description="Top-p 采样参数")
    temperature: Optional[float] = Field(default=0.9, description="采样温度")
    repetition_penalty: Optional[float] = Field(default=1.05, description="重复惩罚系数")
    subtalker_dosample: Optional[bool] = Field(default=True, description="子说话者采样开关")
    subtalker_top_k: Optional[int] = Field(default=50, description="子说话者 Top-k 采样")
    subtalker_top_p: Optional[float] = Field(default=1.0, description="子说话者 Top-p 采样")
    subtalker_temperature: Optional[float] = Field(default=0.9, description="子说话者采样温度")
    max_new_tokens: Optional[int] = Field(default=1024, description="最大生成 Token 数")


class TTSResponse(BaseModel):
    """TTS 响应模型"""

    audio_base64: str = Field(description="生成的音频文件 (Base64 编码)")
    info: str = Field(description="生成信息")


class ErrorResponse(BaseModel):
    """错误响应模型"""

    error: str = Field(description="错误信息")


class ModelInfo(BaseModel):
    """模型信息"""

    id: str = Field(description="模型标识符")
    object: str = Field(default="model", description="对象类型")
    created: int = Field(description="创建时间戳")
    owned_by: str = Field(description="模型所有者")


class ModelsResponse(BaseModel):
    """模型列表响应"""

    object: str = Field(default="list", description="对象类型")
    data: list[ModelInfo] = Field(description="模型列表")
