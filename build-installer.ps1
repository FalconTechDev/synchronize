#Requires -Version 5.1
<#
  Builds dist\syn-setup.exe - the Windows installer wizard.

  Prerequisite: NSIS 3.x  (winget install NSIS.NSIS)
#>
param(
    [string]$Version = "1.0.0"
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot

$makensis = @(
    "${env:ProgramFiles(x86)}\NSIS\makensis.exe",
    "$env:ProgramFiles\NSIS\makensis.exe"
) | Where-Object { Test-Path $_ } | Select-Object -First 1

if (-not $makensis) {
    throw "NSIS not found. Install it with:  winget install NSIS.NSIS"
}

Write-Host "Building synchronize installer $Version" -ForegroundColor Cyan
Write-Host ""

Write-Host "  [1/2] building syn.exe"
& go build -ldflags="-s -w -X main.Version=v$Version" -o (Join-Path $root 'bin\syn.exe') ./cmd/syn/
if ($LASTEXITCODE -ne 0) { throw "syn.exe build failed" }

Write-Host "  [2/2] compiling installer"
New-Item -ItemType Directory -Force -Path (Join-Path $root 'dist') | Out-Null
Push-Location (Join-Path $root 'installer')
& $makensis "-DAPPVERSION=$Version" syn.nsi | Select-Object -Last 3
$code = $LASTEXITCODE
Pop-Location
if ($code -ne 0) { throw "installer compilation failed" }

$out = Join-Path $root 'dist\syn-setup.exe'
$mb = [Math]::Round((Get-Item $out).Length / 1MB, 1)
Write-Host ""
Write-Host "  Done: $out ($mb MB)" -ForegroundColor Green
