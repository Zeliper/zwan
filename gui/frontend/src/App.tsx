import { useState } from 'react'
import { Network } from 'lucide-react'
import { ThemeToggle } from '@/components/theme-toggle'
import ClientView from './ClientView'
import HostView from './HostView'

type Mode = 'client' | 'host'

export default function App() {
  const [mode, setMode] = useState<Mode>('client')

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

      <main className="mx-auto w-full max-w-3xl p-6">
        {mode === 'client' ? <ClientView /> : <HostView />}
      </main>
    </div>
  )
}
