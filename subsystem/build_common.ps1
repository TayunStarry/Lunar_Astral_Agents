# 构建脚本共享辅助 —— 由各 build.ps1 点源引入
#
# 用法（放到脚本靠前位置，$PSScriptRoot 可用之后）：
#     $repoRoot = $PSScriptRoot
#     while ($repoRoot -and -not (Test-Path (Join-Path $repoRoot "subsystem\build_common.ps1"))) {
#         $repoRoot = Split-Path -Parent $repoRoot
#     }
#     if (-not $repoRoot) { throw "未找到仓库根目录（subsystem\build_common.ps1）" }
#     . (Join-Path $repoRoot "subsystem\build_common.ps1")
#
# 背景：Go 工具链定位逻辑此前被复制在 agent_search / ltp3_keygen / ltp9_keygen 三个脚本里，
# 且那份实现存在缺陷（安装多个 Go SDK 时会返回数组，见 Find-Go 注释）。这里收敛为唯一实现。

# 将控制台输出编码统一为 UTF-8：Go/npm 等原生程序经管道输出 UTF-8 字节时，
# PowerShell 默认按系统 OEM 代码页（中文系统为 GBK）解码，会把
# "✓ 端点自述文档已生成" 之类输出显示成 "鉁?绔偣..." 乱码。
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch { }

<#
.SYNOPSIS
    定位 Go 工具链可执行文件。
.DESCRIPTION
    查找顺序：PATH -> 常见安装目录 -> go SDK 布局 %USERPROFILE%\sdk\go<版本>（版本号大者优先）。
    始终返回【单个】路径字符串；找不到返回 $null。

    注意：不要用管道 + ForEach-Object 直接输出候选路径。安装多个 Go SDK（例如同时有
    sdk\go1.25.0 与 sdk\go1.26.2）时，那会把多条路径一起输出成数组，调用方 & $Go 时
    PowerShell 会把整个数组拼成一个非法命令名，报 CommandNotFoundException。
    这里显式只挑一个，并按版本号取最新。
#>
function Find-Go {
    # 1) PATH（Windows 上优先 go.exe，其它平台退回 go）
    $cmd = Get-Command go.exe -ErrorAction SilentlyContinue
    if (-not $cmd) { $cmd = Get-Command go -ErrorAction SilentlyContinue }
    if ($cmd -and $cmd.Source) { return $cmd.Source }

    # 2) 常见安装目录
    foreach ($c in @(
            "C:\Program Files\Go\bin\go.exe",
            "C:\Go\bin\go.exe",
            (Join-Path $env:USERPROFILE "go\bin\go.exe")
        )) {
        if ($c -and (Test-Path $c)) { return $c }
    }

    # 3) %USERPROFILE%\sdk\goX.Y.Z —— 取版本号最大的一个
    if ($env:USERPROFILE) {
        $sdkRoot = Join-Path $env:USERPROFILE "sdk"
        if (Test-Path $sdkRoot) {
            $best = $null
            $bestVer = $null
            foreach ($d in @(Get-ChildItem $sdkRoot -Directory -Filter "go*" -ErrorAction SilentlyContinue)) {
                $exe = Join-Path $d.FullName "bin\go.exe"
                if (-not (Test-Path $exe)) { continue }

                $ver = $null
                if ($d.Name -match '^go(\d+(?:\.\d+)*)$') {
                    try { $ver = [version]$matches[1] } catch { $ver = $null }
                }

                if (-not $best) { $best = $exe; $bestVer = $ver; continue }
                if ($ver -and (-not $bestVer -or $ver -gt $bestVer)) { $best = $exe; $bestVer = $ver }
            }
            if ($best) { return $best }
        }
    }

    return $null
}

<#
.SYNOPSIS
    取 Go 工具链并校验存在，返回可执行文件路径；找不到则抛出可读错误。
#>
function Resolve-Go {
    param([string]$Purpose = "构建")

    $go = Find-Go
    if (-not $go) {
        throw "未找到 Go 工具链（$Purpose）：已查找 PATH、常见安装目录与 %USERPROFILE%\sdk\go*，请安装 Go 或将其加入 PATH。"
    }
    return $go
}
