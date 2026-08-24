import { useEffect, useState } from 'react'
import { Download, Network } from 'lucide-react'
import { ThemeToggle } from '@/components/theme-toggle'
import { Button } from '@/components/ui/button'
import ClientView from './ClientView'
import HostView from './HostView'
import { ApplyUpdate, CheckUpdate, QuitApp } from '../wailsjs/go/main/App'

type Mode = 'client' | 'host'

interface Release {
  tag: string
  version: string
  installerUrl: string
  notes: string
}

export default function App() {
  const [mode, setMode] = useState<Mode>('client')
  const [update, setUpdate] = useState<Release | null>(null)
  const [updating, setUpdating] = useState(false)

  useEffect(() => {
    CheckUpdate()
      .then((r) => setUpdate((r as unknown as Release) ?? null))
      .catch(() => {})
  }, [])

  async function doUpdate() {
    if (!update) return
    setUpdating(true)
    try {
      await ApplyUpdate(update.installerUrl)
      await QuitApp()
    } catch {
      setUpdating(false)
    }
  }

  const tab = (m: Mode, label: string) => (
    <button
      onClick={() => setMode(m)}
      className={`rounded px-3 py-1 text-sm transition-colors ${
        mode === m ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground'
      }`}
    >
      {label}
    </button>
  )

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
          <div className="flex rounded-md border p-0.5">
            {tab('client', 'Join')}
            {tab('host', 'Host')}
          </div>
          <ThemeToggle />
        </div>
      </header>

      {update && (
        <div className="flex items-center justify-between gap-3 border-b bg-primary/10 px-6 py-2 text-sm">
          <span className="flex items-center gap-2">
            <Download className="h-4 w-4" /> Update available: <span className="font-semibold">v{update.version}</span>
          </span>
          <Button size="sm" onClick={doUpdate} disabled={updating || !update.installerUrl}>
            {updating ? 'Updating…' : 'Update now'}
          </Button>
        </div>
      )}

      <main className="mx-auto w-full max-w-3xl p-6">
        {mode === 'client' ? <ClientView /> : <HostView />}
      </main>
    </div>
  )
}
