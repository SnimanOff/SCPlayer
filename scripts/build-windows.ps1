[CmdletBinding()]
param(
    [string]$Version = '0.1.0',
    [string]$MPVDirectory,
    [string]$ConfigFile,
    [string]$DiscordClientID = $env:DISCORD_CLIENT_ID,
    [switch]$Installer
)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
$savedArch = $env:GOARCH
$savedCGO = $env:CGO_ENABLED
$savedPath = $env:PATH
try {
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '1'
    $localGCC = Join-Path $projectRoot '.tools\w64devkit\bin'
    if (Test-Path (Join-Path $localGCC 'gcc.exe')) { $env:PATH = "$localGCC;$env:PATH" }
    if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) { throw 'Install a 64-bit MinGW GCC compiler (see README.md).' }
    if ($Version -notmatch '^\d+\.\d+\.\d+(\.\d+)?$') { throw 'Version must be numeric: 0.1.0 or 0.1.0.1' }
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed.' }
    $stage = Join-Path $projectRoot 'dist\windows-amd64'
    New-Item -ItemType Directory -Force $stage | Out-Null
    
    if (Test-Path (Join-Path $stage 'sc-player.json')) { Remove-Item -LiteralPath (Join-Path $stage 'sc-player.json') }
    
    $keys = @{}
    $keysFile = Join-Path $projectRoot 'keys.env'
    if (Test-Path $keysFile) {
        foreach ($line in Get-Content -LiteralPath $keysFile) {
            if ($line -match '^\s*([A-Z_]+)\s*=\s*(.*?)\s*$') { $keys[$Matches[1]] = $Matches[2] }
        }
    }
    if (-not $DiscordClientID) { $DiscordClientID = $keys['DISCORD_CLIENT_ID'] }
    $ldflags = '-s -w -H=windowsgui'
    
    if ($DiscordClientID) {
        if ($DiscordClientID -notmatch '^\d+$') { throw 'DiscordClientID must be numeric.' }
        $ldflags += " -X sc_player/api/discord.DefaultClientID=$DiscordClientID"
    }
    
    & go run ./scripts/icon -syso
    if ($LASTEXITCODE -ne 0) { throw 'Icon generation failed.' }
    & go build -trimpath -ldflags $ldflags -o (Join-Path $stage 'sc-player.exe') .
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
    Copy-Item -LiteralPath 'README.md','sc-player.example.json' -Destination $stage
    if ($ConfigFile) { Copy-Item -LiteralPath $ConfigFile -Destination (Join-Path $stage 'sc-player.json') }
    if ($MPVDirectory) {
        if (-not (Test-Path (Join-Path $MPVDirectory 'mpv.exe'))) { throw 'MPVDirectory must contain mpv.exe and its distribution licenses.' }
        New-Item -ItemType Directory -Force (Join-Path $stage 'bin') | Out-Null
        Copy-Item -Path (Join-Path $MPVDirectory '*') -Destination (Join-Path $stage 'bin') -Recurse -Force
    }
    if ($Installer) {
        if (-not $MPVDirectory) { throw 'Installer requires -MPVDirectory so playback works after installation.' }
        $iscc = Get-Command ISCC.exe -ErrorAction SilentlyContinue
        if ($iscc) { $compiler = $iscc.Source }
        else {
            $compiler = @("$projectRoot\.tools\inno\ISCC.exe", "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe", "${env:ProgramFiles(x86)}\Inno Setup 7\ISCC.exe", "$env:ProgramFiles\Inno Setup 7\ISCC.exe") | Where-Object { Test-Path $_ } | Select-Object -First 1
        }
        if (-not $compiler) { throw 'Install Inno Setup or add ISCC.exe to PATH.' }
        & $compiler "/DAppVersion=$Version" (Join-Path $projectRoot 'packaging\windows\installer.iss')
        if ($LASTEXITCODE -ne 0) { throw 'Installer compilation failed.' }
    }
    Write-Host "Windows build: $stage"
} finally {
    $env:GOARCH = $savedArch
    $env:CGO_ENABLED = $savedCGO
    $env:PATH = $savedPath
    Pop-Location
}
