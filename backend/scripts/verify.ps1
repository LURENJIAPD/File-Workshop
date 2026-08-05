$ErrorActionPreference = 'Stop'

$backendRoot = Split-Path -Parent $PSScriptRoot
Push-Location $backendRoot
try {
    $requiredGoVersion = 'go1.26.5'
    $actualGoVersion = (go env GOVERSION).Trim()
    if ($LASTEXITCODE -ne 0) { throw "read Go version failed with exit code $LASTEXITCODE" }
    if ($actualGoVersion -ne $requiredGoVersion) {
        throw "Go version must be $requiredGoVersion, actual version is $actualGoVersion"
    }

    & (Join-Path $PSScriptRoot 'generate.ps1')

    & (Join-Path $PSScriptRoot 'verify-api-doc.ps1')

    $goFiles = @(rg --files -g '*.go')
    $unformatted = @(gofmt -l $goFiles)
    if ($unformatted.Count -gt 0) {
        throw "The following Go files require gofmt:`n$($unformatted -join [Environment]::NewLine)"
    }

    go mod verify
    if ($LASTEXITCODE -ne 0) { throw "go mod verify failed with exit code $LASTEXITCODE" }

    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw "go vet failed with exit code $LASTEXITCODE" }

    go test ./...
    if ($LASTEXITCODE -ne 0) { throw "go test failed with exit code $LASTEXITCODE" }

    go build ./...
    if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }

    & (Join-Path $PSScriptRoot 'vulnerability-check.ps1')
}
finally {
    Pop-Location
}
