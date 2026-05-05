"""Qwen TTS Voice Clone API 主启动文件"""

import argparse
import logging

from .logger import get_logger
from .utils import find_port
from .api import VoiceCloneAPI

logger = get_logger()


def main():
    """主函数"""
    parser = argparse.ArgumentParser(description="Qwen TTS Voice Clone API")
    parser.add_argument(
        "--host",
        type=str,
        default="0.0.0.0",
        help="服务器绑定地址 (默认: 0.0.0.0)",
    )
    parser.add_argument(
        "--port",
        type=int,
        default=8000,
        help="服务器端口 (默认: 8000)",
    )
    parser.add_argument(
        "--debug",
        action="store_true",
        help="启用调试模式",
    )
    args = parser.parse_args()

    if args.debug:
        logger.info("启用 Debug 模式")
        logging.basicConfig(level=logging.DEBUG)

    logger.info("初始化 Qwen TTS Voice Clone API")

    api = VoiceCloneAPI()

    port = find_port(args.port)
    logger.info("Qwen TTS Voice Clone API 已启动")
    logger.info("API 文档地址: http://%s:%d/docs", args.host, port)
    logger.info("API 地址: http://%s:%d/qwen_tts/voice-clone", args.host, port)

    api.run(host=args.host, port=port)


if __name__ == "__main__":
    main()
