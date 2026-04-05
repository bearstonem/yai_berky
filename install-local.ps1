#Requires -Version 5.1
<#
.SYNOPSIS
    Build and install Helm from source on Windows.

.DESCRIPTION
    Checks prerequisites (Go, C compiler), builds the Helm binary with CGO,
    installs it, and optionally adds it to the user PATH.

.PARAMETER InstallDir
    Directory to install the binary into. Defaults to $HOME\.local\bin.
#>

[CmdletBinding()]
param(
    [string]$InstallDir = (Join-Path $env:USERPROFILE ".local\bin")
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$BinaryName = "helm.exe"
$ScriptDir  = Split-Path -Parent $MyInvocation.MyCommand.Definition

# ── Helpers ──────────────────────────────────────────────────────────

function Write-Info  { param([string]$Msg) Write-Host "==> " -ForegroundColor Blue -NoNewline; Write-Host $Msg -ForegroundColor White }
function Write-Ok    { param([string]$Msg) Write-Host " +  " -ForegroundColor Green -NoNewline; Write-Host $Msg }
function Write-Warn  { param([string]$Msg) Write-Host " !  " -ForegroundColor Yellow -NoNewline; Write-Host $Msg }
function Write-Fail  { param([string]$Msg) Write-Host " X  " -ForegroundColor Red -NoNewline; Write-Host $Msg; exit 1 }

function Install-WithWinget {
    param([string]$PackageId, [string]$Name)
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        Write-Fail "$Name is not installed and winget is not available to install it automatically.`n  Install $Name manually, then re-run this script."
    }
    Write-Warn "$Name not found. Installing via winget..."
    & winget install --id $PackageId --exact --accept-source-agreements --accept-package-agreements
    if ($LASTEXITCODE -ne 0) {
        Write-Fail "winget install of $Name failed (exit code $LASTEXITCODE). Install it manually and re-run."
    }
    Write-Ok "Installed $Name via winget"
}

function Refresh-Path {
    # Reload PATH from the registry so newly-installed tools are visible
    $machinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $userPath    = [Environment]::GetEnvironmentVariable("Path", "User")
    $env:Path    = "$machinePath;$userPath"
}

# ── Check prerequisites ─────────────────────────────────────────────

Write-Info "Checking prerequisites"

# Git (needed for go modules)
if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Install-WithWinget "Git.Git" "Git"
    Refresh-Path
}
if (Get-Command git -ErrorAction SilentlyContinue) {
    Write-Ok "Git $(git --version | ForEach-Object { $_ -replace 'git version ','' })"
} else {
    Write-Warn "Git was installed but is not on PATH yet. You may need to restart your terminal after this script."
}

# Go
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Install-WithWinget "GoLang.Go" "Go"
    Refresh-Path
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Fail "Go was installed but is not on PATH. Restart your terminal and re-run this script."
}
$goVer = (go version) -replace '^go version go','' -replace ' .*',''
Write-Ok "Go $goVer"

# C compiler (CGO required for sqlite-vec / sqlite3)
$ccFound = $false
foreach ($cc in @("gcc", "clang", "cc")) {
    if (Get-Command $cc -ErrorAction SilentlyContinue) {
        Write-Ok "C compiler: $cc"
        $ccFound = $true
        break
    }
}

# Also check common MSYS2 MinGW paths even if not on PATH
if (-not $ccFound) {
    $mingwPaths = @(
        "C:\msys64\ucrt64\bin",
        "C:\msys64\mingw64\bin",
        "C:\msys64\clang64\bin"
    )
    foreach ($mp in $mingwPaths) {
        if (Test-Path (Join-Path $mp "gcc.exe")) {
            Write-Warn "Found gcc at $mp but it's not on PATH. Adding to session PATH."
            $env:Path = "$mp;$env:Path"
            Write-Ok "C compiler: gcc (from $mp)"
            $ccFound = $true
            # Also persist to user PATH
            $currentUserPath = [Environment]::GetEnvironmentVariable("Path", "User")
            if (-not ($currentUserPath -split ";" | Where-Object { $_ -eq $mp })) {
                [Environment]::SetEnvironmentVariable("Path", "$mp;$currentUserPath", "User")
                Write-Ok "Added $mp to user PATH permanently"
            }
            break
        }
    }
}

if (-not $ccFound) {
    # Try to auto-install MSYS2 via winget, then install gcc inside it
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        Write-Warn "No C compiler found. Installing MSYS2 via winget..."
        & winget install --id MSYS2.MSYS2 --exact --accept-source-agreements --accept-package-agreements
        if ($LASTEXITCODE -ne 0) {
            Write-Fail "MSYS2 installation failed. Install a C compiler manually and re-run."
        }
        Write-Ok "Installed MSYS2"

        # Install gcc and sqlite3 headers inside MSYS2
        $msys2Bash = "C:\msys64\usr\bin\bash.exe"
        if (Test-Path $msys2Bash) {
            Write-Info "Installing MinGW-w64 GCC and SQLite3 headers via MSYS2 pacman..."
            & $msys2Bash -lc "pacman -S --noconfirm mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-sqlite3" 2>&1 | Out-Null
            if ($LASTEXITCODE -eq 0) {
                Write-Ok "Installed mingw-w64-ucrt-x86_64-gcc and sqlite3 headers"
            } else {
                Write-Warn "pacman install failed. You may need to run manually:"
                Write-Host "    C:\msys64\usr\bin\bash.exe -lc 'pacman -S --noconfirm mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-sqlite3'"
            }
        }

        # Add MSYS2 ucrt64 to PATH
        $ucrt64Bin = "C:\msys64\ucrt64\bin"
        if (Test-Path (Join-Path $ucrt64Bin "gcc.exe")) {
            $env:Path = "$ucrt64Bin;$env:Path"
            $currentUserPath = [Environment]::GetEnvironmentVariable("Path", "User")
            if (-not ($currentUserPath -split ";" | Where-Object { $_ -eq $ucrt64Bin })) {
                [Environment]::SetEnvironmentVariable("Path", "$ucrt64Bin;$currentUserPath", "User")
            }
            Write-Ok "C compiler: gcc (from $ucrt64Bin)"
            $ccFound = $true
        }
    }

    if (-not $ccFound) {
        Write-Fail "No C compiler found and could not install one automatically.`n  Install MSYS2 from https://www.msys2.org/ then run:`n    pacman -S mingw-w64-ucrt-x86_64-gcc`n  Add C:\msys64\ucrt64\bin to your PATH, then re-run this script."
    }
}

# SQLite3 development headers (required by sqlite-vec CGO binding)
$sqlite3HeaderFound = $false
$ucrt64Include = "C:\msys64\ucrt64\include"
if (Test-Path (Join-Path $ucrt64Include "sqlite3.h")) {
    Write-Ok "SQLite3 development headers"
    $sqlite3HeaderFound = $true
} else {
    # Try to install via MSYS2 pacman
    $msys2Bash = "C:\msys64\usr\bin\bash.exe"
    if (Test-Path $msys2Bash) {
        Write-Warn "SQLite3 headers not found. Installing via MSYS2 pacman..."
        & $msys2Bash -lc "pacman -S --noconfirm mingw-w64-ucrt-x86_64-sqlite3" 2>&1 | Out-Null
        if (($LASTEXITCODE -eq 0) -and (Test-Path (Join-Path $ucrt64Include "sqlite3.h"))) {
            Write-Ok "Installed SQLite3 development headers"
            $sqlite3HeaderFound = $true
        }
    }
    if (-not $sqlite3HeaderFound) {
        Write-Fail "SQLite3 development headers not found.`n  Run: C:\msys64\usr\bin\bash.exe -lc 'pacman -S --noconfirm mingw-w64-ucrt-x86_64-sqlite3'`n  Then re-run this script."
    }
}

# ── Build ────────────────────────────────────────────────────────────

Write-Info "Building Helm from source"

Push-Location $ScriptDir
try {
    $env:CGO_ENABLED = "1"

    # Point CGO at MSYS2 ucrt64 headers and libraries so sqlite-vec can find sqlite3.h
    $ucrt64Base = "C:\msys64\ucrt64"
    if (Test-Path (Join-Path $ucrt64Base "include\sqlite3.h")) {
        $env:CGO_CFLAGS  = "-I$ucrt64Base\include"
        $env:CGO_LDFLAGS = "-L$ucrt64Base\lib"
    }

    & go build -ldflags="-s -w" -o $BinaryName .
    if ($LASTEXITCODE -ne 0) {
        Write-Fail "Build failed (exit code $LASTEXITCODE)"
    }
    Write-Ok "Built $ScriptDir\$BinaryName"
} finally {
    Pop-Location
}

# ── Install ──────────────────────────────────────────────────────────

Write-Info "Installing to $InstallDir"

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$src = Join-Path $ScriptDir $BinaryName
$dst = Join-Path $InstallDir $BinaryName
Move-Item -Path $src -Destination $dst -Force
Write-Ok "Installed $dst"

# ── Clean up old yai binary if present ───────────────────────────────

$oldBinary = Join-Path $InstallDir "yai.exe"
if (Test-Path $oldBinary) {
    Remove-Item $oldBinary -Force
    Write-Ok "Removed old yai.exe binary"
}

# ── Ensure it's on user PATH ────────────────────────────────────────

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -split ";" | Where-Object { $_ -eq $InstallDir }) {
    Write-Ok "$InstallDir is already on PATH"
} else {
    Write-Warn "$InstallDir is not on your PATH"
    try {
        $newPath = "$InstallDir;$userPath"
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        # Also update the current session so the verify step works
        $env:Path = "$InstallDir;$env:Path"
        Write-Ok "Added $InstallDir to user PATH"
        Write-Warn "Restart your terminal for PATH changes to take effect in new sessions"
    } catch {
        Write-Warn "Could not update PATH automatically. Add this directory to your PATH manually:"
        Write-Host "    $InstallDir"
    }
}

# ── Migrate data from yai if needed (one-time) ──────────────────────

$helmConfigDir = Join-Path $env:USERPROFILE ".config\helm"
$migrationMarker = Join-Path $helmConfigDir ".migrated_from_yai"
$yaiConfigDir = Join-Path $env:USERPROFILE ".config\yai"
$yaiConfigFile = Join-Path $env:USERPROFILE ".config\yai.json"
$helmConfigFile = Join-Path $env:USERPROFILE ".config\helm.json"

if ((-not (Test-Path $migrationMarker)) -and ((Test-Path $yaiConfigDir) -or (Test-Path $yaiConfigFile))) {
    Write-Info "Migrating data from yai -> helm (one-time)"

    if ((Test-Path $yaiConfigFile) -and (-not (Test-Path $helmConfigFile))) {
        Copy-Item $yaiConfigFile $helmConfigFile
        Write-Ok "Migrated config"
    }

    foreach ($subdir in @("sessions", "skills", "agents")) {
        $src = Join-Path $yaiConfigDir $subdir
        $dstDir = Join-Path $helmConfigDir $subdir
        if ((Test-Path $src) -and (Get-ChildItem $src -ErrorAction SilentlyContinue | Select-Object -First 1)) {
            if (-not (Test-Path $dstDir)) {
                New-Item -ItemType Directory -Path $dstDir -Force | Out-Null
            }
            Copy-Item "$src\*" $dstDir -Recurse -Force -ErrorAction SilentlyContinue
            Write-Ok "Migrated $subdir"
        }
    }

    $yaiMemDb = Join-Path $yaiConfigDir "memory.db"
    $helmMemDb = Join-Path $helmConfigDir "memory.db"
    if ((Test-Path $yaiMemDb) -and (-not (Test-Path $helmMemDb))) {
        if (-not (Test-Path $helmConfigDir)) {
            New-Item -ItemType Directory -Path $helmConfigDir -Force | Out-Null
        }
        Copy-Item $yaiMemDb $helmMemDb
        Write-Ok "Migrated memory.db"
    }

    if (-not (Test-Path $helmConfigDir)) {
        New-Item -ItemType Directory -Path $helmConfigDir -Force | Out-Null
    }
    New-Item -ItemType File -Path $migrationMarker -Force | Out-Null
    Write-Ok "Migration complete (won't run again)"
}

# ── Verify ───────────────────────────────────────────────────────────

Write-Host ""
if (Get-Command helm -ErrorAction SilentlyContinue) {
    Write-Info "Helm is ready!"
    Write-Host ""
    Write-Host "  helm --setup          " -ForegroundColor White -NoNewline; Write-Host "first-time setup wizard (start here!)"
    Write-Host "  helm                  " -ForegroundColor White -NoNewline; Write-Host "interactive REPL"
    Write-Host "  helm --gui            " -ForegroundColor White -NoNewline; Write-Host "web GUI (agents, skills, settings)"
    Write-Host "  helm -a <task>        " -ForegroundColor White -NoNewline; Write-Host "agent mode (autonomous)"
    Write-Host "  helm -c <question>    " -ForegroundColor White -NoNewline; Write-Host "chat with the AI"
    Write-Host "  helm -e <query>       " -ForegroundColor White -NoNewline; Write-Host "generate a single command"
    Write-Host "  helm --pipe -a <task> " -ForegroundColor White -NoNewline; Write-Host "headless mode (no TUI, for scripts)"
    Write-Host ""
    Write-Host "  Press " -NoNewline; Write-Host "tab" -ForegroundColor White -NoNewline; Write-Host " inside the REPL to switch modes"
} else {
    Write-Info "Almost ready!"
    Write-Host ""
    Write-Host "  Restart your terminal, then run:"
    Write-Host ""
    Write-Host "  helm --setup          " -ForegroundColor White -NoNewline; Write-Host "first-time setup wizard (start here!)"
    Write-Host "  helm                  " -ForegroundColor White -NoNewline; Write-Host "interactive REPL"
}
Write-Host ""
