import { useEffect, useState } from 'react'
import { Monitor, Moon, Sun } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { getStoredTheme, setTheme, type Theme } from '@/lib/theme'

const order: Theme[] = ['system', 'light', 'dark']
const icons: Record<Theme, typeof Monitor> = { system: Monitor, light: Sun, dark: Moon }

export function ThemeToggle() {
  const [theme, setLocal] = useState<Theme>('system')
  useEffect(() => setLocal(getStoredTheme()), [])

  const cycle = () => {
    const next = order[(order.indexOf(theme) + 1) % order.length]
    setLocal(next)
    setTheme(next)
  }

  const Icon = icons[theme]
  return (
    <Button variant="outline" size="icon" onClick={cycle} title={`Theme: ${theme} (click to change)`}>
      <Icon className="h-4 w-4" />
    </Button>
  )
}
