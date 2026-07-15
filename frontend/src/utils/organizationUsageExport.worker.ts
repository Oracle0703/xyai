import * as XLSX from 'xlsx'

import type { OrganizationUsageReportData } from '@/api/admin/organizationUsage'
import { buildOrganizationUsageWorkbook } from './organizationUsageReport'
import type { OrganizationUsageExportStage } from './organizationUsageExportWorker'

export type OrganizationUsageExportWorkerResponse =
  | { type: 'stage'; stage: OrganizationUsageExportStage }
  | { type: 'success'; buffer: ArrayBuffer }
  | { type: 'error'; message: string }

interface OrganizationUsageExportWorkerRequest {
  type: 'start'
  data: OrganizationUsageReportData
}

type WorkerPostMessage = (
  message: OrganizationUsageExportWorkerResponse,
  transfer?: Transferable[]
) => void

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

export async function processOrganizationUsageExport(
  data: OrganizationUsageReportData,
  postMessage: WorkerPostMessage
): Promise<void> {
  try {
    postMessage({ type: 'stage', stage: 'building' })
    const workbook = buildOrganizationUsageWorkbook(data)
    postMessage({ type: 'stage', stage: 'serializing' })
    const buffer = XLSX.write(workbook, { bookType: 'xlsx', type: 'array' }) as ArrayBuffer
    postMessage({ type: 'success', buffer }, [buffer])
  } catch (error) {
    postMessage({ type: 'error', message: errorMessage(error) })
  }
}

interface WorkerScope {
  document?: unknown
  addEventListener: (
    type: 'message',
    listener: (event: MessageEvent<OrganizationUsageExportWorkerRequest>) => void
  ) => void
  postMessage: WorkerPostMessage
}

const workerScope = globalThis as unknown as WorkerScope
if (typeof workerScope.document === 'undefined' && typeof workerScope.addEventListener === 'function') {
  workerScope.addEventListener('message', (event) => {
    if (event.data.type !== 'start') return
    void processOrganizationUsageExport(
      event.data.data,
      (message, transfer) => workerScope.postMessage(message, transfer)
    )
  })
}
