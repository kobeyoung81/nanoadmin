// The one slice of browser state this module keeps: when the directory was last
// probed.
//
// The shell owns the key namespace (`nanosec.portal.u.<userId>.<area>.<suffix>`),
// so the name is built the same way here; the page passes the signed-in user id.
// Nothing else from the shell's state store is needed by these screens.

const NAMESPACE = 'nanosec.portal.u.'

export function readRawState(userId: string, area: string, suffix: string): string | null {
  if (!userId) {
    return null
  }
  try {
    return localStorage.getItem(`${NAMESPACE}${userId}.${area}.${suffix}`)
  } catch {
    return null
  }
}

export function writeRawState(userId: string, area: string, suffix: string, value: string): void {
  if (!userId) {
    return
  }
  try {
    localStorage.setItem(`${NAMESPACE}${userId}.${area}.${suffix}`, value)
  } catch {
    // A full or unavailable storage must not break the probe; the stamp simply
    // does not survive the reload.
  }
}
