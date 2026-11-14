# scripts/build.ps1
param(
    [string]$Target = "all"
)

$BinDir = "build"
if (-not (Test-Path $BinDir)) {
    New-Item -ItemType Directory -Name $BinDir | Out-Null
}

function Build-PollerWindows {
    Write-Host "Building poller for Windows..." -ForegroundColor Cyan
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags="-s -w" -o "$BinDir/poller.exe" ./cmd/poller
    if ($LASTEXITCODE -eq 0) { Write-Host "✅ poller.exe built" -ForegroundColor Green }
}

function Build-IngestLinux {
    Write-Host "Building ingest for Linux..." -ForegroundColor Cyan
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags="-s -w" -o "$BinDir/ingest-linux" ./cmd/ingest
    if ($LASTEXITCODE -eq 0) { Write-Host "✅ ingest-linux built" -ForegroundColor Green }
}

function Build-WebLinux {
    Write-Host "Building frontend..." -ForegroundColor Cyan
    Push-Location web
    if (-not (Test-Path node_modules)) { npm ci }
    npm run build
    Pop-Location

    Write-Host "Building web (linux)..." -ForegroundColor Cyan
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags="-s -w" -o "$BinDir/web-linux" ./cmd/web
    if ($LASTEXITCODE -eq 0) { Write-Host "✅ web-linux built" -ForegroundColor Green }
}

function Build-WebWindows {
    Write-Host "Building web (windows, for dev)..." -ForegroundColor Cyan
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -o "$BinDir/web.exe" ./cmd/web
}

switch ($Target) {
    "poller-windows" { Build-PollerWindows }
    "ingest-linux"   { Build-IngestLinux }
    "web-linux"      { Build-WebLinux }
    "web-windows"    { Build-WebWindows }
    "all"            { Build-IngestLinux; Build-PollerWindows; Build-WebLinux }
    default          { Write-Host "Unknown target: $Target" }
}