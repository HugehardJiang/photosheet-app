$ErrorActionPreference = "Stop"

Write-Host "PhotoSheet build environment"

$required = @("git", "node", "npm", "go")
foreach ($name in $required) {
    $command = Get-Command $name -ErrorAction SilentlyContinue
    if (-not $command) {
        Write-Error "Missing required command: $name"
    }
    if ($name -eq "go") {
        & $command.Source version
    } else {
        & $command.Source --version
    }
}

Write-Host ""
Write-Host "Core builds use the pure-Go RAW fallback by default."
Write-Host "Native RAW builds are optional and require the separately reviewed SDK inputs documented in native/README.md."
