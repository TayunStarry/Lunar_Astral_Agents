# Luna Astral - 编译脚本
# 由根目录脚本统一调用，仅处理图标编译和项目编译

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",
    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64"
)

$ErrorActionPreference = "Stop"

# ---------- 图标资源处理 ----------
function Build-IconIfNeeded {
    if ($TargetOS -ne "windows" -or -not (Test-Path "icon.ico")) {
        return
    }

    if (Test-Path "icon.syso") {
        return
    }

    & rsrc -ico icon.ico -o icon.syso
    if ($LASTEXITCODE -ne 0) { throw "rsrc 图标编译失败" }
}

# ---------- 辅助函数：执行命令并保留退出码 ----------
function Invoke-NativeCommand {
    param(
        [scriptblock]$ScriptBlock
    )
    
    # 临时将 ErrorActionPreference 改为 Continue，避免 stderr 输出被当作错误
    $originalEAP = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    
    try {
        & $ScriptBlock 2>&1 | ForEach-Object {
            if ($_ -is [System.Management.Automation.ErrorRecord]) {
                Write-Host $_.Exception.Message -ForegroundColor DarkYellow
            } else {
                Write-Host $_
            }
        }
        return $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $originalEAP
    }
}

# ---------- 编译主流程 ----------
try {
    # 编译图标
    Build-IconIfNeeded

    # 启用CGO
    $env:CGO_ENABLED = 1
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_CFLAGS = "-w"
    $env:CGO_LDFLAGS = "-static-libgcc -static-libstdc++ -Wl,-Bstatic,-lwinpthread,-Bdynamic"

    # 编译服务端脚本 
    $exitCode = Invoke-NativeCommand { npm run server.side }
    if ($exitCode -ne 0) { throw "npm server.side 执行失败" }

    # 构建可执行文件
    $binaryName = "Lunar_Astral.exe"
    if ($TargetOS -ne "windows") { $binaryName = "Lunar_Astral" }
    $outputPath = "..\$binaryName"

    $ldflags = "-s -w -extldflags=-Wl,-Bstatic,-lstdc++,-lgcc,-lgcc_eh,-lwinpthread,-Bdynamic"
    $buildArgs = @(
        "build",
        "-tags", "webview",
        "-ldflags", $ldflags,
        "-o", $outputPath
    )
    $exitCode = Invoke-NativeCommand { go $buildArgs }
    if ($exitCode -ne 0) { throw "Go build 失败" }

    Write-Host "✓ Luna Astral 构建成功: $outputPath" -ForegroundColor Green
}
catch {
    Write-Host "`n[ERROR] Luna Astral 构建失败: $_" -ForegroundColor Red
    exit 1
}
