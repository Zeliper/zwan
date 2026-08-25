import { Fragment, type ReactNode, useSyncExternalStore } from 'react'
import { snapshot, subscribe, translate, type Key, type Lang } from './i18n'

/**
 * useT gives a component the current language and the two ways to use it.
 *
 * `t` returns a string, for anywhere a string is all that fits — a title
 * attribute, a placeholder, a button label.
 *
 * `tx` returns nodes, because several of these sentences embed a command or an
 * example that has to keep its monospace styling. Interpolating markup into a
 * translated string is otherwise the moment a UI starts assembling sentences
 * from fragments, which does not survive translation: the placeholder can sit
 * anywhere in the sentence, and each language puts it where its own grammar
 * wants it.
 */
export function useT(): {
  lang: Lang
  t: (key: Key, vars?: Record<string, string | number>) => string
  tx: (key: Key, nodes: Record<string, ReactNode>) => ReactNode
} {
  const lang = useSyncExternalStore(subscribe, snapshot, snapshot)
  return {
    lang,
    t: (key, vars) => translate(lang, key, vars),
    tx: (key, nodes) => interpolate(translate(lang, key), nodes),
  }
}

/** interpolate splits a translated string on {placeholders} and drops nodes in. */
function interpolate(text: string, nodes: Record<string, ReactNode>): ReactNode {
  const parts = text.split(/(\{\w+\})/g)
  return (
    <>
      {parts.map((part, i) => {
        const name = /^\{(\w+)\}$/.exec(part)?.[1]
        if (name && name in nodes) return <Fragment key={i}>{nodes[name]}</Fragment>
        return <Fragment key={i}>{part}</Fragment>
      })}
    </>
  )
}

/** Mono is the styling those embedded fragments carry — a command, a name, an example. */
export function Mono({ children }: { children: ReactNode }) {
  return <span className="font-mono">{children}</span>
}
