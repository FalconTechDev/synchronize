#Requires -Version 5.1
<#
  Builds a Linux server tarball from Windows (or anywhere Go runs).

  Produces: dist/syn-server-linux-amd64.tar.gz
            dist/syn-server-linux-arm64.tar.gz  (when cross-compiling arm64)

  Usage:
    .\packaging\server\build.ps1
    .\packaging\server\build.ps1 -Arch amd64
    .\packaging\server\build.ps1 -Arch arm64
#>
param(
    [ValidateSet('amd64', 'arm64')]
    [string]$Arch = 'amd64',

    [string]$Version = ''
)

$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..\..')
$dist = Join-Path $root 'dist'
$staging = Join-Path $env:TEMP "syn-server-$Arch"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go is not installed. Get it from https://go.dev/dl"
}

$ldflags = '-s -w'
if ($Version) {
    $ldflags += " -X main.Version=v$Version"
}

Write-Host "Building syn for linux/$Arch (server, no tray/CGO)..." -ForegroundColor Cyan

# Set only for this build and put back afterwards: leaving GOOS=linux behind
# makes every later `go build` in the same shell produce a Linux binary, which
# is a confusing thing to debug.
$oldGoos, $oldGoarch, $oldCgo = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
$env:GOOS = 'linux'
$env:GOARCH = $Arch
$env:CGO_ENABLED = '0'

$outBin = Join-Path $root "bin/syn-linux-$Arch"
try {
    & go build -ldflags $ldflags -o $outBin ./cmd/syn/
    $buildCode = $LASTEXITCODE
} finally {
    $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $oldGoos, $oldGoarch, $oldCgo
}
if ($buildCode -ne 0) { throw "go build failed" }

Remove-Item -Recurse -Force $staging -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $staging | Out-Null

Copy-Item $outBin (Join-Path $staging 'syn')
Copy-Item (Join-Path $PSScriptRoot 'install.sh') (Join-Path $staging 'install.sh')

New-Item -ItemType Directory -Force -Path $dist | Out-Null
$tarball = Join-Path $dist "syn-server-linux-$Arch.tar.gz"

# Windows has no execute bit to carry into the tarball, so the files would
# unpack on the server as rw-rw-rw- and `sudo ./install.sh` would fail with
# Permission denied. GNU tar can set the mode on the way in; the bsdtar that
# ships in System32 cannot, so prefer git's copy of GNU tar - this project
# needs git anyway.
$gnuTar = @(
    (Join-Path $env:ProgramFiles 'Git\usr\bin\tar.exe'),
    (Join-Path ${env:ProgramFiles(x86)} 'Git\usr\bin\tar.exe')
) | Where-Object { $_ -and (Test-Path $_) } | Select-Object -First 1

Push-Location $staging
if ($gnuTar) {
    # Two things GNU tar needs when driven from PowerShell rather than from a
    # git shell: gzip on PATH, which lives beside it, and --force-local, or
    # it reads the "C:" in the output path as a remote host name.
    $oldPath = $env:PATH
    $env:PATH = (Split-Path $gnuTar) + ';' + $env:PATH
    try {
        & $gnuTar czf $tarball --force-local --mode=755 syn install.sh
    } finally {
        $env:PATH = $oldPath
    }
} elseif (Get-Command tar -ErrorAction SilentlyContinue) {
    & tar czf $tarball syn install.sh
} else {
    Pop-Location
    throw "no tar found. Install Git for Windows, or build on Linux/macOS."
}
$tarCode = $LASTEXITCODE
Pop-Location
if ($tarCode -ne 0) { throw "tar failed" }

# Prove it rather than trust it: a tarball whose installer will not run is
# worse than none, because it fails on the server instead of here.
$executable = $true
foreach ($line in (& tar tzvf $tarball)) {
    if ($line -notmatch '^-rwx') { $executable = $false }
}
if (-not $executable) {
    Write-Host ""
    Write-Host "  NOTE: this tar could not set the execute bit." -ForegroundColor Yellow
    Write-Host "        On the server run:  sudo sh install.sh" -ForegroundColor Yellow
}

$mb = [Math]::Round((Get-Item $tarball).Length / 1MB, 2)
Write-Host ""
Write-Host "Done: $tarball ($mb MB)" -ForegroundColor Green
Write-Host ""
Write-Host "Copy to the server and run:"
Write-Host "  scp $tarball user@server:/tmp/"
Write-Host "  ssh user@server"
Write-Host "  cd /tmp && tar xzf syn-server-linux-$Arch.tar.gz"
if ($executable) {
    Write-Host "  sudo ./install.sh --service --as deploy"
} else {
    Write-Host "  sudo sh install.sh --service --as deploy"
}
