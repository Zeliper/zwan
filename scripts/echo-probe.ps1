# An echo service and the client that talks to it, for the two-machine test.
#
# Everything else has been proved on one machine. What has never been tried is a
# packet leaving one host and arriving at another: the tunnel, the per-network
# address translation and the L4 proxy have only ever seen loopback.
#
# On the node that hosts the service - the backend the agent forwards to:
#
#   powershell -ExecutionPolicy Bypass -File scripts\echo-probe.ps1 -Listen -Proto tcp -Port 31001
#   powershell -ExecutionPolicy Bypass -File scripts\echo-probe.ps1 -Listen -Proto udp -Port 31003
#
# From the other machine, by name, with no port to look up beyond the service's own:
#
#   powershell -ExecutionPolicy Bypass -File scripts\echo-probe.ps1 -Connect echo.home -Port 25565
#   powershell -ExecutionPolicy Bypass -File scripts\echo-probe.ps1 -Connect voice.home -Proto udp -Port 64738
#
# -Size sends a payload of that many bytes, which is how the tunnel's MTU gets
# tested: a large UDP datagram cannot be segmented away the way a TCP stream can,
# so it either survives the path or it does not. Useful sizes are 64 (does it work
# at all), 1400 (just under the tunnel MTU) and 2000 (must fragment). Over UDP the
# ceiling is the 65535-byte datagram limit; over TCP any size works, and a few
# hundred KB is enough to see a stream reassembled across many segments.

[CmdletBinding(DefaultParameterSetName = 'Listen')]
param(
    [Parameter(ParameterSetName = 'Listen', Mandatory = $true)][switch]$Listen,
    [Parameter(ParameterSetName = 'Connect', Mandatory = $true)][string]$Connect,
    [ValidateSet('tcp', 'udp')][string]$Proto = 'tcp',
    [int]$Port = 31001,
    [int]$Size = 64,
    [int]$TimeoutMs = 5000
)

$ErrorActionPreference = 'Stop'

function Payload([int]$n) {
    # Recognisable and position-revealing, so a truncated or reordered reply is
    # obvious rather than merely "different".
    $sb = New-Object System.Text.StringBuilder
    $i = 0
    while ($sb.Length -lt $n) {
        [void]$sb.Append(("[{0}]" -f $i))
        $i++
    }
    return $sb.ToString().Substring(0, $n)
}

# --- listen ------------------------------------------------------------------

function ListenTcp([int]$port) {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $port)
    $listener.Start()
    Write-Host "tcp echo on 127.0.0.1:$port - Ctrl+C to stop" -ForegroundColor Cyan
    try {
        while ($true) {
            $client = $listener.AcceptTcpClient()
            $peer = $client.Client.RemoteEndPoint
            try {
                # Read to end of stream before echoing anything. A tunnel with a
                # 1420-byte MTU splits a payload across many reads, so one Read
                # is one segment, not one message - and echoing while still
                # reading can wedge both ends once the socket buffers fill.
                $stream = $client.GetStream()
                $sink = New-Object System.IO.MemoryStream
                $buf = New-Object byte[] 65536
                while (($n = $stream.Read($buf, 0, $buf.Length)) -gt 0) {
                    $sink.Write($buf, 0, $n)
                }
                $body = $sink.ToArray()
                if ($body.Length -gt 0) {
                    $stream.Write($body, 0, $body.Length)
                    $stream.Flush()
                }
                Write-Host ("  {0} bytes from {1}" -f $body.Length, $peer) -ForegroundColor Green
            } finally {
                $client.Close()
            }
        }
    } finally {
        $listener.Stop()
    }
}

function ListenUdp([int]$port) {
    $sock = [System.Net.Sockets.UdpClient]::new(
        [System.Net.IPEndPoint]::new([System.Net.IPAddress]::Loopback, $port))
    Write-Host "udp echo on 127.0.0.1:$port - Ctrl+C to stop" -ForegroundColor Cyan
    try {
        while ($true) {
            $from = [System.Net.IPEndPoint]::new([System.Net.IPAddress]::Any, 0)
            $data = $sock.Receive([ref]$from)
            [void]$sock.Send($data, $data.Length, $from)
            Write-Host ("  {0} bytes from {1}" -f $data.Length, $from) -ForegroundColor Green
        }
    } finally {
        $sock.Close()
    }
}

# --- connect -----------------------------------------------------------------

function ConnectTcp([string]$target, [int]$port, [string]$text) {
    $client = New-Object System.Net.Sockets.TcpClient
    try {
        # Connect with a deadline: a name that resolves to an address nothing
        # answers at would otherwise hang for the OS's own timeout.
        $async = $client.BeginConnect($target, $port, $null, $null)
        if (-not $async.AsyncWaitHandle.WaitOne($TimeoutMs)) {
            throw "no answer at ${target}:${port} within ${TimeoutMs}ms"
        }
        $client.EndConnect($async)
        $client.ReceiveTimeout = $TimeoutMs
        $stream = $client.GetStream()
        $send = [System.Text.Encoding]::ASCII.GetBytes($text)
        $stream.Write($send, 0, $send.Length)
        $stream.Flush()
        # Close our half so the far end sees end of stream and knows the message
        # is complete. Length is not on the wire, so this is what delimits it.
        $client.Client.Shutdown([System.Net.Sockets.SocketShutdown]::Send)

        $got = New-Object byte[] $send.Length
        $read = 0
        while ($read -lt $send.Length) {
            $n = $stream.Read($got, $read, $send.Length - $read)
            if ($n -le 0) { break }
            $read += $n
        }
        return [System.Text.Encoding]::ASCII.GetString($got, 0, $read)
    } finally {
        $client.Close()
    }
}

function ConnectUdp([string]$target, [int]$port, [string]$text) {
    $sock = New-Object System.Net.Sockets.UdpClient
    try {
        $sock.Client.ReceiveTimeout = $TimeoutMs
        $sock.Connect($target, $port)
        $send = [System.Text.Encoding]::ASCII.GetBytes($text)
        [void]$sock.Send($send, $send.Length)
        $from = [System.Net.IPEndPoint]::new([System.Net.IPAddress]::Any, 0)
        $data = $sock.Receive([ref]$from)
        return [System.Text.Encoding]::ASCII.GetString($data)
    } finally {
        $sock.Close()
    }
}

# --- main --------------------------------------------------------------------

if ($Listen) {
    if ($Proto -eq 'udp') { ListenUdp $Port } else { ListenTcp $Port }
    return
}

# Report what the name resolved to before using it: when this fails, whether the
# name or the path is at fault is the first thing worth knowing.
try {
    $addrs = [System.Net.Dns]::GetHostAddresses($Connect) | ForEach-Object { $_.IPAddressToString }
    Write-Host ("{0} -> {1}" -f $Connect, ($addrs -join ', ')) -ForegroundColor Cyan
} catch {
    Write-Host "FAIL  $Connect did not resolve: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

$text = Payload $Size
$started = Get-Date
try {
    $echoed = if ($Proto -eq 'udp') {
        ConnectUdp $Connect $Port $text
    } else {
        ConnectTcp $Connect $Port $text
    }
} catch {
    Write-Host "FAIL  $Proto to ${Connect}:${Port} - $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
$ms = [int]((Get-Date) - $started).TotalMilliseconds

if ($echoed -eq $text) {
    Write-Host ("PASS  {0} {1} bytes echoed by {2}:{3} in {4}ms" -f $Proto, $Size, $Connect, $Port, $ms) -ForegroundColor Green
    exit 0
}
Write-Host ("FAIL  sent {0} bytes, got {1} back" -f $text.Length, $echoed.Length) -ForegroundColor Red
if ($echoed.Length -gt 0) {
    Write-Host ("      first difference at byte {0}" -f (
        (0..([Math]::Min($text.Length, $echoed.Length) - 1) | Where-Object { $text[$_] -ne $echoed[$_] } | Select-Object -First 1)))
}
exit 1
