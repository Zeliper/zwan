import { useEffect, useState } from 'react'
import { Monitor, Moon, Sun } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { getStoredTheme, setTheme, type Theme } from '@/lib/theme'
import { useT } from '@/lib/use-i18n'
import type { Key } from '@/lib/i18n'

const names: Record<Theme, Key> = {
  system: 'app.theme.system',
  light: 'app.theme.light',
  dark: 'app.theme.dark',
}

const order: Theme[] = ['system', 'light', 'dark']
const icons: Record<Theme, typeof Monitor> = { system: Monitor, light: Sun, dark: Moon }

export function ThemeToggle() {
  const { t } = useT()
  const [theme, setLocal] = useState<Theme>('system')
  useEffect(() => setLocal(getStoredTheme()), [])

  const cycle = () => {
    const next = order[(order.indexOf(theme) + 1) % order.length]
    setLocal(next)
    setTheme(next)
  }

  const Icon = icons[theme]
  return (
    <Button variant="outline" size="icon" onClick={cycle} title={t('app.theme.title', { theme: t(names[theme]) })}>
      <Icon className="h-4 w-4" />
    </Button>
  )
}
