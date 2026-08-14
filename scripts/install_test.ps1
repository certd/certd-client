$ErrorActionPreference = "Stop"

$scriptPath = Join-Path $PSScriptRoot "install.ps1"
$bytes = [System.IO.File]::ReadAllBytes($scriptPath)

if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
    throw "install.ps1 must not use a UTF-8 BOM because irm | iex treats it as the first script character."
}

$content = [System.Text.Encoding]::UTF8.GetString($bytes)
if (-not $content.StartsWith("[CmdletBinding()]")) {
    throw "install.ps1 must start with [CmdletBinding()] for irm | iex."
}

if ($content -match "RuntimeInformation\]::OSArchitecture\.ToString") {
    throw "install.ps1 must not rely on RuntimeInformation.OSArchitecture because older Windows/.NET can return null."
}

if ($content -notmatch "PROCESSOR_ARCHITEW6432") {
    throw "install.ps1 must support architecture detection from 32-bit PowerShell on 64-bit Windows."
}

Write-Host "install.ps1 checks passed"
