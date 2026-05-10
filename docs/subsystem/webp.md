# WebP 图像处理子系统

> 🖼️ **WebP子系统**是星月智能的WebP图像格式转换和处理模块，提供高效的图像格式转换和压缩功能。

---

## 🏗️ 架构设计

### 模块结构

```
subsystem/webp/
├── main.go       # 主程序入口
├── build.ps1     # 构建脚本
├── go.mod
└── go.sum
```

---

## 🎯 功能特性

### 格式转换

支持WebP格式与以下格式的相互转换：

| 源格式 | 目标格式 | 说明 |
|--------|----------|------|
| PNG | WebP | PNG转WebP |
| JPEG | WebP | JPEG转WebP |
| JPG | WebP | JPG转WebP |
| WebP | PNG | WebP转PNG |
| WebP | JPEG | WebP转JPEG |

### 压缩功能

- **有损压缩**：调整质量参数控制文件大小
- **无损压缩**：保持最高质量
- **透明度支持**：保留Alpha通道

### 批量处理

支持批量转换多个文件：

```powershell
.\webp.exe convert -input ./images -output ./webp -format webp
```

---

## 🔧 使用方式

### 命令行使用

```powershell
# 单文件转换
.\webp.exe convert -input photo.png -output photo.webp

# 指定质量
.\webp.exe convert -input photo.png -output photo.webp -quality 85

# 批量转换
.\webp.exe batch -input ./images -output ./webp -format webp

# 查看帮助
.\webp.exe -help
```

### 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-input` | 输入文件/目录 | 必填 |
| `-output` | 输出文件/目录 | 必填 |
| `-format` | 输出格式 | webp |
| `-quality` | 质量1-100 | 80 |
| `-lossless` | 无损模式 | false |

---

## 📡 API接口

### 图像转换接口

#### POST /convert

**功能**：转换图像格式

**请求体**：
```json
{
  "input_path": "/images/photo.png",
  "output_path": "/images/photo.webp",
  "format": "webp",
  "quality": 85,
  "lossless": false
}
```

**响应格式**：
```json
{
  "success": true,
  "input_path": "/images/photo.png",
  "output_path": "/images/photo.webp",
  "original_size": 1024000,
  "converted_size": 256000,
  "compression_ratio": 0.25
}
```

---

### 批量转换接口

#### POST /batch/convert

**功能**：批量转换图像

**请求体**：
```json
{
  "input_dir": "/images/",
  "output_dir": "/webp/",
  "format": "webp",
  "quality": 80,
  "pattern": "*.png"
}
```

**响应格式**：
```json
{
  "success": true,
  "total": 10,
  "converted": 10,
  "failed": 0,
  "results": [
    {
      "input": "photo1.png",
      "output": "photo1.webp",
      "success": true
    }
  ]
}
```

---

## 🖼️ WebP格式说明

### 格式优势

| 特性 | WebP | JPEG | PNG |
|------|------|------|-----|
| 透明度支持 | ✅ | ❌ | ✅ |
| 动画支持 | ✅ | ❌ | ❌ |
| 压缩率 | 高 | 中 | 低 |
| 浏览器支持 | 现代浏览器 | 所有浏览器 | 所有浏览器 |

### 适用场景

- **网站图片**：减小页面加载体积
- **移动端**：节省流量和存储
- **图标系统**：支持透明背景
- **游戏资源**：高效的纹理压缩

---

WebP子系统为星月智能提供高效的图像格式转换和压缩功能，支持多种格式的相互转换。

---

## 🔗 关联文档

- [主项目README](../../README.md)
- [星图·月华 文档](../luna_astral.md)
- [星图·琉璃 文档](../crystal_astral.md)
- [存储子系统文档](storage.md)