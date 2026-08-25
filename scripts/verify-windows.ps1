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

# --- teardown ----------------------------------------------------------------

$script:started = @()
function Launch($exe, $arguments, $log) {
    $p = Start-Process -FilePath $exe -ArgumentList $arguments -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput $log -RedirectStandardError "$log.err"
    $script:started += $p
    return $p
}
$nrptTag = 'zwan-split-dns'
function OurNrptRules {
    Get-DnsClientNrptRule -ErrorAction SilentlyContinue | Where-Object { $_.Comment -eq $nrptTag }
}
function Teardown {
    foreach ($p in $script:started) {
        if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
    }
    Start-Sleep -Seconds 1
    # Killing the engine is exactly the case that leaves name resolution rules
    # behind, and this script must not leave the machine changed.
    OurNrptRules | ForEach-Object { Remove-DnsClientNrptRule -Name $_.Name -Force -ErrorAction SilentlyContinue }
    Clear-DnsClientCache -ErrorAction SilentlyContinue
    Get-NetAdapter -ErrorAction SilentlyContinue |
        Where-Object { $_.InterfaceDescription -like "*Wintun*" -and $_.Name -like "MyWAN-*" } |
        ForEach-Object { Write-Host "  (adapter $($_.Name) still present; it should disappear when the engine exits)" }
}

# A check that throws rather than failing must still put the machine back:
# otherwise servers keep running, their binaries stay locked against the next
# build, and adapters and name resolution rules are left behind.
trap {
    Write-Host "`n  ERROR  $_" -ForegroundColor Red
    Write-Host "  (tearing down what was started)" -ForegroundColor Yellow
    Teardown
    break
}

# --- preconditions -----------------------------------------------------------

Step "Preconditions"
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "This needs an elevated prompt: it creates network adapters."
}
Pass "running elevated"

# The engine under test needs 127.0.0.1:53 and the machine's name resolution
# policy to itself. An installed engine already holds both.
$installed = Get-Service zwanEngine -ErrorAction SilentlyContinue
if ($installed -and $installed.Status -eq 'Running') {
    throw "The installed engine service holds 127.0.0.1:53. Stop it first: sc.exe stop zwanEngine"
}
Pass "no installed engine is competing for the resolver"

# The engine's resolver binds 127.0.0.1:53, so ask the question the way it does
# - by binding. Looking at who else is listening answers a different question:
# Internet Connection Sharing holds 0.0.0.0:53 on any machine with WSL2 or the
# Hyper-V default switch, and a specific address binds straight through it.
$probe = New-Object System.Net.Sockets.UdpClient
try {
    $probe.Client.Bind([System.Net.IPEndPoint]::new([System.Net.IPAddress]::Loopback, 53))
    Pass "127.0.0.1:53 is free for the resolver"
} catch {
    $who = (Get-NetUDPEndpoint -LocalPort 53 -ErrorAction SilentlyContinue |
        ForEach-Object { (Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue).ProcessName } |
        Sort-Object -Unique) -join ', '
    throw "127.0.0.1:53 cannot be bound, so the engine's resolver will not start. Holding UDP 53: $who"
} finally {
    $probe.Close()
}

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
    # The server announces its pin through Go's log package, which writes to
    # stderr - so the banner is in the .err file, not the one next to it.
    $line = Select-String -Path @($log, "$log.err") -Pattern 'sha256:[A-Za-z0-9+/=]+' `
        -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $line) { throw "no pin in $log or $log.err (did the server start?)" }
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

Step "The system resolves those names too, not just our resolver"
# Without this the resolver answers only whoever queries 127.0.0.1 directly, so
# `ping probe.alice` still goes to the public internet (design doc 39).
$rules = @(OurNrptRules)
Check ($rules.Count -eq 2) "one policy rule per joined network ($(($rules | ForEach-Object { $_.Namespace }) -join ', '))"
foreach ($alias in @('alice', 'bob')) {
    $rule = $rules | Where-Object { $_.Namespace -contains ".$alias" }
    Check ($null -ne $rule -and $rule.NameServers -contains '127.0.0.1') "*.$alias is pointed at 127.0.0.1"
}

Clear-DnsClientCache
$viaSystem = @{}
foreach ($alias in @('alice', 'bob')) {
    try {
        # No -Server: this is the path every other program on the machine takes.
        $answer = Resolve-DnsName -Name "probe.$alias" -Type A -ErrorAction Stop
        $viaSystem[$alias] = $answer.IPAddress
        Check ($answer.IPAddress) "probe.$alias -> $($answer.IPAddress) through the system resolver"
    } catch {
        Fail "probe.$alias did not resolve through the system resolver: $_"
    }
}
Check ($viaSystem['alice'] -and $viaSystem['bob'] -and $viaSystem['alice'] -ne $viaSystem['bob']) `
    "the same name in two networks resolves to two different local addresses"
try {
    $direct = [System.Net.Dns]::GetHostAddresses("probe.alice")
    Check ($direct.Count -ge 1) "a plain socket lookup of probe.alice works ($($direct[0]))"
} catch {
    Fail "a plain socket lookup of probe.alice failed: $_"
}
try {
    $null = [System.Net.Dns]::GetHostAddresses("localhost")
    Pass "ordinary name resolution is untouched"
} catch {
    Fail "ordinary name resolution broke: $_"
}

Step "Leaving a network takes its rule out"
$d = Engine '{"op":"disconnect","alias":"bob"}'
Check ($d.ok) "disconnected bob: $($d.error)"
Start-Sleep -Seconds 4
$rules = @(OurNrptRules)
Check ($rules.Count -eq 1 -and ($rules[0].Namespace -contains '.alice')) `
    "only alice's rule is left ($(($rules | ForEach-Object { $_.Namespace }) -join ', '))"
Clear-DnsClientCache
$gone = $null
try { $gone = Resolve-DnsName -Name "probe.bob" -Type A -ErrorAction Stop } catch { }
Check ($null -eq $gone) "probe.bob stopped resolving the moment the network was left"

# Put bob back: the checks below expect both networks up.
$b = Engine $joinB
Check ($b.ok) "rejoined bob: $($b.error)"
Start-Sleep -Seconds 4

# The agent reports through Go's log package, which writes to stderr. Merging
# that into the pipeline turns every line into an ErrorRecord, and under
# ErrorActionPreference=Stop the banner line alone ends the script - so the
# preference is relaxed for the length of the call. It is function-scoped, so it
# is back to Stop the moment this returns.
function Publish($device, $node, $service, $port, $backend) {
    $ErrorActionPreference = 'Continue'
    $out = & (Join-Path $work "zwan-agent.exe") --server "https://127.0.0.1:18901" --pin $pinA --token tokA `
        --device $device --name $node --publish-name $service --publish-port $port `
        --publish-backend-port $backend 2>&1
    $out | Out-File -FilePath (Join-Path $work "publish-$service.log") -Encoding utf8
    $said = $out | Select-String "published"
    $said | ForEach-Object { Write-Host "  $_" }
    return [bool]$said
}

Step "Two services, same port, told apart by address"
Check (Publish 'svc1' 'host1' 'minecraft' 25565 31001) "published minecraft (else see publish-minecraft.log)"
Check (Publish 'svc2' 'host2' 'survival' 25565 31002) "published survival (else see publish-survival.log)"

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

Step "Stopping cleanly leaves no rule behind"
# A rule outlives the process that wrote it, so a clean stop has to take every
# one of them out; a killed one is caught by the purge on the next start.
foreach ($alias in @('alice', 'bob')) { $null = Engine ('{"op":"disconnect","alias":"' + $alias + '"}') }
Start-Sleep -Seconds 4
$rules = @(OurNrptRules)
Check ($rules.Count -eq 0) "no policy rules of ours remain ($(($rules | ForEach-Object { $_.Namespace }) -join ', '))"

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
