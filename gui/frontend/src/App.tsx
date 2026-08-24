import { useEffect, useRef, useState } from 'react'
import { Loader2, Network, RefreshCw, Server, Share2, LogOut, Radio } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { ThemeToggle } from '@/components/theme-toggle'
import { Join, Refresh } from '../wailsjs/go/main/App'

interface Peer {
  hostname: string
  publicKey: string
  assignedIp: string
  endpoint: string
}
interface Service {
  name: string
  proto: string
  port: number
  backendPort: number
  nodeIp: string
  fqdn: string
}
interface JoinResult {
  networkId: string
  dnsSuffix: string
  overlayCidr: string
  assignedIp: string
  relayAddr: string
  publicKey: string
  deviceUuid: string
  peers: Peer[]
  services: Service[]
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
  const [state, setState] = useState<JoinResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const timer = useRef<number | null>(null)

  async function connect() {
    setBusy(true)
    setError('')
    try {
      const r = (await Join(server, token, name)) as unknown as JoinResult
      setState(r)
    } catch (e: any) {
      setError(String(e?.message ?? e))
    } finally {
      setBusy(false)
    }
  }

  async function refresh() {
    try {
      const r = (await Refresh()) as unknown as JoinResult
      setState(r)
    } catch {
      /* transient */
    }
  }

  function leave() {
    if (timer.current) window.clearInterval(timer.current)
    setState(null)
  }

  useEffect(() => {
    if (!state) return
    timer.current = window.setInterval(refresh, 4000)
    return () => {
      if (timer.current) window.clearInterval(timer.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state !== null])

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
          {state && (
            <Badge variant="success" className="gap-1">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" /> joined
            </Badge>
          )}
          <ThemeToggle />
        </div>
      </header>

      <main className="mx-auto w-full max-w-3xl p-6">
        {!state ? (
          <Card className="mx-auto max-w-md">
            <CardHeader>
              <CardTitle>Join a network</CardTitle>
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
              {error && <p className="text-sm text-destructive">{error}</p>}
              <Button className="w-full" onClick={connect} disabled={busy || !token}>
                {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Radio className="h-4 w-4" />}
                {busy ? 'Joining…' : 'Join'}
              </Button>
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <h1 className="text-lg font-semibold">{state.networkId}</h1>
                <p className="text-sm text-muted-foreground">*.{state.dnsSuffix}</p>
              </div>
              <div className="flex gap-2">
                <Button variant="outline" size="sm" onClick={refresh}>
                  <RefreshCw className="h-4 w-4" /> Refresh
                </Button>
                <Button variant="outline" size="sm" onClick={leave}>
                  <LogOut className="h-4 w-4" /> Leave
                </Button>
              </div>
            </div>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Server className="h-4 w-4" /> This device
                </CardTitle>
              </CardHeader>
              <CardContent>
                <Row label="Overlay IP" value={state.assignedIp} mono />
                <Row label="Overlay CIDR" value={state.overlayCidr} mono />
                <Row label="Relay" value={state.relayAddr} mono />
                <Row label="Public key" value={state.publicKey} mono />
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Network className="h-4 w-4" /> Peers <Badge variant="muted">{state.peers.length}</Badge>
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-1">
                {state.peers.length === 0 && <p className="text-sm text-muted-foreground">No peers yet.</p>}
                {state.peers.map((p) => (
                  <div key={p.publicKey} className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
                    <span className="font-medium">{p.hostname || '(unnamed)'}</span>
                    <span className="font-mono text-muted-foreground">{p.assignedIp}</span>
                  </div>
                ))}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Share2 className="h-4 w-4" /> Services <Badge variant="muted">{state.services.length}</Badge>
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-1">
                {state.services.length === 0 && <p className="text-sm text-muted-foreground">No services published.</p>}
                {state.services.map((s) => (
                  <div key={s.name} className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
                    <span className="font-mono">{s.fqdn}</span>
                    <span className="font-mono text-muted-foreground">
                      {s.nodeIp}:{s.port}/{s.proto}
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
