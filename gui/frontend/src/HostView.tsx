import { useCallback, useEffect, useRef, useState } from 'react'
import { Copy, Play, RefreshCw, Server, Square, Users, Share2, ShieldAlert, ShieldCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { HostGenToken, HostStart, HostStatus, HostStop } from '../wailsjs/go/main/App'

interface Peer {
  hostname: string
  public_key: string
  assigned_ip: string
}
interface Service {
  name: string
  proto: string
  port: number
  node_ip: string
}
interface HostState {
  running: boolean
  networkId: string
  dnsSuffix: string
  cidr: string
  token: string
  controlAddr: string
  relayAddr: string
  tlsMode: string
  pin: string
  joinUrl: string
  peers: Peer[]
  services: Service[]
}

export default function HostView() {
  const [networkId, setNetworkId] = useState('home')
  const [suffix, setSuffix] = useState('home.zwan')
  const [cidr, setCidr] = useState('100.64.0.0/16')
  const [token, setToken] = useState('')
  const [controlAddr, setControlAddr] = useState('0.0.0.0:8787')
  const [relayAddr, setRelayAddr] = useState('0.0.0.0:3478')
  const [tlsMode, setTlsMode] = useState('auto')
  const [domain, setDomain] = useState('')
  const [publicHost, setPublicHost] = useState('')
  const [state, setState] = useState<HostState | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const poll = useRef<number | null>(null)

  const gen = useCallback(async () => {
    try {
      setToken(await HostGenToken())
    } catch {
      /* ignore */
    }
  }, [])

  const refresh = useCallback(async () => {
    try {
      const s = (await HostStatus()) as unknown as HostState
      setState(s?.running ? s : null)
    } catch {
      setState(null)
    }
  }, [])

  useEffect(() => {
    gen()
    refresh()
    poll.current = window.setInterval(refresh, 3000)
    return () => {
      if (poll.current) window.clearInterval(poll.current)
    }
  }, [gen, refresh])

  async function start() {
    setBusy(true)
    setError('')
    try {
      await HostStart(networkId, suffix, cidr, token, controlAddr, relayAddr, tlsMode, domain, publicHost)
      await refresh()
    } catch (e: any) {
      setError(String(e?.message ?? e))
    } finally {
      setBusy(false)
    }
  }

  async function stop() {
    setBusy(true)
    try {
      await HostStop()
      setState(null)
    } finally {
      setBusy(false)
    }
  }

  const running = !!state?.running

  if (!running) {
    return (
      <Card className="mx-auto max-w-md">
        <CardHeader>
          <CardTitle>Host a network</CardTitle>
          <CardDescription>Run a control server + relay on this machine (needs a public IP for remote clients).</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="net">Network</Label>
              <Input id="net" value={networkId} onChange={(e) => setNetworkId(e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="suffix">DNS suffix</Label>
              <Input id="suffix" value={suffix} onChange={(e) => setSuffix(e.target.value)} />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="cidr">Overlay CIDR</Label>
            <Input id="cidr" value={cidr} onChange={(e) => setCidr(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="token">Join token</Label>
            <div className="flex gap-2">
              <Input id="token" value={token} onChange={(e) => setToken(e.target.value)} className="font-mono" />
              <Button variant="outline" size="icon" onClick={gen} title="Generate">
                <RefreshCw className="h-4 w-4" />
              </Button>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="ctrl">Control addr</Label>
              <Input id="ctrl" value={controlAddr} onChange={(e) => setControlAddr(e.target.value)} className="font-mono" />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="relay">Relay addr</Label>
              <Input id="relay" value={relayAddr} onChange={(e) => setRelayAddr(e.target.value)} className="font-mono" />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="tls">TLS</Label>
            <select
              id="tls"
              value={tlsMode}
              onChange={(e) => setTlsMode(e.target.value)}
              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            >
              <option value="auto">Automatic — certificate for the domain, otherwise a pinned key</option>
              <option value="self">Self-signed key, clients pin it</option>
              <option value="acme">Public certificate (ACME) for the domain</option>
              <option value="off">Off — plaintext HTTP (local testing only)</option>
            </select>
            {tlsMode === 'off' && (
              <p className="text-xs text-destructive">Join tokens and all control traffic would be sent unencrypted.</p>
            )}
          </div>
          {tlsMode !== 'off' && (
            <div className="space-y-1.5">
              <Label htmlFor="domain">Domain (optional)</Label>
              <Input
                id="domain"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
                placeholder="vpn.example.com — leave empty to use a pinned key"
                className="font-mono"
              />
              <p className="text-xs text-muted-foreground">
                With a domain pointed at this machine, a certificate is issued automatically (needs port 443, or port 80 reachable).
              </p>
            </div>
          )}
          <div className="space-y-1.5">
            <Label htmlFor="public">Public address (optional)</Label>
            <Input
              id="public"
              value={publicHost}
              onChange={(e) => setPublicHost(e.target.value)}
              placeholder="203.0.113.5:8787 — what clients should connect to"
              className="font-mono"
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button className="w-full" onClick={start} disabled={busy || !token}>
            <Play className="h-4 w-4" /> Start hosting
          </Button>
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-lg font-semibold">
            {state!.networkId} <Badge variant="success">hosting</Badge>
          </h1>
          <p className="text-sm text-muted-foreground">*.{state!.dnsSuffix}</p>
        </div>
        <Button variant="outline" size="sm" onClick={stop} disabled={busy}>
          <Square className="h-4 w-4" /> Stop
        </Button>
      </div>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <Server className="h-4 w-4" /> Share with clients
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <div className="flex items-center justify-between gap-2">
            <span className="shrink-0 text-muted-foreground">Join address</span>
            <span className="flex min-w-0 items-center gap-2">
              <span className="truncate font-mono" title={state!.joinUrl}>
                {state!.joinUrl}
              </span>
              <Button variant="ghost" size="icon" onClick={() => navigator.clipboard?.writeText(state!.joinUrl)} title="Copy">
                <Copy className="h-4 w-4" />
              </Button>
            </span>
          </div>
          <div className="flex items-center justify-between gap-2">
            <span className="text-muted-foreground">Token</span>
            <span className="flex items-center gap-2">
              <span className="font-mono">{state!.token}</span>
              <Button variant="ghost" size="icon" onClick={() => navigator.clipboard?.writeText(state!.token)} title="Copy">
                <Copy className="h-4 w-4" />
              </Button>
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">Control</span>
            <span className="font-mono">{state!.controlAddr}</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">Relay</span>
            <span className="font-mono">{state!.relayAddr}</span>
          </div>
          <div className="flex items-center justify-between gap-2">
            <span className="shrink-0 text-muted-foreground">Trust</span>
            <span className={`flex min-w-0 items-center gap-1.5 ${state!.tlsMode === 'off' ? 'text-destructive' : ''}`}>
              {state!.tlsMode === 'off' ? <ShieldAlert className="h-4 w-4 shrink-0" /> : <ShieldCheck className="h-4 w-4 shrink-0" />}
              <span className="truncate font-mono" title={state!.pin || state!.tlsMode}>
                {state!.tlsMode === 'off' ? 'plaintext — no TLS' : state!.tlsMode === 'acme' ? 'ACME certificate' : state!.pin}
              </span>
              {state!.pin && (
                <Button variant="ghost" size="icon" onClick={() => navigator.clipboard?.writeText(state!.pin)} title="Copy pin">
                  <Copy className="h-4 w-4" />
                </Button>
              )}
            </span>
          </div>
          <p className="pt-1 text-xs text-muted-foreground">
            The join address already contains the key pin — clients can paste it into the Server field as-is.
          </p>
          {/loopback|127\.0\.0\.1|localhost/.test(state!.joinUrl) && (
            <p className="text-xs text-muted-foreground">
              It points at this machine. Restart with a public address filled in so remote clients get a reachable host.
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <Users className="h-4 w-4" /> Members <Badge variant="muted">{state!.peers?.length ?? 0}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-1">
          {(state!.peers?.length ?? 0) === 0 && <p className="text-sm text-muted-foreground">No members yet.</p>}
          {state!.peers?.map((p) => (
            <div key={p.public_key} className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
              <span className="font-medium">{p.hostname || '(unnamed)'}</span>
              <span className="font-mono text-muted-foreground">{p.assigned_ip}</span>
            </div>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <Share2 className="h-4 w-4" /> Services <Badge variant="muted">{state!.services?.length ?? 0}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-1">
          {(state!.services?.length ?? 0) === 0 && <p className="text-sm text-muted-foreground">No services published.</p>}
          {state!.services?.map((s) => (
            <div key={s.name} className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
              <span className="font-mono">{s.name}.{state!.dnsSuffix}</span>
              <span className="font-mono text-muted-foreground">{s.node_ip}:{s.port}/{s.proto}</span>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  )
}
