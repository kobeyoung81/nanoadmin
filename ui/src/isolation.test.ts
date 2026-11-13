import { describe, expect, it } from 'vitest'
import manifest from '../public/plugin.json'
import remoteConfig from '../vite.config.ts?raw'
import pages from './pages.tsx?raw'

// This application renders inside the portal's tree and may not import the
// portal's source: the kit, the strings and the data client all arrive through the
// page context. These checks pin that boundary, because an import that sneaks back
// in would build here and fail at runtime (the module does not exist in the shell's
// bundle).

describe('the boundary', () => {
  it('imports only its own modules and icons', () => {
    const imports = [...pages.matchAll(/from '([^']+)'/g)].map((match) => match[1])
    const external = imports.filter((spec) => !spec.startsWith('.'))
    expect(external.sort()).toEqual(['lucide-react'])
  })

  it('takes the kit, the strings and the client from the context', () => {
    expect(pages).toContain('const { t, ui, api } = ctx')
    expect(pages).not.toContain("from '../components")
    expect(pages).not.toContain("from '../lib")
  })

  it('declares the page in the manifest and exposes it from the remote build', () => {
    expect(manifest.pages.map((page) => page.path)).toEqual(['/admin/runtime'])
    expect(manifest.entry).toBe('/plugins/admin/remoteEntry.js')
    expect(remoteConfig).toContain("'./pages': './src/pages.tsx'")
    expect(remoteConfig).toContain("base: '/plugins/admin/'")
  })
})
