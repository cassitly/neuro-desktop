#!/usr/bin/env bash
set -euo pipefail

echo "=== Development Bundle ==="

# --------------------------------------------------
# OS DETECTION
# --------------------------------------------------
OS_UNAME="$(uname -s | tr '[:upper:]' '[:lower:]')"

IS_WINDOWS=false
IS_WSL=false

if [[ "$OS_UNAME" == mingw* || "$OS_UNAME" == msys* || "$OS_UNAME" == cygwin* ]]; then
  IS_WINDOWS=true
elif grep -qi microsoft /proc/version 2>/dev/null; then
  IS_WSL=true
fi

BIN_EXT=""
$IS_WINDOWS && BIN_EXT=".exe"

RUST_BIN="neuro-desktop$BIN_EXT"
GO_BIN="neuro-integration$BIN_EXT"

echo "Detected OS: $OS_UNAME"
$IS_WINDOWS && echo "→ Windows mode"
$IS_WSL && echo "→ WSL mode"

# --------------------------------------------------
# PATHS
# --------------------------------------------------
DIST="apps/neuro-desktop/target/release"
PY_DIST="$DIST/python"

# --------------------------------------------------
# CLEAN DEV ASSETS
# --------------------------------------------------
rm -rf "$PY_DIST" "$DIST/frontend" "$DIST/config"

# --------------------------------------------------
# BUILD GO INTEGRATION
# --------------------------------------------------
echo "Building Neuro integration..."

mkdir -p apps/neuro-integration/dist
pushd apps/neuro-integration > /dev/null
go build -o "dist/$GO_BIN" .
popd > /dev/null

cp "apps/neuro-integration/dist/$GO_BIN" "$DIST/$GO_BIN"

mkdir -p "$DIST/integration-docs"
cp \
  "apps/neuro-integration/integration-docs/Action Script Documentation.md" \
  "$DIST/integration-docs/Action Script Documentation.md"

echo "  ✓ Neuro Integration binary copied to $DIST"

# --------------------------------------------------
# BUILD FRONTEND
# --------------------------------------------------
echo "Building frontend..."
pushd frontend > /dev/null
npm install # install the npm dependencies before building
npm run build
popd > /dev/null

mkdir -p "$DIST/frontend"
cp -r frontend/dist/* "$DIST/frontend/"

# --------------------------------------------------
# COPY CONFIG
# --------------------------------------------------
echo "Copying configuration files..."
cp -r config "$DIST/config"
echo "  ✓ Config files copied"

# --------------------------------------------------
# PYTHON VENV DETECTION
# --------------------------------------------------
echo "Bundling Python files and libraries..."

mkdir -p "$PY_DIST"


VENV_BASE="backend/python/.venv"
REQ_FILE="backend/python/requirements.txt"
python -m venv "$VENV_BASE"
source "$VENV_BASE/bin/activate"

if [ -f "$REQ_FILE" ]; then
    echo "Installing/updating dependencies from $REQ_FILE..."
    pip install --upgrade pip
    pip install -r "$REQ_FILE"
else
    echo "Warning: $REQ_FILE not found. Skipping installation."
fi

if [[ -d "$VENV_BASE/Lib" ]]; then
  # Windows venv
  PY_LIB_SRC="$VENV_BASE/Lib"
elif [[ -d "$VENV_BASE/lib" ]]; then
  # Unix venv
  PY_SITE=$(find "$VENV_BASE/lib" -maxdepth 1 -type d -name "python*" | head -n 1)
  if [[ -z "$PY_SITE" ]]; then
    echo "❌ Could not find python site-packages"
    exit 1
  fi
  PY_LIB_SRC="$PY_SITE/site-packages"
else
  echo "❌ Python venv not found"
  exit 1
fi

cp -r "$PY_LIB_SRC" "$PY_DIST/Lib"
cp -r backend/python/controller "$PY_DIST/controller"

echo
echo "=== Dev bundle complete ==="
echo "Run from: $DIST"
echo "Execute:  ./$RUST_BIN"

# --------------------------------------------------
# LAUNCH
# --------------------------------------------------
pushd "$DIST" > /dev/null
chmod +x "$RUST_BIN" 2>/dev/null || true
chmod +x "$GO_BIN" 2>/dev/null || true
./"$RUST_BIN"
popd > /dev/null
