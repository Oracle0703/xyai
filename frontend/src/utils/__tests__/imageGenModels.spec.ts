import { describe, expect, it } from 'vitest'

import {
  MODEL_PREFERENCE_STORAGE_KEY,
  SAVED_API_KEY_STORAGE_KEY,
  compareModelIds,
  endpointsShareOrigin,
  isImageModelCandidate,
  isValidModelId,
  parseImageModelCatalog,
  readModelPreference,
  readSavedApiKey,
  redactCredential,
  removeSavedApiKey,
  writeModelPreference,
  writeSavedApiKey,
} from '../imageGenModels'

function createStorage(initial: Record<string, string> = {}): Storage {
  const values = new Map(Object.entries(initial))
  return {
    get length() {
      return values.size
    },
    clear: () => values.clear(),
    getItem: (key: string) => values.get(key) ?? null,
    key: (index: number) => Array.from(values.keys())[index] ?? null,
    removeItem: (key: string) => {
      values.delete(key)
    },
    setItem: (key: string, value: string) => {
      values.set(key, value)
    },
  }
}

function createBrokenStorage(): Storage {
  const fail = () => {
    throw new DOMException('storage disabled', 'SecurityError')
  }
  return {
    length: 0,
    clear: fail,
    getItem: fail,
    key: fail,
    removeItem: fail,
    setItem: fail,
  }
}

describe('isImageModelCandidate', () => {
  it('mirrors the backend images endpoint model predicate', () => {
    for (const id of [
      'gpt-image-1',
      'gpt-image-2',
      'gpt-image-2.5-flare',
      'gpt-image-2.5-sunburst-2026-09-08',
      'GPT-Image-2',
      'grok-imagine',
      'grok-imagine-edit',
      'grok-imagine-image',
      'grok-imagine-image-quality',
      'gemini-2.5-flash-image',
      'gemini-3-pro-image-preview',
    ]) {
      expect(isImageModelCandidate(id), id).toBe(true)
    }
    for (const id of [
      'gpt-imagex',
      'gpt-image',
      'gpt-5.4',
      'grok-imagine-video',
      'grok-imagine-1',
      'gemini-2.5-pro',
      'dall-e-3',
    ]) {
      expect(isImageModelCandidate(id), id).toBe(false)
    }
  })
})

describe('isValidModelId', () => {
  it('rejects empty, padded and control-character ids', () => {
    expect(isValidModelId('gpt-image-2')).toBe(true)
    expect(isValidModelId('')).toBe(false)
    expect(isValidModelId(' gpt-image-2')).toBe(false)
    expect(isValidModelId('gpt-image-2 ')).toBe(false)
    expect(isValidModelId('gpt-image\n-2')).toBe(false)
    expect(isValidModelId('gpt-image\u007f')).toBe(false)
    expect(isValidModelId('gpt-image\u0085')).toBe(false)
  })
})

describe('parseImageModelCatalog', () => {
  it('distinguishes malformed responses from an empty catalog', () => {
    expect(parseImageModelCatalog(undefined)).toBeNull()
    expect(parseImageModelCatalog('<html>')).toBeNull()
    expect(parseImageModelCatalog({})).toBeNull()
    expect(parseImageModelCatalog({ data: {} })).toBeNull()
    expect(parseImageModelCatalog({ data: [] })).toEqual([])
  })

  it('filters, de-duplicates and naturally sorts image models without rewriting ids', () => {
    const result = parseImageModelCatalog({
      object: 'list',
      data: [
        { id: 'gpt-image-2.5-sunburst' },
        { id: 'gpt-5.4' },
        { id: 'gpt-image-10' },
        { id: 'gpt-image-2' },
        { id: 'gpt-image-2' },
        { id: 'gpt-image-1.5' },
        { id: ' gpt-image-3' },
        { id: 42 },
        null,
        'gpt-image-9',
        { id: 'gpt-image-2.5-flare' },
        { id: 'gpt-image-1' },
        { id: 'grok-imagine-video' },
      ],
    })

    expect(result).toEqual([
      'gpt-image-1',
      'gpt-image-1.5',
      'gpt-image-2',
      'gpt-image-2.5-flare',
      'gpt-image-2.5-sunburst',
      'gpt-image-10',
    ])
  })

  it('sorts deterministically', () => {
    const ids = ['grok-imagine-image', 'gpt-image-2', 'gemini-2.5-flash-image', 'GPT-image-2']
    expect([...ids].sort(compareModelIds)).toEqual([...ids].reverse().sort(compareModelIds))
  })
})

describe('endpointsShareOrigin', () => {
  const base = 'https://console.example.com'

  it('accepts same-origin relative and absolute endpoints', () => {
    expect(endpointsShareOrigin(['/v1/models', '/v1/images/generations', '/v1/images/edits'], base)).toBe(true)
    expect(endpointsShareOrigin(
      ['https://console.example.com/v1/models', '/v1/images/generations', '/v1/images/edits'],
      base
    )).toBe(true)
  })

  it('rejects a models endpoint on a different origin than the image endpoints', () => {
    expect(endpointsShareOrigin(
      ['/v1/models', 'https://gateway.example.net/v1/images/generations', 'https://gateway.example.net/v1/images/edits'],
      base
    )).toBe(false)
    expect(endpointsShareOrigin(['http://console.example.com/v1/models', '/v1/images/generations'], base)).toBe(false)
  })
})

describe('preference storage', () => {
  it('round-trips the model preference and drops corrupted entries', () => {
    const storage = createStorage()
    expect(writeModelPreference({ model: 'gpt-image-2', source: 'list' }, storage)).toBe(true)
    expect(readModelPreference(storage)).toEqual({ model: 'gpt-image-2', source: 'list' })

    storage.setItem(MODEL_PREFERENCE_STORAGE_KEY, JSON.stringify({ version: 2, model: 'gpt-image-2', modelSource: 'list' }))
    expect(readModelPreference(storage)).toBeNull()
    expect(storage.getItem(MODEL_PREFERENCE_STORAGE_KEY)).toBeNull()
  })

  it('reads a saved key and reports invalid entries', () => {
    const storage = createStorage()
    expect(readSavedApiKey(storage)).toEqual({ kind: 'none' })

    expect(writeSavedApiKey('sk-saved', storage)).toBe(true)
    expect(JSON.parse(storage.getItem(SAVED_API_KEY_STORAGE_KEY) || '{}')).toEqual({ version: 1, apiKey: 'sk-saved' })
    expect(readSavedApiKey(storage)).toEqual({ kind: 'ok', apiKey: 'sk-saved' })

    storage.setItem(SAVED_API_KEY_STORAGE_KEY, '{broken')
    expect(readSavedApiKey(storage)).toEqual({ kind: 'invalid', removed: true })
    expect(storage.getItem(SAVED_API_KEY_STORAGE_KEY)).toBeNull()
  })

  it('reports failures when storage is unavailable', () => {
    const storage = createBrokenStorage()
    expect(readModelPreference(storage)).toBeNull()
    expect(readSavedApiKey(storage)).toEqual({ kind: 'none' })
    expect(writeSavedApiKey('sk-saved', storage)).toBe(false)
    expect(writeModelPreference({ model: 'gpt-image-2', source: 'manual' }, storage)).toBe(false)
    expect(removeSavedApiKey(storage)).toBe(false)
    expect(removeSavedApiKey(null)).toBe(false)
  })
})

describe('redactCredential', () => {
  it('replaces every occurrence of the request key and leaves other text intact', () => {
    expect(redactCredential('Invalid API key sk-abc, retry with sk-abc', 'sk-abc')).toBe('Invalid API key ***, retry with ***')
    expect(redactCredential('Insufficient account balance', 'sk-abc')).toBe('Insufficient account balance')
    expect(redactCredential('message', '')).toBe('message')
  })
})
