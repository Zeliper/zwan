import { useCallback, useEffect, useRef, useState } from 'react'
import { AlertTriangle, Copy, Play, RefreshCw, Server, ShieldAlert, ShieldCheck, Share2, Square, Users } from 'lucide-react'
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
interface ServerConfig {
  networkId: string
  dnsSuffix: string
  cidr: string
  token: string
  controlAddr: string
  relayAddr: string
  relayPublic: string
  tlsMode: string
  domains: string[]
  publicHost: string
  autoStart: boolean
}
interface HostState {
  running: boolean
  config: ServerConfig
  tlsMode: string
  pin: string
  joinUrl: string
  peers: Peer[]
  services: Service[]
  lastError: string
  managedByService: boolean
}

const defaults: ServerConfig = {
  networkId: 'home',
  dnsSuffix: 'home.zwan',
  cidr: '100.64.0.0/16',
  token: '',
  controlAddr: '0.0.0.0:8787',
  relayAddr: '0.0.0.0:3478',
  relayPublic: '',
  tlsMode: 'auto',
  domains: [],
  publicHost: '',
  autoStart: false,
}

const inputClass =
  'flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring'

export default function HostView() {
  const [cfg, setCfg] = useState<ServerConfig>(defaults)
  const [state, setState] = useState<HostState | null>(null)
  const [managed, setManaged] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const poll = useRef<number | null>(null)
  // The saved configuration seeds the form once; after that the operator owns it.
  const seeded = useRef(false)

  function set<K extends keyof ServerConfig>(key: K, value: ServerConfig[K]) {
    setCfg((c) => ({ ...c, [key]: value }))
  }

  const gen = useCallback(async () => {
    try {
      set('token', await HostGenToken())
    } catch {
      /* ignore */
    }
  }, [])

  const refresh = useCallback(async () => {
    try {
      const s = (await HostStatus()) as unknown as HostState
      setManaged(!!s?.managedByService)
      setState(s?.running ? s : null)
      if (!seeded.current && s?.config) {
        seeded.current = true
        // A saved config with no token is a fresh install: generate one.
        if (s.config.token) setCfg({ ...defaults, ...s.config })
        else {
          setCfg({ ...defaults, ...s.config, token: '' })
          gen()
        }
      }
    } catch {
      setState(null)
    }
  }, [gen])

  useEffect(() => {
    refresh()
    poll.current = window.setInterval(refresh, 3000)
    return () => {
      if (poll.current) window.clearInterval(poll.current)
    }
  }, [refresh])

  async function start() {
    setBusy(true)
    setError('')
    try {
      await HostStart(cfg as any)
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
    } catch (e: any) {
      setError(String(e?.message ?? e))
    } finally {
      setBusy(false)
    }
  }

  const running = !!state?.running

  const serviceNotice = !managed && (
    <div className="mb-4 flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
      <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-400" />
      <div>
        The control-server service is not installed, so the network runs inside this app and stops when you quit.
        Re-run the installer and tick <span className="font-medium">Server</span>, or run
        <span className="mx-1 font-mono">zwan-server service install</span> as Administrator.
      </div>
    </div>
  )

  if (!running) {
    return (
      <div>
        {serviceNotice}
        <Card className="mx-auto max-w-md">
          <CardHeader>
            <CardTitle>Host a network</CardTitle>
            <CardDescription>
              Run a control server + relay on this machine (needs a public IP for remote clients).
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="net">Network</Label>
                <Input id="net" value={cfg.networkId} onChange={(e) => set('networkId', e.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="suffix">DNS suffix</Label>
                <Input id="suffix" value={cfg.dnsSuffix} onChange={(e) => set('dnsSuffix', e.target.value)} />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="cidr">Overlay CIDR</Label>
              <Input id="cidr" value={cfg.cidr} onChange={(e) => set('cidr', e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="token">Join token</Label>
              <div className="flex gap-2">
                <Input
                  id="token"
                  value={cfg.token}
                  onChange={(e) => set('token', e.target.value)}
                  className="font-mono"
                />
                <Button variant="outline" size="icon" onClick={gen} title="Generate">
                  <RefreshCw className="h-4 w-4" />
                </Button>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="ctrl">Control addr</Label>
                <Input
                  id="ctrl"
                  value={cfg.controlAddr}
                  onChange={(e) => set('controlAddr', e.target.value)}
                  className="font-mono"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="relay">Relay addr</Label>
                <Input
                  id="relay"
                  value={cfg.relayAddr}
                  onChange={(e) => set('relayAddr', e.target.value)}
                  className="font-mono"
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="tls">TLS</Label>
              <select id="tls" value={cfg.tlsMode} onChange={(e) => set('tlsMode', e.target.value)} className={inputClass}>
                <option value="auto">Automatic — certificate for the domain, otherwise a pinned key</option>
                <option value="self">Self-signed key, clients pin it</option>
                <option value="acme">Public certificate (ACME) for the domain</option>
                <option value="off">Off — plaintext HTTP (local testing only)</option>
              </select>
              {cfg.tlsMode === 'off' && (
                <p className="text-xs text-destructive">Join tokens and all control traffic would be sent unencrypted.</p>
              )}
            </div>
            {cfg.tlsMode !== 'off' && (
              <div className="space-y-1.5">
                <Label htmlFor="domain">Domain (optional)</Label>
                <Input
                  id="domain"
                  value={cfg.domains.join(', ')}
                  onChange={(e) =>
                    set(
                      'domains',
                      e.target.value
                        .split(',')
                        .map((d) => d.trim())
                        .filter(Boolean),
                    )
                  }
                  placeholder="vpn.example.com — leave empty to use a pinned key"
                  className="font-mono"
                />
                <p className="text-xs text-muted-foreground">
                  With a domain pointed at this machine, a certificate is issued automatically (needs port 443, or port 80
                  reachable).
                </p>
              </div>
            )}
            <div className="space-y-1.5">
              <Label htmlFor="public">Public address (optional)</Label>
              <Input
                id="public"
                value={cfg.publicHost}
                onChange={(e) => set('publicHost', e.target.value)}
                placeholder="203.0.113.5:8787 — what clients should connect to"
                className="font-mono"
              />
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <Button className="w-full" onClick={start} disabled={busy || !cfg.token}>
              <Play className="h-4 w-4" /> Start hosting
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {serviceNotice}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-lg font-semibold">
            {state!.config.networkId} <Badge variant="success">hosting</Badge>
          </h1>
          <p className="text-sm text-muted-foreground">
            *.{state!.config.dnsSuffix}
            {managed && ' · runs in the background service'}
          </p>
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
              <span className="font-mono">{state!.config.token}</span>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => navigator.clipboard?.writeText(state!.config.token)}
                title="Copy"
              >
                <Copy className="h-4 w-4" />
              </Button>
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">Control</span>
            <span className="font-mono">{state!.config.controlAddr}</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">Relay</span>
            <span className="font-mono">{state!.config.relayAddr}</span>
          </div>
          <div className="flex items-center justify-between gap-2">
            <span className="shrink-0 text-muted-foreground">Trust</span>
            <span className={`flex min-w-0 items-center gap-1.5 ${state!.tlsMode === 'off' ? 'text-destructive' : ''}`}>
              {state!.tlsMode === 'off' ? (
                <ShieldAlert className="h-4 w-4 shrink-0" />
              ) : (
                <ShieldCheck className="h-4 w-4 shrink-0" />
              )}
              <span className="truncate font-mono" title={state!.pin || state!.tlsMode}>
                {state!.tlsMode === 'off'
                  ? 'plaintext — no TLS'
                  : state!.tlsMode === 'acme'
                    ? 'ACME certificate'
                    : state!.pin}
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
              It points at this machine. Set a public address and start again so remote clients get a reachable host.
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
              <span className="font-mono">
                {s.name}.{state!.config.dnsSuffix}
              </span>
              <span className="font-mono text-muted-foreground">
                {s.node_ip}:{s.port}/{s.proto}
              </span>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  )
}
