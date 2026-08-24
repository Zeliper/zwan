# M1b-2 two-client tunnel test: start the control server and two agents (--up) on
# this host, and verify they establish a WireGuard handshake with each other.
#
# NOTE: A real overlay *ping* between the two clients needs two separate machines
# (on one host both overlay IPs are local, so ping never enters the tunnel). What
# this test proves on a single PC is that the encrypted WireGuard tunnel between
# the two clients establishes (handshake OK) — i.e. keys, endpoints and the data
# plane all work.
#
# Run in an ELEVATED PowerShell:
#   powershell -ExecutionPolicy Bypass -File .\scripts\m1b2_admin_test.ps1
# Results are written to bin\m1b2-result.log.

$ErrorActionPreference = 'Continue'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$bin  = Join-Path $root 'bin'
$log  = Join-Path $bin 'm1b2-result.log'
$aOut = Join-Path $bin 'alpha.out'; $aErr = Join-Path $bin 'alpha.err'
$bOut = Join-Path $bin 'beta.out';  $bErr = Join-Path $bin 'beta.err'
Remove-Item $log, $aOut, $aErr, $bOut, $bErr -ErrorAction SilentlyContinue

function W([string]$m) { $m | Tee-Object -FilePath $log -Append }

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
W "== zwan M1b-2 two-client tunnel test ==  admin=$isAdmin  time=$(Get-Date -Format o)"
if (-not $isAdmin) { W "ERROR: not elevated. Open PowerShell via 'Run as Administrator' and re-run."; exit 1 }

Get-Process zwan-agent, zwan-server -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 500

$srv = Start-Process -PassThru -WindowStyle Hidden (Join-Path $bin 'zwan-server.exe') `
    -ArgumentList '--token','demo-token-123'
Start-Sleep -Seconds 1

$alpha = Start-Process -PassThru -WindowStyle Hidden -RedirectStandardOutput $aOut -RedirectStandardError $aErr `
    (Join-Path $bin 'zwan-agent.exe') `
    -ArgumentList '--up','--token','demo-token-123','--device','pc-alpha','--name','alpha','--adapter','MyWAN-demo-a','--wg-port','51820'
$beta = Start-Process -PassThru -WindowStyle Hidden -RedirectStandardOutput $bOut -RedirectStandardError $bErr `
    (Join-Path $bin 'zwan-agent.exe') `
    -ArgumentList '--up','--token','demo-token-123','--device','pc-beta','--name','beta','--adapter','MyWAN-demo-b','--wg-port','51821'

W "waiting 16s for tunnel handshake ..."
Start-Sleep -Seconds 16

W "`n-- MyWAN adapters --"
W ((Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    Where-Object { $_.InterfaceAlias -like 'MyWAN-*' } |
    Format-Table IPAddress, InterfaceAlias -Auto | Out-String).Trim())

W "`n-- alpha log --"
Get-Content $aErr, $aOut -ErrorAction SilentlyContinue | ForEach-Object { W $_ }
W "`n-- beta log --"
Get-Content $bErr, $bOut -ErrorAction SilentlyContinue | ForEach-Object { W $_ }

Stop-Process -Id $alpha.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $beta.Id  -Force -ErrorAction SilentlyContinue
Stop-Process -Id $srv.Id   -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 800
W "`n== done (adapters removed) =="
W "log: $log"
