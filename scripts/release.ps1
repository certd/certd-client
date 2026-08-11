[CmdletBinding()]
param(
    [ValidateSet("auto", "major", "minor", "patch")]
    [string]$Bump = "auto",
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Invoke-GitText {
    param([Parameter(Mandatory = $true)][string[]]$Arguments)

    $startInfo = New-Object System.Diagnostics.ProcessStartInfo
    $startInfo.FileName = "git"
    $startInfo.Arguments = (($Arguments | ForEach-Object { '"' + $_.Replace('"', '\"') + '"' }) -join " ")
    $startInfo.UseShellExecute = $false
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $startInfo.StandardOutputEncoding = New-Object System.Text.UTF8Encoding($false)
    $startInfo.StandardErrorEncoding = New-Object System.Text.UTF8Encoding($false)

    $process = New-Object System.Diagnostics.Process
    $process.StartInfo = $startInfo
    if (-not $process.Start()) {
        throw "无法启动 git 命令"
    }
    $output = $process.StandardOutput.ReadToEnd()
    $errorOutput = $process.StandardError.ReadToEnd()
    $process.WaitForExit()

    if ($process.ExitCode -ne 0) {
        throw ("git " + ($Arguments -join " ") + " 执行失败：" + $errorOutput.Trim())
    }
    return $output.Trim()
}

function Set-Utf8NoBomContent {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Content
    )

    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $Content, $encoding)
}

function Parse-SemVer {
    param([Parameter(Mandatory = $true)][string]$Value)

    $semVerMatch = [regex]::Match($Value, '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$')
    if (-not $semVerMatch.Success) {
        throw "版本号不是有效的 Node.js SemVer：$Value"
    }
    foreach ($identifier in $semVerMatch.Groups[4].Value.Split(".")) {
        if ($identifier.Length -gt 1 -and $identifier.StartsWith("0") -and $identifier -match '^[0-9]+$') {
            throw "版本号不是有效的 Node.js SemVer：$Value"
        }
    }
    return [pscustomobject]@{
        Major = [int]$semVerMatch.Groups[1].Value
        Minor = [int]$semVerMatch.Groups[2].Value
        Patch = [int]$semVerMatch.Groups[3].Value
    }
}

function Get-BumpType {
    param(
        [Parameter(Mandatory = $true)][string]$CommitText,
        [Parameter(Mandatory = $true)][string]$Body
    )

    if ($Body -match '(?im)^BREAKING CHANGE(?:S)?\s*:|(?m)^[a-z]+(?:\([^)]*\))?!:') {
        return "major"
    }
    if ($CommitText -match '(?im)^[^\t]+\tfeat(?:\([^)]*\))?:') {
        return "minor"
    }
    if ($CommitText -match '(?im)^[^\t]+\t(fix|perf)(?:\([^)]*\))?:') {
        return "patch"
    }
    throw "最近的提交中没有 feat、fix 或 perf，无法自动决定版本段"
}

function Is-ChangelogCommit {
    param([Parameter(Mandatory = $true)][string]$Commit)

    return $Commit -match '^[^\t]+\t(?:feat|fix|perf)(?:\([^)]*\))?!?:'
}

function Get-NextVersion {
    param(
        [Parameter(Mandatory = $true)][string]$Current,
        [Parameter(Mandatory = $true)][string]$BumpType
    )

    $version = Parse-SemVer $Current
    switch ($BumpType) {
        "major" { return "$($version.Major + 1).0.0" }
        "minor" { return "$($version.Major).$($version.Minor + 1).0" }
        "patch" { return "$($version.Major).$($version.Minor).$($version.Patch + 1)" }
        default { throw "未知版本段：$BumpType" }
    }
}

function Invoke-LocalChecks {
    Write-Host "运行本地测试：go test ./..."
    & go test ./...
    if ($LASTEXITCODE -ne 0) {
        throw "本地测试失败，取消发布"
    }

    Write-Host "运行本地静态检查：go vet ./..."
    & go vet ./...
    if ($LASTEXITCODE -ne 0) {
        throw "本地静态检查失败，取消发布"
    }
}

try {
    $root = Invoke-GitText @("rev-parse", "--show-toplevel")
    Set-Location $root

    $status = Invoke-GitText @("status", "--porcelain")
    if ($status) {
        throw "工作区不干净，请先提交或清理以下变更：`n$status"
    }
    Invoke-LocalChecks

    $versionPath = Join-Path $root "internal/version/version.go"
    $versionSource = Get-Content -Raw -Encoding UTF8 $versionPath
    $versionMatch = [regex]::Match($versionSource, 'var Version = "([^"]+)"')
    if (-not $versionMatch.Success) {
        throw "未找到版本配置：$versionPath"
    }
    $currentVersion = $versionMatch.Groups[1].Value

    $lastTag = ""
    try {
        $lastTag = Invoke-GitText @("describe", "--tags", "--abbrev=0", "--match", "v[0-9]*")
    }
    catch {
        $lastTag = ""
    }
    $range = if ($lastTag) { "$lastTag..HEAD" } else { "HEAD" }
    $commitText = Invoke-GitText @("log", $range, "--format=%H`t%s")
    if (-not $commitText) {
        throw "没有可发布的新提交"
    }
    $body = Invoke-GitText @("log", $range, "--format=%B")
    $bumpType = if ($Bump -eq "auto") { Get-BumpType $commitText $body } else { $Bump }
    $nextVersion = Get-NextVersion $currentVersion $bumpType
    $tag = "v$nextVersion"
    $date = Get-Date -Format "yyyy-MM-dd"

    $entries = @($commitText -split "`n" | Where-Object { $_.Trim() -and (Is-ChangelogCommit $_) })
    $changelogLines = @("## [$nextVersion] - $date", "")
    foreach ($entry in $entries) {
        $parts = $entry -split "`t", 2
        $subject = if ($parts.Count -gt 1) { $parts[1].Trim() } else { $parts[0].Trim() }
        $shortHash = $parts[0].Substring(0, [Math]::Min(8, $parts[0].Length))
        $changelogLines += "- $subject ($shortHash)"
    }
    $changelogLines += ""

    Write-Host "当前版本：$currentVersion"
    Write-Host "版本段：$bumpType"
    Write-Host "目标版本：$nextVersion"
    if ($DryRun) {
        $changelogLines | Write-Host
        exit 0
    }

    $updatedSource = $versionSource.Replace($versionMatch.Value, "var Version = `"$nextVersion`"")
    Set-Utf8NoBomContent -Path $versionPath -Content $updatedSource
    $changelogPath = Join-Path $root "CHANGELOG.md"
    $oldChangelog = if (Test-Path $changelogPath) { Get-Content -Raw -Encoding UTF8 $changelogPath } else { "# Changelog`n`n" }
    Set-Utf8NoBomContent -Path $changelogPath -Content (($changelogLines -join "`n") + $oldChangelog)

    Invoke-GitText @("add", "internal/version/version.go", "CHANGELOG.md") | Out-Null
    Invoke-GitText @("commit", "-m", "chore(release): $tag") | Write-Host
    Invoke-GitText @("tag", "-a", $tag, "-m", "Release $tag") | Out-Null
    $branch = Invoke-GitText @("branch", "--show-current")
    if (-not $branch) {
        throw "当前处于 detached HEAD，无法推送发布提交"
    }
    Invoke-GitText @("push", "origin", $branch) | Write-Host
    Invoke-GitText @("push", "origin", $tag) | Write-Host
    Write-Host "发布已推送：$tag"
}
catch {
    Write-Error $_
    exit 1
}
