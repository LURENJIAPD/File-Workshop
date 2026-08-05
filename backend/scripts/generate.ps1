$ErrorActionPreference = 'Stop'

$backendRoot = Split-Path -Parent $PSScriptRoot
Push-Location $backendRoot
try {
    go tool goose -dir migrations validate
    if ($LASTEXITCODE -ne 0) { throw "Goose migration validation failed with exit code $LASTEXITCODE" }

    go tool sqlc generate -f sqlc.yaml
    if ($LASTEXITCODE -ne 0) { throw "sqlc generation failed with exit code $LASTEXITCODE" }

    Push-Location (Join-Path $backendRoot 'api')
    try {
        go tool oapi-codegen -config oapi-codegen.yaml openapi.yaml
        if ($LASTEXITCODE -ne 0) { throw "OpenAPI generation failed with exit code $LASTEXITCODE" }
    }
    finally {
        Pop-Location
    }
}
finally {
    Pop-Location
}

