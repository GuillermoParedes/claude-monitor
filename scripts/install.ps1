# claude-monitor installer for Windows (native Claude Code).
#
#   irm https://raw.githubusercontent.com/GuillermoParedes/claude-monitor/main/scripts/install.ps1 | iex
#
# If you run Claude Code inside WSL, run install.sh inside WSL instead: it sets
# up the collector there and installs this tray app on the Windows side.
$ErrorActionPreference = 'Stop'

$repo = 'GuillermoParedes/claude-monitor'
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$asset = "claude-monitor_windows_$arch.exe"
$base = "https://github.com/$repo/releases/latest/download"

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("claude-monitor-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    $exe = Join-Path $tmp 'claude-monitor.exe'
    Write-Host "Downloading $asset ..."
    Invoke-WebRequest -UseBasicParsing "$base/$asset" -OutFile $exe

    try {
        $sums = (Invoke-WebRequest -UseBasicParsing "$base/checksums.txt").Content
        $line = ($sums -split "`n") | Where-Object { $_ -match " $([regex]::Escape($asset))$" } | Select-Object -First 1
        if ($line) {
            $expected = ($line -split ' ')[0].Trim()
            $actual = (Get-FileHash $exe -Algorithm SHA256).Hash.ToLower()
            if ($expected -ne $actual) { throw "Checksum mismatch for $asset" }
        }
    } catch [System.Net.WebException] { }

    # claude-monitor.exe is a GUI app (no console window). -NoNewWindow lets it
    # print here. We wait on the process itself, not with -Wait: in Windows
    # PowerShell 5.1 that also waits for children, and install leaves the tray
    # app running in the background.
    $argList = @('install') + @($args)
    $p = Start-Process -FilePath $exe -ArgumentList $argList -NoNewWindow -PassThru
    $null = $p.Handle  # needed for ExitCode to be populated
    $p.WaitForExit()
    if ($p.ExitCode -ne 0) { throw "install failed ($($p.ExitCode))" }
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
