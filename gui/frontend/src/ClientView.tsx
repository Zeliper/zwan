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
import PublishEditor from './PublishEditor'
import type { Key } from '@/lib/i18n'
import { Mono, useT } from '@/lib/use-i18n'

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
  publish?: PublishedService[]
}

export interface PublishedService {
  name: string
  proto: string
  port: number
  backend_port?: number
  allow_groups?: string[]
}
interface NetworkStatus {
  network: NetworkProfile
  engine: EngineStatus
  warning?: string
}

const blank: NetworkProfile = { alias: '', server: 'https://127.0.0.1:8787', pin: '', token: '', name: '', useRelay: true, autoConnect: true }

// trustOf describes how a control server's identity was verified, mirroring the
// server's own three modes: ACME certificate, pinned key, or no TLS at all. It
// names the wording rather than choosing it, so the sentence is picked in the
// language the component is being rendered in.
function trustOf(s: EngineStatus): { ok: boolean; label: Key } {
  if (s.server?.startsWith('http://')) return { ok: false, label: 'client.trust.plaintext' }
  if (s.pinned) return { ok: true, label: 'client.trust.pinned' }
  return { ok: true, label: 'client.trust.ca' }
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
  const { t, tx } = useT()
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
        <CardTitle>{t('client.add.title')}</CardTitle>
        <CardDescription>{t('client.add.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="alias">{t('client.alias')}</Label>
          <Input
            id="alias"
            value={draft.alias}
            onChange={(e) => setDraft({ ...draft, alias: e.target.value })}
            placeholder="alice"
            className="font-mono"
          />
          <p className="text-xs text-muted-foreground">
            {tx('client.alias.help', { example: <Mono>nas.alice</Mono> })}
          </p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="server">{t('client.server')}</Label>
          <Input id="server" value={draft.server} onChange={(e) => onServerInput(e.target.value)} placeholder="https://host:8787" />
          {draft.server.startsWith('http://') && (
            <p className="text-xs text-destructive">{t('client.server.insecure')}</p>
          )}
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="pin">{t('client.pin')}</Label>
          <Input
            id="pin"
            value={draft.pin ?? ''}
            onChange={(e) => setDraft({ ...draft, pin: e.target.value })}
            placeholder={t('client.pin.placeholder')}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="token">{t('client.token')}</Label>
          <Input
            id="token"
            type="password"
            value={draft.token}
            onChange={(e) => setDraft({ ...draft, token: e.target.value })}
            placeholder={t('client.token.placeholder')}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="name">{t('client.deviceName')}</Label>
          <Input id="name" value={draft.name ?? ''} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={draft.useRelay}
            onChange={(e) => setDraft({ ...draft, useRelay: e.target.checked })}
            className="h-4 w-4 accent-primary"
          />
          {t('client.useRelay')}
        </label>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="flex gap-2">
          <Button className="flex-1" onClick={addNetwork} disabled={busy === 'add' || !draft.token || !draft.alias || !serviceUp}>
            {busy === 'add' ? <Loader2 className="h-4 w-4 animate-spin" /> : <Radio className="h-4 w-4" />}
            {busy === 'add' ? t('client.joining') : t('client.join')}
          </Button>
          {nets.length > 0 && (
            <Button variant="outline" onClick={() => setAdding(false)}>
              {t('common.cancel')}
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
          <div>{tx('client.serviceDown', { command: <Mono>zwan-service install</Mono> })}</div>
        </div>
      )}

      {adding || nets.length === 0 ? (
        addForm
      ) : (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-lg font-semibold">{t('client.networks')}</h1>
              <p className="text-sm text-muted-foreground">
                {t('client.connectedCount', {
                  connected: nets.filter((n) => n.engine.connected).length,
                  total: nets.length,
                })}
              </p>
            </div>
            <Button variant="outline" size="sm" onClick={() => setAdding(true)}>
              <Plus className="h-4 w-4" /> {t('client.joinAnother')}
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
                        title={st.connected ? t('client.state.connected') : t('client.state.disconnected')}
                      />
                      <span className="truncate font-semibold">{n.network.alias}</span>
                      {st.connected && <Badge variant="muted">{st.assignedIp}</Badge>}
                      {st.connected && (
                        <span className="text-xs text-muted-foreground">{t('client.via', { via: st.via })}</span>
                      )}
                    </button>
                    <div className="flex shrink-0 gap-1">
                      {st.connected ? (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={busy === n.network.alias}
                          onClick={() => run(n.network.alias, () => Disconnect(n.network.alias))}
                        >
                          <Power className="h-4 w-4" /> {t('client.leave')}
                        </Button>
                      ) : (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={busy === n.network.alias || !serviceUp}
                          onClick={() => run(n.network.alias, () => Connect(n.network as any))}
                        >
                          {busy === n.network.alias ? <Loader2 className="h-4 w-4 animate-spin" /> : <Radio className="h-4 w-4" />}
                          {t('client.join')}
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="icon"
                        title={t('client.forget')}
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
                        {t(trust.label)}
                      </p>
                      <Row label={t('client.row.controlServer')} value={n.network.server} mono />
                      <Row
                        label={t('client.row.names')}
                        value={st.dnsSuffix ? `*.${st.dnsSuffix}` : `*.${n.network.alias}`}
                        mono
                      />
                      <Row label={t('client.row.addressHere')} value={st.assignedIp} mono />
                      {st.localCidr ? (
                        <>
                          <Row label={t('client.row.addressInNetwork')} value={st.overlayIp} mono />
                          <Row label={t('client.row.localRange')} value={st.localCidr} mono />
                        </>
                      ) : (
                        <Row label={t('client.row.overlayCidr')} value={st.overlayCidr} mono />
                      )}
                      <Row label={t('common.relay')} value={st.relayAddr} mono />
                    </div>

                    <div>
                      <p className="mb-1 flex items-center gap-2 text-sm font-medium">
                        <Network className="h-4 w-4" /> {t('client.peers')}{' '}
                        <Badge variant="muted">{st.peers?.length ?? 0}</Badge>
                      </p>
                      {(st.peers?.length ?? 0) === 0 && (
                        <p className="text-sm text-muted-foreground">{t('client.peers.none')}</p>
                      )}
                      {st.peers?.map((p) => (
                        <div key={p.public_key} className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
                          <span className="flex items-center gap-2 font-medium">
                            <span
                              className={`h-1.5 w-1.5 rounded-full ${st.handshakes?.[p.assigned_ip] ? 'bg-emerald-500' : 'bg-muted-foreground/40'}`}
                              title={
                                st.handshakes?.[p.assigned_ip]
                                  ? t('client.peer.established')
                                  : t('client.peer.connecting')
                              }
                            />
                            {p.hostname || t('common.unnamed')}
                            {p.group && <Badge variant="muted">{p.group}</Badge>}
                          </span>
                          <span className="font-mono text-muted-foreground">{p.assigned_ip}</span>
                        </div>
                      ))}
                    </div>

                    <PublishEditor
                      network={n.network}
                      busy={busy === n.network.alias}
                      onSave={(publish) =>
                        run(n.network.alias, () => Connect({ ...n.network, publish } as any))
                      }
                    />

                    <div>
                      <p className="mb-1 flex items-center gap-2 text-sm font-medium">
                        <Share2 className="h-4 w-4" /> {t('client.services')}{' '}
                        <Badge variant="muted">{st.services?.length ?? 0}</Badge>
                      </p>
                      {(st.services?.length ?? 0) === 0 && (
                        <p className="text-sm text-muted-foreground">{t('client.services.none')}</p>
                      )}
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
              {t('client.isolationNote')}
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  )
}
