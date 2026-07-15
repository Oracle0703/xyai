import type { OrganizationUsageReportData } from '@/api/admin/organizationUsage'
import type { OrganizationUsageExportWorkerResponse } from './organizationUsageExport.worker'

export type OrganizationUsageExportStage = 'building' | 'serializing'

export interface OrganizationUsageExportWorkerOptions {
  signal?: AbortSignal
  onStage?: (stage: OrganizationUsageExportStage) => void
}

function abortError(signal: AbortSignal): Error | DOMException {
  if (signal.reason instanceof Error) return signal.reason
  return new DOMException('The operation was aborted', 'AbortError')
}

export function generateOrganizationUsageWorkbook(
  data: OrganizationUsageReportData,
  options: OrganizationUsageExportWorkerOptions = {}
): Promise<ArrayBuffer> {
  const { signal, onStage } = options
  if (signal?.aborted) return Promise.reject(abortError(signal))

  return new Promise<ArrayBuffer>((resolve, reject) => {
    const worker = new Worker(new URL('./organizationUsageExport.worker.ts', import.meta.url), { type: 'module' })
    let settled = false

    const finish = (callback: () => void) => {
      if (settled) return
      settled = true
      signal?.removeEventListener('abort', handleAbort)
      worker.onmessage = null
      worker.onmessageerror = null
      worker.onerror = null
      worker.terminate()
      callback()
    }
    const handleAbort = () => finish(() => reject(abortError(signal!)))

    signal?.addEventListener('abort', handleAbort, { once: true })
    worker.onmessage = (event: MessageEvent<OrganizationUsageExportWorkerResponse>) => {
      const message = event.data
      if (message.type === 'stage') {
        onStage?.(message.stage)
        return
      }
      if (message.type === 'success') {
        finish(() => resolve(message.buffer))
        return
      }
      finish(() => reject(new Error(message.message)))
    }
    worker.onerror = (event) => {
      const error = event.error instanceof Error
        ? event.error
        : new Error(event.message || 'Organization usage export worker failed')
      finish(() => reject(error))
    }
    worker.onmessageerror = () => {
      const error = new DOMException(
        'Organization usage export worker message could not be deserialized',
        'DataCloneError'
      )
      finish(() => reject(error))
    }
    try {
      worker.postMessage({ type: 'start', data })
    } catch (error) {
      finish(() => reject(error))
    }
  })
}
