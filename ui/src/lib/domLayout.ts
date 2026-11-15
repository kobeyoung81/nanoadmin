// Whether an element is actually laid out.
//
// The workbench keeps every panel mounted, moving the hidden ones into a
// `display: none` vault; a measurement taken there is a degenerate value (zero
// height, no box). Callers ask this before believing one.

/** The two members a laid-out element must expose; a real element has more. */
export type VisibleCheck = {
  offsetParent: unknown
  getClientRects?: () => { length: number }
}

export function isLaidOut(el: VisibleCheck | null | undefined): boolean {
  if (!el) {
    return false
  }
  if (el.offsetParent) {
    return true
  }
  return typeof el.getClientRects === 'function' && el.getClientRects().length > 0
}
