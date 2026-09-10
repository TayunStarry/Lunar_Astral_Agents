# LTP9 天气查询插件

查询指定城市的实时天气与未来 2 天预报，支持 Open-Meteo / wttr.in 自动回退，和风天气（API Key 或 JWT）可选。

## 插件能力

- **同步模型**：网络走 `engine.http.get`（同步阻塞），JWT 走 `engine.crypto.signJWT`，不产生 Promise。
- **事件订阅**：`weather.query`，负载 `{ city }`。
- **导出函数**：`queryWeather(city)`（经 `engine.call` 调用）。
- **结果回执**：查询结果经 `engine.signal.all({ topic: "weather.query", result })` 广播。
- **多提供者回退**：`auto`（默认）/ `qweather` / `openmeteo` / `wttr`；高级别提供者失败自动降级到免费提供者。

**JWT 说明**：和风天气 JWT 认证经 `engine.crypto.signJWT` 生成 **EdDSA（Ed25519）** 令牌（`sub=项目ID`、`iat/exp`），
`Authorization: Bearer <token>`。配置 `qweather_private_key`（PKCS8 PEM）与 `qweather_project_id`、`qweather_key_id`。

## 权限申请（用 ltp9_keygen 生成 permissions.key 时勾选）

- `allow-network`：`engine.http` 调用 Open-Meteo / wttr.in / 和风天气
- `allow-signal`：把查询结果广播出去
- `allow-certificate`：和风天气 JWT（EdDSA）认证时勾选

## 调用方式

1. **导出函数**：`engine.call("com.yaraflow.weather-ltp9").run("queryWeather", [city])`（同步阻塞）
   返回 `{ ok: true, data: {...} }` 或 `{ ok: false, error }`；目标包不存在或未导出时抛出异常。
2. **事件**：客户端 `Emit("weather.query", { city: "北京" }, requestID)`；
   插件结果经 `engine.signal.all({ topic: "weather.query", result })` 广播。

## 配置文件说明（config.yaml）

- `api.provider`：`auto` / `qweather` / `openmeteo` / `wttr`
- `api.qweather_auth`：`api_key` / `jwt`
- `api.qweather_host` / `api.qweather_key`：和风天气 API Key 凭据（可选）
- `api.qweather_project_id` / `api.qweather_key_id` / `api.qweather_private_key`：和风天气 JWT（Ed25519）凭据（可选）
- `display.default_city`：默认城市
- `display.show_aqi`：是否显示空气质量指数（仅和风天气）

> 生成权限密钥的步骤见 `subsystem/ltp9_keygen`。
