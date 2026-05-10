# Database Manager 数据库管理器

> 🗄️ **Database Manager** 组件提供数据库管理界面，用于浏览和管理本地SQLite数据库。

---

## 📁 文件结构

```
database_manager/
├── index.html   # 管理界面主页面
├── script.js    # 交互逻辑脚本
└── styles.css   # 界面样式
```

---

## 🎯 功能特性

### 数据浏览

- 查看数据库表列表
- 浏览表数据记录
- 分页显示大量数据

### 数据操作

- 执行SQL查询（只读）
- 导出查询结果
- 数据筛选与搜索

### 界面功能

- 表结构展示
- 字段类型显示
- 记录数统计

---

## 🔧 使用方式

### HTML引用

```html
<!-- 引入数据库管理器 -->
<iframe src="/package/database_manager/index.html" width="100%" height="600"></iframe>
```

### API调用

组件通过 `postMessage` 与父页面通信：

```javascript
// 请求表列表
parent.postMessage({ type: 'get_tables' }, '*');

// 接收响应
window.addEventListener('message', (event) => {
    if (event.data.type === 'tables_list') {
        console.log('Tables:', event.data.tables);
    }
});
```

---

## 📡 接口协议

### 请求消息

| 消息类型 | 说明 | 参数 |
|----------|------|------|
| `get_tables` | 获取所有表 | 无 |
| `get_records` | 获取表记录 | `{ table: string, limit: number, offset: number }` |
| `search` | 搜索记录 | `{ table: string, keyword: string }` |

### 响应消息

| 消息类型 | 说明 | 数据格式 |
|----------|------|----------|
| `tables_list` | 表列表 | `[{ name: string, record_count: number }]` |
| `records` | 记录数据 | `{ columns: string[], rows: any[][], total: number }` |
| `error` | 错误信息 | `{ message: string }` |

---

## 🎨 界面预览

![琉璃数据管理主页面](../image/琉璃-数据管理-主页面.png)

![琉璃数据管理配置说明](../image/琉璃-数据管理-配置说明.png)

界面采用星月智能统一设计风格：
- 深色主题背景
- 圆润的卡片式布局
- 清晰的表格展示

---

## 🔗 关联文档

- [扩展包总览](index.md)
- [星图·琉璃文档](../crystal_astral.md)
- [主项目README](../README.md)
