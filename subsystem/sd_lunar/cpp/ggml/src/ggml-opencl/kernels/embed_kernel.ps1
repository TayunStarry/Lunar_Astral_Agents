param(
    [Parameter(Mandatory=$true)]
    [string]$input_file,
    [Parameter(Mandatory=$true)]
    [string]$output_file
)

if (-not (Test-Path $input_file)) {
    Write-Error "Input file not found: $input_file"
    exit 1
}

$outputDir = [System.IO.Path]::GetDirectoryName($output_file)
if (-not (Test-Path $outputDir)) {
    New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
}

$inputContent = Get-Content $input_file -Encoding UTF8
$outputLines = @()

foreach ($line in $inputContent) {
    $escapedLine = $line -replace '\)', ')\)'
    $outputLines += "R`"({0})`"" -f $line
}

$outputLines -join "`n" | Out-File -FilePath $output_file -Encoding UTF8 -NoNewline
