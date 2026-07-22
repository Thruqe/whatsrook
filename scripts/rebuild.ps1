# PowerShell rebuild script for Windows executable updates
param (
    [string]$PIDToWait = ""
)

if ($PIDToWait -ne "") {
    Write-Host "Waiting for process $PIDToWait to exit..."
    try {
        Wait-Process -Id $PIDToWait -Timeout 10 -ErrorAction SilentlyContinue
    } catch {}
    Start-Sleep -Seconds 1
}

Write-Host "Rebuilding whatsrook binary via go build..."
$buildOut = & go build -o whatsrook.exe . 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Error "Build failed: $buildOut"
    exit $LASTEXITCODE
}

Write-Host "Starting updated whatsrook process..."
$argsToPass = $args
Start-Process -FilePath ".\whatsrook.exe" -ArgumentList $argsToPass -NoNewWindow
