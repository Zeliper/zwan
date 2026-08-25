import { useCallback, useEffect, useRef, useState } from 'react'
import { AlertTriangle, Copy, KeyRound, Play, Plus, RefreshCw, Server, ShieldAlert, ShieldCheck, Share2, Square, Users, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { HostGenToken, HostStart, HostStatus, HostStop, ParseACL } from '../wailsjs/go/main/App'
import { Mono, useT } from '@/lib/use-i18n'

interface Peer {
  hostname: string
  public_key: string
  assigned_ip: string
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
interface Rule {
  src: string[]
  dst: string[]
}
interface GroupRow {
  name: string
  token: string
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
  groupTokens?: Record<string, string>
  acl?: Rule[]
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

// merge lays a saved configuration over the defaults without letting a missing
// or null field win.
//
// Spreading the server's object directly would do that: a list that arrives as
// null replaces the empty list the form expects, and the failure then surfaces
// as a call on nothing while rendering — which takes the whole view down rather
// than the one field. The fields that must be a list are named here so a new
// null cannot reach the form unnoticed.
function merge(saved?: Partial<ServerConfig>): ServerConfig {
  const c = { ...defaults, ...(saved ?? {}) }
  return {
    ...c,
    domains: Array.isArray(c.domains) ? c.domains : [],
    acl: Array.isArray(c.acl) ? c.acl : [],
    groupTokens: c.groupTokens ?? {},
  }
}

// ruleLines renders stored rules back into the one-per-line text the operator
// typed, matching the shorthand the Go parser accepts.
function ruleLines(rules?: Rule[]): string {
  return (rules ?? []).map((r) => `${r.src.join(',')}->${r.dst.join(',')}`).join('\n')
}

export default function HostView() {
  const { t, tx } = useT()
  const [cfg, setCfg] = useState<ServerConfig>(defaults)
  const [groups, setGroups] = useState<GroupRow[]>([])
  const [rulesText, setRulesText] = useState('')
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
        if (s.config.token) setCfg(merge(s.config))
        else {
          setCfg({ ...merge(s.config), token: '' })
          gen()
        }
        setGroups(Object.entries(s.config.groupTokens ?? {}).map(([name, token]) => ({ name, token })))
        setRulesText(ruleLines(s.config.acl))
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
      const acl = await ParseACL(rulesText)
      const groupTokens: Record<string, string> = {}
      for (const g of groups) {
        const name = g.name.trim()
        const token = g.token.trim()
        if (name && token) groupTokens[name] = token
      }
      await HostStart({ ...cfg, groupTokens, acl } as any)
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
        {tx('host.serviceNotice', {
          server: <span className="font-medium">{t('host.serviceNotice.server')}</span>,
          command: <Mono>zwan-server service install</Mono>,
        })}
      </div>
    </div>
  )

  if (!running) {
    return (
      <div>
        {serviceNotice}
        <Card className="mx-auto max-w-md">
          <CardHeader>
            <CardTitle>{t('host.title')}</CardTitle>
            <CardDescription>{t('host.description')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="net">{t('host.network')}</Label>
                <Input id="net" value={cfg.networkId} onChange={(e) => set('networkId', e.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="suffix">{t('host.dnsSuffix')}</Label>
                <Input id="suffix" value={cfg.dnsSuffix} onChange={(e) => set('dnsSuffix', e.target.value)} />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="cidr">{t('host.cidr')}</Label>
              <Input id="cidr" value={cfg.cidr} onChange={(e) => set('cidr', e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="token">{t('host.joinToken')}</Label>
              <div className="flex gap-2">
                <Input
                  id="token"
                  value={cfg.token}
                  onChange={(e) => set('token', e.target.value)}
                  className="font-mono"
                />
                <Button variant="outline" size="icon" onClick={gen} title={t('common.generate')}>
                  <RefreshCw className="h-4 w-4" />
                </Button>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="ctrl">{t('host.controlAddr')}</Label>
                <Input
                  id="ctrl"
                  value={cfg.controlAddr}
                  onChange={(e) => set('controlAddr', e.target.value)}
                  className="font-mono"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="relay">{t('host.relayAddr')}</Label>
                <Input
                  id="relay"
                  value={cfg.relayAddr}
                  onChange={(e) => set('relayAddr', e.target.value)}
                  className="font-mono"
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="tls">{t('host.tls')}</Label>
              <select id="tls" value={cfg.tlsMode} onChange={(e) => set('tlsMode', e.target.value)} className={inputClass}>
                <option value="auto">{t('host.tls.auto')}</option>
                <option value="self">{t('host.tls.self')}</option>
                <option value="acme">{t('host.tls.acme')}</option>
                <option value="off">{t('host.tls.off')}</option>
              </select>
              {cfg.tlsMode === 'off' && <p className="text-xs text-destructive">{t('host.tls.offWarning')}</p>}
            </div>
            {cfg.tlsMode !== 'off' && (
              <div className="space-y-1.5">
                <Label htmlFor="domain">{t('host.domain')}</Label>
                <Input
                  id="domain"
                  value={(cfg.domains ?? []).join(', ')}
                  onChange={(e) =>
                    set(
                      'domains',
                      e.target.value
                        .split(',')
                        .map((d) => d.trim())
                        .filter(Boolean),
                    )
                  }
                  placeholder={t('host.domain.placeholder')}
                  className="font-mono"
                />
                <p className="text-xs text-muted-foreground">{t('host.domain.help')}</p>
              </div>
            )}
            <div className="space-y-1.5">
              <Label htmlFor="public">{t('host.publicHost')}</Label>
              <Input
                id="public"
                value={cfg.publicHost}
                onChange={(e) => set('publicHost', e.target.value)}
                placeholder={t('host.publicHost.placeholder')}
                className="font-mono"
              />
            </div>
            <div className="space-y-2 rounded-md border border-border p-3">
              <div className="flex items-center gap-2">
                <KeyRound className="h-4 w-4" />
                <Label className="text-sm font-medium">{t('host.acl')}</Label>
              </div>
              <p className="text-xs text-muted-foreground">
                {tx('host.acl.help', { default: <Mono>default</Mono> })}
              </p>
              {groups.map((g, i) => (
                <div key={i} className="flex gap-2">
                  <Input
                    value={g.name}
                    onChange={(e) => setGroups(groups.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))}
                    placeholder={t('host.acl.groupPlaceholder')}
                    className="w-28"
                  />
                  <Input
                    value={g.token}
                    onChange={(e) => setGroups(groups.map((x, j) => (j === i ? { ...x, token: e.target.value } : x)))}
                    placeholder={t('client.token.placeholder')}
                    className="font-mono"
                  />
                  <Button
                    variant="outline"
                    size="icon"
                    title={t('common.generate')}
                    onClick={async () => {
                      const t = await HostGenToken()
                      setGroups(groups.map((x, j) => (j === i ? { ...x, token: t } : x)))
                    }}
                  >
                    <RefreshCw className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    title={t('common.remove')}
                    onClick={() => setGroups(groups.filter((_, j) => j !== i))}
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              ))}
              <Button variant="outline" size="sm" onClick={() => setGroups([...groups, { name: '', token: '' }])}>
                <Plus className="h-4 w-4" /> {t('host.acl.addGroup')}
              </Button>
              <div className="space-y-1.5 pt-1">
                <Label htmlFor="rules">{t('host.acl.rules')}</Label>
                <textarea
                  id="rules"
                  rows={3}
                  value={rulesText}
                  onChange={(e) => setRulesText(e.target.value)}
                  placeholder={'dev->*\nguest->nas'}
                  className={`${inputClass} h-auto py-2 font-mono`}
                />
                <p className="text-xs text-muted-foreground">
                  {tx('host.acl.rulesHelp', { form: <Mono>src-&gt;dst</Mono> })}
                </p>
              </div>
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <Button className="w-full" onClick={start} disabled={busy || !cfg.token}>
              <Play className="h-4 w-4" /> {t('host.start')}
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
            {state!.config.networkId} <Badge variant="success">{t('host.badge.hosting')}</Badge>
          </h1>
          <p className="text-sm text-muted-foreground">
            *.{state!.config.dnsSuffix}
            {managed && ` · ${t('host.inBackground')}`}
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={stop} disabled={busy}>
          <Square className="h-4 w-4" /> {t('host.stop')}
        </Button>
      </div>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <Server className="h-4 w-4" /> {t('host.share')}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <div className="flex items-center justify-between gap-2">
            <span className="shrink-0 text-muted-foreground">{t('host.share.joinAddress')}</span>
            <span className="flex min-w-0 items-center gap-2">
              <span className="truncate font-mono" title={state!.joinUrl}>
                {state!.joinUrl}
              </span>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => navigator.clipboard?.writeText(state!.joinUrl)}
                title={t('common.copy')}
              >
                <Copy className="h-4 w-4" />
              </Button>
            </span>
          </div>
          <div className="flex items-center justify-between gap-2">
            <span className="text-muted-foreground">{t('host.share.token')}</span>
            <span className="flex items-center gap-2">
              <span className="font-mono">{state!.config.token}</span>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => navigator.clipboard?.writeText(state!.config.token)}
                title={t('common.copy')}
              >
                <Copy className="h-4 w-4" />
              </Button>
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">{t('host.share.control')}</span>
            <span className="font-mono">{state!.config.controlAddr}</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">{t('common.relay')}</span>
            <span className="font-mono">{state!.config.relayAddr}</span>
          </div>
          <div className="flex items-center justify-between gap-2">
            <span className="shrink-0 text-muted-foreground">{t('host.share.trust')}</span>
            <span className={`flex min-w-0 items-center gap-1.5 ${state!.tlsMode === 'off' ? 'text-destructive' : ''}`}>
              {state!.tlsMode === 'off' ? (
                <ShieldAlert className="h-4 w-4 shrink-0" />
              ) : (
                <ShieldCheck className="h-4 w-4 shrink-0" />
              )}
              <span className="truncate font-mono" title={state!.pin || state!.tlsMode}>
                {state!.tlsMode === 'off'
                  ? t('host.trust.off')
                  : state!.tlsMode === 'acme'
                    ? t('host.trust.acme')
                    : state!.pin}
              </span>
              {state!.pin && (
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => navigator.clipboard?.writeText(state!.pin)}
                  title={t('host.share.copyPin')}
                >
                  <Copy className="h-4 w-4" />
                </Button>
              )}
            </span>
          </div>
          <p className="pt-1 text-xs text-muted-foreground">{t('host.share.pinNote')}</p>
          {/loopback|127\.0\.0\.1|localhost/.test(state!.joinUrl) && (
            <p className="text-xs text-muted-foreground">{t('host.share.loopbackNote')}</p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <KeyRound className="h-4 w-4" /> {t('host.acl')}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          {(state!.config.acl?.length ?? 0) === 0 ? (
            <p className="text-muted-foreground">{t('host.acl.noRules')}</p>
          ) : (
            <div className="space-y-1">
              <p className="text-muted-foreground">{t('host.acl.denyExcept')}</p>
              {state!.config.acl!.map((r, i) => (
                <div key={i} className="font-mono text-xs">
                  {r.src.join(',')}-&gt;{r.dst.join(',')}
                </div>
              ))}
            </div>
          )}
          <div className="flex flex-wrap items-center gap-2 pt-1">
            <span className="text-muted-foreground">{t('host.acl.groups')}</span>
            <Badge variant="muted">default</Badge>
            {Object.keys(state!.config.groupTokens ?? {}).map((g) => (
              <Badge key={g} variant="muted">
                {g}
              </Badge>
            ))}
          </div>
          {Object.entries(state!.config.groupTokens ?? {}).map(([g, tok]) => (
            <div key={g} className="flex items-center justify-between gap-2">
              <span className="text-muted-foreground">{t('host.acl.groupToken', { group: g })}</span>
              <span className="flex items-center gap-2">
                <span className="font-mono">{tok}</span>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => navigator.clipboard?.writeText(tok)}
                  title={t('common.copy')}
                >
                  <Copy className="h-4 w-4" />
                </Button>
              </span>
            </div>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <Users className="h-4 w-4" /> {t('host.members')}{' '}
            <Badge variant="muted">{state!.peers?.length ?? 0}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-1">
          {(state!.peers?.length ?? 0) === 0 && (
            <p className="text-sm text-muted-foreground">{t('host.members.none')}</p>
          )}
          {state!.peers?.map((p) => (
            <div key={p.public_key} className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
              <span className="flex items-center gap-2 font-medium">
                {p.hostname || t('common.unnamed')}
                {p.group && <Badge variant="muted">{p.group}</Badge>}
              </span>
              <span className="font-mono text-muted-foreground">{p.assigned_ip}</span>
            </div>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <Share2 className="h-4 w-4" /> {t('host.services')}{' '}
            <Badge variant="muted">{state!.services?.length ?? 0}</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-1">
          {(state!.services?.length ?? 0) === 0 && (
            <p className="text-sm text-muted-foreground">{t('host.services.none')}</p>
          )}
          {state!.services?.map((s) => (
            <div key={s.name} className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent">
              <span className="font-mono">
                {s.name}.{state!.config.dnsSuffix}
              </span>
              <span className="flex items-center gap-2">
                {(s.allow_groups?.length ?? 0) > 0 && <Badge variant="muted">{s.allow_groups!.join(', ')}</Badge>}
                <span className="font-mono text-muted-foreground">
                  {s.vip || s.node_ip}:{s.port}/{s.proto}
                </span>
              </span>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  )
}
