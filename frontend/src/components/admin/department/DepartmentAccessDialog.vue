<template>
  <BaseDialog :show="show" :title="`${t('admin.departments.manageAccess')} · ${department?.name ?? ''}`" width="wide" :show-close-button="!saving" :close-on-escape="!saving" @close="emit('close')">
    <div class="space-y-4">
      <form class="flex gap-2" @submit.prevent="search"><input v-model="query" class="input flex-1" :placeholder="t('admin.departments.searchManagers')" :disabled="saving" /><button class="btn btn-secondary" :disabled="searching || saving">{{ t('admin.departments.search') }}</button></form>
      <label class="block space-y-1"><span>{{ t('admin.departments.selectManager') }}</span><Select v-model="selectedUserID" :options="userOptions" :disabled="saving || searching" :searchable="false" /></label>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <div v-if="access && !loading" class="space-y-3">
        <label class="flex items-center gap-2"><input v-model="grantCurrent" type="checkbox" :disabled="saving || (!alreadyGranted && department?.status !== 'active')" />{{ t('admin.departments.allowDepartment') }}</label>
        <label class="flex items-center gap-2"><input v-model="report" type="checkbox" :disabled="saving" />{{ t('admin.departments.reportPermission') }}</label>
        <label class="flex items-center gap-2"><input v-model="resetQuota" type="checkbox" :disabled="saving" />{{ t('admin.departments.resetPermission') }}</label>
        <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.departments.resetHint') }}</p>
        <label v-if="hasGlobalSubscriptions && resetQuota" class="flex items-start gap-2 rounded-lg bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-900/20 dark:text-amber-200"><input v-model="replaceGlobal" type="checkbox" :disabled="saving" class="mt-0.5" />{{ t('admin.departments.replaceGlobal') }}</label>
        <p v-if="otherPermissionLabels.length" class="text-sm text-amber-700 dark:text-amber-300">{{ t('admin.departments.otherPermissions', { permissions: otherPermissionLabels.join(', ') }) }}</p>
        <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.departments.accessHint') }}</p>
      </div>
      <p v-if="loading" class="text-sm text-gray-500">{{ t('common.loading') }}</p>
    </div>
    <template #footer><button class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('admin.departments.cancel') }}</button><button class="btn btn-primary" :disabled="!canSave" data-testid="save-department-access" @click="save">{{ t('admin.departments.save') }}</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { departmentErrorStatus } from '@/utils/departmentErrors'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import { departmentsAPI, type Department, type DepartmentAccess } from '@/api/admin/departments'
import usersAPI from '@/api/admin/users'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ show: boolean; department: Department | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const app = useAppStore()
const query = ref('')
const candidates = ref<{ id: number; email: string }[]>([])
const selectedUserID = ref('')
const access = ref<DepartmentAccess | null>(null)
const grantCurrent = ref(true)
const report = ref(true)
const resetQuota = ref(true)
const replaceGlobal = ref(false)
const searching = ref(false)
const loading = ref(false)
const saving = ref(false)
const error = ref('')
let searchSequence = 0
let accessSequence = 0
let searchController: AbortController | null = null
let accessController: AbortController | null = null
const userOptions = computed(() => [{ value: '', label: t('admin.departments.selectManager') }, ...candidates.value.map(user => ({ value: String(user.id), label: user.email }))])
const alreadyGranted = computed(() => !!props.department && !!access.value?.department_ids.includes(props.department.id))
const hasGlobalSubscriptions = computed(() => access.value?.permissions.includes('admin.subscriptions') ?? false)
const otherPermissionLabels = computed(() => {
  const labels: string[] = []
  if (access.value?.permissions.includes('admin.usage')) labels.push(t('nav.usage'))
  if (access.value?.permissions.includes('admin.token_analysis')) labels.push(t('nav.tokenAnalysis'))
  if (hasGlobalSubscriptions.value && !(resetQuota.value && replaceGlobal.value)) labels.push(t('nav.subscriptions'))
  return labels
})
const canSave = computed(() => !!access.value && !!props.department && access.value.user_id === Number(selectedUserID.value) && !loading.value && !saving.value && (!resetQuota.value || !hasGlobalSubscriptions.value || replaceGlobal.value))

async function search() {
  searchController?.abort()
  searchController = new AbortController()
  const signal = searchController.signal
  const request = ++searchSequence
  searching.value = true
  error.value = ''
  try {
    const result = await usersAPI.list(1, 30, { role: 'sub_admin', search: query.value.trim(), include_subscriptions: false }, { signal })
    if (request !== searchSequence || signal.aborted) return
    const combined = new Map((props.department?.managers ?? []).map(user => [user.id, user]))
    for (const user of result.items) combined.set(user.id, { id: user.id, email: user.email })
    candidates.value = [...combined.values()]
  } catch {
    if (request === searchSequence && !signal.aborted) error.value = t('admin.departments.failed')
  } finally { if (request === searchSequence) searching.value = false }
}
async function loadAccess() {
  accessController?.abort()
  const request = ++accessSequence
  access.value = null
  replaceGlobal.value = false
  error.value = ''
  if (!props.show || !selectedUserID.value) { loading.value = false; return }
  accessController = new AbortController()
  const signal = accessController.signal
  loading.value = true
  try {
    const result = await departmentsAPI.getAccess(Number(selectedUserID.value), signal)
    if (request !== accessSequence || signal.aborted) return
    access.value = result
    const hasDepartmentPermission = result.permissions.includes('admin.organization_usage') || result.permissions.includes('admin.department_subscriptions')
    report.value = hasDepartmentPermission ? result.permissions.includes('admin.organization_usage') : true
    resetQuota.value = hasDepartmentPermission ? result.permissions.includes('admin.department_subscriptions') : true
    grantCurrent.value = alreadyGranted.value || props.department?.status === 'active'
  } catch {
    if (request === accessSequence && !signal.aborted) error.value = t('admin.departments.failed')
  } finally { if (request === accessSequence) loading.value = false }
}
async function save() {
  if (!canSave.value || !access.value || !props.department) return
  saving.value = true
  error.value = ''
  const selected = access.value
  const departmentIDs = new Set(selected.department_ids)
  if (grantCurrent.value) departmentIDs.add(props.department.id)
  else departmentIDs.delete(props.department.id)
  try {
    access.value = await departmentsAPI.setAccess(selected.user_id, { department_ids: [...departmentIDs].sort((a, b) => a - b), report: report.value, reset_quota: resetQuota.value, replace_global_subscriptions: replaceGlobal.value, expected_version: selected.version })
    app.showSuccess(t('admin.departments.saved'))
    emit('saved')
    emit('close')
  } catch (cause) {
    access.value = null
    await loadAccess()
    error.value = t(departmentErrorStatus(cause) === 409 ? 'admin.departments.conflict' : 'admin.departments.failed')
  } finally { saving.value = false }
}
watch(selectedUserID, loadAccess)
watch(() => [props.show, props.department?.id], () => {
  ++accessSequence
  ++searchSequence
  accessController?.abort()
  searchController?.abort()
  access.value = null
  selectedUserID.value = ''
  query.value = ''
  candidates.value = props.department?.managers ?? []
  if (props.show) void search()
}, { immediate: true })
onUnmounted(() => { ++accessSequence; ++searchSequence; accessController?.abort(); searchController?.abort() })
</script>
