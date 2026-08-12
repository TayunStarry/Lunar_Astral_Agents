param(
    [Parameter(Mandatory=$true)][string]$InputFile,
    [Parameter(Mandatory=$true)][string]$OutputFile
)

Get-Content -Path $InputFile | ForEach-Object {
    'R"({0})"' -f $_
} | Set-Content -Path $OutputFile -Encoding UTF8
