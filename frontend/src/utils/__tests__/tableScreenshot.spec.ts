import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { captureTablePng } from '../tableScreenshot'

const { html2canvas } = vi.hoisted(() => ({ html2canvas: vi.fn() }))
vi.mock('html2canvas', () => ({ default: html2canvas }))

function renderedTable(width: number, height: number) {
  const container = document.createElement('div')
  container.style.overflow = 'auto'
  const table = document.createElement('table')
  container.append(table)
  document.body.append(container)
  Object.defineProperties(table, { scrollWidth: { value: width }, scrollHeight: { value: height } })
  return table
}

describe('captureTablePng', () => {
  beforeEach(() => {
    html2canvas.mockReset()
  })
  afterEach(() => {
    document.body.replaceChildren()
    document.documentElement.classList.remove('dark')
  })

  it.each([1200, 3200, 6400])('captures the full wide table at height %i with a bounded canvas', async height => {
    const table = renderedTable(2600, height)
    table.parentElement!.scrollLeft = 900
    const blob = new Blob(['png'], { type: 'image/png' })
    const canvas = { width: 2600, height, toBlob: (callback: BlobCallback) => callback(blob) }
    html2canvas.mockResolvedValue(canvas)
    expect(await captureTablePng(table, new AbortController().signal)).toBe(blob)
    const options = html2canvas.mock.calls[0][1]
    expect(options.width).toBe(2600)
    expect(options.height).toBe(height)
    expect(options.scale * options.scale * 2600 * height).toBeLessThanOrEqual(16_000_001)
    const cloneParent = document.createElement('div')
    cloneParent.style.overflow = 'auto'
    cloneParent.scrollLeft = 900
    const clone = table.cloneNode(true) as HTMLTableElement
    cloneParent.append(clone)
    options.onclone(document, clone)
    expect(cloneParent.style.overflow).toBe('visible')
    expect(cloneParent.scrollLeft).toBe(0)
    expect(table.parentElement!.style.overflow).toBe('auto')
    expect(table.parentElement!.scrollLeft).toBe(900)
    expect(canvas.width).toBe(0)
    expect(canvas.height).toBe(0)
  })

  it('uses an opaque dark background for dark table text', async () => {
    document.documentElement.classList.add('dark')
    html2canvas.mockResolvedValue({ toBlob: (callback: BlobCallback) => callback(new Blob(['png'])) })
    await captureTablePng(renderedTable(2600, 1200), new AbortController().signal)
    expect(html2canvas.mock.calls[0][1].backgroundColor).toBe('#111827')
  })

  it('rejects an aborted capture before rendering', async () => {
    const controller = new AbortController()
    controller.abort()
    await expect(captureTablePng(renderedTable(2600, 1200), controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
    expect(html2canvas).not.toHaveBeenCalled()
  })

  it('rejects empty PNG encoding and releases the canvas', async () => {
    const canvas = { width: 2600, height: 1200, toBlob: (callback: BlobCallback) => callback(null) }
    html2canvas.mockResolvedValue(canvas)
    await expect(captureTablePng(renderedTable(2600, 1200), new AbortController().signal)).rejects.toThrow('PNG encoding failed')
    expect(canvas.width).toBe(0)
    expect(canvas.height).toBe(0)
  })
})
