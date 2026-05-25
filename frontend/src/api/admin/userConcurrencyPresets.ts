import { apiClient } from '../client'

export interface UserConcurrencyPreset {
  id: number
  name: string
  description: string
  target_concurrency: number
  user_ids: number[]
  schedule_enabled: boolean
  schedule_time: string
  last_scheduled_run_date?: string | null
  created_at: string
  updated_at: string
}

export interface UserConcurrencyPresetRun {
  id: number
  preset_id: number
  trigger: 'manual' | 'scheduled'
  target_concurrency: number
  user_ids: number[]
  affected_count: number
  status: 'success' | 'failed'
  error_message: string
  started_at: string
  finished_at: string
  created_at: string
}

export interface UserConcurrencyPresetPayload {
  name: string
  description?: string
  target_concurrency: number
  user_ids: number[]
  schedule_enabled: boolean
  schedule_time: string
}

const basePath = '/admin/users/concurrency-presets'

export async function listPresets(): Promise<UserConcurrencyPreset[]> {
  const { data } = await apiClient.get<UserConcurrencyPreset[]>(basePath)
  return data
}

export async function createPreset(payload: UserConcurrencyPresetPayload): Promise<UserConcurrencyPreset> {
  const { data } = await apiClient.post<UserConcurrencyPreset>(basePath, payload)
  return data
}

export async function updatePreset(id: number, payload: UserConcurrencyPresetPayload): Promise<UserConcurrencyPreset> {
  const { data } = await apiClient.put<UserConcurrencyPreset>(`${basePath}/${id}`, payload)
  return data
}

export async function deletePreset(id: number): Promise<void> {
  await apiClient.delete(`${basePath}/${id}`)
}

export async function applyPreset(id: number): Promise<UserConcurrencyPresetRun> {
  const { data } = await apiClient.post<UserConcurrencyPresetRun>(`${basePath}/${id}/apply`)
  return data
}

export async function listPresetRuns(id: number, limit = 20): Promise<UserConcurrencyPresetRun[]> {
  const { data } = await apiClient.get<UserConcurrencyPresetRun[]>(`${basePath}/${id}/runs`, {
    params: { limit }
  })
  return data
}

export const userConcurrencyPresetsAPI = {
  listPresets,
  createPreset,
  updatePreset,
  deletePreset,
  applyPreset,
  listPresetRuns
}

export default userConcurrencyPresetsAPI
