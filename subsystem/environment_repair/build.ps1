# Environment Repair - 编译脚本

# 将资源修复和端口释放功能编译为独立可执行文件，输出到项目根目录

#

# 用法: .\build.ps1 [-TargetOS windows|linux|darwin] [-TargetArch amd64|arm64]



param(

    [ValidateSet("windows", "linux", "darwin")]

    [string]$TargetOS = "windows",

    [ValidateSet("amd64", "arm64")]

    [string]$TargetArch = "amd64"

)



$ErrorActionPreference = "Stop"



# ---------- 同步嵌入资源 ----------

# 从 ../../local_data 同步需要嵌入二进制的资源到 embedded_data/

# 规则：

#   1. audios/ 整个目录

#   2. images/background/ 与 images/placeholder/

#   3. package/ 下所有没有 metadata.json 的子目录（库/资源目录）

#   4. package/ 下直接放置的裸露文件（如 *.js）

# 同步前会清空 embedded_data/，确保没有过时残留

function Sync-EmbeddedData {

    $sourceBase = Join-Path (Join-Path ".." "..") "local_data"

    $targetBase = "embedded_data"



    if (-not (Test-Path $sourceBase)) {

        throw "本地资源目录不存在: $sourceBase"

    }



    # 清空旧目录，避免残留过时文件

    if (Test-Path $targetBase) {

        Remove-Item $targetBase -Recurse -Force

    }

    New-Item -ItemType Directory -Path $targetBase -Force | Out-Null



    $syncCount = 0



    # 1. 同步 audios 目录

    $srcAudios = Join-Path $sourceBase "audios"

    if (Test-Path $srcAudios) {

        Copy-Item -Path $srcAudios -Destination (Join-Path $targetBase "audios") -Recurse -Force

        $syncCount++

        Write-Host "    同步 audios/" -ForegroundColor Gray

    }



    # 2. 同步 images/background 与 images/placeholder

    $imgDirs = @("background", "placeholder")

    foreach ($imgDir in $imgDirs) {

        $srcImg = Join-Path $sourceBase "images\$imgDir"

        if (Test-Path $srcImg) {

            $dstParent = Join-Path $targetBase "images"

            if (-not (Test-Path $dstParent)) {

                New-Item -ItemType Directory -Path $dstParent -Force | Out-Null

            }

            Copy-Item -Path $srcImg -Destination (Join-Path $dstParent $imgDir) -Recurse -Force

            $syncCount++

            Write-Host "    同步 images/$imgDir/" -ForegroundColor Gray

        }

    }



    # 3 & 4. 同步 package 目录

    $srcPackage = Join-Path $sourceBase "package"

    $dstPackage = Join-Path $targetBase "package"

    New-Item -ItemType Directory -Path $dstPackage -Force | Out-Null



    if (Test-Path $srcPackage) {

        # 处理子目录：仅复制没有 metadata.json 的（库/资源目录）

        $subDirs = Get-ChildItem -Path $srcPackage -Directory

        foreach ($subDir in $subDirs) {

            $metadataPath = Join-Path $subDir.FullName "metadata.json"

            if (-not (Test-Path $metadataPath)) {

                Copy-Item -Path $subDir.FullName -Destination $dstPackage -Recurse -Force

                $syncCount++

                Write-Host "    同步 package/$($subDir.Name)/" -ForegroundColor Gray

            }

        }



        # 处理裸露文件：复制 package 根目录下的代码文件（排除压缩包等非代码资源）

        $files = Get-ChildItem -Path $srcPackage -File

        $excludeExts = @(".7z", ".zip", ".rar", ".tar", ".gz")

        foreach ($file in $files) {

            $ext = $file.Extension.ToLower()

            $shouldCopy = $true

            foreach ($exExt in $excludeExts) {

                if ($ext -eq $exExt) {

                    $shouldCopy = $false

                    break

                }

            }

            if ($shouldCopy) {

                Copy-Item -Path $file.FullName -Destination $dstPackage -Force

                $syncCount++

                Write-Host "    同步 package/$($file.Name)" -ForegroundColor Gray

            }

        }

    }



    if ($syncCount -eq 0) {

        throw "未同步任何资源，请检查 ../../local_data 目录结构"

    }



    Write-Host "嵌入资源同步完成（共 $syncCount 项）" -ForegroundColor Green

}



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

# ---------- 编译主流程 ----------

try {

    # 同步嵌入资源（必须在编译前完成，否则 //go:embed 找不到文件）

    Sync-EmbeddedData



    # 启用 CGO
    $env:CGO_ENABLED = 1
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch

    # 编译图标资源（Windows 下嵌入 .ico 到可执行文件）
    Build-IconIfNeeded



    # 构建可执行文件（CLI 工具，不隐藏控制台窗口）

    $binaryName = "Environment_Repair.exe"

    if ($TargetOS -ne "windows") { $binaryName = "Environment_Repair" }

    $outputPath = "..\..\$binaryName"



    # CLI 工具不使用 -H windowsgui（保留控制台窗口用于交互）

    $ldflags = "-s -w"



    $buildArgs = @(

        "build",

        "-ldflags=$ldflags",

        "-trimpath",

        "-o", $outputPath

    )

    & go $buildArgs 2>&1 | Out-Host

    if ($LASTEXITCODE -ne 0) { throw "Go build 失败" }



    Write-Host "Environment Repair 构建成功: $outputPath" -ForegroundColor Green

}

catch {

    Write-Host "`n[ERROR] Environment Repair 构建失败: $_" -ForegroundColor Red

    exit 1

}