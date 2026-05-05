"""日志模块"""

import logging
import sys


def get_logger(level: int = logging.INFO, color: bool = True) -> logging.Logger:
    """创建并返回日志记录器"""
    logger = logging.getLogger("Qwen TTS API")
    logger.setLevel(level)

    if logger.handlers:
        return logger

    formatter = logging.Formatter(
        "%(asctime)s - %(name)s - %(levelname)s - %(message)s"
    )

    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(formatter)
    logger.addHandler(handler)

    return logger
