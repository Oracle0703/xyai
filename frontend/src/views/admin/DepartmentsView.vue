<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-end gap-3">
          <label class="w-44 space-y-1"><span class="text-sm">{{ t('admin.departments.organization') }}</span><Select v-model="organization" :options="organizationOptions" :searchable="false" /></label>
          <label class="w-36 space-y-1"><span class="text-sm">{{ t('admin.departments.status') }}</span><Select v-model="status" :options="statusOptions" :searchable="false" /></label>
          <div class="ml-auto flex gap-2">
            <button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('admin.departments.refresh') }}</button>
            <button class="btn btn-primary" data-testid="create-department" @click="openEditor(null)">{{ t('admin.departments.create') }}</button>
          </div>
        </div>
        <p class="mt-3 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.departments.lifecycleHint') }}</p>
        <p v-if="error" role="alert" class="mt-2 text-sm text-red-600">{{ error }}</p>
      </template>
      <template #table>
        <DataTable :columns="columns" :data="departments" :loading="loading">
          <template #cell-status="{ row }"><span :class="['badge', row.status === 'active' ? 'badge-success' : 'badge-gray']">{{ t(`admin.departments.${row.status}`) }}</span></template>
          <template #cell-managers="{ row }">{{ managerEmails(row) }}</template>
          <template #cell-actions="{ row }">
            <div class="flex flex-wrap gap-2">
              <button class="text-primary-600 hover:underline" :disabled="saving" @click="openEditor(row)">{{ t('admin.departments.edit') }}</button>
              <button class="text-primary-600 hover:underline" :disabled="saving" @click="membersTarget = row">{{ t('admin.departments.manageMembers') }}</button>
              <button class="text-primary-600 hover:underline" :disabled="saving" @click="accessTarget = row">{{ t('admin.departments.manageAccess') }}</button>
              <button class="text-primary-600 hover:underline" :disabled="saving" @click="toggleStatus(row)">{{ t(row.status === 'active' ? 'admin.departments.deactivate' : 'admin.departments.reactivate') }}</button>
              <RouterLink class="text-primary-600 hover:underline" :to="{ path: '/admin/organization-usage', query: { organization: row.organization_key, department_id: String(row.id) } }">{{ t('admin.departments.viewUsage') }}</RouterLink>
            </div>
          </template>
        </DataTable>
      </template>
      <template #pagination><Pagination v-if="total > 0" :show-page-size-selector="false" :page="page" :page-size="pageSize" :total="total" @update:page="changePage" @update:page-size="changePageSize" /></template>
    </TablePageLayout>

    <BaseDialog :show="editorOpen" :title="t(editing ? 'admin.departments.edit' : 'admin.departments.create')" :show-close-button="!saving" :close-on-escape="!saving" @close="editorOpen = false">
      <form id="department-editor" class="space-y-4" @submit.prevent="save">
        <p class="text-sm">{{ t('admin.departments.organization') }}：{{ organizationLabel(form.organization_key) }}</p>
        <label class="block space-y-1"><span>{{ t('admin.departments.name') }}</span><input v-model="form.name" data-testid="department-name" class="input" :placeholder="t('admin.departments.namePlaceholder')" maxlength="100" required :disabled="saving" /></label>
        <label class="block space-y-1"><span>{{ t('admin.departments.sortOrder') }}</span><input v-model.number="form.sort_order" type="number" class="input" step="1" min="0" max="2147483647" required :disabled="saving" /></label>
        <p v-if="editorError" role="alert" class="text-sm text-red-600">{{ editorError }}</p>
      </form>
      <template #footer><button class="btn btn-secondary" :disabled="saving" @click="editorOpen = false">{{ t('admin.departments.cancel') }}</button><button form="department-editor" type="submit" class="btn btn-primary" :disabled="saving || !form.name.trim() || !Number.isInteger(form.sort_order)">{{ t('admin.departments.save') }}</button></template>
    </BaseDialog>
    <DepartmentMembersDialog :show="membersTarget !== null" :department="membersTarget" @close="membersTarget = null" @saved="load" />
    <DepartmentAccessDialog :show="accessTarget !== null" :department="accessTarget" @close="accessTarget = null" @saved="load" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { departmentErrorKey } from '@/utils/departmentErrors'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import type { Column } from '@/components/common/types'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DepartmentMembersDialog from '@/components/admin/department/DepartmentMembersDialog.vue'
import DepartmentAccessDialog from '@/components/admin/department/DepartmentAccessDialog.vue'
import { departmentsAPI, type Department, type DepartmentSaveInput, type OrganizationKey } from '@/api/admin/departments'
import { useAppStore } from '@/stores/app'
import { formatOrganizationUsageOrganization } from '@/utils/organizations'

const { t } = useI18n()
const app = useAppStore()
const organization = ref<OrganizationKey>('xunyou')
const status = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const departments = ref<Department[]>([])
const loading = ref(false)
const error = ref('')
const saving = ref(false)
const editorOpen = ref(false)
const editorError = ref('')
const editing = ref<Department | null>(null)
const membersTarget = ref<Department | null>(null)
const accessTarget = ref<Department | null>(null)
const form = reactive<DepartmentSaveInput>({ organization_key: 'xunyou', name: '', status: 'active', sort_order: 0 })
let controller: AbortController | null = null
let sequence = 0

const organizationLabel = (key: OrganizationKey) => formatOrganizationUsageOrganization(key, t('admin.organizationUsage.organizations.other'))
const organizationOptions = computed(() => (['xunyou', 'wsdashi', 'other'] as const).map(value => ({ value, label: organizationLabel(value) })))
const statusOptions = computed(() => [{ value: '', label: t('admin.departments.allStatuses') }, ...(['active', 'inactive'] as const).map(value => ({ value, label: t(`admin.departments.${value}`) }))])
const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.departments.name') }, { key: 'status', label: t('admin.departments.status') },
  { key: 'member_count', label: t('admin.departments.members') }, { key: 'active_member_count', label: t('admin.departments.activeMembers') },
  { key: 'managers', label: t('admin.departments.managers') }, { key: 'sort_order', label: t('admin.departments.sortOrder') },
  { key: 'actions', label: t('admin.departments.actions') }
])

function managerEmails(department: Department) { return department.managers?.map(manager => manager.email).join(', ') || t('admin.departments.noManagers') }
const errorMessage = (cause: unknown) => t(departmentErrorKey(cause))
async function load() {
  controller?.abort()
  const request = ++sequence
  controller = new AbortController()
  const signal = controller.signal
  loading.value = true
  error.value = ''
  departments.value = []
  try {
    const result = await departmentsAPI.list({ organization: organization.value, status: status.value, page: page.value, page_size: pageSize.value }, signal)
    if (request !== sequence || signal.aborted) return
    departments.value = result.items
    total.value = result.total
  } catch (cause) {
    if (request === sequence && !signal.aborted) { error.value = errorMessage(cause); total.value = 0 }
  } finally { if (request === sequence) loading.value = false }
}
function changePage(value: number) { page.value = value; void load() }
function changePageSize(value: number) { pageSize.value = Math.min(value, 200); page.value = 1; void load() }
function openEditor(department: Department | null) {
  editing.value = department
  Object.assign(form, { organization_key: department?.organization_key ?? organization.value, name: department?.name ?? '', status: department?.status ?? 'active', sort_order: department?.sort_order ?? 0, expected_version: department?.version })
  editorError.value = ''
  editorOpen.value = true
}
async function save() {
  if (saving.value || !form.name.trim() || !Number.isInteger(form.sort_order)) return
  saving.value = true
  try {
    await departmentsAPI.save(editing.value?.id ?? null, { ...form, name: form.name.trim() })
    editorOpen.value = false
    app.showSuccess(t('admin.departments.saved'))
    await load()
  } catch (cause) { editorError.value = errorMessage(cause) } finally { saving.value = false }
}
async function toggleStatus(department: Department) {
  if (saving.value) return
  saving.value = true
  try {
    await departmentsAPI.save(department.id, { organization_key: department.organization_key, name: department.name, status: department.status === 'active' ? 'inactive' : 'active', sort_order: department.sort_order, expected_version: department.version })
    app.showSuccess(t('admin.departments.saved'))
    await load()
  } catch (cause) { app.showError(errorMessage(cause)); await load() } finally { saving.value = false }
}
watch([organization, status], () => { page.value = 1; void load() })
onMounted(load)
onUnmounted(() => { ++sequence; controller?.abort() })
</script>
