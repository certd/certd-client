[CmdletBinding()]
param(
    [string]$InstallDir,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ClientArgs
)

$ErrorActionPreference = "Stop"
$repository = if ($env:CERTD_CLIENT_REPOSITORY) { $env:CERTD_CLIENT_REPOSITORY } else { "certd/certd-client" }
if (-not $InstallDir) {
    $defaultDir = Join-Path (Get-Location) "certd-client"
    $inputDir = Read-Host "安装目录（默认：$defaultDir）"
    $InstallDir = if ($inputDir) { $inputDir } else { $defaultDir }
}

$processorArchitecture = $env:PROCESSOR_ARCHITEW6432
if (-not $processorArchitecture) {
    $processorArchitecture = $env:PROCESSOR_ARCHITECTURE
}
$architecture = switch -Regex ($processorArchitecture) {
    "^(AMD64|X64)$" { "amd64" }
    "^ARM64$" { "arm64" }
    default {
        if ([Environment]::Is64BitOperatingSystem) {
            "amd64"
        }
        else {
            throw "不支持的 CPU 架构：$processorArchitecture"
        }
    }
}
$asset = "certd-client-windows-$architecture.zip"
$githubUrl = "https://github.com/$repository/releases/latest/download/$asset"
$atomGitUrl = "https://atomgit.com/$repository/-/releases/permalink/latest/downloads/$asset"

function Measure-Endpoint {
    param([string]$Url)

    try {
        $watch = [System.Diagnostics.Stopwatch]::StartNew()
        Invoke-WebRequest -Uri $Url -Method Head -MaximumRedirection 5 -UseBasicParsing | Out-Null
        $watch.Stop()
        return $watch.Elapsed.TotalMilliseconds
    }
    catch {
        return [double]::PositiveInfinity
    }
}

$githubTime = Measure-Endpoint $githubUrl
$atomGitTime = Measure-Endpoint $atomGitUrl
$sources = @()
if ($atomGitTime -lt $githubTime) {
    $sources = @(@{ Name = "AtomGit"; Url = $atomGitUrl }, @{ Name = "GitHub"; Url = $githubUrl })
} else {
    $sources = @(@{ Name = "GitHub"; Url = $githubUrl }, @{ Name = "AtomGit"; Url = $atomGitUrl })
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$archive = Join-Path ([System.IO.Path]::GetTempPath()) ("certd-client-" + [guid]::NewGuid() + ".zip")
try {
    $downloaded = $false
    foreach ($source in $sources) {
        try {
            Write-Host "从 $($source.Name) 下载 $asset..."
            Invoke-WebRequest -Uri $source.Url -OutFile $archive -UseBasicParsing
            $downloaded = $true
            break
        }
        catch {
            Write-Warning "$($source.Name) 下载失败：$($_.Exception.Message)"
        }
    }
    if (-not $downloaded) {
        throw "GitHub 和 AtomGit 均下载失败"
    }
    Expand-Archive -Path $archive -DestinationPath $InstallDir -Force
} finally {
    Remove-Item -LiteralPath $archive -Force -ErrorAction SilentlyContinue
}

$binary = Join-Path $InstallDir "certd-client.exe"
if (-not (Test-Path $binary)) {
    throw "安装包中未找到 certd-client.exe"
}
Write-Host "安装或更新完成：$binary"
& $binary @ClientArgs
exit $LASTEXITCODE
