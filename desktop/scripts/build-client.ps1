# Builds neuro-client.exe: the program that runs on the PC Neuro controls. It is the
# controller agent packaged as one executable, so the controlled PC needs no Python
# install and runs nothing else (no dashboard, no vision, no policy).
#
#   .\scripts\build-client.ps1          # -> desktop\dist\neuro-client\neuro-client.exe
#
# PyInstaller does not cross-compile: run this on Windows. Not verified on Windows
# from the development environment; see docs/PRODUCTION_TODO.md.
$ErrorActionPreference = 'Stop'

$Root = Resolve-Path (Join-Path $PSScriptRoot '..')
Set-Location $Root

$PyInstallerVersion = '6.22.3'
$Out = if ($env:NEURO_CLIENT_OUT) { $env:NEURO_CLIENT_OUT } else { Join-Path $Root 'dist\neuro-client' }
$Work = Join-Path $Root 'dist\.client-build'
$Venv = Join-Path $Work 'venv'

New-Item -ItemType Directory -Force -Path $Work | Out-Null
if (-not (Test-Path (Join-Path $Venv 'Scripts\python.exe'))) {
    python -m venv $Venv
}
$Py = Join-Path $Venv 'Scripts\python.exe'
& $Py -m pip install --quiet --upgrade pip
& $Py -m pip install --quiet -r backend\python\requirements.txt -r backend\python\requirements-windows.txt "pyinstaller==$PyInstallerVersion"

Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $Work 'pyi'), $Out
New-Item -ItemType Directory -Force -Path (Join-Path $Work 'pyi'), $Out | Out-Null
& (Join-Path $Venv 'Scripts\pyinstaller.exe') --noconfirm --onefile --clean `
    --name neuro-client `
    --paths backend\python `
    --collect-submodules pynput `
    --distpath $Out `
    --workpath (Join-Path $Work 'pyi\build') `
    --specpath (Join-Path $Work 'pyi') `
    apps\neuro-client\neuro_client.py
if ($LASTEXITCODE -ne 0) { throw 'PyInstaller failed' }
Write-Host "built: $(Join-Path $Out 'neuro-client.exe')"
