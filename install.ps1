#Requires -Version 5.1
<#
.SYNOPSIS
  Installs synchronize (syn) on Windows.
.DESCRIPTION
  Copies syn.exe to %LOCALAPPDATA%\Programs\syn\ and adds it to the user PATH.
  Run from the folder that contains syn.exe, or pass -BinPath to point at it.
.PARAMETER BinPath
  Path to syn.exe. Defaults to .\syn.exe (same folder as this script).
.PARAMETER Uninstall
  Remove syn and clean PATH instead of installing.
#>
param(
    [string]$BinPath,
    [switch]$Uninstall
)

$ErrorActionPreference = 'Stop'
$installDir = Join-Path $env:LOCALAPPDATA 'Programs\syn'

function Add-ToUserPath([string]$dir) {
    $cur = [Environment]::GetEnvironmentVariable('PATH', 'User')
    if ($cur -split ';' -contains $dir) { return }
    [Environment]::SetEnvironmentVariable('PATH', "$cur;$dir", 'User')
    $env:PATH = "$env:PATH;$dir"
}

function Remove-FromUserPath([string]$dir) {
    $cur = [Environment]::GetEnvironmentVariable('PATH', 'User')
    $next = ($cur -split ';' | Where-Object { $_ -ne $dir }) -join ';'
    [Environment]::SetEnvironmentVariable('PATH', $next, 'User')
}

if ($Uninstall) {
    Write-Host 'Uninstalling synchronize...'
    if (Test-Path $installDir) {
        Remove-Item $installDir -Recurse -Force
        Write-Host "  removed $installDir"
    }
    Remove-FromUserPath $installDir
    Write-Host '  removed from PATH'
    Write-Host 'Done. Open a new terminal and syn will be gone.'
    exit 0
}

# --- install ---
if (-not $BinPath) {
    $BinPath = Join-Path $PSScriptRoot 'syn.exe'
}
if (-not (Test-Path $BinPath)) {
    Write-Error "syn.exe not found at: $BinPath`nPass -BinPath <path\to\syn.exe>"
}

Write-Host 'Installing synchronize (syn)...'
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
Copy-Item -Force $BinPath (Join-Path $installDir 'syn.exe')
Write-Host "  copied to $installDir\syn.exe"

Add-ToUserPath $installDir
Write-Host '  added to user PATH'

Write-Host ''
Write-Host 'Done! Open a new terminal and run: syn --version'
Write-Host ''
Write-Host 'Quick start:'
Write-Host '  syn auth login --pat <your-github-token>'
Write-Host '  syn repo add owner/repo-name'
Write-Host '  syn watch'
