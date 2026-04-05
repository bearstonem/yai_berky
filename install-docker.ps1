#Requires -Version 5.1
<#
.SYNOPSIS
    Build and install Helm as a Docker container on Windows.

.DESCRIPTION
    Checks for Docker, builds the Helm image, and creates a wrapper script
    so 'helm' can be invoked directly from the terminal.

.PARAMETER InstallDir
    Directory to place the wrapper script. Defaults to $HOME\.local\bin.
#>

[CmdletBinding()]
param(
    [string]$InstallDir = (Join-Path $env:USERPROFILE ".local\bin")
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ImageName  = "helm:latest"
$ScriptDir  = Split-Path -Parent $MyInvocation.MyCommand.Definition

# ── Helpers ──────────────────────────────────────────────────────────

function Write-Info  { param([string]$Msg) Write-Host "==> " -ForegroundColor Blue -NoNewline; Write-Host $Msg -ForegroundColor White }
function Write-Ok    { param([string]$Msg) Write-Host " +  " -ForegroundColor Green -NoNewline; Write-Host $Msg }
function Write-Warn  { param([string]$Msg) Write-Host " !  " -ForegroundColor Yellow -NoNewline; Write-Host $Msg }
function Write-Fail  { param([string]$Msg) Write-Host " X  " -ForegroundColor Red -NoNewline; Write-Host $Msg; exit 1 }

# ── Check prerequisites ─────────────────────────────────────────────

Write-Info "Checking prerequisites"

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Fail "Docker is not installed or not on PATH.`n  Install Docker Desktop from https://www.docker.com/products/docker-desktop/ and re-run this script."
}
Write-Ok "Docker $(docker --version | ForEach-Object { $_ -replace 'Docker version ','' -replace ',.*','' })"

# Verify Docker daemon is running
$null = docker info 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Fail "Docker daemon is not running. Start Docker Desktop and re-run this script."
}
Write-Ok "Docker daemon is running"

# ── Build image ─────────────────────────────────────────────────────

Write-Info "Building Docker image"

Push-Location $ScriptDir
try {
    & docker build -t $ImageName .
    if ($LASTEXITCODE -ne 0) {
        Write-Fail "Docker build failed (exit code $LASTEXITCODE)"
    }
    Write-Ok "Built image $ImageName"
} finally {
    Pop-Location
}

# ── Create wrapper script ───────────────────────────────────────────

Write-Info "Creating wrapper script"

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$envFileFlag = ""
$envFile = Join-Path $ScriptDir ".env"
if (Test-Path $envFile) {
    $envFileFlag = "--env-file `"$envFile`""
}

$wrapperPath = Join-Path $InstallDir "helm.cmd"
$wrapperContent = @"
@echo off
docker run --rm -it ^
    -v helm-data:/root/.config/helm ^
    $envFileFlag ^
    -p 8080:8080 ^
    $ImageName %*
"@

Set-Content -Path $wrapperPath -Value $wrapperContent -Encoding ASCII
Write-Ok "Created $wrapperPath"

# ── Ensure it's on user PATH ────────────────────────────────────────

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -split ";" | Where-Object { $_ -eq $InstallDir }) {
    Write-Ok "$InstallDir is already on PATH"
} else {
    try {
        $newPath = "$InstallDir;$userPath"
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        $env:Path = "$InstallDir;$env:Path"
        Write-Ok "Added $InstallDir to user PATH"
        Write-Warn "Restart your terminal for PATH changes to take effect in new sessions"
    } catch {
        Write-Warn "Could not update PATH automatically. Add this directory to your PATH manually:"
        Write-Host "    $InstallDir"
    }
}

# ── Done ────────────────────────────────────────────────────────────

Write-Host ""
Write-Info "Helm (Docker) is ready!"
Write-Host ""
Write-Host "  helm --setup          " -ForegroundColor White -NoNewline; Write-Host "first-time setup wizard (start here!)"
Write-Host "  helm                  " -ForegroundColor White -NoNewline; Write-Host "interactive REPL"
Write-Host "  helm --gui            " -ForegroundColor White -NoNewline; Write-Host "web GUI (agents, skills, settings)"
Write-Host "  helm -a <task>        " -ForegroundColor White -NoNewline; Write-Host "agent mode (autonomous)"
Write-Host "  helm -c <question>    " -ForegroundColor White -NoNewline; Write-Host "chat with the AI"
Write-Host "  helm -e <query>       " -ForegroundColor White -NoNewline; Write-Host "generate a single command"
Write-Host ""
Write-Host "  Data is persisted in Docker volume " -NoNewline; Write-Host "helm-data" -ForegroundColor White
Write-Host ""
