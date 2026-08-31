param(
  [ValidateSet("amd64", "arm64")]
  [string]$Architecture = "amd64",
  [string]$Version = "0.1.0",
  [switch]$NativeRaw
)

$ErrorActionPreference = "Stop"

if ($env:OS -ne "Windows_NT") {
  throw "Windows GUI 必须在 Windows 上原生构建。"
}

$rootDir = (Resolve-Path (Join-Path $PSScriptRoot "..\")).Path
$frontendDir = Join-Path $rootDir "frontend"
$guiDir = Join-Path $rootDir "cmd\photosheet-gui"
$distDir = Join-Path $rootDir "dist"
$nativeArtifact = Join-Path $rootDir "internal\raw\native\windows\libraw.dll"
$goCommand = if ($env:PHOTOSHEET_GO) { $env:PHOTOSHEET_GO } else { "go" }
$npmCommand = if ($env:PHOTOSHEET_NPM) { $env:PHOTOSHEET_NPM } else { "npm" }
if ($env:PHOTOSHEET_GO) {
  $goBinDir = Split-Path -Parent $env:PHOTOSHEET_GO
  $env:Path = "$goBinDir;$env:Path"
}

Push-Location $frontendDir
try {
  & $npmCommand ci
  & $npmCommand run build:wails
  if ($LASTEXITCODE -ne 0) { throw "前端构建失败。" }
}
finally {
  Pop-Location
}

Push-Location $guiDir
try {
  $wailsTags = "wails production"
  if ($NativeRaw) {
    if ($Architecture -ne "amd64") { throw "Windows native RAW backend 当前仅支持 amd64。" }
    if (-not (Test-Path -LiteralPath $nativeArtifact -PathType Leaf)) {
      throw "找不到本地 native RAW artifact：$nativeArtifact"
    }
    $wailsTags = "wails production photosheet_native"
  }
  & $goCommand run "github.com/wailsapp/wails/v2/cmd/wails@v2.10.2" build `
    -clean `
    -platform "windows/$Architecture" `
    -tags $wailsTags `
    -trimpath `
    -s `
    -m `
    -webview2 browser
  if ($LASTEXITCODE -ne 0) { throw "Wails Windows GUI 构建失败。" }
}
finally {
  Pop-Location
}

$builtExe = Join-Path $guiDir "build\bin\PhotoSheet.exe"
if (-not (Test-Path -LiteralPath $builtExe -PathType Leaf)) {
  throw "找不到 Wails 产物：$builtExe"
}

New-Item -ItemType Directory -Force -Path $distDir | Out-Null
$suffix = if ($Architecture -eq "amd64") { "x64" } else { "arm64" }
$portableExe = Join-Path $distDir "PhotoSheet-windows-$suffix-portable.exe"
Copy-Item -LiteralPath $builtExe -Destination $portableExe -Force
$hash = (Get-FileHash -LiteralPath $portableExe -Algorithm SHA256).Hash.ToLowerInvariant()
$hashLine = "$hash  $(Split-Path -Leaf $portableExe)"
$hashFile = Join-Path $distDir "PhotoSheet-windows-$suffix-portable.SHA256"
Set-Content -LiteralPath $hashFile -Value $hashLine -Encoding ascii

Write-Host "PhotoSheet $Version Windows 便携版已生成：$portableExe"
Write-Host "SHA-256：$hash"
