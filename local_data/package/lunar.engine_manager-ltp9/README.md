# 『 星月智能 』LTP9 引擎可视化测试

纯 LTP9 的**可视化测试工具**（蓝图/节点式）：无需写代码，通过**拖入节点、连线、填参数**即可搭建对 LTP9
引擎各能力的测试流程并运行回显。琉璃前端在 iframe 打开本包 `index.html`。`tags["Zero-LTP"]` 表明它是前端工具包，
非 LTP9 插件（引擎不加载）。

## 用法
1. 点击左下角「**节点模块**」打开分类面板（按类型分页：入口启动/引擎交互/数据安全/智能网络/流程处理/逻辑门），点击节点加入画布（图标按各节点主题色着色）。
2. 点击节点弹出参数表单，按下拉/输入/JSON 编辑区填参（不用写代码）。
3. **连线**：节点**上=输入、下=输出**。从源节点下方「●」输出端口拖到目标上方「◯」输入端口。双击连线删除。
   - **时钟线（金色实线）**：连到目标「顺序」输入口，决定执行链——上游完成且成功，下游才运行。启动激活走时钟线。
   - **信号线（灰色虚线）**：连到目标数据输入口，**仅传入参数**，不驱动执行（需目标另有时钟线被激活）。
   - 逻辑门的任一输入都被视为时钟/门控输入。
4. **触发入口**：启动（`start`）、事件启动（`event_start`）、时钟启动（`clock_start`）都可作为链起点；运行全部将从它们出发链式执行。
5. **运行 / 停止**：运行中「运行全部」按钮变为「停止运行」，可随时点击停止（含清除时钟/事件等待）。
6. 逻辑门（AND/OR/NOT/NAND/NOR）**仅作门控**：按输入成败计算后决定是否放行下游。
7. 回执显示在右下角「引擎日志」悬浮窗（有新日志时按钮高亮闪烁）。
8. **保存 / 加载**：画布以 JSON 文件持久化到 `local_data/database/lunar.engine_manager_ltp9.graph.json`（与项目数据库同目录），启动时自动加载，不存在则载入示例。

## 能力节点
| 节点 | 对应 engine 能力 | 后端信封 |
|---|---|---|
| 启动节点 | 运行全部从本节点起链式执行 | —（本地） |
| 事件启动 | 目标事件触发后启动链并传递载荷 | —（本地，前端总线） |
| 时钟启动 | 每隔设定时间启动一次下游链（直至停止） | —（本地） |
| 与/或/非/与非/或非 | 逻辑门控（仅门控不传数据） | —（本地） |
| 事件触发 | `engine.event`（Emit weather.query 等） | `ltp9/event` |
| 全局/定向广播 | `engine.signal.all / target` | `ltp9/broadcast` / `ltp9/test action=broadcast_target` |
| 跨包调用 | `engine.call(包ID).run` | `ltp9/call` |
| 前端智能体 | `engine.agent(包ID).run`（Mini-LTP/Node-LTP） | `ltp9/test action=agent` |
| 数据库 | `engine.database.query/exec`（SQLite knowledge.db） | `ltp9/test action=db` |
| 记忆库 | `engine.memory.store/search`（向量库 ltp9_memory） | `ltp9/test action=memory` |
| 文件 | `engine.file.write/read/delete`（探针目录 database/ltp9_probe） | `ltp9/test action=file` |
| 加密/解密 | `engine.encoder/decoder`（lunar_decoder 往返） | `ltp9/test action=crypto` |
| JWT 签名 | `engine.crypto.signJWT`（HS256/EdDSA/none） | `ltp9/test action=jwt` |
| LLM 对话 | `engine.llm.chat`（读 lunar_config.json agent） | `ltp9/test action=llm` |
| 同步 HTTP | `engine.http.get/post` | `ltp9/test action=http` |
| 格式转换 | 多入单出：逐路提取字段 + `{{inN.字段}}` 模板组合（模板必填） | —（本地） |
| 同步等待 | `engine.sleep` 语义（前端 await） | —（本地） |
| 插件状态 / 探测 | `PluginStates` / 引擎在线 | `ltp9/stats` / `ltp9/probe` |

> **数据流**：LLM 对话输入「用户文本」→ 输出「AI 应答」；加密 encode 输入明文+密钥 → 输出密文、decode 反之；
> 跨包调用把上游输出作参数；文件读出内容、JWT 令牌、HTTP 响应、DB 行集等均可经输出端口传给下游。

## 后端依赖
- `crystal_astral/agent/StarLTP/api_probe.go`：Probe 系列测试接口（复用 api.go/api_ext.go/api_net.go 实现）。
- `crystal_astral/ltp9_debug.go`：`ltp9/test` 信封分发 → `ltp9/test_ack` 回执。
- LTP9 插件包：`com.yaraflow.weather-ltp9` / `com.yaraflow.art-ltp9`（`tags` 含 `LTP9`）用于事件/跨包调用测试。