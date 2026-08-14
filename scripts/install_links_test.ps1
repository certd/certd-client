$ErrorActionPreference = "Stop"

$readmePath = Join-Path $PSScriptRoot "..\README.md"
$readme = [System.IO.File]::ReadAllText($readmePath, [System.Text.Encoding]::UTF8)

if ($readme -match "raw\.atomgit\.com") {
    throw "README must not use raw.atomgit.com: it does not serve install scripts."
}

if ($readme -notmatch "https://raw\.githubusercontent\.com/certd/certd-client/main/scripts/install\.ps1") {
    throw "README must provide the GitHub PowerShell install script URL."
}

if ($readme -notmatch "https://raw\.githubusercontent\.com/certd/certd-client/main/scripts/install\.sh") {
    throw "README must provide the GitHub Shell install script URL."
}

Write-Host "Install link check passed"
