[CmdletBinding()]
param(
    [string]$InstallDir,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ClientArgs
)

$ErrorActionPreference = "Stop"
$repository = if ($env:CERTD_CLIENT_REPOSITORY) { $env:CERTD_CLIENT_REPOSITORY } else { "certd/certd-client" }
if (-not $InstallDir) {
    $currentDir = (Get-Location).Path
    $defaultDir = if ([System.IO.Path]::GetFileName($currentDir) -ieq "certd-client") {
        $currentDir
    }
    else {
        Join-Path $currentDir "certd-client"
    }
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
$atomGitReleaseApi = "https://api.atomgit.com/api/v5/repos/$repository/releases/latest"
$atomGitUrl = $null
try {
    $atomGitRelease = Invoke-RestMethod -Uri $atomGitReleaseApi -UseBasicParsing
    $atomGitAsset = $atomGitRelease.assets | Where-Object { $_.name -eq $asset } | Select-Object -First 1
    if ($atomGitAsset -and $atomGitAsset.browser_download_url) {
        $atomGitUrl = $atomGitAsset.browser_download_url
    }
    elseif ($atomGitRelease.tag_name) {
        $atomGitUrl = "https://atomgit.com/$repository/releases/download/$($atomGitRelease.tag_name)/$asset"
    }
}
catch {
    Write-Warning "获取 AtomGit 最新版本失败：$($_.Exception.Message)"
}

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

function Test-ZipArchive {
    param([string]$Path)

    $stream = $null
    try {
        $stream = [System.IO.File]::OpenRead($Path)
        $header = New-Object byte[] 2
        $read = $stream.Read($header, 0, $header.Length)
        return $read -eq 2 -and $header[0] -eq 0x50 -and $header[1] -eq 0x4B
    }
    catch {
        return $false
    }
    finally {
        if ($stream) {
            $stream.Dispose()
        }
    }
}

$githubTime = Measure-Endpoint $githubUrl
$atomGitTime = if ($atomGitUrl) { Measure-Endpoint $atomGitReleaseApi } else { [double]::PositiveInfinity }
if ($atomGitUrl -and $atomGitTime -lt $githubTime) {
    $sources = @(@{ Name = "AtomGit"; Url = $atomGitUrl }, @{ Name = "GitHub"; Url = $githubUrl })
}
elseif ($atomGitUrl) {
    $sources = @(@{ Name = "GitHub"; Url = $githubUrl }, @{ Name = "AtomGit"; Url = $atomGitUrl })
}
else {
    $sources = @(@{ Name = "GitHub"; Url = $githubUrl })
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$archive = Join-Path ([System.IO.Path]::GetTempPath()) ("certd-client-" + [guid]::NewGuid() + ".zip")
try {
    $downloaded = $false
    foreach ($source in $sources) {
        try {
            Write-Host "从 $($source.Name) 下载 $asset..."
            Invoke-WebRequest -Uri $source.Url -OutFile $archive -UseBasicParsing
            if (-not (Test-ZipArchive $archive)) {
                throw "$($source.Name) 返回的下载内容不是有效的 ZIP 文件"
            }
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
