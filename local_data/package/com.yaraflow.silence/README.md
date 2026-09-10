# 沉默插件 (com.yaraflow.silence)

> 使语瞳在合适的时候保持沉默（窥屏）| YaraFlow 平台

当聊天气氛不对、有人反感语瞳说话、或管理员需要语瞳安静时，插件进入沉默状态，暂停回复消息。消息照常记录，终端显示 `(窥屏ing)` 标记，但不会触发 Planner 和回复。

## 功能

- **LLM 自主沉默**：语瞳判断当前不合适说话时，通过 `set_silence` 工具自行进入沉默
- **管理员手动控制**：`/silence true [时长]` 进入沉默，`/silence false` 解除
- **@ 打断沉默**：沉默中被 @ 时自动解除（强制沉默除外）
- **三级策略**：low（适当收敛）、medium（说错话/气氛不对）、serious（被人明确要求）
- **消息照常记录**：沉默期间消息正常入库，终端显示 `(窥屏ing)`，但不会回复

## 指令

| 指令 | 说明 |
|------|------|
| `/silence true [时长(秒)]` | 进入沉默，不填时长则永久沉默 |
| `/silence false` | 解除沉默 |

## 工具

| 工具名 | 说明 |
|--------|------|
| `set_silence` | LLM 自主调用，进入沉默状态。参数：`case`（low/medium/serious）、`time`（选填，秒） |

## 配置

配置文件 `config.yaml` 在首次运行时自动生成到插件目录。

```yaml
components:
  enable_silence_tool: true
  enable_silence_command: true
  enable_silence_hook: true

permissions:
  white_or_black_list: whitelist
  admin_users:
    - "qq:你的QQ号"

adjustment:
  low_case_min: 120
  low_case_max: 600
  medium_case_min: 600
  medium_case_max: 1200
  serious_case_min: 1200
  serious_case_max: 5400
  max_action_silence_time: 10800

experimental:
  silence_special_check: false
  silence_someone_list: []
  silence_group_list: []
```

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `components.enable_silence_tool` | bool | true | LLM 自主沉默工具 |
| `components.enable_silence_command` | bool | true | /silence 命令 |
| `components.enable_silence_hook` | bool | true | 消息拦截 Hook |
| `permissions.admin_users` | array | [] | 管理员列表，格式 `平台:ID` |
| `adjustment.low_case_min` | int | 120 | low 级别最小沉默秒数 |
| `adjustment.low_case_max` | int | 600 | low 级别最大沉默秒数 |
| `adjustment.medium_case_min` | int | 600 | medium 级别最小沉默秒数 |
| `adjustment.medium_case_max` | int | 1200 | medium 级别最大沉默秒数 |
| `adjustment.serious_case_min` | int | 1200 | serious 级别最小沉默秒数 |
| `adjustment.serious_case_max` | int | 5400 | serious 级别最大沉默秒数 |
| `adjustment.max_action_silence_time` | int | 10800 | LLM 动作触发的最大沉默秒数 |
| `experimental.silence_special_check` | bool | false | 启用特殊沉默列表 |
| `experimental.silence_someone_list` | array | [] | 默认沉默的用户列表 |
| `experimental.silence_group_list` | array | [] | 默认沉默的群聊列表 |

## 原理

插件注册两个 Hook：

1. **`chat.receive.before_process`**：放行消息（保证入库），同时传递 `logSuffix: "(窥屏ing)"` 给主程序拼接到日志行
2. **`chat.receive.after_process`**：返回 `allowContinue: false` 阻断 pipeline，消息不再进入 Gate（Planner）和 Reply 阶段

```
用户消息
  │
  ▼
before_process → allowContinue: true + logSuffix: "(窥屏ing)"  ← 消息放行，日志标记
  │
  ▼
消息入库 → 终端打印 [消息] xxx(窥屏ing)
  │
  ▼
after_process → allowContinue: false  ← 阻断，不触发 Planner/回复
  │
  ▼
  ✗ (无回复)
```

## 许可

GPL-3.0