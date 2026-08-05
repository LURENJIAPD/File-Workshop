$ErrorActionPreference = 'Stop'

$backendRoot = Split-Path -Parent $PSScriptRoot
$repositoryRoot = Split-Path -Parent $backendRoot
$openAPIPath = Join-Path $backendRoot 'api\openapi.yaml'
$documentMatches = @(Get-ChildItem -LiteralPath (Join-Path $repositoryRoot 'docs') -Filter 'File-Workshop-V1.0-API*.md' -File)

if ($documentMatches.Count -ne 1) {
    throw "Expected exactly one cumulative API interface document, found $($documentMatches.Count)"
}
$documentPath = $documentMatches[0].FullName

$openAPI = Get-Content -LiteralPath $openAPIPath -Encoding UTF8
$document = Get-Content -LiteralPath $documentPath -Raw -Encoding UTF8
$paths = @(
    $openAPI |
        Select-String -Pattern '^  (/[^:]+):\s*$' |
        ForEach-Object { $_.Matches[0].Groups[1].Value }
)
$operationIDs = @(
    $openAPI |
        Select-String -Pattern '^      operationId:\s*([^\s]+)\s*$' |
        ForEach-Object { $_.Matches[0].Groups[1].Value }
)

$missing = [System.Collections.Generic.List[string]]::new()
foreach ($path in $paths) {
    if (-not $document.Contains($path)) {
        $missing.Add("path $path")
    }
}
foreach ($operationID in $operationIDs) {
    if (-not $document.Contains('`' + $operationID + '`')) {
        $missing.Add("operationId $operationID")
    }
}

if ($missing.Count -gt 0) {
    throw "API interface document is missing OpenAPI entries:`n$($missing -join [Environment]::NewLine)"
}

Write-Output "API interface document covers $($paths.Count) paths and $($operationIDs.Count) operations"
