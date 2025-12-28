$DIST = "apps/neuro-desktop/target/release"

Remove-Item -Recurse -Force $DIST/python -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $DIST/frontend -ErrorAction SilentlyContinue

# ---------- Build frontend ----------
Write-Host "Building frontend..."
Push-Location frontend
npm run build
Pop-Location

Copy-Item frontend/dist -Recurse $DIST/frontend

$PY_DIST = "$DIST/python"
Write-Host "Bundling Python files and libraries..."

New-Item -ItemType Directory -Force -Path $PY_DIST | Out-Null

Copy-Item backend/python/.venv/Lib $PY_DIST/Lib -Recurse

# ---------- Copy Controller Drivers ----------
Copy-Item `
  backend/python/controller `
  "$PY_DIST/controller" `
  -Recurse
