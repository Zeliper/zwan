import { useCallback, useEffect, useRef, useState } from 'react'
import {
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  Loader2,
  Network,
  Plus,
  Power,
  Radio,
  Server,
  Share2,
  ShieldAlert,
  ShieldCheck,
  Trash2,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Connect, Disconnect, Forget, Networks, ServiceUp } from '../wailsjs/go/main/App'

interface Peer {
  hostname: string
  public_key: string
  assigned_ip: string
  endpoint: string
  group?: string
}
interface Service {
  name: string
  proto: string
  port: number
  node_ip: string
  vip?: string
  allow_groups?: string[]
}
interface EngineStatus {
  connected: boolean
  server: string
  pinned: boolean
  networkId: string
  dnsSuffix: string
  overlayCidr: string
  assignedIp: string
  overlayIp: string
  localCidr: string
  relayAddr: string
  publicKey: string
  via: string
  peers: Peer[]
  services: Service[]
  handshakes: Record<string, boolean>
  lastError: string
}
interface NetworkProfile {
  alias: string
  server: string
  pin?: string
  token: string
  name?: string
  useRelay: boolean
  autoConnect: boolean
}
interface NetworkStatus {
  network: NetworkProfile
  engine: EngineStatus
  warning?: string
}

const blank: NetworkProfile = { alias: '', server: 'https://127.0.0.1:8787', pin: '', token: '', name: '', useRelay: true, autoConnect: true }

// trustOf describes how a control server's identity was verified, mirroring the
// server's own three modes: ACME certificate, pinned key, or no TLS at all.
function trustOf(s: EngineStatus): { ok: boolean; label: string } {
  if (s.server?.startsWith('http://')) return { ok: false, label: 'plaintext — not authenticated' }
  if (s.pinned) return { ok: true, label: 'TLS · pinned key' }
  return { ok: true, label: 'TLS · CA certificate' }
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-4 py-1.5 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className={mono ? 'font-mono' : ''}>{value || '—'}</span>
    </div>
  )
}

export default function ClientView() {
  const [nets, setNets] = useState<NetworkStatus[]>([])
  const [serviceUp, setServiceUp] = useState(true)
  const [adding, setAdding] = useState(false)
  const [draft, setDraft] = useState<NetworkProfile>(blank)
  const [open, setOpen] = useState<string | null>(null)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const poll = useRef<number | null>(null)

  const refresh = useCallback(async () => {
    try {
      const up = await ServiceUp()
      setServiceUp(up)
      if (!up) {
        setNets([])
        return
      }
      setNets(((await Networks()) ?? []) as unknown as NetworkStatus[])
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

  // The host hands out one string, "https://host:port#sha256:...". Accept it
  // whole and split the pin out so both fields end up filled in.
  function onServerInput(value: string) {
    const hash = value.lastIndexOf('#')
    if (hash < 0) {
      setDraft({ ...draft, server: value })
      return
    }
    setDraft({ ...draft, server: value.slice(0, hash).trim(), pin: decodeURIComponent(value.slice(hash + 1)).trim() })
  }

  async function run(key: string, fn: () => Promise<unknown>) {
    setBusy(key)
    setError('')
    try {
      const list = (await fn()) as unknown as NetworkStatus[]
      if (list) setNets(list)
      return true
    } catch (e: any) {
      setError(String(e?.message ?? e))
      await refresh()
      return false
    } finally {
      setBusy('')
    }
  }

  async function addNetwork() {
    if (await run('add', () => Connect(draft as any))) {
      setAdding(false)
      setDraft(blank)
    }
  }

  const addForm = (
    <Card className="mx-auto max-w-md">
      <CardHeader>
        <CardTitle>Join a network</CardTitle>
        <CardDescription>Paste the join address the host gave you, with your token.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="alias">Local name</Label>
          <Input
            id="alias"
            value={draft.alias}
            onChange={(e) => setDraft({ ...draft, alias: e.target.value })}
            placeholder="alice"
            className="font-mono"
          />
          <p className="text-xs text-muted-foreground">
            Your own short name for this network. Services live under it — <span className="font-mono">nas.alice</span> — so it keeps two
            networks apart even when both servers use the same suffix.
          </p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="server">Server</Label>
          <Input id="server" value={draft.server} onChange={(e) => onServerInput(e.target.value)} placeholder="https://host:8787" />
          {draft.server.startsWith('http://') && (
            <p className="text-xs text-destructive">Plain http: the token and control traffic are sent unencrypted.</p>
          )}
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="pin">Server key pin</Label>
          <Input
            id="pin"
            value={draft.pin ?? ''}
            onChange={(e) => setDraft({ ...draft, pin: e.target.value })}
            placeholder="sha256:… (leave empty for a public domain)"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="token">Token</Label>
          <Input
            id="token"
            type="password"
            value={draft.token}
            onChange={(e) => setDraft({ ...draft, token: e.target.value })}
            placeholder="join token"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="name">This device's name (optional)</Label>
          <Input id="name" value={draft.name ?? ''} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={draft.useRelay}
            onChange={(e) => setDraft({ ...draft, useRelay: e.target.checked })}
            className="h-4 w-4 accent-primary"
          />
          Route via server relay (works behind NAT)
        </label>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="flex gap-2">
          <Button className="flex-1" onClick={addNetwork} disabled={busy === 'add' || !draft.token || !draft.alias || !serviceUp}>
            {busy === 'add' ? <Loader2 className="h-4 w-4 animate-spin" /> : <Radio className="h-4 w-4" />}
            {busy === 'add' ? 'Joining…' : 'Join'}
          </Button>
          {nets.length > 0 && (
            <Button variant="outline" onClick={() => setAdding(false)}>
              Cancel
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )

  return (
    <div>
      {!serviceUp && (
        <div className="mb-4 flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <div>
            Engine service is not running. Install it (the installer does this), or run
            <span className="mx-1 font-mono">zwan-service install</span> as Administrator.
          </div>
        </div>
      )}

      {adding || nets.length === 0 ? (
        addForm
      ) : (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-lg font-semibold">Networks</h1>
              <p className="text-sm text-muted-foreground">
                {nets.filter((n) => n.engine.connected).length} of {nets.length} connected
              </p>
            </div>
            <Button variant="outline" size="sm" onClick={() => setAdding(true)}>
              <Plus className="h-4 w-4" /> Join another
            </Button>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}

          {nets.map((n) => {
            const st = n.engine
            const expanded = open === n.network.alias
            const trust = trustOf(st)
            return (
              <Card key={n.network.alias}>
                <CardHeader className="pb-2">
                  <div className="flex items-center justify-between gap-2">
                    <button
                      className="flex min-w-0 items-center gap-2 text-left"
                      onClick={() => setOpen(expanded ? null : n.network.alias)}
                    >
                      {expanded ? <ChevronDown className="h-4 w-4 shrink-0" /> : <ChevronRight className="h-4 w-4 shrink-0" />}
                      <span
                        className={`h-2 w-2 shrink-0 rounded-full ${st.connected ? 'bg-emerald-500' : 'bg-muted-foreground/40'}`}
                        title={st.connected ? 'connected' : 'disconnected'}
                      />
                      <span className="truncate font-semibold">{n.network.alias}</span>
                      {st.connected && <Badge variant="muted">{st.assignedIp}</Badge>}
                      {st.connected && <span className="text-xs text-muted-foreground">via {st.via}</span>}
                    </button>
                    <div className="flex shrink-0 gap-1">
                      {st.connected ? (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={busy === n.network.alias}
                          onClick={() => run(n.network.alias, () => Disconnect(n.network.alias))}
                        >
                          <Power className="h-4 w-4" /> Leave
                        </Button>
                      ) : (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={busy === n.network.alias || !serviceUp}
                          onClick={() => run(n.network.alias, () => Connect(n.network as any))}
                        >
                          {busy === n.network.alias ? <Loader2 className="h-4 w-4 animate-spin" /> : <Radio className="h-4 w-4" />}
                          Join
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="icon"
                        title="Forget this network"
                        disabled={busy === n.network.alias}
                        onClick={() => run(n.network.alias, () => Forget(n.network.alias))}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                  {n.warning && (
                    <p className="flex items-start gap-1.5 pt-1 text-xs text-amber-600 dark:text-amber-400">
                      <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                      {n.warning}
                    </p>
                  )}
                  {st.lastError && !st.connected && <p className="pt-1 text-xs text-destructive">{st.lastError}</p>}
                </CardHeader>

                {expanded && (
                  <CardContent className="space-y-4">
                    <div>
                      <p className={`mb-1 flex items-center gap-1 text-xs ${trust.ok ? 'text-muted-foreground' : 'text-destructive'}`}>
                        {trust.ok ? <ShieldCheck className="h-3.5 w-3.5" /> : <ShieldAlert className="h-3.5 w-3.5" />}
                        {trust.label}
                      </p>
                      <Row label="Control server" value={n.network.server} mono />
                      <Row label="Names" value={st.dnsSuffix ? `*.${st.dnsSuffix}` : `*.${n.network.alias}`} mono />
                      <Row label="Address on this device" value={st.assignedIp} mono />
                      {st.localCidr ? (
                        <>
                          <Row label="Address in the network" value={st.overlayIp} mono />
                          <Row label="Local range" value={st.localCidr} mono />
                        </>
                      ) : (
                        <Row label="Overlay CIDR" value={st.overlayCidr} mono />
                      )}
                      <Row label="Relay" value={st.relayAddr} mono />
                    </div>

                    <div>
                      <p className="mb-1 flex items-center gap-2 text-sm font-medium">
                        <Network className="h-4 w-4" /> Peers <Badge variant="muted">{st.peers?.length ?? 0}</Badge>
                      </p>
                      {(st.peers?.length ?? 0) === 0 && <p className="text-sm text-muted-foreground">No peers yet.</p>}
                      {st.peers?.map((p) => (
                        <div key={p.public_key} className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
                          <span className="flex items-center gap-2 font-medium">
                            <span
                              className={`h-1.5 w-1.5 rounded-full ${st.handshakes?.[p.assigned_ip] ? 'bg-emerald-500' : 'bg-muted-foreground/40'}`}
                              title={st.handshakes?.[p.assigned_ip] ? 'tunnel established' : 'connecting'}
                            />
                            {p.hostname || '(unnamed)'}
                            {p.group && <Badge variant="muted">{p.group}</Badge>}
                          </span>
                          <span className="font-mono text-muted-foreground">{p.assigned_ip}</span>
                        </div>
                      ))}
                    </div>

                    <div>
                      <p className="mb-1 flex items-center gap-2 text-sm font-medium">
                        <Share2 className="h-4 w-4" /> Services <Badge variant="muted">{st.services?.length ?? 0}</Badge>
                      </p>
                      {(st.services?.length ?? 0) === 0 && <p className="text-sm text-muted-foreground">No services published.</p>}
                      {st.services?.map((s) => (
                        <div key={s.name} className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
                          <span className="font-mono">
                            {s.name}.{st.dnsSuffix || n.network.alias}
                          </span>
                          <span className="flex items-center gap-2">
                            {(s.allow_groups?.length ?? 0) > 0 && <Badge variant="muted">{s.allow_groups!.join(', ')}</Badge>}
                            <span className="font-mono text-muted-foreground">
                              {s.vip || s.node_ip}:{s.port}/{s.proto}
                            </span>
                          </span>
                        </div>
                      ))}
                    </div>
                  </CardContent>
                )}
              </Card>
            )
          })}

          <Card>
            <CardContent className="flex items-center gap-2 py-3 text-sm text-muted-foreground">
              <Server className="h-4 w-4 shrink-0" />
              Each network gets its own adapter, key, address range and names, and nothing routes between them — so two
              networks may use the same overlay range without colliding here.
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  )
}
