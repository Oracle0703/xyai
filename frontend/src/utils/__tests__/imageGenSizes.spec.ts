import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  CUSTOM_SIZES_STORAGE_KEY,
  MAX_CUSTOM_SIZES,
  formatImageSizeLabel,
  isCustomImageSize,
  normalizeCustomSizes,
  parseImageSize,
  readCustomSizes,
  writeCustomSizes,
} from '../imageGenSizes'

describe('imageGenSizes', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('parses width and height from number inputs into WxH', () => {
    expect(parseImageSize(1280, 720)).toEqual({ ok: true, size: '1280x720' })
    expect(parseImageSize('2048', ' 1152 ')).toEqual({ ok: true, size: '2048x1152' })
    expect(parseImageSize('', 720)).toEqual({ ok: false, error: '请填写宽和高' })
    expect(parseImageSize(1280, '')).toEqual({ ok: false, error: '请填写宽和高' })
    expect(parseImageSize(1280.5, 720).ok).toBe(false)
    expect(parseImageSize(63, 720).ok).toBe(false)
    expect(parseImageSize(4097, 720).ok).toBe(false)
    expect(parseImageSize(64, 4096)).toEqual({ ok: true, size: '64x4096' })
  })

  it('accepts only canonical sizes read back from storage', () => {
    expect(isCustomImageSize('1280x720')).toBe(true)
    expect(isCustomImageSize('01280x720')).toBe(false)
    expect(isCustomImageSize('1280X720')).toBe(false)
    expect(isCustomImageSize('1280 x 720')).toBe(false)
    expect(isCustomImageSize('8192x720')).toBe(false)
    expect(isCustomImageSize(1280)).toBe(false)
  })

  it('dedupes, drops presets and invalid values, and caps the list', () => {
    expect(normalizeCustomSizes(['1280x720', '1024x1024', 'bad', '1280x720', '720x1280'])).toEqual(['1280x720', '720x1280'])
    const many = Array.from({ length: MAX_CUSTOM_SIZES + 5 }, (_, index) => `${100 + index}x100`)
    expect(normalizeCustomSizes(many)).toHaveLength(MAX_CUSTOM_SIZES)
  })

  it('round-trips through localStorage and ignores malformed records', () => {
    expect(readCustomSizes()).toEqual([])
    expect(writeCustomSizes(['1280x720'])).toBe(true)
    expect(JSON.parse(localStorage.getItem(CUSTOM_SIZES_STORAGE_KEY) || '')).toEqual({ version: 1, sizes: ['1280x720'] })
    expect(readCustomSizes()).toEqual(['1280x720'])

    expect(writeCustomSizes([])).toBe(true)
    expect(localStorage.getItem(CUSTOM_SIZES_STORAGE_KEY)).toBeNull()

    localStorage.setItem(CUSTOM_SIZES_STORAGE_KEY, '{not json')
    expect(readCustomSizes()).toEqual([])
    localStorage.setItem(CUSTOM_SIZES_STORAGE_KEY, JSON.stringify({ version: 2, sizes: ['1280x720'] }))
    expect(readCustomSizes()).toEqual([])
  })

  it('reports storage write failures', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('quota exceeded', 'QuotaExceededError')
    })
    expect(writeCustomSizes(['1280x720'])).toBe(false)
  })

  it('formats labels with spaced x', () => {
    expect(formatImageSizeLabel('1280x720')).toBe('1280 x 720')
  })
})
