$env:GOOS="linux"
$env:GOARCH="amd64"
go build -o dist/certd-client-linux-amd64 ./cmd/certd-client
Remove-Item Env:GOOS
Remove-Item Env:GOARCH