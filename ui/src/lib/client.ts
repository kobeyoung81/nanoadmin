// The data client for modules that are not components.
//
// A lib module (the capability API, the model API) has no props to read a context
// from, so the client comes from the page that is mounted. Same shapes as the
// shell's helpers, so a ported lib only changed its import line.

import { hostApi } from '../host'

export const fetchJson = <T>(path: string, base?: string): Promise<T> => hostApi().get<T>(path, base)
export const postJson = <T>(path: string, body: unknown, base?: string): Promise<T> =>
  hostApi().post<T>(path, body, base)
export const putJson = <T>(path: string, body: unknown, base?: string): Promise<T> => hostApi().put<T>(path, body, base)
export const patchJson = <T>(path: string, body: unknown, base?: string): Promise<T> =>
  hostApi().patch<T>(path, body, base)
export const deleteJson = <T>(path: string, base?: string): Promise<T> => hostApi().del<T>(path, base)

/** The user-facing base, for the endpoints a person calls about their own things. */
export const userBase = (): string => hostApi().bases.user
