import { useCallback, useEffect, useRef, useState } from 'react'
import { AlertTriangle, Loader2, Network, Power, Radio, Server, Share2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { ThemeToggle } from '@/components/theme-toggle'
import { Connect, Disconnect, ServiceUp, Status } from '../wailsjs/go/main/App'

interface Peer {
  hostname: string
  public_key: string
  assigned_ip: string
  endpoint: string
}
interface Service {
  name: string
  proto: string
  port: number
  backend_port: number
  node_ip: string
}
interface EngineStatus {
  connected: boolean
  networkId: string
  dnsSuffix: string
  overlayCidr: string
  assignedIp: string
  relayAddr: string
  publicKey: string
  via: string
  peers: Peer[]
  services: Service[]
  handshakes: Record<string, boolean>
  lastError: string
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-4 py-1.5 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className={mono ? 'font-mono' : ''}>{value || '—'}</span>
    </div>
  )
}

export default function App() {
  const [server, setServer] = useState('http://127.0.0.1:8787')
  const [token, setToken] = useState('')
  const [name, setName] = useState('')
  const [useRelay, setUseRelay] = useState(true)
  const [status, setStatus] = useState<EngineStatus | null>(null)
  const [serviceUp, setServiceUp] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const poll = useRef<number | null>(null)

  const refresh = useCallback(async () => {
    try {
      const up = await ServiceUp()
      setServiceUp(up)
      if (!up) {
        setStatus(null)
        return
      }
      const s = (await Status()) as unknown as EngineStatus
      setStatus(s?.connected ? s : null)
    } catch {
      setServiceUp(false)
    }
  }, [])

  useEffect(() => {
    refresh()
    poll.current = window.setInterval(refresh, 3000)
    return () => {
      if (poll.current) window.clearInterval(poll.current)
    }
  }, [refresh])

  async function connect() {
    setBusy(true)
    setError('')
    try {
      const s = (await Connect(server, token, name, useRelay)) as unknown as EngineStatus
      setStatus(s?.connected ? s : null)
    } catch (e: any) {
      setError(String(e?.message ?? e))
    } finally {
      setBusy(false)
    }
  }

  async function disconnect() {
    setBusy(true)
    try {
      await Disconnect()
      setStatus(null)
    } catch {
      /* ignore */
    } finally {
      setBusy(false)
    }
  }

  const connected = !!status?.connected

  return (
    <div className="min-h-full bg-background">
      <header className="flex items-center justify-between border-b px-6 py-3">
        <div className="flex items-center gap-2">
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <Network className="h-4 w-4" />
          </div>
          <div>
            <div className="text-sm font-semibold leading-none">zwan</div>
            <div className="text-xs text-muted-foreground">private overlay network</div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {connected ? (
            <Badge variant="success" className="gap-1">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" /> connected
            </Badge>
          ) : (
            <Badge variant="muted">disconnected</Badge>
          )}
          <ThemeToggle />
        </div>
      </header>

      <main className="mx-auto w-full max-w-3xl p-6">
        {!serviceUp && (
          <div className="mb-4 flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            <div>
              Engine service is not running. Install it (the installer does this), or run
              <span className="mx-1 font-mono">zwan-service install</span> as Administrator.
            </div>
          </div>
        )}

        {!connected ? (
          <Card className="mx-auto max-w-md">
            <CardHeader>
              <CardTitle>Connect to a network</CardTitle>
              <CardDescription>Enter the server address and your join token.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="server">Server</Label>
                <Input id="server" value={server} onChange={(e) => setServer(e.target.value)} placeholder="http://host:8787" />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="token">Token</Label>
                <Input id="token" type="password" value={token} onChange={(e) => setToken(e.target.value)} placeholder="join token" />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="name">Name (optional)</Label>
                <Input id="name" value={name} onChange={(e) => setName(e.target.value)} placeholder="this device's label" />
              </div>
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={useRelay} onChange={(e) => setUseRelay(e.target.checked)} className="h-4 w-4 accent-primary" />
                Route via server relay (works behind NAT)
              </label>
              {error && <p className="text-sm text-destructive">{error}</p>}
              <Button className="w-full" onClick={connect} disabled={busy || !token || !serviceUp}>
                {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Radio className="h-4 w-4" />}
                {busy ? 'Connecting…' : 'Connect'}
              </Button>
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <h1 className="text-lg font-semibold">{status!.networkId}</h1>
                <p className="text-sm text-muted-foreground">
                  *.{status!.dnsSuffix} · via {status!.via}
                </p>
              </div>
              <Button variant="outline" size="sm" onClick={disconnect} disabled={busy}>
                <Power className="h-4 w-4" /> Disconnect
              </Button>
            </div>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Server className="h-4 w-4" /> This device
                </CardTitle>
              </CardHeader>
              <CardContent>
                <Row label="Overlay IP" value={status!.assignedIp} mono />
                <Row label="Overlay CIDR" value={status!.overlayCidr} mono />
                <Row label="Relay" value={status!.relayAddr} mono />
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Network className="h-4 w-4" /> Peers <Badge variant="muted">{status!.peers?.length ?? 0}</Badge>
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-1">
                {(status!.peers?.length ?? 0) === 0 && <p className="text-sm text-muted-foreground">No peers yet.</p>}
                {status!.peers?.map((p) => (
                  <div key={p.public_key} className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
                    <span className="flex items-center gap-2 font-medium">
                      <span
                        className={`h-1.5 w-1.5 rounded-full ${status!.handshakes?.[p.assigned_ip] ? 'bg-emerald-500' : 'bg-muted-foreground/40'}`}
                        title={status!.handshakes?.[p.assigned_ip] ? 'tunnel established' : 'connecting'}
                      />
                      {p.hostname || '(unnamed)'}
                    </span>
                    <span className="font-mono text-muted-foreground">{p.assigned_ip}</span>
                  </div>
                ))}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Share2 className="h-4 w-4" /> Services <Badge variant="muted">{status!.services?.length ?? 0}</Badge>
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-1">
                {(status!.services?.length ?? 0) === 0 && <p className="text-sm text-muted-foreground">No services published.</p>}
                {status!.services?.map((s) => (
                  <div key={s.name} className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
                    <span className="font-mono">
                      {s.name}.{status!.dnsSuffix}
                    </span>
                    <span className="font-mono text-muted-foreground">
                      {s.node_ip}:{s.port}/{s.proto}
                    </span>
                  </div>
                ))}
              </CardContent>
            </Card>
          </div>
        )}
      </main>
    </div>
  )
}
