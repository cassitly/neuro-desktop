$ErrorActionPreference = "Stop"

Write-Host "=== Building Neuro Desktop Bundle ==="

$DIST = "dist/neuro-desktop"
$PY_DIST = "$DIST/python"

# ---------- Clean ----------
Remove-Item -Recurse -Force dist -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $DIST | Out-Null

# ---------- Build Rust ----------
Write-Host "Building Rust app..."
Push-Location apps/neuro-desktop
cargo build --release
Pop-Location

Copy-Item `
  apps/neuro-desktop/target/release/neuro-desktop.exe `
  $DIST

# ---------- Build frontend ----------
Write-Host "Building frontend..."
Push-Location frontend
npm run build
Pop-Location

Copy-Item frontend/dist -Recurse $DIST/frontend

# ---------- Build Go Neuro SDK ----------
Write-Host "Building Go Neuro SDK..."
Push-Location backend/go/neuro-sdk

$env:GOOS = "windows"
$env:GOARCH = "amd64"

go build -o neuro-sdk.exe ./cmd/neuro-sdk

Pop-Location

New-Item -ItemType Directory -Force -Path $GO_DIST | Out-Null
Copy-Item `
  backend/go/neuro-sdk/neuro-sdk.exe `
  "$GO_DIST/neuro-sdk.exe"

# ---------- Bundle Python (EMBEDDED) ----------
Write-Host "Bundling Python files and libraries..."

New-Item -ItemType Directory -Force -Path $PY_DIST | Out-Null

Copy-Item backend/python/.venv/Lib $PY_DIST/Lib -Recurse

# ---------- Copy Controller Drivers ----------
Copy-Item `
  backend/python/controller `
  "$PY_DIST/controller" `
  -Recurse

# ---------- Metadata ----------
@"
Neuro Desktop
-------------
This folder contains all runtime dependencies.
Do not move files individually.
"@ | Out-File "$DIST/README.txt"

Write-Host "=== Bundle complete ==="
