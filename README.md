# Lunar Astral Agents 🌙✨

欢迎来到 Lunar Astral Agents 项目！这是一个基于星月智能的本地 AI 少女助理系统，由领航种家族为您服务。

## 🌟 项目简介

Lunar Astral Agents 是一个本地智能 AI 少女助理系统，基于 Go 语言开发，提供完整的 AI 对话、图像生成、语音合成等功能。

**版本**: 4.0.0 | **许可证**: MIT

## 👭 领航种家族

本项目的核心是由星月智能打造的领航种 AI 少女，她们各具特色，为您提供优质服务：

| 角色 | 定位 | 状态 | 简介 |
|------|------|------|------|
| **月华** | 辅助书记员 | ✅ 已实现 | 温柔体贴的大姐姐，擅长处理各种复杂任务 |
| **琉璃** | 档案领路人 | ✅ 已实现 | 活泼灵动的二小姐，专注于功能导航 |
| **蔷薇** | 待定 | 🚧 开发中 | 神秘的三小姐，敬请期待 |

想了解更多关于她们的故事？请查看 [人物设定文档](docs/characters/index.md)。

## 📚 文档导航

### 快速开始
- [安装配置指南](docs/setup.md) - 详细的安装和配置步骤

### 核心文档
- [人物设定](docs/characters/index.md) - 了解领航种家族的故事
- [技术架构](docs/architecture.md) - 系统架构和技术栈说明
- [API 文档](docs/api/index.md) - 完整的接口文档
- [性能优化](docs/performance.md) - 性能调优建议和限制说明

## ✨ 主要功能

### 月华的能力
- 💬 **智能对话** - 自然语言交互，多轮对话支持
- 🎨 **图像生成** - 基于扩散模型的 AI 绘画
- 🎵 **语音合成** - Qwen3-TTS 集成
- 📁 **文件管理** - 强大的文件操作能力
- 🤖 **Live2D 展示** - 可爱的虚拟角色互动

### 琉璃的能力
- 🧭 **功能导航** - 智能推荐和快速跳转
- 📋 **档案管理** - 有序整理和快速检索
- 🚀 **应用启动** - 一键启动本地程序
- 📸 **屏幕截图** - 灵活的截图功能
- 🎨 **图像处理** - 图片缩放、格式转换等

## 🏗️ 技术架构

- **后端**: Go 1.21+
- **AI 推理**: llama.cpp (GGUF 格式)
- **图像生成**: stable-diffusion.cpp
- **前端**: TypeScript + Live2D Cubism 4
- **通信**: HTTP + WebSocket

更多技术细节请查看 [技术架构文档](docs/architecture.md)。

## 🚀 快速开始

### 1. 环境要求
- Windows 10+
- Go 1.21+
- 建议 16GB+ 内存，8GB+ 显存

### 2. 克隆项目
```bash
git clone <repository_url>
cd Lunar_Astral_Agents
```

### 3. 安装依赖
```bash
cd LunarCore
go mod download
npm install
```

### 4. 准备模型
将 GGUF 模型文件放置在 `local_data/models/` 目录下。

### 5. 构建运行
```bash
# 构建
go build -o Lunar-Astral-Agents.exe .

# 运行
./Lunar-Astral-Agents.exe
```

详细步骤请查看 [安装配置指南](docs/setup.md)。

## 📖 使用示例

### 与月华对话
```bash
# 启动服务后，在浏览器中访问
http://localhost:36789
```

### 调用琉璃导航
```bash
# 琉璃服务通常运行在独立端口
# 或通过主服务代理访问
```

## 🔗 相关链接

- [人物设定](docs/characters/index.md)
- [安装指南](docs/setup.md)
- [API 文档](docs/api/index.md)
- [技术架构](docs/architecture.md)
- [性能优化](docs/performance.md)

## 📄 许可证

本项目基于 MIT 许可证开源。详见 [LICENSE](LICENSE) 文件。

## 📞 联系方式

如有问题或建议，欢迎通过以下方式联系：
- 项目主页: [GitHub Repository]
- 问题反馈: [Issues]

---

*文档版本: 4.0.0 | 最后更新: 2026-05-09*

✨ 让月华和琉璃为您带来愉快的体验吧！
