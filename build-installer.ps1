#Requires -Version 5.1
<#
  Builds dist\syn-setup.exe — a single self-contained Windows installer
  with syn.exe and the logo embedded, and the app icon on the installer itself.

  Prerequisites (one time):
    go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
#>
param(
    [string]$Version = "v1.0.0"
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$setup = Join-Path $root 'cmd\syn-setup'

Write-Host "Building synchronize installer $Version" -ForegroundColor Cyan
Write-Host ""

Write-Host "  [1/4] building syn.exe"
& go build -ldflags="-s -w -X main.Version=$Version" -o (Join-Path $root 'bin\syn.exe') ./cmd/syn/
if ($LASTEXITCODE -ne 0) { throw "syn.exe build failed" }

Write-Host "  [2/4] staging embedded assets"
Copy-Item -Force (Join-Path $root 'bin\syn.exe') (Join-Path $setup 'syn.exe')
$logo = Join-Path $env:USERPROFILE '.synchronize\logo.png'
if (-not (Test-Path $logo)) { $logo = Join-Path $root 'internal\syn\assets\tray.png' }
Copy-Item -Force $logo (Join-Path $setup 'logo.png')

Write-Host "  [3/4] generating icon resource"
$gvi = Join-Path (& go env GOPATH) 'bin\goversioninfo.exe'
if (-not (Test-Path $gvi)) { throw "goversioninfo not found - run: go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest" }
Push-Location $setup
& $gvi -o resource_windows_amd64.syso versioninfo.json
Pop-Location
if ($LASTEXITCODE -ne 0) { throw "icon resource generation failed" }

Write-Host "  [4/4] building syn-setup.exe"
New-Item -ItemType Directory -Force -Path (Join-Path $root 'dist') | Out-Null
& go build -ldflags="-s -w -X main.Version=$Version" -o (Join-Path $root 'dist\syn-setup.exe') ./cmd/syn-setup/
if ($LASTEXITCODE -ne 0) { throw "installer build failed" }

$out = Join-Path $root 'dist\syn-setup.exe'
$mb = [Math]::Round((Get-Item $out).Length / 1MB, 1)
Write-Host ""
Write-Host "  Done: $out ($mb MB)" -ForegroundColor Green
