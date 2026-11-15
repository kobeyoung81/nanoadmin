// The one-shot transcription the console's "test recognition" button uses.
//
// It is the same call the composer's batch tier makes (POST /voice/transcribe),
// with `target=batch` asking the accurate pass's model instead of the realtime
// one. A file upload needs multipart, which the page's JSON client does not do, so
// this posts directly — with the token and the base from the mounted page.

export async function transcribeSample(
  host: { token: string; base: string },
  blob: Blob,
  filename = 'clip.wav',
  target: 'realtime' | 'batch' = 'realtime',
): Promise<string> {
  const form = new FormData()
  form.append('audio', blob, filename)
  if (target === 'batch') {
    form.append('target', 'batch')
  }
  const response = await fetch(`${host.base}/voice/transcribe`, {
    method: 'POST',
    headers: host.token ? { Authorization: `Bearer ${host.token}` } : undefined,
    body: form,
  })
  const payload = (await response.json().catch(() => null)) as { text?: string; error?: string } | null
  if (!response.ok) {
    throw new Error(payload?.error ?? `transcribe failed (${response.status})`)
  }
  return payload?.text ?? ''
}
