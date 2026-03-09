# ============================================================
# desktop/scripts/bundle/dev.ps1
# ============================================================
param(
    [Parameter(Mandatory = $false)]
    [switch]$RunAfterBuild
)

$ErrorActionPreference = "Stop"

Write-Host "=== Development Bundle ==="

$DIST = "apps/neuro-desktop/target/release"
$PY_DIST = "$DIST/python"

Remove-Item -Recurse -Force $DIST/python -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $DIST/frontend -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $DIST/config -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $DIST/catalog -ErrorAction SilentlyContinue

# ---------- Build Neuro Integration ----------
Write-Host "Building Neuro integration..."

New-Item -ItemType Directory -Force -Path apps/neuro-integration/dist | Out-Null
Push-Location apps/neuro-integration/

go build -o dist/neuro-integration.exe .
Pop-Location

Copy-Item `
  apps/neuro-integration/dist/neuro-integration.exe `
  $DIST/neuro-integration.exe

New-Item -ItemType Directory -Force -Path $DIST/integration-docs | Out-Null

Copy-Item `
  apps/neuro-integration/integration-docs/"Action Script Documentation.md" `
  $DIST/integration-docs/"Action Script Documentation.md"

Copy-Item `
  apps/neuro-integration/permissions.example.json `
  $DIST/permissions.json

$relayBinarySource = $env:NEURO_RELAY_BINARY_SOURCE
if ($relayBinarySource -and (Test-Path $relayBinarySource)) {
    Copy-Item $relayBinarySource "$DIST/neuro-relay.exe"
    Write-Host "  [ok] Relay binary copied to $DIST/neuro-relay.exe"
}
else {
    Write-Host "  [..] Relay binary not bundled (set NEURO_RELAY_BINARY_SOURCE to include it)"
}

Write-Host "  [ok] Neuro Integration binary copied to $DIST"

# ---------- Copy Process Handler ----------
$processHandlerCandidates = @(
  "apps/process-handler/build/Release/process-handler.exe",
  "apps/process-handler/build/process-handler.exe"
)

$processHandlerSource = $processHandlerCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1
if ($processHandlerSource) {
  Copy-Item $processHandlerSource "$DIST/process-handler.exe"
  Write-Host "  [ok] Process Handler binary copied to $DIST/process-handler.exe"
} else {
  Write-Host "  [..] Process Handler binary not found (build apps/process-handler first)"
}

# ---------- Build frontend ----------
Write-Host "Building frontend..."
Push-Location frontend
npm run build
Pop-Location

Copy-Item frontend/dist -Recurse $DIST/frontend

# ---------- Copy Config ----------
Write-Host "Copying configuration files..."
Copy-Item config "$DIST/config" -Recurse
Write-Host "  ??? Config files copied"

Write-Host "Copying catalog files..."
Copy-Item catalog "$DIST/catalog" -Recurse
Write-Host "  [ok] Catalog files copied"

# ---------- Bundle Python (EMBEDDED) ----------
$PY_DIST = "$DIST/python"
Write-Host "Bundling Python files and libraries..."

New-Item -ItemType Directory -Force -Path $PY_DIST | Out-Null

Copy-Item backend/python/.venv/Lib $PY_DIST/Lib -Recurse

# ---------- Copy Controller Drivers ----------
Copy-Item backend/python/controller "$PY_DIST/controller" -Recurse

Write-Host ""
Write-Host "=== Dev bundle complete ==="
Write-Host "Run from: $DIST"
Write-Host "Execute:  .\neuro-desktop.exe"
Write-Host "Supervised: .\process-handler.exe"

if ($RunAfterBuild) {
    Set-Location $DIST
    .\neuro-desktop.exe
}

