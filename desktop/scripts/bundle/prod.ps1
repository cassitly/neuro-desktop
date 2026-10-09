# ============================================================
# desktop/scripts/bundle/prod.ps1 - release bundle
#   .\scripts\bundle\prod.ps1
#   .\scripts\bundle\prod.ps1 -SkipFrontend -SkipVenv
#
# Produces dist\neuro-desktop\ with:
#   neuro-integration.exe   the server (Neuro client + dashboard API + hub)
#   agent\                  the Python agent + its requirements
#   frontend\               dashboard client, served at /ui/
#   catalog\, config\       game profiles, policy example, docs
# ============================================================
param(
    [switch]$SkipFrontend,
    [switch]$SkipVenv
)

$ErrorActionPreference = 'Stop'

$Root = Resolve-Path (Join-Path $PSScriptRoot '..\..')
Push-Location $Root
try {
    $Dist = 'dist/neuro-desktop'
    $AgentDist = "$Dist/agent"
    $Server = 'neuro-integration.exe'

    Write-Host '=== Building Neuro Desktop bundle ==='

    if (Test-Path $Dist) { Remove-Item -Recurse -Force $Dist }
    New-Item -ItemType Directory -Force -Path $Dist | Out-Null

    # ----------------------------------------------------------
    # 1. Server (Go)
    # ----------------------------------------------------------
    Write-Host '[1/3] Building the server...'
    Push-Location 'apps/neuro-integration'
    try {
        New-Item -ItemType Directory -Force -Path 'dist' | Out-Null
        # No -tags neurodev: a release build has no unsigned-extension escape hatch.
        & go build -trimpath -o "dist/$Server" .
        if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
    }
    finally {
        Pop-Location
    }
    Copy-Item "apps/neuro-integration/dist/$Server" "$Dist/$Server" -Force

    # ----------------------------------------------------------
    # 2. Dashboard client
    # ----------------------------------------------------------
    if (!$SkipFrontend) {
        Write-Host '[2/3] Building the dashboard client...'
        Push-Location 'frontend'
        try {
            if (!(Test-Path 'node_modules')) {
                & npm install
                if ($LASTEXITCODE -ne 0) { throw 'npm install failed' }
            }
            & npm run build
            if ($LASTEXITCODE -ne 0) { throw 'npm run build failed' }
        }
        finally {
            Pop-Location
        }
        New-Item -ItemType Directory -Force -Path "$Dist/frontend" | Out-Null
        Copy-Item 'frontend/dist/*' "$Dist/frontend" -Recurse -Force
    } else {
        Write-Host '[2/3] Dashboard client skipped (-SkipFrontend)'
    }

    # ----------------------------------------------------------
    # 3. Agent (Python) + config, catalog, docs
    # ----------------------------------------------------------
    Write-Host '[3/3] Bundling the agent, config and catalog...'
    New-Item -ItemType Directory -Force -Path $AgentDist | Out-Null
    Copy-Item 'backend/python/controller' $AgentDist -Recurse -Force
    Get-ChildItem -Path $AgentDist -Recurse -Directory -Filter '__pycache__' |
        Remove-Item -Recurse -Force
    Copy-Item 'backend/python/requirements.txt' "$AgentDist/requirements.txt" -Force
    if (Test-Path 'backend/python/requirements-windows.txt') {
        Copy-Item 'backend/python/requirements-windows.txt' "$AgentDist/requirements-windows.txt" -Force
    }

    # A vendored runtime is a convenience, not a requirement: a build machine
    # without Python or without network still produces a usable bundle.
    if ($SkipVenv) {
        Write-Host '      ! agent runtime not vendored (-SkipVenv)'
    } else {
        try {
            $Venv = 'backend/python/.venv'
            if (!(Test-Path $Venv)) { & python -m venv $Venv | Out-Null }
            & "$Venv/Scripts/pip.exe" install --quiet --upgrade pip
            & "$Venv/Scripts/pip.exe" install --quiet -r 'backend/python/requirements-windows.txt'
            if ($LASTEXITCODE -ne 0) { throw 'pip install failed' }
            Copy-Item "$Venv/Lib" "$AgentDist/Lib" -Recurse -Force
            Write-Host '      agent runtime vendored'
        }
        catch {
            Write-Host "      ! could not vendor the runtime: $_"
            Write-Host '        The bundle runs with the system python + agent\requirements.txt'
        }
    }

    Copy-Item 'config' "$Dist/config" -Recurse -Force
    Copy-Item 'catalog' "$Dist/catalog" -Recurse -Force
    Copy-Item 'apps/neuro-integration/permissions.example.json' "$Dist/permissions.json" -Force
    New-Item -ItemType Directory -Force -Path "$Dist/integration-docs" | Out-Null
    Copy-Item 'apps/neuro-integration/integration-docs/*' "$Dist/integration-docs" -Recurse -Force
    Copy-Item 'docs/*.md' "$Dist/integration-docs" -Force -ErrorAction SilentlyContinue

    # ----------------------------------------------------------
    # Templates (launchers + README) — checked in, so they are reviewable
    # ----------------------------------------------------------
    $Templates = 'scripts/bundle/templates'
    foreach ($pair in @(@('README.txt', 'README.txt'), @('start.bat', 'start.bat'), @('start.sh', 'start.sh'))) {
        $body = Get-Content (Join-Path $Templates $pair[0]) -Raw
        Set-Content -Path (Join-Path $Dist $pair[1]) -Value ($body -replace '@SERVER@', $Server) -NoNewline
    }

    Write-Host ''
    Write-Host '=== Bundle complete ==='
    Write-Host "Location: $Dist"
    Write-Host ''
    Get-ChildItem $Dist | Select-Object Name
    Write-Host ''
    Write-Host 'Run it: cd dist\neuro-desktop && .\start.bat'
}
finally {
    Pop-Location
}
