# ============================================================
# scripts/build-all.ps1 - Complete Build System
# ============================================================

param(
    [Parameter(Mandatory = $false)]
    [ValidateSet('Debug', 'Release')]
    [string]$Configuration = 'Release',

    [Parameter(Mandatory = $false)]
    [switch]$SkipTests,

    [Parameter(Mandatory = $false)]
    [switch]$Clean,

    # The Rust executor and the C++ supervisor are reference/optional code and
    # are not part of the shipped product (docs/ARCHITECTURE.md).
    [Parameter(Mandatory = $false)]
    [switch]$WithLegacy
)

$ErrorActionPreference = 'Stop'

$Green = [ConsoleColor]::Green
$Red = [ConsoleColor]::Red
$Yellow = [ConsoleColor]::Yellow
$Cyan = [ConsoleColor]::Cyan

function Write-Step {
    param([string]$Message)
    Write-Host "`n[*] $Message" -ForegroundColor $Cyan
}

function Write-Success {
    param([string]$Message)
    Write-Host "  [OK] $Message" -ForegroundColor $Green
}

function Write-ErrorLine {
    param([string]$Message)
    Write-Host "  [ERR] $Message" -ForegroundColor $Red
}

function Write-Info {
    param([string]$Message)
    Write-Host "  [..] $Message" -ForegroundColor $Yellow
}

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
Push-Location $RepoRoot

try {
    Write-Host '=======================================================' -ForegroundColor $Cyan
    Write-Host '        Neuro Desktop Build System' -ForegroundColor $Cyan
    Write-Host '=======================================================' -ForegroundColor $Cyan
    Write-Host ''
    Write-Info "Configuration: $Configuration"
    Write-Info "Skip Tests: $SkipTests"
    Write-Info "Clean Build: $Clean"

    if ($Clean) {
        Write-Step 'Cleaning previous builds...'

        Remove-Item -Recurse -Force 'dist' -ErrorAction SilentlyContinue
        Remove-Item -Recurse -Force 'apps/neuro-integration/dist' -ErrorAction SilentlyContinue
        Remove-Item -Recurse -Force 'frontend/dist' -ErrorAction SilentlyContinue

        Write-Success 'Clean completed'
    }

    if ($WithLegacy) {
        Write-Step 'Building the optional C++ supervisor (not shipped)...'
        if (!(Get-Command cmake -ErrorAction SilentlyContinue)) {
            Write-Info 'cmake not found - skipping the supervisor'
        } else {
            Push-Location 'apps/process-handler'
            try {
                & cmake -S . -B build "-DCMAKE_BUILD_TYPE=$Configuration" -DBUILD_TESTS=ON
                if ($LASTEXITCODE -ne 0) { throw 'CMake configure failed' }
                & cmake --build build --config $Configuration
                if ($LASTEXITCODE -ne 0) { throw 'CMake build failed' }
            }
            catch {
                Write-ErrorLine "Supervisor build failed: $_"
                exit 1
            }
            finally {
                Pop-Location
            }
        }
    }

    if ($WithLegacy) {
        Write-Step 'Building the optional Rust executor (not shipped)...'
        Push-Location 'apps/neuro-desktop'
        try {
            if (Get-Command cargo -ErrorAction SilentlyContinue) {
                & cargo build --release
                if ($LASTEXITCODE -ne 0) { throw 'cargo build failed' }
            } else {
                Write-Info 'cargo not found - skipping the legacy executor'
            }
        }
        finally {
            Pop-Location
        }
    }

    Write-Step 'Building the server (Go)...'
    Push-Location 'apps/neuro-integration'
    try {
        New-Item -ItemType Directory -Force -Path 'dist' | Out-Null

        $previousGOOS = $env:GOOS
        $previousGOARCH = $env:GOARCH

        try {
            $env:GOOS = 'windows'
            $env:GOARCH = 'amd64'

            Write-Info 'Building neuro-integration.exe...'
            & go build -o 'dist/neuro-integration.exe' .
            if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }

            if (!$SkipTests) {
                Write-Info 'Running Go tests...'
                & go test -v ./...
                if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
            }
        }
        finally {
            $env:GOOS = $previousGOOS
            $env:GOARCH = $previousGOARCH
        }

        Write-Success 'Go integration built successfully'
    }
    catch {
        Write-ErrorLine "Go integration build failed: $_"
        exit 1
    }
    finally {
        Pop-Location
    }

    if ($WithLegacy) {
    Write-Step 'Building Rust Application (legacy, not shipped)...'
    Push-Location 'apps/neuro-desktop'
    try {
        $cargoBuildArgs = @('build')
        $cargoTestArgs = @('test')

        if ($Configuration -eq 'Release') {
            $cargoBuildArgs += '--release'
            $cargoTestArgs += '--release'
        }

        Write-Info 'Running cargo build...'
        & cargo @cargoBuildArgs
        if ($LASTEXITCODE -ne 0) { throw 'Cargo build failed' }

        if (!$SkipTests) {
            Write-Info 'Running cargo test...'
            & cargo @cargoTestArgs
            if ($LASTEXITCODE -ne 0) { throw 'Cargo tests failed' }
        }

        Write-Success 'Rust application built successfully'
    }
    catch {
        Write-ErrorLine "Rust build failed: $_"
        exit 1
    }
    finally {
        Pop-Location
    }
    }

    Write-Step 'Building the dashboard client...'
    Push-Location 'frontend'
    try {
        if (!(Test-Path 'node_modules')) {
            Write-Info 'Installing npm dependencies...'
            & npm install
            if ($LASTEXITCODE -ne 0) { throw 'npm install failed' }
        }

        Write-Info 'Running npm build...'
        & npm run build
        if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }

        if (!$SkipTests) {
            Write-Info 'Running frontend tests (if present)...'
            & npm run test --if-present
            if ($LASTEXITCODE -ne 0) { throw 'Frontend tests failed' }
        }

        Write-Success 'Frontend built successfully'
    }
    catch {
        Write-ErrorLine "Frontend build failed: $_"
        exit 1
    }
    finally {
        Pop-Location
    }

    Write-Step 'Creating development bundle...'
    try {
        & .\scripts\bundle\dev.ps1
        if ($LASTEXITCODE -ne 0) { throw 'Bundle script failed' }
        Write-Success 'Development bundle created'
    }
    catch {
        Write-ErrorLine "Bundle creation failed: $_"
        exit 1
    }

    if (!$SkipTests -and (Test-Path 'tests/package.json')) {
        Write-Step 'Running integration tests...'
        Push-Location 'tests'
        try {
            if (!(Test-Path 'node_modules')) {
                Write-Info 'Installing integration test dependencies...'
                & npm install
                if ($LASTEXITCODE -ne 0) { throw 'Integration npm install failed' }
            }

            Write-Info 'Running integration tests...'
            & npm test
            if ($LASTEXITCODE -ne 0) { throw 'Integration tests failed' }

            Write-Success 'Integration tests passed'
        }
        catch {
            Write-ErrorLine "Integration tests failed: $_"
            exit 1
        }
        finally {
            Pop-Location
        }
    }

    Write-Host ''
    Write-Host '=======================================================' -ForegroundColor $Green
    Write-Host '        Build Completed Successfully' -ForegroundColor $Green
    Write-Host '=======================================================' -ForegroundColor $Green
    Write-Host ''
    Write-Info 'Build artifacts:'
    Write-Host '  - Server:          apps/neuro-integration/dist/neuro-integration.exe'
    Write-Host '  - Dashboard:       frontend/dist/'
    Write-Host '  - Agent:           backend/python/controller (no build step)'
    if ($WithLegacy) {
        Write-Host '  - (legacy) Process Handler: apps/process-handler/build/'
        Write-Host '  - (legacy) Rust executor:   apps/neuro-desktop/target/release/'
    }
    Write-Host ''
    Write-Info 'To create a production bundle:'
    Write-Host '  .\scripts\bundle\prod.ps1'
}
finally {
    Pop-Location
}

