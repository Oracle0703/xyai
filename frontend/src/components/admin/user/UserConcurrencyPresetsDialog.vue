<template>
  <BaseDialog :show="show" title="并发方案" width="extra-wide" @close="emit('close')">
    <div class="grid gap-5 lg:grid-cols-[280px_1fr]">
      <aside class="rounded-lg border border-gray-200 bg-gray-50/60 p-3 dark:border-dark-600 dark:bg-dark-800/40">
        <div class="mb-3 flex items-center justify-between gap-2">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">方案列表</h3>
          <button type="button" class="btn btn-secondary px-2 py-1 text-xs" @click="startCreate">
            新建
          </button>
        </div>
        <div v-if="loadingPresets" class="py-8 text-center text-sm text-gray-500">加载中...</div>
        <div v-else-if="presets.length === 0" class="py-8 text-center text-sm text-gray-500">
          暂无方案
        </div>
        <div v-else class="space-y-2">
          <button
            v-for="preset in presets"
            :key="preset.id"
            type="button"
            class="w-full rounded-md border px-3 py-2 text-left transition"
            :class="selectedPreset?.id === preset.id
              ? 'border-primary-400 bg-primary-50 text-primary-700 dark:border-primary-500 dark:bg-primary-900/30 dark:text-primary-300'
              : 'border-gray-200 bg-white text-gray-700 hover:border-gray-300 dark:border-dark-600 dark:bg-dark-700 dark:text-dark-200'"
            @click="selectPreset(preset)"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="truncate text-sm font-medium">{{ preset.name }}</span>
              <span class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-xs dark:bg-dark-600">
                {{ preset.target_concurrency }}
              </span>
            </div>
            <div class="mt-1 flex items-center justify-between text-xs text-gray-500 dark:text-dark-300">
              <span>{{ preset.user_ids.length }} 个用户</span>
              <span v-if="preset.schedule_enabled">{{ preset.schedule_time }}</span>
              <span v-else>手动</span>
            </div>
          </button>
        </div>
      </aside>

      <section class="space-y-5">
        <div class="grid gap-4 md:grid-cols-2">
          <label class="block">
            <span class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">方案名称</span>
            <input v-model.trim="form.name" class="input" placeholder="例如 白天高并发" />
          </label>
          <label class="block">
            <span class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">目标并发数</span>
            <input v-model.number="form.target_concurrency" class="input" type="number" min="1" />
          </label>
          <label class="block md:col-span-2">
            <span class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">说明</span>
            <input v-model.trim="form.description" class="input" placeholder="可选" />
          </label>
        </div>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">目标用户</h3>
              <p class="text-xs text-gray-500 dark:text-dark-300">管理员账号会被后端拒绝，请只选择普通用户。</p>
            </div>
            <div class="flex w-full gap-2 sm:w-auto">
              <input
                v-model.trim="userSearch"
                class="input sm:w-64"
                placeholder="搜索邮箱或用户名"
                @keyup.enter="searchUsers"
              />
              <button type="button" class="btn btn-secondary" :disabled="searchingUsers" @click="searchUsers">
                搜索
              </button>
            </div>
          </div>

          <div v-if="selectedUsers.length > 0" class="mb-3 flex flex-wrap gap-2">
            <span
              v-for="user in selectedUsers"
              :key="user.id"
              class="inline-flex items-center gap-1 rounded-md bg-primary-50 px-2 py-1 text-xs text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
            >
              {{ user.email }}
              <button type="button" class="text-primary-500 hover:text-primary-700" @click="removeUser(user.id)">x</button>
            </span>
          </div>
          <div v-else class="mb-3 text-sm text-gray-500">尚未选择用户</div>

          <div v-if="searchResults.length > 0" class="max-h-56 overflow-y-auto rounded-md border border-gray-100 dark:border-dark-700">
            <button
              v-for="user in searchResults"
              :key="user.id"
              type="button"
              class="flex w-full items-center justify-between gap-3 px-3 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-700"
              :disabled="isSelected(user.id)"
              @click="addUser(user)"
            >
              <span>
                <span class="font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                <span class="ml-2 text-xs text-gray-500">{{ user.username || '-' }}</span>
              </span>
              <span class="text-xs text-gray-500">并发 {{ user.concurrency }}</span>
            </button>
          </div>
        </div>

        <div class="grid gap-4 rounded-lg border border-gray-200 p-4 dark:border-dark-600 md:grid-cols-2">
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="form.schedule_enabled" type="checkbox" class="h-4 w-4" />
            每天定时应用
          </label>
          <label class="block">
            <span class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">执行时间</span>
            <input
              v-model.trim="form.schedule_time"
              class="input"
              placeholder="09:00"
              :disabled="!form.schedule_enabled"
            />
          </label>
        </div>

        <div v-if="runs.length > 0" class="rounded-lg border border-gray-200 dark:border-dark-600">
          <div class="border-b border-gray-100 px-4 py-2 text-sm font-semibold dark:border-dark-700">最近执行</div>
          <div class="divide-y divide-gray-100 dark:divide-dark-700">
            <div v-for="run in runs.slice(0, 5)" :key="run.id" class="flex items-center justify-between gap-3 px-4 py-2 text-sm">
              <span class="text-gray-700 dark:text-gray-300">
                {{ run.trigger === 'manual' ? '手动' : '定时' }} · {{ run.affected_count }} 人
              </span>
              <span :class="run.status === 'success' ? 'text-emerald-600' : 'text-red-600'">
                {{ run.status === 'success' ? '成功' : run.error_message || '失败' }}
              </span>
            </div>
          </div>
        </div>
      </section>
    </div>

    <template #footer>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <button
          v-if="selectedPreset"
          type="button"
          class="btn btn-danger"
          :disabled="saving"
          @click="deleteCurrent"
        >
          删除
        </button>
        <span v-else></span>
        <div class="flex flex-wrap gap-2">
          <button
            v-if="selectedPreset"
            type="button"
            class="btn btn-secondary"
            :disabled="saving"
            @click="applyCurrent"
          >
            应用方案
          </button>
          <button type="button" class="btn btn-secondary" @click="emit('close')">关闭</button>
          <button type="button" class="btn btn-primary" :disabled="saving" @click="save">
            {{ selectedPreset ? '保存' : '创建' }}
          </button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { adminAPI } from '@/api/admin'
import type { AdminUser } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import {
  applyPreset,
  createPreset,
  deletePreset,
  listPresetRuns,
  listPresets,
  updatePreset,
  type UserConcurrencyPreset,
  type UserConcurrencyPresetRun
} from '@/api/admin/userConcurrencyPresets'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{
  close: []
  applied: []
}>()

const appStore = useAppStore()

const presets = ref<UserConcurrencyPreset[]>([])
const runs = ref<UserConcurrencyPresetRun[]>([])
const selectedPreset = ref<UserConcurrencyPreset | null>(null)
const selectedUsers = ref<AdminUser[]>([])
const searchResults = ref<AdminUser[]>([])
const userSearch = ref('')
const loadingPresets = ref(false)
const searchingUsers = ref(false)
const saving = ref(false)

const form = reactive({
  name: '',
  description: '',
  target_concurrency: 1,
  schedule_enabled: false,
  schedule_time: '09:00'
})

const selectedUserIDs = computed(() => selectedUsers.value.map((u) => u.id))

watch(
  () => props.show,
  (show) => {
    if (show) {
      loadPresets()
      if (selectedUsers.value.length === 0) {
        searchUsers()
      }
    }
  },
  { immediate: true }
)

async function loadPresets() {
  loadingPresets.value = true
  try {
    const loaded = await listPresets()
    presets.value = Array.isArray(loaded) ? loaded : []
    if (presets.value.length > 0 && !selectedPreset.value) {
      selectPreset(presets.value[0])
    }
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || '加载并发方案失败')
  } finally {
    loadingPresets.value = false
  }
}

function startCreate() {
  selectedPreset.value = null
  selectedUsers.value = []
  runs.value = []
  Object.assign(form, {
    name: '',
    description: '',
    target_concurrency: 1,
    schedule_enabled: false,
    schedule_time: '09:00'
  })
}

async function selectPreset(preset: UserConcurrencyPreset) {
  selectedPreset.value = preset
  Object.assign(form, {
    name: preset.name,
    description: preset.description || '',
    target_concurrency: preset.target_concurrency,
    schedule_enabled: preset.schedule_enabled,
    schedule_time: preset.schedule_time || '09:00'
  })
  selectedUsers.value = preset.user_ids.map((id) => ({
    id,
    email: `#${id}`,
    username: '',
    role: 'user',
    balance: 0,
    concurrency: preset.target_concurrency,
    status: 'active',
    allowed_groups: [],
    balance_notify_enabled: false,
    balance_notify_threshold: null,
    balance_notify_extra_emails: [],
    created_at: '',
    updated_at: '',
    notes: '',
    current_concurrency: 0
  } as AdminUser))
  try {
    runs.value = await listPresetRuns(preset.id, 20)
  } catch {
    runs.value = []
  }
}

async function searchUsers() {
  searchingUsers.value = true
  try {
    const result = await adminAPI.users.list(1, 20, {
      search: userSearch.value || undefined,
      status: 'active',
      role: 'user',
      include_subscriptions: false
    })
    searchResults.value = result.items
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || '搜索用户失败')
  } finally {
    searchingUsers.value = false
  }
}

function isSelected(id: number) {
  return selectedUsers.value.some((user) => user.id === id)
}

function addUser(user: AdminUser) {
  if (!isSelected(user.id)) {
    selectedUsers.value.push(user)
  }
}

function removeUser(id: number) {
  selectedUsers.value = selectedUsers.value.filter((user) => user.id !== id)
}

function validateForm() {
  if (!form.name.trim()) {
    appStore.showError('请输入方案名称')
    return false
  }
  if (!Number.isFinite(form.target_concurrency) || form.target_concurrency < 1) {
    appStore.showError('目标并发数必须大于等于 1')
    return false
  }
  if (selectedUserIDs.value.length === 0) {
    appStore.showError('请选择目标用户')
    return false
  }
  if (form.schedule_enabled && !/^([01]\d|2[0-3]):[0-5]\d$/.test(form.schedule_time)) {
    appStore.showError('执行时间必须是 HH:mm 格式')
    return false
  }
  return true
}

async function save() {
  if (!validateForm()) return
  saving.value = true
  const payload = {
    name: form.name.trim(),
    description: form.description.trim(),
    target_concurrency: Math.trunc(form.target_concurrency),
    user_ids: selectedUserIDs.value,
    schedule_enabled: form.schedule_enabled,
    schedule_time: form.schedule_enabled ? form.schedule_time : ''
  }
  try {
    const saved = selectedPreset.value
      ? await updatePreset(selectedPreset.value.id, payload)
      : await createPreset(payload)
    appStore.showSuccess(selectedPreset.value ? '并发方案已保存' : '并发方案已创建')
    await loadPresets()
    const found = presets.value.find((preset) => preset.id === saved.id)
    if (found) {
      await selectPreset(found)
    }
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || '保存并发方案失败')
  } finally {
    saving.value = false
  }
}

async function applyCurrent() {
  if (!selectedPreset.value) return
  if (!window.confirm(`确认把 ${selectedPreset.value.user_ids.length} 个用户的并发设置为 ${selectedPreset.value.target_concurrency}？`)) {
    return
  }
  saving.value = true
  try {
    const run = await applyPreset(selectedPreset.value.id)
    appStore.showSuccess(`已应用，影响 ${run.affected_count} 个用户`)
    runs.value = await listPresetRuns(selectedPreset.value.id, 20)
    emit('applied')
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || '应用并发方案失败')
  } finally {
    saving.value = false
  }
}

async function deleteCurrent() {
  if (!selectedPreset.value) return
  if (!window.confirm(`确认删除方案「${selectedPreset.value.name}」？`)) return
  saving.value = true
  try {
    await deletePreset(selectedPreset.value.id)
    appStore.showSuccess('并发方案已删除')
    selectedPreset.value = null
    startCreate()
    await loadPresets()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || '删除并发方案失败')
  } finally {
    saving.value = false
  }
}
</script>
