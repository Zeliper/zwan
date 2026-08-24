# M1b-1 admin verification: create the Wintun adapter, assign the overlay IP,
# ping it, then remove the adapter. Run in an ELEVATED PowerShell:
#
#   powershell -ExecutionPolicy Bypass -File .\scripts\m1b1_admin_test.ps1
#
# Results are printed and written to bin\m1b1-result.log.

$ErrorActionPreference = 'Continue'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$bin      = Join-Path $root 'bin'
$log      = Join-Path $bin  'm1b1-result.log'
$agentOut = Join-Path $bin  'agent.out'
$agentErr = Join-Path $bin  'agent.err'
Remove-Item $log, $agentOut, $agentErr -ErrorAction SilentlyContinue

function W([string]$m) { $m | Tee-Object -FilePath $log -Append }

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
W "== zwan M1b-1 admin test ==  admin=$isAdmin  time=$(Get-Date -Format o)"
if (-not $isAdmin) {
    W "ERROR: not elevated. Open PowerShell via 'Run as Administrator' and re-run."
    exit 1
}

if (-not (Test-Path (Join-Path $bin 'zwan-agent.exe'))) { W "ERROR: bin\zwan-agent.exe missing (build first)."; exit 1 }
if (-not (Test-Path (Join-Path $bin 'wintun.dll')))     { W "ERROR: bin\wintun.dll missing."; exit 1 }

$srv = Start-Process -PassThru -WindowStyle Hidden (Join-Path $bin 'zwan-server.exe') `
    -ArgumentList '--token','demo-token-123'
Start-Sleep -Seconds 1

$agent = Start-Process -PassThru -WindowStyle Hidden `
    -RedirectStandardOutput $agentOut -RedirectStandardError $agentErr `
    (Join-Path $bin 'zwan-agent.exe') `
    -ArgumentList '--tun','--token','demo-token-123','--device','pc-alpha','--name','alpha'
Start-Sleep -Seconds 6

W "`n-- agent output --"
Get-Content $agentOut, $agentErr -ErrorAction SilentlyContinue | ForEach-Object { W $_ }

W "`n-- Get-NetIPAddress MyWAN-demo --"
W ((Get-NetIPAddress -InterfaceAlias 'MyWAN-demo' -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    Format-Table IPAddress, PrefixLength, InterfaceAlias -Auto | Out-String).Trim())

W "`n-- ping 100.64.0.1 --"
W ((cmd /c "ping -n 2 100.64.0.1") | Out-String).Trim()

Stop-Process -Id $agent.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $srv.Id   -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 500
W "`n== done (adapter removed) =="
W "log: $log"
