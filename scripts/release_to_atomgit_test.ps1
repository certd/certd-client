$ErrorActionPreference = "Stop"

$workflowPath = Join-Path $PSScriptRoot "..\.github\workflows\release-to-atomgit.yml"
$workflow = [System.IO.File]::ReadAllText($workflowPath, [System.Text.Encoding]::UTF8)

if ($workflow -match "published") {
    throw "AtomGit Release request must not use the published status."
}

if ($workflow -notmatch "release_status.*latest") {
    throw "AtomGit Release request must explicitly use the latest status."
}

Write-Host "AtomGit Release request check passed"
