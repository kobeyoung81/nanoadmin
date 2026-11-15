// The page context, reachable from a component that is not the page root.
//
// A page receives the shell's services as a prop, but the components it renders are
// nested — a provider form, a capability table — and threading the context through
// every one of them would change their props for no reason. This module keeps the
// current page's context where those components can read it.
//
// Only one page is on screen at a time (the shell unmounts a route's page when it
// navigates), so a module-level value is enough; the provider writes it on every
// render, and the last render wins.

import { createContext, useContext, type ReactNode } from 'react'
import type { PageContext } from './contract'

const HostContext = createContext<PageContext | null>(null)

let currentContext: PageContext | null = null

export function HostProvider({ ctx, children }: { ctx: PageContext; children: ReactNode }) {
  // Also kept module-level: a lib module (an API wrapper) cannot read a context.
  // Writing it during render is safe because only one page is mounted at a time.
  currentContext = ctx
  return <HostContext.Provider value={ctx}>{children}</HostContext.Provider>
}

/**
 * hostApi is the data client for code that is not a component.
 *
 * It throws outside a page, which is the honest answer: a lib call before a page
 * mounts would have no token and no base.
 */
export function hostApi(): PageContext['api'] {
  if (!currentContext) {
    throw new Error('no page is mounted: a lib module called the API before a page rendered')
  }
  return currentContext.api
}

/** useHost returns the current page's context; a component outside a page is a bug. */
export function useHost(): PageContext {
  const ctx = useContext(HostContext)
  if (!ctx) {
    throw new Error('this component must render inside a page (HostProvider is missing)')
  }
  return ctx
}

/**
 * cn joins class names, dropping the false-y ones.
 *
 * The shell's own helper also merges conflicting Tailwind classes (tailwind-merge);
 * a plugin cannot import that package without shipping it, and the components moved
 * here pass complete class lists rather than overriding one another, so a plain join
 * is enough. A component that needs to override a default class must build the full
 * string instead of appending to it.
 */
export function cn(...classes: Array<string | false | null | undefined>): string {
  return classes.filter(Boolean).join(' ')
}
