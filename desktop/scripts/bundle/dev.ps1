# ============================================================
# desktop/scripts/bundle/dev.ps1 - development bundle + run
#   .\scripts\bundle\dev.ps1
#   $env:NEURO_BUNDLE_NO_LAUNCH = '1'; .\scripts\bundle\dev.ps1   # stage only
#
# Stages dist\dev and runs the server (dashboard at http://127.0.0.1:8300/ui/).
# Start the agent separately on the PC Neuro should control:
#   cd dist\dev\agent ; python -m controller.agent --bridge 127.0.0.1:9876
# ============================================================
$ErrorActionPreference = 'Stop'

$Root = Resolve-Path (Join-Path $PSScriptRoot '..\..')
Push-Location $Root
try {
    $Dist = 'dist/dev'
    $Server = 'neuro-integration.exe'

    Write-Host '=== Development Bundle ==='

    if (Test-Path $Dist) { Remove-Item -Recurse -Force $Dist }
    New-Item -ItemType Directory -Force -Path $Dist | Out-Null

    Write-Host 'Building the server...'
    Push-Location 'apps/neuro-integration'
    try {
        New-Item -ItemType Directory -Force -Path 'dist' | Out-Null
        # -tags neurodev: only development bundles accept NEURO_EXTENSIONS_ALLOW_UNSIGNED.
        & go build -tags neurodev -o "dist/$Server" .
        if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
    }
    finally {
        Pop-Location
    }
    Copy-Item "apps/neuro-integration/dist/$Server" "$Dist/$Server" -Force

    Write-Host 'Building the dashboard client...'
    Push-Location 'frontend'
    try {
        & npm install
        if ($LASTEXITCODE -ne 0) { throw 'npm install failed' }
        & npm run build
        if ($LASTEXITCODE -ne 0) { throw 'npm run build failed' }
    }
    finally {
        Pop-Location
    }
    New-Item -ItemType Directory -Force -Path "$Dist/frontend" | Out-Null
    Copy-Item 'frontend/dist/*' "$Dist/frontend" -Recurse -Force

    Write-Host 'Staging the agent, config and catalog...'
    New-Item -ItemType Directory -Force -Path "$Dist/agent" | Out-Null
    Copy-Item 'backend/python/controller' "$Dist/agent/controller" -Recurse -Force
    Get-ChildItem -Path "$Dist/agent" -Recurse -Directory -Filter '__pycache__' |
        Remove-Item -Recurse -Force
    Copy-Item 'backend/python/requirements.txt' "$Dist/agent/requirements.txt" -Force
    Copy-Item 'config' "$Dist/config" -Recurse -Force
    Copy-Item 'catalog' "$Dist/catalog" -Recurse -Force
    Copy-Item 'apps/neuro-integration/permissions.example.json' "$Dist/permissions.json" -Force

    Write-Host ''
    Write-Host "=== Dev bundle complete: $Dist ==="
    Write-Host ''
    Write-Host '1. Set up, then start the server (this window unless NEURO_BUNDLE_NO_LAUNCH=1):'
    Write-Host "     cd $Dist ; .\$Server setup ; .\$Server --ws-url ws://localhost:8000"
    Write-Host '   setup prints the dashboard token once: copy it. It is the sign-in token.' 
    Write-Host '2. Agent on the controlled PC:'
    Write-Host '     cd dist\dev\agent ; python -m controller.agent --bridge 127.0.0.1:9876'
    Write-Host '3. Dashboard: http://127.0.0.1:8300/ui/'
    Write-Host ''

    if ($env:NEURO_BUNDLE_NO_LAUNCH -eq '1') {
        Write-Host 'Skipping launch (NEURO_BUNDLE_NO_LAUNCH=1)'
        return
    }

    Push-Location $Dist
    try {
        $env:NEURO_UI_DIR = (Join-Path $PWD 'frontend')
        $env:NEURO_IPC_FILE = (Join-Path $PWD 'neuro_ipc.json')
        $env:NEURO_PERMISSIONS_FILE = (Join-Path $PWD 'permissions.json')
        $env:NEURO_CATALOG_FILE = (Join-Path $PWD 'catalog/index.json')
        $env:NEURO_GAME_PROFILES_DIR = (Join-Path $PWD 'catalog/games')

        # First-run step. Idempotent: it prints the dashboard token only the first time.
        & ".\$Server" setup
        if ($LASTEXITCODE -ne 0) { throw 'setup is not complete; fix the FAIL lines above' }

        Write-Host 'Launching the server (Ctrl-C to stop)...'
        & ".\$Server"
    }
    finally {
        Pop-Location
    }
}
finally {
    Pop-Location
}
