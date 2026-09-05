# yaraflow-weather

YaraFlow 天气查询工具插件，供 LLM（Planner）通过 Function Calling 自动调用。用户在群里聊天气相关话题时，LLM 自动判断并调用本工具获取天气数据。

## 特点

- 🎯 **LLM 工具**：不是指令型插件，由 Planner 自动决策调用
- 🔄 **多 API 自动回退**：和风天气 → Open-Meteo → wttr.in，按优先级自动降级
- 🆓 **零配置可用**：不填任何 Key 也能用（自动使用免费 API）
- 🇨🇳 **国内精准**：配置和风天气 Key 后，国内城市天气数据最精准
- 🌍 **全球覆盖**：Open-Meteo 和 wttr.in 支持全球城市

## API 提供者

| 优先级 | 提供者 | 费用 | 需要 Key | 精准度 | 说明 |
|--------|--------|------|----------|--------|------|
| 🥇 首选 | [和风天气](https://dev.qweather.com) | 免费 1000次/天 | API Key/JWT | ⭐⭐⭐⭐⭐ | 国内最精准，中文城市名直接查，推荐使用JWT |
| 🥈 回退 | [Open-Meteo](https://open-meteo.com) | 完全免费 | 否 | ⭐⭐⭐⭐ | 开源气象模型，全球覆盖，需经纬度转换 |
| 🥉 兜底 | [wttr.in](https://wttr.in) | 完全免费 | 否 | ⭐⭐⭐ | 极简天气服务，无需任何配置 |

## 配置

### ⚠️ 和风天气重要提醒

从 **2027年1月1日** 起，和风天气将对 API KEY 认证方式实施请求量限制。官方推荐使用 **JSON Web Token (JWT)** 认证方式以获得更高安全性和不受限的 API 请求。

插件已支持 **JWT 认证**，建议优先使用此方式。

### 默认配置（开箱即用）

不填任何 Key，插件自动使用 Open-Meteo → wttr.in 回退链，完全免费可用。

### 增强配置（API Key 方式，不推荐）

在插件配置页填入和风天气 API Key，获得更精准的国内天气数据：

1. 前往 [和风天气开发平台](https://dev.qweather.com) 注册
2. 创建项目获取 API Key（免费版每天 1000 次调用）
3. 在 YaraFlow 插件配置页填入 Key

> ⚠️ 注意：API Key 方式从 2027年1月1日起将受限，建议使用 JWT 认证。

### 增强配置（JWT 方式，推荐）

在插件配置页填入和风天气 JWT 凭据信息：

1. 前往 [和风天气开发平台](https://dev.qweather.com) 注册
2. 创建项目后，在「凭据管理」中创建凭据
3. 选择 **JWT** 认证方式
4. 下载 Ed25519 私钥文件
5. 在 YaraFlow 插件配置页填入：
   - **Project ID**（项目ID）
   - **Key ID**（密钥ID）
   - **Private Key**（完整的 PEM 私钥内容）

JWT 认证优势：
- ✅ 更高安全性
- ✅ 不受请求量限制
- ✅ 官方推荐方式

配置后，插件会优先使用和风天气，失败时自动回退到免费 API。

## 返回数据

工具返回结构化 JSON 给 Planner：

```json
{
  "city": "北京, 北京",
  "current": {
    "temperature": 28,
    "feels_like": 30,
    "condition": "晴",
    "humidity": 65,
    "wind": "东南风 3级"
  },
  "forecast": [
    { "date": "2026-07-08", "high": 32, "low": 22, "condition": "晴" },
    { "date": "2026-07-09", "high": 28, "low": 20, "condition": "小雨" }
  ],
  "source": "和风天气",
  "aqi": { "value": 45, "level": "1", "category": "优" }
}
```

Planner 会从中选取关键信息，交给回复器生成自然语言回复。

## 触发示例

用户在群里说：
- "今天北京天气怎么样"
- "上海热不热"
- "深圳明天会下雨吗"
- "杭州这几天冷不冷"

LLM 会识别出天气查询意图，自动调用 `get_weather` 工具获取数据并回复。

## 技术架构

```
用户消息 → Planner 识别意图 → 调用 get_weather(city)
  → 和风天气（有Key）→ 失败？→ Open-Meteo → 失败？→ wttr.in
  → 返回结构化JSON → Planner 选取关键信息 → 回复器生成自然语言回复
```

## 权限声明

- `tool.register` — 注册 LLM 工具
- `http.request` — 调用天气 API
- `plugin.config.read/write` — 读写配置
- `plugin.file.read/write` — 自动生成配置文件

## 许可证

GPL-3.0 License

## 仓库链接

https://gitee.com/luoxiyilian/yaraflow-weather0712

