# M1b-3 relay test (optional full-path check): start the control server (with the
# relay) and two agents in --relay mode on this host. The two clients tunnel to
# each other THROUGH the server relay (neither sends directly to the other), so a
# "handshake OK" here exercises the whole relay path end to end.
#
# The relay path is already covered without admin by `go test ./test` and by
# `wgprobe --relay`; this script additionally confirms the full agent + Wintun path.
#
# Run in an ELEVATED PowerShell:
#   powershell -ExecutionPolicy Bypass -File .\scripts\m1b3_admin_test.ps1
# Results are written to bin\m1b3-result.log.

$ErrorActionPreference = 'Continue'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$bin  = Join-Path $root 'bin'
$log  = Join-Path $bin 'm1b3-result.log'
$aOut = Join-Path $bin 'alpha.out'; $aErr = Join-Path $bin 'alpha.err'
$bOut = Join-Path $bin 'beta.out';  $bErr = Join-Path $bin 'beta.err'
Remove-Item $log, $aOut, $aErr, $bOut, $bErr -ErrorAction SilentlyContinue

function W([string]$m) { $m | Tee-Object -FilePath $log -Append }

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
W "== zwan M1b-3 relay test ==  admin=$isAdmin  time=$(Get-Date -Format o)"
if (-not $isAdmin) { W "ERROR: not elevated. Open PowerShell via 'Run as Administrator' and re-run."; exit 1 }

Get-Process zwan-agent, zwan-server -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 500

$srv = Start-Process -PassThru -WindowStyle Hidden (Join-Path $bin 'zwan-server.exe') `
    -ArgumentList '--token','demo-token-123','--relay-addr','127.0.0.1:3478'
Start-Sleep -Seconds 1

$alpha = Start-Process -PassThru -WindowStyle Hidden -RedirectStandardOutput $aOut -RedirectStandardError $aErr `
    (Join-Path $bin 'zwan-agent.exe') `
    -ArgumentList '--up','--relay','--token','demo-token-123','--device','pc-alpha','--name','alpha','--adapter','MyWAN-demo-a'
$beta = Start-Process -PassThru -WindowStyle Hidden -RedirectStandardOutput $bOut -RedirectStandardError $bErr `
    (Join-Path $bin 'zwan-agent.exe') `
    -ArgumentList '--up','--relay','--token','demo-token-123','--device','pc-beta','--name','beta','--adapter','MyWAN-demo-b'

W "waiting 16s for relay handshake ..."
Start-Sleep -Seconds 16

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
