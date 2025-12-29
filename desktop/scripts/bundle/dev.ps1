$ErrorActionPreference = "Stop"

$DIST = "apps/neuro-desktop/target/release"

Remove-Item -Recurse -Force $DIST/python -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $DIST/frontend -ErrorAction SilentlyContinue

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

# ---------- Build Go Neuro Integration ----------
Write-Host "Building Go Neuro Integration..."
Push-Location native/go

$env:GOOS = "windows"
$env:GOARCH = "amd64"

go build -o neuro-integration.exe ./main.go

Pop-Location

New-Item -ItemType Directory -Force -Path "$DIST/go" | Out-Null
Copy-Item `
  native/go/neuro-integration.exe `
  "$DIST/go/neuro-integration.exe"

# ---------- Copy Controller Drivers ----------
Copy-Item `
  backend/python/controller `
  "$PY_DIST/controller" `
  -Recurse
