# 子系统——环境修复工具（environment_repair）

> 📚 代码级文档参见 [Code Wiki 05·独立AI引擎与运维工具](../../docs/code-wiki/05-独立AI引擎与运维工具.md)，入口 [Code Wiki 门户](../../docs/code-wiki/README.md)。

星月智能平台的运维工具箱，以交互式终端菜单提供资源补全、端口释放、HTTPS 代理与分卷打包四项功能。

---

## 功能概述

| 功能 | 说明 |
|------|------|
| 资源补全修复 | 从内嵌资源释放缺失的 `local_data` 文件（audios/、images/、package/ 等），仅补缺失、不覆盖已有 |
| 端口占用释放 | 扫描指定端口范围，终止占用端口的进程，自动验证释放结果 |
| HTTPS 代理服务 | HTTPS → HTTP 反向代理：自动生成/持久化 TLS 证书、WebSocket 升级检测 + TCP 隧道转发、CORS 中间件、健康检查 |
| 分卷打包归档 | 将项目文件打包为 7z 分卷压缩包，支持包含/排除路径配置 |

---

## 使用方式

### 编译

```powershell
cd d:\Lunar_Astral_Agents\subsystem\environment_repair
.\build.ps1
```

编译产物：`d:\Lunar_Astral_Agents\Environment_Repair.exe`

### 运行

```powershell
.\Environment_Repair.exe
```

启动后按菜单选择功能：

```
[1] 资源补全修复   从内嵌资源释放缺失的 local_data 文件
[2] 端口占用释放   扫描并终止占用端口的进程
[3] HTTPS 代理服务 启动 HTTPS → HTTP 反向代理（自动生成 TLS 证书）
[4] 分卷打包归档   打包为 7z 分卷压缩包
[5] 退出
```

---

## 依赖

- **7z**：仅分卷打包功能需要
- **TLS 证书**：自动生成（自签名）并持久化到 `GeneralConfig.CertFile` / `KeyFile` 指定路径，二次启动复用

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [配置管理子系统](../general_config/README.md) —— 端口范围、证书路径等配置
