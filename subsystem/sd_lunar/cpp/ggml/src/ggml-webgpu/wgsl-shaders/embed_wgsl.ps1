param(
    [Parameter(Mandatory=$true)]
    [string]$input_dir,
    [Parameter(Mandatory=$true)]
    [string]$output_file
)

function Expand-Includes {
    param(
        [string]$shaderCode,
        [string]$inputDir
    )
    
    $includePattern = '^\s*#include\s+"([^"]+)"\s*$'
    $lines = $shaderCode -split "`n"
    $result = @()
    
    foreach ($line in $lines) {
        if ($line -match $includePattern) {
            $fileName = $matches[1]
            $filePath = Join-Path $inputDir $fileName
            if (Test-Path $filePath) {
                $includedCode = Get-Content $filePath -Raw -Encoding UTF8
                $expandedCode = Expand-Includes -shaderCode $includedCode -inputDir $inputDir
                $result += $expandedCode
            } else {
                Write-Error "Included file not found: $filePath"
                exit 1
            }
        } else {
            $result += $line
        }
    }
    
    return $result -join "`n"
}

function Get-Delimiter {
    param([string]$shaderCode)
    
    $delim = "wgsl"
    while ($shaderCode -like "*`"$delim`"*") {
        $delim += "_x"
    }
    return $delim
}

function Write-Shader {
    param(
        [string]$shaderName,
        [string]$shaderCode,
        [System.IO.StreamWriter]$outFile,
        [string]$inputDir
    )
    
    $shaderCode = Expand-Includes -shaderCode $shaderCode -inputDir $inputDir
    $delim = Get-Delimiter -shaderCode $shaderCode
    
    $maxChunkLen = 60000
    $chunks = @()
    for ($i = 0; $i -lt $shaderCode.Length; $i += $maxChunkLen) {
        $len = [Math]::Min($maxChunkLen, $shaderCode.Length - $i)
        $chunks += $shaderCode.Substring($i, $len)
    }
    
    if ($chunks.Count -eq 1) {
        $outFile.WriteLine("const char* wgsl_{0} = R`"{1}({2}){1}`";`n" -f $shaderName, $delim, $shaderCode)
    } else {
        for ($idx = 0; $idx -lt $chunks.Count; $idx++) {
            $outFile.WriteLine("static const char wgsl_{0}_part{1}[] = R`"{2}({3}){2}`";`n" -f $shaderName, $idx, $delim, $chunks[$idx])
        }
        $outFile.WriteLine("static const std::string& wgsl_{0}_str() {{" -f $shaderName)
        $outFile.WriteLine("    static const std::string s = []{")
        $outFile.WriteLine("        std::string tmp;")
        $outFile.WriteLine("        tmp.reserve({0});" -f $shaderCode.Length)
        for ($idx = 0; $idx -lt $chunks.Count; $idx++) {
            $outFile.WriteLine("        tmp.append(wgsl_{0}_part{1});" -f $shaderName, $idx)
        }
        $outFile.WriteLine("        return tmp;")
        $outFile.WriteLine("    }();")
        $outFile.WriteLine("    return s;")
        $outFile.WriteLine("}")
        $outFile.WriteLine("const char* wgsl_{0} = wgsl_{0}_str().c_str();`n" -f $shaderName)
    }
}

$outputDir = [System.IO.Path]::GetDirectoryName($output_file)
if (-not (Test-Path $outputDir)) {
    New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
}

$outStream = New-Object System.IO.StreamWriter($output_file, $false, [System.Text.Encoding]::UTF8)
try {
    $outStream.WriteLine("// Auto-generated shader embedding")
    $outStream.WriteLine("#include <string>`n")
    
    $wgslFiles = Get-ChildItem -Path $input_dir -Filter "*.wgsl" | Sort-Object Name
    foreach ($file in $wgslFiles) {
        $shaderCode = Get-Content $file.FullName -Raw -Encoding UTF8
        $shaderName = [System.IO.Path]::GetFileNameWithoutExtension($file.Name)
        Write-Shader -shaderName $shaderName -shaderCode $shaderCode -outFile $outStream -inputDir $input_dir
    }
} finally {
    $outStream.Close()
}
