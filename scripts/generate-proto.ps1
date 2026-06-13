$ErrorActionPreference = "Stop"

if (-not (Get-Command protoc -ErrorAction SilentlyContinue)) {
    throw "protoc is required"
}
if (-not (Get-Command protoc-gen-go -ErrorAction SilentlyContinue)) {
    throw "protoc-gen-go is required"
}
if (-not (Get-Command protoc-gen-go-grpc -ErrorAction SilentlyContinue)) {
    throw "protoc-gen-go-grpc is required"
}

New-Item -ItemType Directory -Force proto/gen/go | Out-Null
$files = Get-ChildItem proto/cost -Recurse -Filter *.proto | ForEach-Object {
    $_.FullName.Substring((Resolve-Path proto).Path.Length + 1)
}

Push-Location proto
try {
    protoc -I . --go_out=gen/go --go_opt=paths=source_relative --go-grpc_out=gen/go --go-grpc_opt=paths=source_relative $files
}
finally {
    Pop-Location
}
