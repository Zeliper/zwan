import { useEffect, useState } from 'react'
import { Loader2, Plus, Share2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Mono, useT } from '@/lib/use-i18n'
import type { PublishedService } from './ClientView'

interface Props {
  network: { alias: string; publish?: PublishedService[] }
  busy: boolean
  onSave: (publish: PublishedService[]) => void
}

const blank: PublishedService = { name: '', proto: 'tcp', port: 0, backend_port: 0, allow_groups: [] }

const inputClass =
  'flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring'

/**
 * PublishEditor offers something on this machine to the network.
 *
 * The list is part of the saved network rather than of the server: the server's
 * service registry lives in memory, so this is what puts the services back after
 * either end restarts. Saving reconnects the network, because what a node
 * publishes is decided when it joins.
 */
export default function PublishEditor({ network, busy, onSave }: Props) {
  const { t, tx } = useT()
  const [rows, setRows] = useState<PublishedService[]>(network.publish ?? [])

  // The saved list is the truth until this device edits it; a reconnect
  // elsewhere should not leave a stale draft on screen.
  useEffect(() => {
    setRows(network.publish ?? [])
  }, [network.publish])

  const dirty = JSON.stringify(rows) !== JSON.stringify(network.publish ?? [])

  function set(i: number, patch: Partial<PublishedService>) {
    setRows(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  }

  // An empty port field should read as empty, not as a leading zero.
  const portValue = (n?: number) => (n ? String(n) : '')
  const portNumber = (v: string) => {
    const n = parseInt(v.replace(/\D/g, ''), 10)
    return Number.isFinite(n) ? n : 0
  }

  return (
    <div className="space-y-2 rounded-md border border-border p-3">
      <p className="flex items-center gap-2 text-sm font-medium">
        <Share2 className="h-4 w-4" /> {t('client.publish')}
      </p>
      <p className="text-xs text-muted-foreground">
        {tx('client.publish.help', { example: <Mono>{`<name>.${network.alias}`}</Mono> })}
      </p>

      {rows.length === 0 && <p className="text-sm text-muted-foreground">{t('client.publish.none')}</p>}

      {rows.map((r, i) => (
        <div key={i} className="space-y-2 rounded-md border border-border/60 p-2">
          <div className="flex items-end gap-2">
            <div className="min-w-0 flex-1 space-y-1">
              <Label className="text-xs">{t('client.publish.name')}</Label>
              <Input
                value={r.name}
                onChange={(e) => set(i, { name: e.target.value })}
                placeholder={t('client.publish.namePlaceholder')}
                className="font-mono"
              />
            </div>
            <div className="w-24 space-y-1">
              <Label className="text-xs">{t('client.publish.proto')}</Label>
              <select
                value={r.proto || 'tcp'}
                onChange={(e) => set(i, { proto: e.target.value })}
                className={inputClass}
              >
                <option value="tcp">tcp</option>
                <option value="udp">udp</option>
              </select>
            </div>
            <Button
              variant="ghost"
              size="icon"
              title={t('common.remove')}
              onClick={() => setRows(rows.filter((_, j) => j !== i))}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>

          <div className="grid grid-cols-2 gap-2">
            <div className="space-y-1">
              <Label className="text-xs">{t('client.publish.port')}</Label>
              <Input
                value={portValue(r.port)}
                onChange={(e) => set(i, { port: portNumber(e.target.value) })}
                inputMode="numeric"
                className="font-mono"
              />
              <p className="text-[11px] text-muted-foreground">{t('client.publish.portHelp')}</p>
            </div>
            <div className="space-y-1">
              <Label className="text-xs">{t('client.publish.backend')}</Label>
              <Input
                value={portValue(r.backend_port)}
                onChange={(e) => set(i, { backend_port: portNumber(e.target.value) })}
                inputMode="numeric"
                className="font-mono"
              />
              <p className="text-[11px] text-muted-foreground">{t('client.publish.backendHelp')}</p>
            </div>
          </div>

          <div className="space-y-1">
            <Label className="text-xs">{t('client.publish.groups')}</Label>
            <Input
              value={(r.allow_groups ?? []).join(', ')}
              onChange={(e) =>
                set(i, {
                  allow_groups: e.target.value
                    .split(',')
                    .map((g) => g.trim())
                    .filter(Boolean),
                })
              }
              placeholder={t('client.publish.groupsPlaceholder')}
              className="font-mono"
            />
          </div>
        </div>
      ))}

      <div className="flex items-center gap-2">
        <Button variant="outline" size="sm" onClick={() => setRows([...rows, { ...blank }])}>
          <Plus className="h-4 w-4" /> {t('client.publish.add')}
        </Button>
        {dirty && (
          <Button size="sm" disabled={busy} onClick={() => onSave(rows)}>
            {busy && <Loader2 className="h-4 w-4 animate-spin" />}
            {busy ? t('client.publish.saving') : t('client.publish.save')}
          </Button>
        )}
      </div>
      {dirty && <p className="text-xs text-muted-foreground">{t('client.publish.pending')}</p>}
    </div>
  )
}
