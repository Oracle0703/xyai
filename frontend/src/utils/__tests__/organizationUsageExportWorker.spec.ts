import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { OrganizationUsageReportData } from '@/api/admin/organizationUsage'
import { generateOrganizationUsageWorkbook } from '../organizationUsageExportWorker'

const reportData = { summary: {}, periods: {} } as OrganizationUsageReportData

class FakeWorker {
  static instances: FakeWorker[] = []
  static postMessageError: Error | null = null

  onmessage: ((event: MessageEvent) => void) | null = null
  onmessageerror: ((event: MessageEvent) => void) | null = null
  onerror: ((event: ErrorEvent) => void) | null = null
  readonly postMessage = vi.fn(() => {
    if (FakeWorker.postMessageError) throw FakeWorker.postMessageError
  })
  readonly terminate = vi.fn()

  constructor(public readonly url: URL, public readonly options?: WorkerOptions) {
    FakeWorker.instances.push(this)
  }

  emitMessage(data: unknown) {
    this.onmessage?.({ data } as MessageEvent)
  }

  emitError(error: Error) {
    this.onerror?.({ error, message: error.message } as ErrorEvent)
  }

  emitMessageError() {
    this.onmessageerror?.({} as MessageEvent)
  }
}

describe('organization usage export worker coordinator', () => {
  beforeEach(() => {
    FakeWorker.instances = []
    FakeWorker.postMessageError = null
    vi.stubGlobal('Worker', FakeWorker)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('forwards stages and resolves the transferred workbook buffer', async () => {
    const onStage = vi.fn()
    const result = generateOrganizationUsageWorkbook(reportData, { onStage })
    void result.catch(() => undefined)

    expect(FakeWorker.instances).toHaveLength(1)
    const worker = FakeWorker.instances[0]
    expect(worker.options).toEqual({ type: 'module' })
    expect(worker.postMessage).toHaveBeenCalledWith({ type: 'start', data: reportData })

    worker.emitMessage({ type: 'stage', stage: 'building' })
    worker.emitMessage({ type: 'stage', stage: 'serializing' })
    const buffer = new ArrayBuffer(16)
    worker.emitMessage({ type: 'success', buffer })

    await expect(result).resolves.toBe(buffer)
    expect(onStage.mock.calls).toEqual([['building'], ['serializing']])
    expect(worker.terminate).toHaveBeenCalledOnce()
  })

  it('terminates immediately and rejects AbortError when the signal aborts', async () => {
    const controller = new AbortController()
    const result = generateOrganizationUsageWorkbook(reportData, { signal: controller.signal })
    void result.catch(() => undefined)
    expect(FakeWorker.instances).toHaveLength(1)

    controller.abort()

    await expect(result).rejects.toMatchObject({ name: 'AbortError' })
    expect(FakeWorker.instances[0].terminate).toHaveBeenCalledOnce()
  })

  it('terminates and preserves an ordinary Worker error', async () => {
    const result = generateOrganizationUsageWorkbook(reportData)
    void result.catch(() => undefined)
    expect(FakeWorker.instances).toHaveLength(1)
    const worker = FakeWorker.instances[0]
    const error = new Error('worker crashed')

    worker.emitError(error)

    await expect(result).rejects.toBe(error)
    expect(worker.terminate).toHaveBeenCalledOnce()
  })

  it('terminates and rejects a structured error message from the worker', async () => {
    const result = generateOrganizationUsageWorkbook(reportData)
    void result.catch(() => undefined)
    expect(FakeWorker.instances).toHaveLength(1)
    const worker = FakeWorker.instances[0]

    worker.emitMessage({ type: 'error', message: 'serialization failed' })

    await expect(result).rejects.toThrow('serialization failed')
    expect(worker.terminate).toHaveBeenCalledOnce()
  })

  it('cleans up and preserves a synchronous postMessage error', async () => {
    const controller = new AbortController()
    const removeAbortListener = vi.spyOn(controller.signal, 'removeEventListener')
    const error = new DOMException('could not clone payload', 'DataCloneError')
    FakeWorker.postMessageError = error

    const result = generateOrganizationUsageWorkbook(reportData, { signal: controller.signal })
    const worker = FakeWorker.instances[0]

    await expect(result).rejects.toBe(error)
    expect(worker.terminate).toHaveBeenCalledOnce()
    expect(removeAbortListener).toHaveBeenCalledWith('abort', expect.any(Function))
    expect(worker.onmessage).toBeNull()
    expect(worker.onmessageerror).toBeNull()
    expect(worker.onerror).toBeNull()
  })

  it('terminates and rejects a DataCloneError when a worker message cannot be read', async () => {
    const result = generateOrganizationUsageWorkbook(reportData)
    void result.catch(() => undefined)
    const worker = FakeWorker.instances[0]

    worker.emitMessageError()

    await expect(result).rejects.toMatchObject({
      name: 'DataCloneError',
      message: 'Organization usage export worker message could not be deserialized'
    })
    expect(worker.terminate).toHaveBeenCalledOnce()
    expect(worker.onmessage).toBeNull()
    expect(worker.onmessageerror).toBeNull()
    expect(worker.onerror).toBeNull()
  })
})
