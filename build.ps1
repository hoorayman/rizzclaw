param(
    [Parameter(Position=0)]
    [string]$Target = "build",

    [string]$Version = $(git describe --tags --always --dirty 2>$null),
    [string]$BuildTime = $(Get-Date -Format "yyyy-MM-dd_HH:mm:ss"),
    [string]$AppName = "rizzclaw",
    [string]$OutputDir = "bin"
)

$ErrorActionPreference = "Stop"

$ldflags = "-s -w -X main.Version=$Version -X main.BuildTime=$BuildTime"
$goBuild = "go build"
$env:CGO_ENABLED = "0"

function New-OutputDir {
    if (-not (Test-Path $OutputDir)) {
        New-Item -ItemType Directory -Path $OutputDir | Out-Null
    }
}

function Build-Current {
    Write-Host "Building for current platform..." -ForegroundColor Cyan
    & go build -ldflags $ldflags -o $AppName .
}

function Build-Linux {
    Write-Host "Building for Linux AMD64..." -ForegroundColor Cyan
    New-OutputDir
    $env:GOOS = "linux"; $env:GOARCH = "amd64"
    & go build -ldflags $ldflags -o "$OutputDir/$AppName-linux-amd64" .
}

function Build-LinuxArm {
    Write-Host "Building for Linux ARM32 (Raspberry Pi 2/3/4)..." -ForegroundColor Cyan
    Write-Host "Note: 32-bit builds may fail due to larksuite/oapi-sdk-go int overflow issue" -ForegroundColor Yellow
    New-OutputDir
    $env:GOOS = "linux"; $env:GOARCH = "arm"; $env:GOARM = "7"
    & go build -ldflags $ldflags -o "$OutputDir/$AppName-linux-arm32v7" .
}

function Build-LinuxArm64 {
    Write-Host "Building for Linux ARM64 (Raspberry Pi 4/5)..." -ForegroundColor Cyan
    New-OutputDir
    $env:GOOS = "linux"; $env:GOARCH = "arm64"
    & go build -ldflags $ldflags -o "$OutputDir/$AppName-linux-arm64" .
}

function Build-Rpi {
    Write-Host "Building for Raspberry Pi Zero/1 (ARMv6)..." -ForegroundColor Cyan
    Write-Host "WARNING: 32-bit builds may fail due to larksuite/oapi-sdk-go int overflow issue" -ForegroundColor Yellow
    Write-Host "Consider using ARM64 builds for Raspberry Pi 4/5 instead" -ForegroundColor Yellow
    New-OutputDir
    $env:GOOS = "linux"; $env:GOARCH = "arm"; $env:GOARM = "6"
    & go build -ldflags $ldflags -o "$OutputDir/$AppName-rpi-arm32v6" .
}

function Build-Windows {
    Write-Host "Building for Windows AMD64..." -ForegroundColor Cyan
    New-OutputDir
    $env:GOOS = "windows"; $env:GOARCH = "amd64"
    & go build -ldflags $ldflags -o "$OutputDir/$AppName-windows-amd64.exe" .
}

function Build-WindowsArm64 {
    Write-Host "Building for Windows ARM64..." -ForegroundColor Cyan
    New-OutputDir
    $env:GOOS = "windows"; $env:GOARCH = "arm64"
    & go build -ldflags $ldflags -o "$OutputDir/$AppName-windows-arm64.exe" .
}

function Build-Darwin {
    Write-Host "Building for macOS AMD64..." -ForegroundColor Cyan
    New-OutputDir
    $env:GOOS = "darwin"; $env:GOARCH = "amd64"
    & go build -ldflags $ldflags -o "$OutputDir/$AppName-darwin-amd64" .
}

function Build-DarwinArm64 {
    Write-Host "Building for macOS ARM64 (Apple Silicon)..." -ForegroundColor Cyan
    New-OutputDir
    $env:GOOS = "darwin"; $env:GOARCH = "arm64"
    & go build -ldflags $ldflags -o "$OutputDir/$AppName-darwin-arm64" .
}

function Build-All {
    Build-Linux
    Build-LinuxArm
    Build-LinuxArm64
    Build-Rpi
    Build-Windows
    Build-WindowsArm64
    Build-Darwin
    Build-DarwinArm64
    Write-Host "`nAll builds completed in $OutputDir/" -ForegroundColor Green
}

function Clear-Build {
    Write-Host "Cleaning build artifacts..." -ForegroundColor Yellow
    if (Test-Path $OutputDir) {
        Remove-Item -Recurse -Force $OutputDir
    }
    if (Test-Path $AppName) {
        Remove-Item -Force $AppName
    }
    if (Test-Path "$AppName.exe") {
        Remove-Item -Force "$AppName.exe"
    }
}

function Show-Help {
    Write-Host "RizzClaw Build System" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "Usage: .\build.ps1 [target]" -ForegroundColor White
    Write-Host ""
    Write-Host "Targets:" -ForegroundColor Yellow
    Write-Host "  build              Build for current platform"
    Write-Host ""
    Write-Host "  Linux Builds:"
    Write-Host "    linux            Build for Linux AMD64"
    Write-Host "    linux-arm        Build for Linux ARM32 (Raspberry Pi 2/3/4)"
    Write-Host "    linux-arm64      Build for Linux ARM64 (Raspberry Pi 4/5)"
    Write-Host "    rpi              Build for Raspberry Pi Zero/1 (ARMv6)"
    Write-Host ""
    Write-Host "  Windows Builds:"
    Write-Host "    windows          Build for Windows AMD64"
    Write-Host "    windows-arm64    Build for Windows ARM64"
    Write-Host ""
    Write-Host "  macOS Builds:"
    Write-Host "    darwin           Build for macOS AMD64"
    Write-Host "    darwin-arm64     Build for macOS ARM64 (Apple Silicon)"
    Write-Host ""
    Write-Host "  Build All:"
    Write-Host "    all              Build for all platforms"
    Write-Host ""
    Write-Host "  Utilities:"
    Write-Host "    clean            Remove build artifacts"
    Write-Host "    help             Show this help message"
    Write-Host ""
    Write-Host "Examples:" -ForegroundColor Yellow
    Write-Host "  .\build.ps1 build          # Build for current platform"
    Write-Host "  .\build.ps1 linux-arm      # Build for Raspberry Pi 2/3/4"
    Write-Host "  .\build.ps1 rpi            # Build for Raspberry Pi Zero/1"
    Write-Host "  .\build.ps1 all            # Build for all platforms"
}

switch ($Target.ToLower()) {
    "build"           { Build-Current }
    "linux"           { Build-Linux }
    "linux-arm"       { Build-LinuxArm }
    "linux-arm64"     { Build-LinuxArm64 }
    "rpi"             { Build-Rpi }
    "windows"         { Build-Windows }
    "windows-arm64"   { Build-WindowsArm64 }
    "darwin"          { Build-Darwin }
    "darwin-arm64"    { Build-DarwinArm64 }
    "all"             { Build-All }
    "clean"           { Clear-Build }
    "help"            { Show-Help }
    default           {
        Write-Host "Unknown target: $Target" -ForegroundColor Red
        Show-Help
        exit 1
    }
}
