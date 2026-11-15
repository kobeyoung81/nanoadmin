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

/**
 * setHostContext records the page that is on screen.
 *
 * The shell mounts every page as `pages.tsx` exports it, so the wrapper there calls
 * this before the page renders. A nested component (or a lib module) then answers
 * from the same value — without it, `useHost()` had a provider that nothing ever
 * rendered and every nested screen threw "HostProvider is missing".
 */
export function setHostContext(ctx: PageContext): void {
  currentContext = ctx
}

export function HostProvider({ ctx, children }: { ctx: PageContext; children: ReactNode }) {
  setHostContext(ctx)
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

/**
 * useHost returns the current page's context.
 *
 * The provider is preferred (it re-renders its subtree when the context changes);
 * the module-level copy covers the pages the shell mounts directly, which install it
 * through `withHost` in pages.tsx. A component outside any page is a bug and says so.
 */
export function useHost(): PageContext {
  const fromProvider = useContext(HostContext)
  const ctx = fromProvider ?? currentContext
  if (!ctx) {
    throw new Error('this component must render inside a page (no host context is installed)')
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
