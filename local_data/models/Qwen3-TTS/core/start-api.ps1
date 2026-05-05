Write-Host "正在启动 Qwen TTS 服务..."
$ScriptPath = Split-Path -Parent $PSScriptRoot
& (Join-Path $ScriptPath "activate.ps1") -NoPause
Write-Host "环境激活成功"
Write-Host "正在启动 Qwen TTS Lunar API 服务..."
& python -m 'QwenTTS-Lunar_API' --host 0.0.0.0 --port 7860