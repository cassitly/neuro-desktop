$ErrorActionPreference = "Stop"

$DIST = "apps/neuro-desktop/target/release"

Remove-Item -Recurse -Force $DIST/python -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $DIST/frontend -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $DIST/config -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $DIST/go -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force "apps/neuro-desktop/libs" -ErrorAction SilentlyContinue

# ---------- Build frontend ----------
Write-Host "Building frontend..."
Push-Location frontend
npm run build
Pop-Location

Copy-Item frontend/dist -Recurse $DIST/frontend

# ---------- Bundle Python files ----------
$PY_DIST = "$DIST/python"
Write-Host "Bundling Python files and libraries..."

New-Item -ItemType Directory -Force -Path $PY_DIST | Out-Null

Copy-Item backend/python/.venv/Lib $PY_DIST/Lib -Recurse

# ---------- Copy Controller Drivers ----------
Copy-Item `
  backend/python/controller `
  "$PY_DIST/controller" `
  -Recurse

# ---------- Copy Configuration Files ----------
New-Item -ItemType Directory -Force -Path "$DIST/config" | Out-Null

Copy-Item `
  config/integration-config.yml `
  "$DIST/config/integration-config.yml" `
  -Recurse

# # ---------- Build Go Neuro Integration ----------
# Write-Host "Building Go Neuro Integration..."
# Push-Location native/go

# $env:GOOS = "windows"
# $env:GOARCH = "amd64"

# go build -buildmode=c-archive -o neuro-integration.lib .

# Pop-Location

# New-Item -ItemType Directory -Force -Path "apps/neuro-desktop/libs" | Out-Null
# New-Item -ItemType Directory -Force -Path "$DIST/go" | Out-Null
# Copy-Item `
#   native/go/neuro-integration.lib `
#   "apps/neuro-desktop/libs/neuro-integration.lib"

# Copy-Item `
#   native/go/neuro-integration.h `
#   "apps/neuro-desktop/libs/neuro-integration.h"
