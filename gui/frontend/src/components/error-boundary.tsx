import { Component, type ErrorInfo, type ReactNode } from 'react'
import { AlertTriangle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { snapshot, translate } from '@/lib/i18n'

interface Props {
  children: ReactNode
  /** Resetting when this changes lets a failed tab recover by switching away and back. */
  resetKey?: unknown
}

interface State {
  error: Error | null
}

/**
 * ErrorBoundary keeps one broken view from taking the window with it.
 *
 * React unmounts the whole tree when a render throws, so without this a single
 * bad field — a list that arrived as null, say — leaves the user staring at an
 * empty black window with no menu, no tabs and nothing to click. That is
 * indistinguishable from the app having died, and it hides the one thing worth
 * knowing: what threw.
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // The window has no console a user can open, so leave a trace for a build
    // that does.
    console.error('view crashed:', error, info.componentStack)
  }

  componentDidUpdate(prev: Props) {
    if (prev.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({ error: null })
    }
  }

  render() {
    const { error } = this.state
    if (!error) return this.props.children

    // Read the language directly rather than through the hook: this is a class
    // component by necessity — only a class can catch a render error — and it
    // renders once, at the moment something already went wrong.
    const t = (key: Parameters<typeof translate>[1]) => translate(snapshot(), key)

    return (
      <div className="space-y-3 rounded-lg border border-destructive/40 bg-destructive/5 p-6">
        <div className="flex items-center gap-2 font-semibold">
          <AlertTriangle className="h-4 w-4 text-destructive" />
          {t('error.title')}
        </div>
        <p className="text-sm text-muted-foreground">{t('error.body')}</p>
        <pre className="max-h-48 overflow-auto rounded bg-muted p-3 font-mono text-xs">
          {error.message}
          {error.stack ? `\n\n${error.stack}` : ''}
        </pre>
        <Button size="sm" variant="outline" onClick={() => this.setState({ error: null })}>
          {t('error.retry')}
        </Button>
      </div>
    )
  }
}
