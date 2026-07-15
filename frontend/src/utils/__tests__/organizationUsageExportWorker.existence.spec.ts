import { existsSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const utilsDirectory = resolve(dirname(fileURLToPath(import.meta.url)), '..')

describe('organization usage export worker files', () => {
  it('provides both the main-thread coordinator and worker entry', () => {
    expect(existsSync(resolve(utilsDirectory, 'organizationUsageExportWorker.ts'))).toBe(true)
    expect(existsSync(resolve(utilsDirectory, 'organizationUsageExport.worker.ts'))).toBe(true)
  })
})
