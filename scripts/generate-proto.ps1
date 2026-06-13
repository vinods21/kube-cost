$ErrorActionPreference = "Stop"

if (-not (Get-Command protoc -ErrorAction SilentlyContinue)) {
    throw "protoc is required"
}

New-Item -ItemType Directory -Force proto/gen/go | Out-Null
$protocGenGo = (& go tool -n protoc-gen-go).Trim()
$protocGenGoGrpc = (& go tool -n protoc-gen-go-grpc).Trim()
$files = Get-ChildItem proto/cost -Recurse -Filter *.proto | ForEach-Object {
    $_.FullName.Substring((Resolve-Path proto).Path.Length + 1)
}

Push-Location proto
try {
    protoc -I . `
        "--plugin=protoc-gen-go=$protocGenGo" `
        "--plugin=protoc-gen-go-grpc=$protocGenGoGrpc" `
        --go_out=gen/go `
        --go_opt=paths=source_relative `
        --go-grpc_out=gen/go `
        --go-grpc_opt=paths=source_relative `
        $files
}
finally {
    Pop-Location
}
