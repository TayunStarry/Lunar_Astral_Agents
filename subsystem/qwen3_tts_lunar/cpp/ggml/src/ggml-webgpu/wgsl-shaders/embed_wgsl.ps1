param(
    [Parameter(Mandatory=$true)][string]$InputDir,
    [Parameter(Mandatory=$true)][string]$OutputFile
)

function Expand-Includes {
    param(
        [string]$ShaderCode,
        [string]$BaseDir
    )
    $pattern = '^\s*#include\s+"([^"]+)"\s*$'
    $lines = $ShaderCode -split "`n"
    $result = @()
    foreach ($line in $lines) {
        if ($line -match $pattern) {
            $fname = $Matches[1]
            $filePath = Join-Path $BaseDir $fname
            if (-not (Test-Path $filePath)) {
                throw "Included file not found: $filePath"
            }
            $includedCode = Get-Content -Path $filePath -Raw -Encoding UTF8
            $expanded = Expand-Includes -ShaderCode $includedCode -BaseDir $BaseDir
            $result += $expanded
        } else {
            $result += $line
        }
    }
    return ($result -join "`n")
}

function Get-RawDelimiter {
    param([string]$Code)
    $delim = "wgsl"
    while ($Code -match "\){$delim}""") {
        $delim += "_x"
    }
    return $delim
}

function Write-Shader {
    param(
        [string]$ShaderName,
        [string]$ShaderCode,
        [string]$InputDir,
        [System.IO.StreamWriter]$Out
    )
    $ShaderCode = Expand-Includes -ShaderCode $ShaderCode -BaseDir $InputDir

    $delim = Get-RawDelimiter -Code $ShaderCode
    $maxChunkLen = 60000
    $chunks = @()
    for ($i = 0; $i -lt $ShaderCode.Length; $i += $maxChunkLen) {
        $len = [Math]::Min($maxChunkLen, $ShaderCode.Length - $i)
        $chunks += $ShaderCode.Substring($i, $len)
    }

    if ($chunks.Count -eq 1) {
        $Out.WriteLine("const char* wgsl_{0} = R`"{1}({2}){1}`";" -f $ShaderName, $delim, $ShaderCode)
        $Out.WriteLine()
    } else {
        for ($idx = 0; $idx -lt $chunks.Count; $idx++) {
            $Out.WriteLine("static const char wgsl_{0}_part{1}[] = R`"{2}({3}){2}`";" -f $ShaderName, $idx, $delim, $chunks[$idx])
            $Out.WriteLine()
        }
        $Out.WriteLine("static const std::string& wgsl_{0}_str() {{" -f $ShaderName)
        $Out.WriteLine("    static const std::string s = []{")
        $Out.WriteLine("        std::string tmp;")
        $Out.WriteLine("        tmp.reserve({0});" -f $ShaderCode.Length)
        for ($idx = 0; $idx -lt $chunks.Count; $idx++) {
            $Out.WriteLine("        tmp.append(wgsl_{0}_part{1});" -f $ShaderName, $idx)
        }
        $Out.WriteLine("        return tmp;")
        $Out.WriteLine("    }();")
        $Out.WriteLine("    return s;")
        $Out.WriteLine("}")
        $Out.WriteLine("const char* wgsl_{0} = wgsl_{0}_str().c_str();" -f $ShaderName)
        $Out.WriteLine()
    }
}

$writer = New-Object System.IO.StreamWriter($OutputFile, $false, [System.Text.Encoding]::UTF8)
$writer.WriteLine("// Auto-generated shader embedding")
$writer.WriteLine("#include <string>")
$writer.WriteLine()

$wgslFiles = Get-ChildItem -Path $InputDir -Filter "*.wgsl" | Sort-Object Name
foreach ($file in $wgslFiles) {
    $shaderCode = Get-Content -Path $file.FullName -Raw -Encoding UTF8
    $shaderName = $file.BaseName
    Write-Shader -ShaderName $shaderName -ShaderCode $shaderCode -InputDir $InputDir -Out $writer
}

$writer.Close()
