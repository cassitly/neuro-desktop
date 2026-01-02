# ============================================================
# scripts/build-all.ps1 - Complete Build System
# ============================================================

param(
    [Parameter(Mandatory=$false)]
    [ValidateSet('Debug', 'Release')]
    [string]$Configuration = 'Release',
    
    [Parameter(Mandatory=$false)]
    [switch]$SkipTests,
    
    [Parameter(Mandatory=$false)]
    [switch]$Clean,
    
    [Parameter(Mandatory=$false)]
    [switch]$Verbose
)

$ErrorActionPreference = "Stop"

# Colors for output
$Green = [ConsoleColor]::Green
$Red = [ConsoleColor]::Red
$Yellow = [ConsoleColor]::Yellow
$Cyan = [ConsoleColor]::Cyan

function Write-Step {
    param([string]$Message)
    Write-Host "`n[" -NoNewline
    Write-Host "●" -ForegroundColor $Cyan -NoNewline
    Write-Host "] $Message" -ForegroundColor $Cyan
}

function Write-Success {
    param([string]$Message)
    Write-Host "  ✓ $Message" -ForegroundColor $Green
}

function Write-Error {
    param([string]$Message)
    Write-Host "  ✗ $Message" -ForegroundColor $Red
}

function Write-Info {
    param([string]$Message)
    Write-Host "  → $Message" -ForegroundColor $Yellow
}

Write-Host "=======================================================" -ForegroundColor $Cyan
Write-Host "        Neuro Desktop Build System" -ForegroundColor $Cyan
Write-Host "=======================================================" -ForegroundColor $Cyan
Write-Host ""
Write-Info "Configuration: $Configuration"
Write-Info "Skip Tests: $SkipTests"
Write-Info "Clean Build: $Clean"
Write-Host ""

# ============================================================
# 1. Clean (if requested)
# ============================================================
if ($Clean) {
    Write-Step "Cleaning previous builds..."
    
    Remove-Item -Recurse -Force "dist" -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force "apps/neuro-desktop/target" -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force "native/process-handler/build" -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force "native/neuro-integration/dist" -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force "frontend/dist" -ErrorAction SilentlyContinue
    
    Write-Success "Clean completed"
}

# ============================================================
# 2. Build Process Handler (C++)
# ============================================================
Write-Step "Building Process Handler (C++)..."

Push-Location native/process-handler

if (!(Test-Path "build")) {
    New-Item -ItemType Directory -Path "build" | Out-Null
}

Push-Location build

try {
    # Configure with CMake
    Write-Info "Running CMake configure..."
    $cmakeArgs = @(
        "..",
        "-DCMAKE_BUILD_TYPE=$Configuration"
    )
    
    if (!$SkipTests) {
        $cmakeArgs += "-DBUILD_TESTS=ON"
    }
    
    & cmake $cmakeArgs
    if ($LASTEXITCODE -ne 0) { throw "CMake configure failed" }
    
    # Build
    Write-Info "Building..."
    & cmake --build . --config $Configuration
    if ($LASTEXITCODE -ne 0) { throw "CMake build failed" }
    
    Write-Success "Process Handler built successfully"
    
    # Run tests if not skipped
    if (!$SkipTests) {
        Write-Info "Running C++ tests..."
        & ctest --output-on-failure -C $Configuration
        if ($LASTEXITCODE -eq 0) {
            Write-Success "All C++ tests passed"
        } else {
            Write-Error "Some C++ tests failed"
        }
    }
}
catch {
    Write-Error "Process Handler build failed: $_"
    Pop-Location
    Pop-Location
    exit 1
}
finally {
    Pop-Location
    Pop-Location
}

# ============================================================
# 3. Build Go Integration
# ============================================================
Write-Step "Building Go Integration..."

Push-Location native/neuro-integration

try {
    New-Item -ItemType Directory -Force -Path "dist" | Out-Null
    
    Write-Info "Building for Windows x64..."
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    & go build -o "dist/neuro-integration.exe" .
    if ($LASTEXITCODE -ne 0) { throw "Go build failed" }
    
    Write-Success "Go integration built successfully"
    
    # Run Go tests if not skipped
    if (!$SkipTests) {
        Write-Info "Running Go tests..."
        & go test -v ./...
        if ($LASTEXITCODE -eq 0) {
            Write-Success "All Go tests passed"
        } else {
            Write-Error "Some Go tests failed"
        }
    }
}
catch {
    Write-Error "Go integration build failed: $_"
    Pop-Location
    exit 1
}
finally {
    Pop-Location
}

# ============================================================
# 4. Build Rust Application
# ============================================================
Write-Step "Building Rust Application..."

Push-Location apps/neuro-desktop

try {
    $buildType = if ($Configuration -eq "Debug") { "--debug" } else { "--release" }
    
    Write-Info "Running cargo build..."
    & cargo build $buildType
    if ($LASTEXITCODE -ne 0) { throw "Cargo build failed" }
    
    Write-Success "Rust application built successfully"
    
    # Run Rust tests if not skipped
    if (!$SkipTests) {
        Write-Info "Running Rust tests..."
        & cargo test $buildType
        if ($LASTEXITCODE -eq 0) {
            Write-Success "All Rust tests passed"
        } else {
            Write-Error "Some Rust tests failed"
        }
    }
}
catch {
    Write-Error "Rust build failed: $_"
    Pop-Location
    exit 1
}
finally {
    Pop-Location
}

# ============================================================
# 5. Build Frontend
# ============================================================
Write-Step "Building Frontend..."

Push-Location frontend

try {
    if (!(Test-Path "node_modules")) {
        Write-Info "Installing npm dependencies..."
        & npm install
        if ($LASTEXITCODE -ne 0) { throw "npm install failed" }
    }
    
    Write-Info "Building frontend..."
    & npm run build
    if ($LASTEXITCODE -ne 0) { throw "Frontend build failed" }
    
    Write-Success "Frontend built successfully"
    
    # Run frontend tests if not skipped
    if (!$SkipTests -and (Test-Path "package.json" -PathType Leaf)) {
        $packageJson = Get-Content "package.json" | ConvertFrom-Json
        if ($packageJson.scripts.test) {
            Write-Info "Running frontend tests..."
            & npm test
            if ($LASTEXITCODE -eq 0) {
                Write-Success "All frontend tests passed"
            } else {
                Write-Error "Some frontend tests failed"
            }
        }
    }
}
catch {
    Write-Error "Frontend build failed: $_"
    Pop-Location
    exit 1
}
finally {
    Pop-Location
}

# ============================================================
# 6. Bundle for Development
# ============================================================
Write-Step "Creating development bundle..."

try {
    & .\scripts\bundle\dev.ps1
    Write-Success "Development bundle created"
}
catch {
    Write-Error "Bundle creation failed: $_"
    exit 1
}

# ============================================================
# 7. Run Integration Tests
# ============================================================
if (!$SkipTests) {
    Write-Step "Running integration tests..."
    
    Push-Location tests
    
    try {
        if (Test-Path "package.json") {
            if (!(Test-Path "node_modules")) {
                Write-Info "Installing test dependencies..."
                & npm install
            }
            
            Write-Info "Running JavaScript integration tests..."
            & npm test
            if ($LASTEXITCODE -eq 0) {
                Write-Success "All integration tests passed"
            } else {
                Write-Error "Some integration tests failed"
            }
        }
    }
    catch {
        Write-Error "Integration tests failed: $_"
        Pop-Location
        exit 1
    }
    finally {
        Pop-Location
    }
}

# ============================================================
# Summary
# ============================================================
Write-Host ""
Write-Host "=======================================================" -ForegroundColor $Green
Write-Host "        Build Completed Successfully!" -ForegroundColor $Green
Write-Host "=======================================================" -ForegroundColor $Green
Write-Host ""

Write-Info "Build artifacts:"
Write-Host "  • Process Handler: native/process-handler/build/process-handler.exe"
Write-Host "  • Go Integration:  native/neuro-integration/dist/neuro-integration.exe"
Write-Host "  • Rust App:        apps/neuro-desktop/target/$($Configuration.ToLower())/neuro-desktop.exe"
Write-Host "  • Frontend:        frontend/dist/"
Write-Host ""

Write-Info "To run the application:"
Write-Host "  cd apps/neuro-desktop/target/$($Configuration.ToLower())"
Write-Host "  .\process-handler.exe"
Write-Host ""

Write-Info "To create production bundle:"
Write-Host "  .\scripts\bundle\prod.ps1"
Write-Host ""

# ============================================================
# Example Usage:
# ============================================================
<#

# Full build with tests
.\scripts\build-all.ps1

# Clean build without tests
.\scripts\build-all.ps1 -Clean -SkipTests

# Debug build
.\scripts\build-all.ps1 -Configuration Debug

# Verbose output
.\scripts\build-all.ps1 -Verbose

#>