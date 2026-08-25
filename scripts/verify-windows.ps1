# Hands-on verification for the parts that need Administrator and Wintun.
#
# Everything else is covered by `go test ./...`. This covers what a test cannot
# reach: real adapters, real addresses and routes, a real tunnel, and the two
# networks-at-once case that the whole per-network address translation exists for.
#
#   powershell -ExecutionPolicy Bypass -File scripts\verify-windows.ps1
#
# Run it from an elevated prompt. It leaves nothing behind: every server, engine
# and adapter it creates is torn down at the end.

[CmdletBinding()]
param(
    [switch]$KeepRunning   # leave everything up at the end, to poke at by hand
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$work = Join-Path $env:TEMP "zwan-verify"
$state = Join-Path $work "state"
$script:failures = @()

function Step($name) { Write-Host "`n== $name" -ForegroundColor Cyan }
function Pass($msg) { Write-Host "  PASS  $msg" -ForegroundColor Green }
function Fail($msg) {
    Write-Host "  FAIL  $msg" -ForegroundColor Red
    $script:failures += $msg
}
function Check($cond, $msg) { if ($cond) { Pass $msg } else { Fail $msg } }

# --- preconditions -----------------------------------------------------------

Step "Preconditions"
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "This needs an elevated prompt: it creates network adapters."
}
Pass "running elevated"

$go = "C:\Program Files\Go\bin\go.exe"
if (-not (Test-Path $go)) { $go = (Get-Command go -ErrorAction SilentlyContinue).Source }
if (-not $go) { throw "Go was not found; install it or put it on PATH." }

New-Item -ItemType Directory -Force -Path $work, $state | Out-Null
if (-not (Test-Path (Join-Path $work "wintun.dll"))) {
    Copy-Item (Join-Path $repo "bin\wintun.dll") $work
}
Pass "wintun.dll staged next to the binaries"

Step "Build"
& $go build -o (Join-Path $work "zwan-server.exe") "$repo\cmd\zwan-server"
& $go build -o (Join-Path $work "zwan-service.exe") "$repo\cmd\zwan-service"
& $go build -o (Join-Path $work "zwan-agent.exe") "$repo\cmd\zwan-agent"
Pass "server, engine service and agent built"

# --- teardown ----------------------------------------------------------------

$script:started = @()
function Launch($exe, $arguments, $log) {
    $p = Start-Process -FilePath $exe -ArgumentList $arguments -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput $log -RedirectStandardError "$log.err"
    $script:started += $p
    return $p
}
function Teardown {
    foreach ($p in $script:started) {
        if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
    }
    Start-Sleep -Seconds 1
    Get-NetAdapter -ErrorAction SilentlyContinue |
        Where-Object { $_.InterfaceDescription -like "*Wintun*" -and $_.Name -like "MyWAN-*" } |
        ForEach-Object { Write-Host "  (adapter $($_.Name) still present; it should disappear when the engine exits)" }
}

# --- two control servers, deliberately sharing an overlay range --------------

Step "Control servers (same overlay range on purpose)"
$srvA = Launch (Join-Path $work "zwan-server.exe") `
    "--addr 127.0.0.1:18901 --relay-addr 127.0.0.1:13901 --token tokA --network alice --dns-suffix alice.zwan --cidr 100.64.0.0/16 --tls-dir `"$work\tlsA`"" `
    (Join-Path $work "srvA.log")
$srvB = Launch (Join-Path $work "zwan-server.exe") `
    "--addr 127.0.0.1:18902 --relay-addr 127.0.0.1:13902 --token tokB --network bob --dns-suffix bob.zwan --cidr 100.64.0.0/16 --tls-dir `"$work\tlsB`"" `
    (Join-Path $work "srvB.log")
Start-Sleep -Seconds 3

function PinOf($log) {
    $line = Select-String -Path $log -Pattern 'sha256:[A-Za-z0-9+/=]+' | Select-Object -First 1
    if (-not $line) { throw "no pin in $log" }
    return [regex]::Match($line.Line, 'sha256:[A-Za-z0-9+/=]+').Value
}
$pinA = PinOf (Join-Path $work "srvA.log")
$pinB = PinOf (Join-Path $work "srvB.log")
Check ($pinA -and $pinB -and $pinA -ne $pinB) "both servers up, each with its own key pin"

# --- the engine service, driven the way the GUI drives it --------------------

Step "Engine service"
$env:ZWAN_STATE_DIR = $state
$eng = Launch (Join-Path $work "zwan-service.exe") "run" (Join-Path $work "engine.log")
Start-Sleep -Seconds 3

function Engine([string]$json) {
    $pipe = New-Object System.IO.Pipes.NamedPipeClientStream('.', 'zwan-engine', [System.IO.Pipes.PipeDirection]::InOut)
    $pipe.Connect(5000)
    $w = New-Object System.IO.StreamWriter($pipe); $w.AutoFlush = $true
    $w.WriteLine($json)
    $r = New-Object System.IO.StreamReader($pipe)
    $line = $r.ReadLine(); $pipe.Dispose()
    return $line | ConvertFrom-Json
}
Check ((Engine '{"op":"status"}').ok) "the engine service answers on its pipe"

Step "Join both networks at once"
$joinA = @{ op = 'connect'; network = @{ alias = 'alice'; server = "https://127.0.0.1:18901"; pin = $pinA; token = 'tokA'; name = 'probe'; useRelay = $true } } | ConvertTo-Json -Compress -Depth 5
$joinB = @{ op = 'connect'; network = @{ alias = 'bob'; server = "https://127.0.0.1:18902"; pin = $pinB; token = 'tokB'; name = 'probe'; useRelay = $true } } | ConvertTo-Json -Compress -Depth 5
$a = Engine $joinA
Check ($a.ok) "joined alice: $($a.error)"
$b = Engine $joinB
Check ($b.ok) "joined bob: $($b.error)"

Start-Sleep -Seconds 4
$st = Engine '{"op":"status"}'
foreach ($n in $st.networks) {
    Write-Host ("  {0,-6} connected={1} device={2} overlay={3} local-range={4}" -f `
        $n.network.alias, $n.engine.connected, $n.engine.assignedIp, $n.engine.overlayIp, $n.engine.localCidr)
}

Step "Each network is separated on this device"
$alice = $st.networks | Where-Object { $_.network.alias -eq 'alice' }
$bob = $st.networks | Where-Object { $_.network.alias -eq 'bob' }

Check ($alice.engine.connected -and $bob.engine.connected) "both networks are up at the same time"
Check ($alice.engine.overlayIp -eq $bob.engine.overlayIp) `
    "both servers handed out the same overlay address ($($alice.engine.overlayIp)) - the case translation exists for"
Check ($alice.engine.assignedIp -ne $bob.engine.assignedIp) `
    "this device holds two different addresses: $($alice.engine.assignedIp) and $($bob.engine.assignedIp)"
Check ($alice.engine.localCidr -ne $bob.engine.localCidr) `
    "each network got its own local range: $($alice.engine.localCidr) and $($bob.engine.localCidr)"
Check (-not $alice.warning -and -not $bob.warning) "no overlap warning, because the ranges are translated"

$adapters = Get-NetAdapter | Where-Object { $_.Name -like "MyWAN-*" }
Check ($adapters.Count -ge 2) "two virtual adapters exist ($($adapters.Name -join ', '))"
foreach ($n in @($alice, $bob)) {
    $addr = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.IPAddress -eq $n.engine.assignedIp }
    Check ($null -ne $addr) "$($n.network.alias): $($n.engine.assignedIp) is on an interface"
}

Step "Names resolve per network"
foreach ($alias in @('alice', 'bob')) {
    try {
        $answer = Resolve-DnsName -Name "probe.$alias" -Server 127.0.0.1 -Type A -DnsOnly -ErrorAction Stop
        Check ($answer.IPAddress) "probe.$alias -> $($answer.IPAddress)"
    } catch {
        Fail "probe.$alias did not resolve against the local resolver: $_"
    }
}

Step "Two services, same port, told apart by address"
& (Join-Path $work "zwan-agent.exe") --server "https://127.0.0.1:18901" --pin $pinA --token tokA `
    --device svc1 --name host1 --publish-name minecraft --publish-port 25565 --publish-backend-port 31001 2>&1 |
    Select-String "published" | ForEach-Object { Write-Host "  $_" }
& (Join-Path $work "zwan-agent.exe") --server "https://127.0.0.1:18901" --pin $pinA --token tokA `
    --device svc2 --name host2 --publish-name survival --publish-port 25565 --publish-backend-port 31002 2>&1 |
    Select-String "published" | ForEach-Object { Write-Host "  $_" }

Start-Sleep -Seconds 4
$st = Engine '{"op":"status"}'
$svcs = ($st.networks | Where-Object { $_.network.alias -eq 'alice' }).engine.services
Check ($svcs.Count -ge 2) "the client sees both services"
if ($svcs.Count -ge 2) {
    $ports = ($svcs | ForEach-Object { $_.port } | Sort-Object -Unique)
    $vips = ($svcs | ForEach-Object { $_.vip } | Sort-Object -Unique)
    Check ($ports.Count -eq 1) "both use the same port ($($ports -join ','))"
    Check ($vips.Count -eq $svcs.Count) "each has its own address ($($vips -join ', '))"
    foreach ($s in $svcs) {
        $route = Get-NetRoute -DestinationPrefix "$($s.vip)/32" -ErrorAction SilentlyContinue
        Check ($null -ne $route) "$($s.name): $($s.vip)/32 is routed into the tunnel"
    }
}

# --- result ------------------------------------------------------------------

Step "Result"
if ($script:failures.Count -eq 0) {
    Write-Host "  everything checked here passed" -ForegroundColor Green
} else {
    Write-Host "  $($script:failures.Count) check(s) failed:" -ForegroundColor Red
    $script:failures | ForEach-Object { Write-Host "    - $_" -ForegroundColor Red }
}
Write-Host "`n  logs: $work"
Write-Host "  still to do by hand, on two machines: reach a peer's address and a service name from the other side."

if ($KeepRunning) {
    Write-Host "`n  -KeepRunning: leaving everything up. Stop it with:" -ForegroundColor Yellow
    Write-Host "    Get-Process zwan-service,zwan-server | Stop-Process -Force"
} else {
    Step "Teardown"
    Teardown
    Pass "stopped"
}

if ($script:failures.Count -gt 0) { exit 1 }
