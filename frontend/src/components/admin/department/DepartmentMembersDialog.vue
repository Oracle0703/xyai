<template>
  <BaseDialog :show="show" :title="`${t('admin.departments.manageMembers')} · ${department?.name ?? ''}`" width="wide" :show-close-button="!saving" :close-on-escape="!saving" @close="emit('close')">
    <div class="space-y-4">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.departments.memberHint') }}</p>
      <div class="flex flex-wrap gap-2">
        <button class="btn" :class="mode === 'current' ? 'btn-primary' : 'btn-secondary'" :disabled="saving" @click="switchMode('current')">{{ t('admin.departments.currentMembers') }}</button>
        <button class="btn" :class="mode === 'add' ? 'btn-primary' : 'btn-secondary'" :disabled="saving || department?.status !== 'active'" @click="switchMode('add')">{{ t('admin.departments.addMembers') }}</button>
      </div>
      <form class="flex gap-2" @submit.prevent="search"><input v-model="query" class="input flex-1" :placeholder="t('admin.departments.searchMembers')" :disabled="saving" /><button class="btn btn-secondary" :disabled="loading || saving">{{ t('admin.departments.search') }}</button></form>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <DataTable :columns="columns" :data="members" :loading="loading">
        <template #cell-select="{ row }"><input type="checkbox" :aria-label="row.email" :checked="selected.has(row.id)" :disabled="saving || (mode === 'add' && row.department_id === department?.id)" @change="toggle(row)" /></template>
        <template #cell-department_name="{ row }">{{ row.department_name || t('admin.departments.unassigned') }}</template>
        <template #cell-status="{ row }">{{ t(row.status === 'active' ? 'admin.departments.active' : 'admin.departments.inactive') }}</template>
      </DataTable>
      <div class="flex items-center justify-between gap-2"><button class="btn btn-secondary" :disabled="loading || saving" @click="selectPage">{{ t('admin.departments.selectCurrentPage') }}</button><Pagination v-if="total > 0" :show-page-size-selector="false" :page="page" :page-size="pageSize" :total="total" @update:page="changePage" @update:page-size="changePageSize" /></div>
      <div v-if="selected.size" class="rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-800">
        <p class="font-medium">{{ t('admin.departments.selectedCount', { count: selected.size }) }}</p>
        <p class="mt-1">{{ t('admin.departments.memberPreview', { department: mode === 'add' ? department?.name : t('admin.departments.unassigned') }) }}</p>
        <ul class="mt-2 space-y-1"><li v-for="member in selectedMembers" :key="member.id" class="break-all">{{ member.email }} · {{ member.department_name || t('admin.departments.unassigned') }}</li></ul>
      </div>
    </div>
    <template #footer><button class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('admin.departments.cancel') }}</button><button class="btn btn-primary" :disabled="saving || loading || !selected.size" data-testid="apply-department-members" @click="save">{{ t(mode === 'add' ? 'admin.departments.addSelected' : 'admin.departments.removeSelected') }}</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { departmentErrorKey } from '@/utils/departmentErrors'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import type { Column } from '@/components/common/types'
import Pagination from '@/components/common/Pagination.vue'
import { departmentsAPI, type Department, type DepartmentMember } from '@/api/admin/departments'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ show: boolean; department: Department | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const app = useAppStore()
const mode = ref<'current' | 'add'>('current')
const query = ref('')
const appliedQuery = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const members = ref<DepartmentMember[]>([])
const selected = ref(new Map<number, DepartmentMember>())
const selectedMembers = computed(() => [...selected.value.values()])
const loading = ref(false)
const saving = ref(false)
const error = ref('')
let sequence = 0
let controller: AbortController | null = null
const columns = computed<Column[]>(() => [
  { key: 'select', label: '', width: '44px' }, { key: 'email', label: t('admin.departments.email') },
  { key: 'department_name', label: t('admin.departments.currentDepartment') }, { key: 'status', label: t('admin.departments.status') }
])

async function load() {
  if (!props.show || !props.department) return
  controller?.abort()
  controller = new AbortController()
  const signal = controller.signal
  const request = ++sequence
  loading.value = true
  error.value = ''
  members.value = []
  try {
    const result = await departmentsAPI.members({ organization: props.department.organization_key, department_id: mode.value === 'current' ? String(props.department.id) : 'all', q: appliedQuery.value, page: page.value, page_size: pageSize.value }, signal)
    if (request !== sequence || signal.aborted) return
    members.value = result.items
    total.value = result.total
  } catch {
    if (request === sequence && !signal.aborted) { error.value = t('admin.departments.failed'); total.value = 0 }
  } finally { if (request === sequence) loading.value = false }
}
function toggle(member: DepartmentMember) {
  if (saving.value || (mode.value === 'add' && member.department_id === props.department?.id)) return
  if (selected.value.has(member.id)) selected.value.delete(member.id)
  else if (selected.value.size < 200) selected.value.set(member.id, member)
  else app.showError(t('admin.departments.maxSelection'))
}
function selectPage() {
  for (const member of members.value) {
    if (selected.value.size >= 200) break
    if (mode.value === 'add' && member.department_id === props.department?.id) continue
    selected.value.set(member.id, member)
  }
}
function switchMode(value: 'current' | 'add') { mode.value = value; query.value = ''; appliedQuery.value = ''; page.value = 1; selected.value.clear(); void load() }
function search() { appliedQuery.value = query.value.trim(); page.value = 1; selected.value.clear(); void load() }
function changePage(value: number) { page.value = value; void load() }
function changePageSize(value: number) { pageSize.value = Math.min(value, 200); page.value = 1; void load() }
async function save() {
  if (saving.value || loading.value || !props.department || !selected.value.size) return
  saving.value = true
  error.value = ''
  try {
    const result = await departmentsAPI.assign(mode.value === 'add' ? props.department.id : null, selectedMembers.value.map(member => ({ user_id: member.id, expected_department_id: member.department_id, expected_department_version: member.department_version })))
    selected.value.clear()
    app.showSuccess(t('admin.departments.moved', { count: result.updated_count }))
    emit('saved')
    await load()
  } catch (cause) {
    selected.value.clear()
    await load()
    error.value = t(departmentErrorKey(cause))
  } finally { saving.value = false }
}
watch(() => [props.show, props.department?.id], () => {
  ++sequence
  controller?.abort()
  selected.value.clear()
  members.value = []
  mode.value = 'current'
  page.value = 1
  query.value = ''
  appliedQuery.value = ''
  if (props.show) void load()
}, { immediate: true })
onUnmounted(() => { ++sequence; controller?.abort() })
</script>
