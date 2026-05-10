# File Explorer 文件浏览器

> 📂 **File Explorer** 组件提供文件浏览与管理界面，支持文件和目录的查看、导航和操作。

---

## 📁 文件结构

```
file_explorer/
├── index.html   # 浏览器主页面
├── script.js    # 文件操作逻辑
└── styles.css   # 界面样式
```

---

## 🎯 功能特性

### 文件浏览

- 目录树形结构展示
- 文件列表网格/列表视图切换
- 文件类型图标显示
- 文件大小与修改时间显示

### 文件操作

- 创建新文件夹
- 文件重命名
- 文件删除
- 文件复制/粘贴（待实现）

### 导航功能

- 路径面包屑导航
- 快速跳转
- 历史记录
- 收藏夹支持

---

## 🔧 使用方式

### HTML引用

```html
<!-- 引入文件浏览器 -->
<iframe src="/package/file_explorer/index.html" width="100%" height="600"></iframe>
```

### JavaScript交互

```javascript
// 导航到指定路径
document.querySelector('iframe').contentWindow.postMessage({
    type: 'navigate',
    path: '/documents'
}, '*');

// 刷新文件列表
document.querySelector('iframe').contentWindow.postMessage({
    type: 'refresh'
}, '*');
```

---

## 📡 接口协议

### 请求消息

| 消息类型 | 说明 | 参数 |
|----------|------|------|
| `navigate` | 导航到路径 | `{ path: string }` |
| `refresh` | 刷新当前目录 | 无 |
| `delete` | 删除文件/目录 | `{ path: string }` |
| `create_folder` | 创建文件夹 | `{ path: string, name: string }` |

### 响应消息

| 消息类型 | 说明 | 数据格式 |
|----------|------|----------|
| `files_list` | 文件列表 | `{ path: string, files: FileInfo[] }` |
| `error` | 错误信息 | `{ message: string }` |

### FileInfo 类型

```typescript
interface FileInfo {
    name: string;        // 文件名
    path: string;       // 完整路径
    is_dir: boolean;    // 是否为目录
    size: number;       // 文件大小
    modified: string;   // 修改时间
    extension: string;  // 文件扩展名
}
```

---

## 🎨 界面预览

- 左侧：目录树
- 中间：文件列表
- 顶部：工具栏与路径导航
- 支持拖拽操作

---

## 🔗 关联文档

- [扩展包总览](index.md)
- [星图·琉璃文档](../crystal_astral.md)
- [星图·月华文档](../luna_astral.md)
- [主项目README](../README.md)