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

  it('declares every page in the manifest and exposes them from the remote build', () => {
    const pagePaths = manifest.pages.map((page) => page.path)
    const navPaths = manifest.nav.map((entry) => entry.path)

    // A page under a menu entry, or a detail route the menu cannot express.
    for (const path of pagePaths) {
      expect(path.includes(':') || navPaths.includes(path)).toBe(true)
    }

    // The screens this plugin renders today. The console's remaining ones
    // (configs, directory, voice, models, agent-types, capabilities,
    // conversations, alerts) still come from the shell's fallback; each moves
    // here by adding a page and deleting its fallback, and this list is the
    // record of how far the migration has got.
    expect(pagePaths).toEqual([
      '/admin/overview',
      '/admin/components',
      '/admin/runtime',
      '/admin/components/:type',
      '/admin/audit',
    ])
    expect(manifest.entry).toBe('/plugins/admin/remoteEntry.js')
    expect(remoteConfig).toContain("'./pages': './src/pages.tsx'")
    expect(remoteConfig).toContain("base: '/plugins/admin/'")
  })
})
