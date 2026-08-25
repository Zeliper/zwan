import { Languages } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { languages, setLang } from '@/lib/i18n'
import { useT } from '@/lib/use-i18n'
import { SetLanguage } from '../../wailsjs/go/main/App'

/**
 * LanguageToggle cycles the interface language, mirroring the theme toggle next
 * to it: one button, no menu, because there are two languages and a menu for two
 * choices is a menu too many.
 *
 * It tells the Go side as well, so the tray menu — which is drawn outside this
 * window and may be all the user sees — changes with it.
 */
export function LanguageToggle() {
  const { lang, t } = useT()
  const label = languages.find((l) => l.code === lang)?.label ?? lang

  const cycle = () => {
    const next = languages[(languages.findIndex((l) => l.code === lang) + 1) % languages.length].code
    setLang(next)
    SetLanguage(next).catch(() => {
      /* the tray keeps the language it had; the window is already correct */
    })
  }

  return (
    <Button variant="outline" size="icon" onClick={cycle} title={t('app.language.title', { language: label })}>
      <Languages className="h-4 w-4" />
    </Button>
  )
}
