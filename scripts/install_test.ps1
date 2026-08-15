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

if ($content -notmatch "New-Object byte\[\] 2") {
    throw "install.ps1 must validate downloaded archives before extraction."
}

if ($content -notmatch "Test-ZipArchive") {
    throw "install.ps1 must fall back when a download is not a valid ZIP archive."
}

if ($content -notmatch "GetFileName") {
    throw "install.ps1 must avoid appending certd-client when the current directory already has that name."
}

if ($content -match "/-/releases/permalink/latest/downloads") {
    throw "install.ps1 must not use the invalid AtomGit permalink download URL."
}

if ($content -notmatch "api\.atomgit\.com/api/v5/repos/.*/releases/latest") {
    throw "install.ps1 must resolve the latest AtomGit release through its API."
}

if ($content -notmatch "browser_download_url") {
    throw "install.ps1 must use the AtomGit asset download URL returned by the API."
}

Write-Host "install.ps1 checks passed"
