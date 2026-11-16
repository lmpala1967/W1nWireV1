<#
.SYNOPSIS
    Build the W1nWire release artifacts on Windows.

.DESCRIPTION
    Produces, in dist/:
      * w1nwire-desktop.exe            the Windows application (icon, version info, GUI subsystem)
      * w1nwire-server-linux-amd64     the server for a normal VPS / Oracle x86 instance
      * w1nwire-server-linux-arm64     the server for Oracle Ampere (ARM) instances
      * w1nwire-client-linux-amd64     the command line client for Linux
      * install.sh                     the server installer, next to the binaries
      * W1nWire-<version>-windows-x64.zip
      * SHA256SUMS.txt

.PARAMETER Version
    Version stamped into the binaries and into the file name of the zip.

.PARAMETER OutDir
    Output directory, dist/ by default.

.PARAMETER SkipTests
    Do not run the unit tests before building.

.EXAMPLE
    pwsh -File scripts/build-windows.ps1 -Version 2.0.0
#>
[CmdletBinding()]
param(
    [string]$Version = "2.0.0",
    [string]$OutDir = "",
    [switch]$SkipTests,
    [switch]$SkipResources
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
if (-not $OutDir) { $OutDir = Join-Path $root "dist" }
$OutDir = [System.IO.Path]::GetFullPath($OutDir)
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

function Step([string]$message) {
    Write-Host "==> $message" -ForegroundColor Cyan
}

function Fail([string]$message) {
    Write-Host "ERROR: $message" -ForegroundColor Red
    exit 1
}

$goCommand = Get-Command go -ErrorAction SilentlyContinue
if (-not $goCommand) {
    Fail "Go was not found in PATH. Install Go 1.24+ from https://go.dev/dl/ and run this script again."
}
$go = $goCommand.Source
Step "Using $(& $go version)"

Push-Location $root
try {
    $env:CGO_ENABLED = "0"
    $env:GOFLAGS = ""

    if (-not $SkipTests) {
        Step "Running the unit tests"
        & $go test ./...
        if ($LASTEXITCODE -ne 0) { Fail "the tests failed" }
    }

    if (-not $SkipResources) {
        Step "Generating the Windows resources (icon, manifest, version info)"
        Push-Location (Join-Path $root "cmd/w1nwire-desktop")
        try {
            & $go run github.com/tc-hib/go-winres@v0.3.3 simply `
                --arch amd64 `
                --out rsrc `
                --manifest gui `
                --icon ../../packaging/icon/w1nwire.ico `
                --product-version $Version `
                --file-version $Version `
                --file-description "W1nWire VPN client" `
                --product-name "W1nWire" `
                --original-filename "w1nwire-desktop.exe" `
                --copyright "MIT License"
            if ($LASTEXITCODE -ne 0) { Fail "go-winres failed" }
        }
        finally {
            Pop-Location
        }
    }

    Step "Building w1nwire-desktop.exe"
    & $go build -trimpath -buildvcs=false `
        -ldflags "-s -w -H windowsgui -X main.version=$Version" `
        -o (Join-Path $OutDir "w1nwire-desktop.exe") ./cmd/w1nwire-desktop
    if ($LASTEXITCODE -ne 0) { Fail "the desktop build failed" }

    Step "Building the Linux server and client"
    $env:GOOS = "linux"
    foreach ($arch in @("amd64", "arm64")) {
        $env:GOARCH = $arch
        & $go build -trimpath -buildvcs=false `
            -ldflags "-s -w -X main.version=$Version" `
            -o (Join-Path $OutDir "w1nwire-server-linux-$arch") ./cmd/w1nwire-server
        if ($LASTEXITCODE -ne 0) { Fail "the server build for linux/$arch failed" }
        & $go build -trimpath -buildvcs=false `
            -ldflags "-s -w -X main.version=$Version" `
            -o (Join-Path $OutDir "w1nwire-client-linux-$arch") ./cmd/w1nwire-client
        if ($LASTEXITCODE -ne 0) { Fail "the client build for linux/$arch failed" }
    }
    Remove-Item Env:GOOS
    Remove-Item Env:GOARCH

    Step "Copying the server installer"
    Copy-Item (Join-Path $root "deploy/install.sh") (Join-Path $OutDir "install.sh") -Force

    Step "Packaging the Windows archive"
    $zip = Join-Path $OutDir "W1nWire-$Version-windows-x64.zip"
    if (Test-Path $zip) { Remove-Item $zip -Force }
    Compress-Archive -Path (Join-Path $OutDir "w1nwire-desktop.exe") -DestinationPath $zip

    Step "Writing SHA256SUMS.txt"
    $sums = Get-ChildItem $OutDir -File |
        Where-Object { $_.Name -ne "SHA256SUMS.txt" } |
        Sort-Object Name |
        ForEach-Object {
            $hash = (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
            "$hash  $($_.Name)"
        }
    $sums | Set-Content -Path (Join-Path $OutDir "SHA256SUMS.txt") -Encoding ascii

    Step "Done"
    Get-ChildItem $OutDir -File |
        Sort-Object Name |
        Select-Object Name, @{n = "Size"; e = { "{0:N1} KB" -f ($_.Length / 1KB) } } |
        Format-Table -AutoSize
}
finally {
    Pop-Location
}
