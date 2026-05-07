<#
.SYNOPSIS
    PNG to WebP Converter - Build Automation Script
.DESCRIPTION
    This script compiles the PNG to WebP Go program with CGO support.
    Includes environment checks, error handling and logging.
.NOTES
    Author: Lunar Astral
    Version: 1.0.0
    Date: 2026-05-07
#>

param(
    [string]$TargetOS = "windows",
    [string]$TargetArch = "amd64",
    [string]$OutputName = "png2webp",
    [string]$BuildTags = "cgo",
    [string]$LogFile = "build.log"
)

$ErrorActionPreference = "Stop"

function Write-Log {
    param(
        [string]$Message,
        [string]$Level = "INFO"
    )
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $logEntry = "[$timestamp] [$Level] $Message"
    Write-Host $logEntry
    Add-Content -Path $LogFile -Value $logEntry
}

function Test-CommandExists {
    param([string]$Command)
    $exists = $null -ne (Get-Command $Command -ErrorAction SilentlyContinue)
    return $exists
}

try {
    Write-Log -Message "=== Starting PNG to WebP Converter Build ===" -Level "INFO"
    
    Write-Log -Message "Target: $TargetOS/$TargetArch" -Level "INFO"
    Write-Log -Message "Output: $OutputName" -Level "INFO"

    Write-Log -Message "Checking Go environment..." -Level "INFO"
    if (-not (Test-CommandExists "go")) {
        throw "Go compiler not found. Please install Go and add to PATH."
    }
    
    $goVersion = go version
    Write-Log -Message "Go Version: $goVersion" -Level "INFO"

    Write-Log -Message "Checking GCC compiler (required for CGO)..." -Level "INFO"
    if (-not (Test-CommandExists "gcc")) {
        Write-Log -Message "WARNING: GCC compiler not found. CGO build may fail." -Level "WARNING"
    }

    Write-Log -Message "Setting environment variables..." -Level "INFO"
    
    $env:CGO_ENABLED = "1"
    Write-Log -Message "CGO_ENABLED = $($env:CGO_ENABLED)" -Level "INFO"

    $env:GOOS = $TargetOS
    Write-Log -Message "GOOS = $($env:GOOS)" -Level "INFO"

    $env:GOARCH = $TargetArch
    Write-Log -Message "GOARCH = $($env:GOARCH)" -Level "INFO"

    Write-Log -Message "Downloading dependencies..." -Level "INFO"
    go mod download
    if ($LASTEXITCODE -ne 0) {
        throw "Dependency download failed with exit code: $LASTEXITCODE"
    }
    Write-Log -Message "Dependencies downloaded successfully" -Level "INFO"

    Write-Log -Message "Updating go.sum..." -Level "INFO"
    go get .
    if ($LASTEXITCODE -ne 0) {
        throw "go get failed with exit code: $LASTEXITCODE"
    }

    Write-Log -Message "Starting build..." -Level "INFO"
    
    $outputExt = if ($TargetOS -eq "windows") { ".exe" } else { "" }
    $outputPath = "$OutputName$outputExt"
    
    $buildCommand = "go build -tags $BuildTags -o $outputPath ."
    Write-Log -Message "Build command: $buildCommand" -Level "INFO"
    
    Invoke-Expression $buildCommand
    if ($LASTEXITCODE -ne 0) {
        throw "Build failed with exit code: $LASTEXITCODE"
    }

    Write-Log -Message "Build successful!" -Level "INFO"
    
    $outputFullPath = Join-Path (Get-Location) $outputPath
    Write-Log -Message "Output file: $outputFullPath" -Level "INFO"
    
    $fileInfo = Get-Item $outputPath -ErrorAction SilentlyContinue
    if ($fileInfo) {
        $fileSize = [math]::Round($fileInfo.Length / 1KB, 2)
        Write-Log -Message "File size: ${fileSize} KB" -Level "INFO"
    }

    Write-Log -Message "=== Build Completed Successfully ===" -Level "INFO"
    
    Write-Host ""
    Write-Host "============================================="
    Write-Host "  BUILD SUCCESSFUL!"
    Write-Host "  Output: $outputFullPath"
    Write-Host "  Log File: $LogFile"
    Write-Host "============================================="
    Write-Host ""

} catch {
    Write-Log -Message "Build failed: $_" -Level "ERROR"
    
    Write-Host ""
    Write-Host "============================================="
    Write-Host "  BUILD FAILED!"
    Write-Host "  Error: $_"
    Write-Host "  Log File: $LogFile"
    Write-Host "============================================="
    Write-Host ""
    
    exit 1
}
