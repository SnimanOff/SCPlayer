[CmdletBinding()]
param([string]$ISCC)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
if (-not $ISCC) {
    $command = Get-Command ISCC.exe -ErrorAction SilentlyContinue
    if ($command) { $ISCC = $command.Source }
    else {
        $ISCC = @("$projectRoot\.tools\inno\ISCC.exe", "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe", "${env:ProgramFiles(x86)}\Inno Setup 7\ISCC.exe", "$env:ProgramFiles\Inno Setup 7\ISCC.exe") | Where-Object { Test-Path $_ } | Select-Object -First 1
    }
}
if (-not $ISCC) { throw 'Install Inno Setup or pass -ISCC /path/to/ISCC.exe.' }
$testID = 'SCPlayer.SmokeTest.' + [guid]::NewGuid().ToString('N')
$outputName = 'installer-smoke-' + [guid]::NewGuid().ToString('N')
$target = Join-Path $projectRoot "build\$outputName"
$installer = Join-Path $projectRoot "dist\$outputName.exe"
& $ISCC "/DAppIdentifier=$testID" "/F$outputName" (Join-Path $projectRoot 'packaging\windows\installer.iss')
if ($LASTEXITCODE -ne 0) { throw 'Test installer compilation failed.' }
try {
    $arguments = @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/SP-', '/NOICONS', "/DIR=`"$target`"")
    $process = Start-Process -FilePath $installer -ArgumentList $arguments -WindowStyle Hidden -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw "Installation failed: $($process.ExitCode)" }
    foreach ($relative in 'sc-player.exe','bin\mpv.exe','sc-player.example.json','unins000.exe') {
        if (-not (Test-Path (Join-Path $target $relative))) { throw "Missing installed file: $relative" }
    }
    $configPath = Join-Path $target 'sc-player.json'
    [System.IO.File]::WriteAllText($configPath, '{"discord_client_id":"preserved"}')
    $process = Start-Process -FilePath $installer -ArgumentList $arguments -WindowStyle Hidden -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw 'Upgrade failed.' }
    if ((Get-Content -Raw $configPath) -notmatch 'preserved') { throw 'Upgrade overwrote configuration.' }
} finally {
    $uninstaller = Join-Path $target 'unins000.exe'
    if (Test-Path $uninstaller) {
        $process = Start-Process -FilePath $uninstaller -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART' -WindowStyle Hidden -Wait -PassThru
        if ($process.ExitCode -ne 0) { throw "Uninstallation failed: $($process.ExitCode)" }
    }
}
if (Test-Path (Join-Path $target 'sc-player.exe')) { throw 'Uninstaller left application binary behind.' }
if (Test-Path (Join-Path $target 'bin\mpv.exe')) { throw 'Uninstaller left mpv behind.' }
if (-not (Test-Path (Join-Path $target 'sc-player.json'))) { throw 'Uninstaller removed user configuration.' }
Write-Host 'Windows installer install/upgrade/uninstall smoke tests passed.'
