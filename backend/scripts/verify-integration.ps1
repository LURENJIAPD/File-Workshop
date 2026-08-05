$ErrorActionPreference = 'Stop'

$backendRoot = Split-Path -Parent $PSScriptRoot
$previousIntegrationValue = [Environment]::GetEnvironmentVariable('FILE_WORKSHOP_RUN_INTEGRATION', 'Process')
Push-Location $backendRoot
try {
    $env:FILE_WORKSHOP_RUN_INTEGRATION = '1'
    go test -p 1 ./tests/... -count=1 -v
    if ($LASTEXITCODE -ne 0) { throw "integration tests failed with exit code $LASTEXITCODE" }
}
finally {
    [Environment]::SetEnvironmentVariable('FILE_WORKSHOP_RUN_INTEGRATION', $previousIntegrationValue, 'Process')
    Pop-Location
}
