$ErrorActionPreference = "Stop"

$scriptPath = Join-Path $PSScriptRoot "install.ps1"
$bytes = [System.IO.File]::ReadAllBytes($scriptPath)

if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
    throw "install.ps1 不能使用 UTF-8 BOM，否则 irm | iex 会将 BOM 作为脚本首字符并解析失败"
}

$content = [System.Text.Encoding]::UTF8.GetString($bytes)
if (-not $content.StartsWith("[CmdletBinding()]")) {
    throw "install.ps1 必须以 [CmdletBinding()] 作为首个字符，确保可通过 irm | iex 运行"
}

Write-Host "install.ps1 编码检查通过"
