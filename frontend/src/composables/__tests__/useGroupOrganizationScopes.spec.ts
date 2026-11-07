import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useGroupOrganizationScopes } from '../useGroupOrganizationScopes'

const list = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin/groupOrganizationScopes', () => ({ listGroupOrganizationScopes: list }))

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

// K24: only the latest request is accepted; a failure is not a verified snapshot.
describe('useGroupOrganizationScopes', () => {
  beforeEach(() => {
    list.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  it('ignores an older response that arrives after a newer one', async () => {
    const older = deferred<Map<number, string[]>>()
    const newer = deferred<Map<number, string[]>>()
    list.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    const loader = useGroupOrganizationScopes(() => true)
    const first = loader.load()
    const second = loader.load()
    newer.resolve(new Map([[1, ['xunyou', 'wsdashi']]]))
    expect(await second).toBe(true)
    older.resolve(new Map([[1, ['xunyou']]]))
    expect(await first).toBe(false)
    expect(loader.scopes.value.get(1)).toEqual(['xunyou', 'wsdashi'])
    expect(loader.generation.value).toBe(2)
    expect(loader.status.value).toBe('ready')
  })

  it('ignores a late error of a superseded request', async () => {
    const older = deferred<Map<number, string[]>>()
    list.mockReturnValueOnce(older.promise).mockResolvedValueOnce(new Map())
    const loader = useGroupOrganizationScopes(() => true)
    const first = loader.load()
    await loader.load()
    older.reject(new Error('late failure'))
    expect(await first).toBe(false)
    expect(loader.status.value).toBe('ready')
  })

  it('marks a failed refresh as unverified and moves past the mark only on success', async () => {
    list.mockResolvedValueOnce(new Map([[1, ['xunyou']]])).mockRejectedValueOnce(new Error('down'))
    const loader = useGroupOrganizationScopes(() => true)
    await loader.load()
    const mark = loader.mark()
    expect(await loader.load()).toBe(false)
    expect(loader.status.value).toBe('error')
    expect(loader.generation.value).toBe(mark, 'a failed load never counts as read after the mark')
    expect(loader.scopes.value.get(1)).toEqual(['xunyou'])
  })

  it('does nothing when disabled', async () => {
    const loader = useGroupOrganizationScopes(() => false)
    expect(await loader.load()).toBe(false)
    expect(list).not.toHaveBeenCalled()
    expect(loader.status.value).toBe('idle')
  })
})
