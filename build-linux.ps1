$ErrorActionPreference = "Stop"

$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH

try {
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"

    New-Item -ItemType Directory -Force -Path "dist" | Out-Null
    Remove-Item "dist/certd-client" -Force -ErrorAction SilentlyContinue
    Remove-Item "dist/certd-client-linux-amd64" -Force -ErrorAction SilentlyContinue
    go build -o "dist/certd-client" ./cmd/certd-client
}
finally {
    if ($null -eq $previousGoos) {
        Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    } else {
        $env:GOOS = $previousGoos
    }

    if ($null -eq $previousGoarch) {
        Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    } else {
        $env:GOARCH = $previousGoarch
    }
}
