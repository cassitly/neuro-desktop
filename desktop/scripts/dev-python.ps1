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
Write-Host "Bundling embedded Python..."

New-Item -ItemType Directory -Force -Path $PY_DIST | Out-Null

# Extract official embedded Python
Expand-Archive `
  scripts/python-embed.zip `
  $PY_DIST/binary `
  -Force
  
Copy-Item backend/python/.venv/Lib $PY_DIST/Lib -Recurse

# Optional but recommended: unzip stdlib so imports work normally
Expand-Archive `
  "$PY_DIST/binary/python311.zip" `
  "$PY_DIST/Lib/site-packages" `
  -Force

Remove-Item "$PY_DIST/binary/python311.zip"

# ---------- Copy OS driver ----------
Copy-Item `
  backend/python/os-driver `
  "$PY_DIST/os-driver" `
  -Recurse
