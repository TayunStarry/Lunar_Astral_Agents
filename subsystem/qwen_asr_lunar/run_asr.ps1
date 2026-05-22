# qwen_asr 语音转文字脚本
param(
    [string]$AudioFile = "audio.wav",
    [string]$OutputFile = ""
)

$MODEL_DIR = "C:\Users\196530\Downloads\Qwen3-ASR-0.6B-0"

# 自动生成输出文件名
if ($OutputFile -eq "") {
    $OutputFile = [System.IO.Path]::ChangeExtension($AudioFile, ".txt")
}

Write-Host "开始转写音频: $AudioFile" -ForegroundColor Cyan
Write-Host "模型目录: $MODEL_DIR" -ForegroundColor Cyan
Write-Host "输出文件: $OutputFile" -ForegroundColor Cyan
Write-Host "----------------------------------------" -ForegroundColor Gray

# 运行 qwen_asr，--silent 将状态信息屏蔽，stdout 重定向到文件
& .\asr_lunar.exe -d $MODEL_DIR -i $AudioFile --silent | Out-File -FilePath $OutputFile -Encoding utf8

Write-Host "----------------------------------------" -ForegroundColor Gray
Write-Host "转写完成! 结果已保存到: $OutputFile" -ForegroundColor Green
