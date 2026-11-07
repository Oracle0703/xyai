<template>
  <BaseDialog :show="show" :title="t('admin.groupOrganizationScope.batchTitle')" width="wide" @close="close">
    <div class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-400">{{ t('admin.groupOrganizationScope.batchDescription') }}</p>
      <div class="flex flex-wrap items-center gap-3">
        <input
          v-model="search"
          type="text"
          class="input w-full sm:w-64"
          :placeholder="t('admin.groupOrganizationScope.searchGroups')"
          data-testid="group-organization-batch-search"
        />
        <button type="button" class="btn btn-secondary" :disabled="loading || saving" @click="selectVisible(true)">
          {{ t('admin.groupOrganizationScope.selectVisible') }}
        </button>
        <button type="button" class="btn btn-secondary" :disabled="loading || saving" @click="selectVisible(false)">
          {{ t('admin.groupOrganizationScope.clearVisible') }}
        </button>
        <span class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.groupOrganizationScope.selectedCount', { count: selected.size }) }}
        </span>
      </div>
      <p v-if="overLimit" class="text-sm text-red-600 dark:text-red-400" data-testid="group-organization-batch-over-limit">
        {{ t('admin.groupOrganizationScope.tooMany', { max: GROUP_ORGANIZATION_SCOPE_BATCH_LIMIT }) }}
      </p>
      <div class="max-h-80 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-600">
        <div v-if="loading" class="p-4 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
        <div v-else-if="visibleGroups.length === 0" class="p-4 text-center text-sm text-gray-500">
          {{ t('admin.groupOrganizationScope.noGroups') }}
        </div>
        <template v-else>
          <label
            v-for="group in visibleGroups"
            :key="group.id"
            class="flex cursor-pointer items-center gap-3 border-b border-gray-100 px-4 py-2 text-sm last:border-b-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700"
          >
            <input
              type="checkbox"
              :checked="selected.has(group.id)"
              :disabled="saving"
              :data-testid="`group-organization-batch-row-${group.id}`"
              @change="toggle(group.id, ($event.target as HTMLInputElement).checked)"
            />
            <span class="font-mono text-xs text-gray-400">#{{ group.id }}</span>
            <span class="font-medium text-gray-900 dark:text-white">{{ group.name }}</span>
            <span v-if="group.status !== 'active'" class="text-xs text-gray-400">{{ t('admin.groupOrganizationScope.inactive') }}</span>
            <span class="ml-auto text-xs text-gray-500 dark:text-gray-400">{{ scopeLabel(group.id) }}</span>
          </label>
        </template>
      </div>
      <GroupOrganizationScopeField v-model="organizations" :disabled="saving" />
      <p v-if="organizations.length === 0" class="text-sm text-amber-600 dark:text-amber-400">
        {{ t('admin.groupOrganizationScope.clearWarning') }}
      </p>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="close">{{ t('common.cancel') }}</button>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="saving || loading || selected.size === 0 || overLimit"
          data-testid="group-organization-batch-save"
          @click="save"
        >
          {{ t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupOrganizationScopeField from './GroupOrganizationScopeField.vue'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatOrganizationUsageOrganization } from '@/utils/organizations'
import {
  GROUP_ORGANIZATION_SCOPE_BATCH_LIMIT,
  setGroupOrganizationScopes,
  type GroupOrganizationKey
} from '@/api/admin/groupOrganizationScopes'
import type { GroupOrganizationScopeStatus } from '@/composables/useGroupOrganizationScopes'
import type { AdminGroup } from '@/types'

// scopesStatus tells whether the labels are known; the replacement itself does
// not depend on them (K24).
const props = withDefaults(
  defineProps<{ show: boolean; scopes: Map<number, GroupOrganizationKey[]>; scopesStatus?: GroupOrganizationScopeStatus }>(),
  { scopesStatus: 'ready' }
)
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const appStore = useAppStore()

const groups = ref<AdminGroup[]>([])
const loading = ref(false)
const saving = ref(false)
const search = ref('')
const selected = ref(new Set<number>())
const organizations = ref<GroupOrganizationKey[]>([])

const visibleGroups = computed(() => {
  const q = search.value.trim().toLowerCase()
  return groups.value.filter(group => !q || group.name.toLowerCase().includes(q) || String(group.id) === q)
})

// The API accepts at most this many groups per request and rejects the whole
// batch above it; splitting would give up its all-or-nothing semantics (K26).
const overLimit = computed(() => selected.value.size > GROUP_ORGANIZATION_SCOPE_BATCH_LIMIT)

function scopeLabel(groupId: number) {
  if (props.scopesStatus === 'error') return t('admin.groupOrganizationScope.unknown')
  if (props.scopesStatus === 'loading' || props.scopesStatus === 'idle') return t('admin.groupOrganizationScope.loading')
  const keys = props.scopes.get(groupId) ?? []
  return keys.length
    ? keys.map(key => formatOrganizationUsageOrganization(key)).join('、')
    : t('admin.groupOrganizationScope.unscoped')
}

function toggle(id: number, checked: boolean) {
  const next = new Set(selected.value)
  if (checked) next.add(id)
  else next.delete(id)
  selected.value = next
}

function selectVisible(checked: boolean) {
  const next = new Set(selected.value)
  for (const group of visibleGroups.value) {
    if (checked) next.add(group.id)
    else next.delete(group.id)
  }
  selected.value = next
}

// Scopes only affect subscription groups, so only those are offered. A reopen
// supersedes an older load; its late response or error is ignored.
let loadSequence = 0
async function load() {
  const request = ++loadSequence
  loading.value = true
  try {
    const all = await adminAPI.groups.getAllIncludingInactive()
    if (request !== loadSequence) return
    groups.value = all.filter(group => group.subscription_type === 'subscription')
  } catch (error) {
    if (request !== loadSequence) return
    appStore.showError(extractApiErrorMessage(error, t('admin.groupOrganizationScope.loadFailed')))
  } finally {
    if (request === loadSequence) loading.value = false
  }
}

watch(
  () => props.show,
  show => {
    if (!show) return
    search.value = ''
    selected.value = new Set()
    organizations.value = []
    void load()
  },
  { immediate: true }
)

async function save() {
  if (selected.value.size === 0 || overLimit.value) return
  saving.value = true
  try {
    const updated = await setGroupOrganizationScopes([...selected.value], organizations.value)
    appStore.showSuccess(t('admin.groupOrganizationScope.saved', { count: updated }))
    emit('saved')
    emit('close')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.groupOrganizationScope.saveFailed')))
  } finally {
    saving.value = false
  }
}

function close() {
  if (!saving.value) emit('close')
}
</script>
