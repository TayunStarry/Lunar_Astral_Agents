# 子系统——图像处理（image_processor）

> 📚 代码级文档参见 [Code Wiki 04·公共子系统](../../docs/code-wiki/04-公共子系统.md)，入口 [Code Wiki 门户](../../docs/code-wiki/README.md)。

图像生成、视频关键帧提取与屏幕截图的共享库模块，为钛宇-月华提供图像相关能力。

---

## 功能概述

`image_processor` 子系统是图像处理能力的共享库，采用 Module（逻辑层）+ Server（HTTP 层）二层架构，为上层应用提供：

| 功能 | 说明 | 适用场景 |
|------|------|----------|
| 扩散图像生成 | 调用 stable-diffusion.cpp 命令行引擎，支持文生图 / 图生图 / 超分，异步任务队列 | 根据文字或参考图生成图片 |
| 视频关键帧提取 | 从视频文件中智能提取关键帧图片 | 视频内容摘要与封面生成 |
| 屏幕截图 | 统一截图接口（auto/window/fullscreen/display/region 五种模式），焦点窗口优先、失败自动降级全屏 | 捕获桌面画面进行 AI 理解 |
| 图像格式处理 | Base64 编解码、像素格式转换、缩放裁剪、着色处理 | 图像数据的通用加工 |

---

## 运行与使用方式

`image_processor` 作为库集成到宿主程序中，提供图像生成、关键帧提取与截图这三类 HTTP 端点，路由由宿主程序注册。完整端点表、请求/响应字段及调用方式见 [Code Wiki 04 §4.4](../../docs/code-wiki/04-公共子系统.md)，此处不重复。

---

## 依赖

- **stable-diffusion.cpp 引擎（`sd-cli.exe`）**：扩散图像生成的命令行引擎，需自行编译或获取，路径通过配置子系统指定
- **FFmpeg**：视频解码与帧提取（需安装并加入系统 PATH，或通过配置指定路径）
- 屏幕截图能力依赖跨平台桌面环境（Windows、macOS、Linux X11）

---

## 常见问题

### Q: sd-cli.exe 在哪里？

sd-cli.exe 是 stable-diffusion.cpp 的命令行引擎，需自行编译或获取。图像生成模块通过 [general_config 子系统](../general_config/README.md) 中的引擎路径配置找到它。

### Q: 图像生成速度慢怎么办？

1. 启用 GPU 加速（Vulkan/CUDA）
2. 使用量化模型减少推理计算量
3. 降低采样步数、减小输出分辨率

### Q: 支持哪些图像生成模型？

支持 stable-diffusion.cpp 兼容的 SafeTensors/GGUF 格式模型，包括 SD 1.x / SD 2.x / SDXL / SD3 等架构。模型路径在配置子系统中配置。

### Q: 视频关键帧提取失败怎么办？

1. 确认 FFmpeg 已安装并添加到系统 PATH
2. 或在配置子系统中指定 FFmpeg 自定义路径
3. 确认视频文件格式兼容（支持 MP4、AVI、MOV、WebM 等主流格式）

### Q: 截图功能在哪些平台可用？

- **焦点窗口捕获**（auto/window 模式）仅 Windows 平台可用，非 Windows 平台会返回明确错误。
- **全屏 / 显示器 / 区域截图**支持 Windows、macOS、Linux（X11）等主流桌面平台；无显示器环境（如远程会话）可能无法捕获画面。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [配置管理子系统](../general_config/README.md) —— 扩散模型、引擎与 FFmpeg 路径配置
- [钛宇-月华](../../lunar_astral/README.md) —— image_processor 子系统的主要集成使用方