<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-end gap-3">
          <label class="w-52 space-y-1">
            <span class="text-sm">{{ t('admin.departments.department') }}</span>
            <Select v-model="departmentID" data-testid="managed-department" :options="departmentOptions" :searchable="false" :disabled="scopeLoading || saving || !scopeReady" @change="changeFilter" />
          </label>
          <label class="w-36 space-y-1">
            <span class="text-sm">{{ t('admin.departments.status') }}</span>
            <Select v-model="status" data-testid="managed-status" :options="statusOptions" :searchable="false" :disabled="scopeLoading || saving || !scopeReady" @change="changeFilter" />
          </label>
          <form class="flex min-w-0 flex-1 gap-2" @submit.prevent="searchMembers">
            <input v-model="query" data-testid="member-query" class="input min-w-0 flex-1" :aria-label="t('admin.departments.memberManagement.searchPlaceholder')" :placeholder="t('admin.departments.memberManagement.searchPlaceholder')" :disabled="saving || !scopeReady" />
            <button class="btn btn-secondary" :disabled="saving || loading || !scopeReady">{{ t('admin.departments.search') }}</button>
          </form>
          <button class="btn btn-secondary" data-testid="refresh-managed-scope" :disabled="scopeLoading || saving" @click="loadScope()">{{ t('admin.departments.refresh') }}</button>
          <button class="btn btn-primary" data-testid="open-add-members" :disabled="!canAdd || saving || loading" @click="openCandidates">{{ t('admin.departments.addMembers') }}</button>
        </div>
        <p class="mt-3 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.departments.memberManagement.description') }}</p>
        <p v-if="scopeReady && !canAdd" class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.departments.memberManagement.selectDepartment') }}</p>
        <p v-if="scopeLoading" class="mt-2 text-sm text-gray-500">{{ t('admin.departments.loadingScope') }}</p>
        <p v-if="error" data-testid="managed-error" role="alert" class="mt-2 text-sm text-red-600">{{ error }}</p>
      </template>
      <template #table>
        <DataTable :columns="columns" :data="members" :loading="loading || scopeLoading">
          <template #cell-department_name="{ row }">{{ departmentLabels.get(row.department_id) ?? row.department_name }}</template>
          <template #cell-department_note="{ row }"><span v-if="row.manageable" class="block max-w-xs whitespace-pre-wrap break-words">{{ row.department_note || '—' }}</span><span v-else>—</span></template>
          <template #cell-status="{ row }">
            <span :class="['badge', row.status === 'active' ? 'badge-success' : 'badge-gray']">{{ t(row.status === 'active' ? 'admin.departments.active' : row.disabled_source === 'department_manager' ? 'admin.departments.managerDisabled' : 'admin.departments.otherDisabled') }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div v-if="row.manageable" class="flex flex-wrap gap-2">
              <button class="text-primary-600 hover:underline" :data-testid="`edit-member-${row.id}`" :disabled="saving || loading" @click="openEditor(row)">{{ t('admin.departments.memberManagement.editProfile') }}</button>
              <button v-if="row.status === 'active'" class="text-amber-700 hover:underline dark:text-amber-400" :data-testid="`disable-member-${row.id}`" :disabled="saving || loading" @click="openAction(row, 'disable')">{{ t('admin.departments.memberManagement.disable') }}</button>
              <button v-else-if="row.can_enable" class="text-primary-600 hover:underline" :data-testid="`enable-member-${row.id}`" :disabled="saving || loading" @click="openAction(row, 'enable')">{{ t('admin.departments.memberManagement.enable') }}</button>
              <button class="text-red-600 hover:underline" :data-testid="`remove-member-${row.id}`" :disabled="saving || loading" @click="openAction(row, 'remove')">{{ t('admin.departments.memberManagement.remove') }}</button>
            </div>
            <span v-else class="text-gray-500">{{ t('admin.departments.memberManagement.readonly') }}</span>
          </template>
          <template #empty><p data-testid="managed-empty" class="p-8 text-center text-gray-500">{{ t(noScope ? 'admin.departments.memberManagement.noScope' : 'admin.departments.memberManagement.noMatches') }}</p></template>
        </DataTable>
      </template>
      <template #pagination><Pagination v-if="total > 0" :page="page" :page-size="pageSize" :total="total" :show-page-size-selector="false" :class="saving ? 'pointer-events-none opacity-50' : ''" @update:page="changePage" /></template>
    </TablePageLayout>

    <BaseDialog :show="editing !== null" :title="t('admin.departments.memberManagement.editProfile')" :show-close-button="!saving" :close-on-escape="!saving" @close="closeEditor">
      <form id="managed-member-editor" class="space-y-4" @submit.prevent="saveProfile">
        <p class="break-all text-sm">{{ editing?.email }}</p>
        <label class="block space-y-1"><span>{{ t('admin.departments.memberManagement.username') }}</span><input v-model="form.username" data-testid="managed-username" class="input" :disabled="saving" /></label>
        <label class="block space-y-1"><span>{{ t('admin.departments.departmentNote') }}</span><textarea v-model="form.department_note" data-testid="managed-note" class="input" rows="4" :disabled="saving || auth.isAdmin"></textarea></label>
        <p class="text-sm text-gray-500">{{ t(auth.isAdmin ? 'admin.departments.memberManagement.adminNoteReadonly' : 'admin.departments.memberManagement.noteHint') }}</p>
        <p v-if="profileError" data-testid="profile-error" role="alert" class="text-sm text-red-600">{{ profileError }}</p>
        <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      </form>
      <template #footer><button class="btn btn-secondary" :disabled="saving" @click="closeEditor">{{ t('admin.departments.cancel') }}</button><button form="managed-member-editor" type="submit" data-testid="save-managed-profile" class="btn btn-primary" :disabled="saving || !profileChanged || !!profileError">{{ t('admin.departments.save') }}</button></template>
    </BaseDialog>

    <BaseDialog :show="pendingAction !== null" :title="actionTitle" :show-close-button="!saving" :close-on-escape="!saving" @close="closeAction">
      <p class="break-all font-medium">{{ pendingAction?.member.email }}</p>
      <p class="mt-3 text-sm text-gray-600 dark:text-dark-300">{{ actionHint }}</p>
      <p v-if="error" role="alert" class="mt-2 text-sm text-red-600">{{ error }}</p>
      <template #footer><button class="btn btn-secondary" :disabled="saving" @click="closeAction">{{ t('admin.departments.cancel') }}</button><button class="btn btn-primary" data-testid="confirm-managed-action" :disabled="saving" @click="confirmAction">{{ t('admin.departments.memberManagement.confirm') }}</button></template>
    </BaseDialog>

    <BaseDialog :show="candidateOpen" :title="`${t('admin.departments.addMembers')} · ${selectedDepartment ? departmentLabels.get(selectedDepartment.id) : ''}`" width="wide" :show-close-button="!saving" :close-on-escape="!saving" @close="closeCandidates">
      <div class="space-y-4">
        <p class="text-sm text-gray-500">{{ t('admin.departments.memberManagement.candidateHint') }}</p>
        <form class="flex gap-2" data-testid="candidate-search" @submit.prevent="searchCandidates">
          <input v-model="candidateQuery" data-testid="candidate-query" class="input min-w-0 flex-1" :aria-label="t('admin.departments.memberManagement.searchPlaceholder')" :placeholder="t('admin.departments.memberManagement.searchPlaceholder')" :disabled="saving" />
          <button class="btn btn-secondary" :disabled="saving || candidateLoading || Array.from(candidateQuery.trim()).length < 2">{{ t('admin.departments.search') }}</button>
        </form>
        <p v-if="candidateError" data-testid="candidate-error" role="alert" class="text-sm text-red-600">{{ candidateError }}</p>
        <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
        <DataTable :columns="candidateColumns" :data="candidates" :loading="candidateLoading">
          <template #cell-select="{ row }"><input type="checkbox" :data-testid="`candidate-${row.id}`" :aria-label="row.email" :checked="selectedCandidates.has(row.id)" :disabled="saving" @change="toggleCandidate(row)" /></template>
          <template #cell-status="{ row }">{{ t(row.status === 'active' ? 'admin.departments.active' : 'admin.departments.inactive') }}</template>
          <template #empty><p class="p-6 text-center text-gray-500">{{ t('admin.departments.memberManagement.noCandidates') }}</p></template>
        </DataTable>
        <p class="text-sm">{{ t('admin.departments.memberManagement.selectedCandidates', { count: selectedCandidates.size }) }}</p>
      </div>
      <template #footer><button class="btn btn-secondary" :disabled="saving" @click="closeCandidates">{{ t('admin.departments.cancel') }}</button><button class="btn btn-primary" data-testid="add-selected-members" :disabled="saving || candidateLoading || !selectedCandidates.size || !canAdd" @click="addSelected">{{ t('admin.departments.addSelected') }}</button></template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import type { Column } from '@/components/common/types'
import { departmentsAPI, type Department, type DepartmentMember, type DepartmentMemberChange, type ManagedDepartmentMember, type ManagedMemberUpdateInput } from '@/api/admin/departments'
import { departmentErrorCode, departmentErrorKey, departmentErrorStatus } from '@/utils/departmentErrors'
import { formatOrganizationUsageOrganization } from '@/utils/organizations'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const app = useAppStore()
const auth = useAuthStore()
const departments = ref<Department[]>([])
const departmentID = ref('all')
const status = ref('')
const query = ref('')
const appliedQuery = ref('')
const page = ref(1)
const pageSize = 20
const total = ref(0)
const members = ref<ManagedDepartmentMember[]>([])
const scopeReady = ref(false)
const noScope = ref(false)
const scopeLoading = ref(false)
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const editing = ref<ManagedDepartmentMember | null>(null)
const form = reactive({ username: '', department_note: '' })
const pendingAction = ref<{ kind: 'disable' | 'enable' | 'remove'; member: ManagedDepartmentMember } | null>(null)
const candidateOpen = ref(false)
const candidateQuery = ref('')
const candidates = ref<DepartmentMember[]>([])
const selectedCandidates = ref(new Map<number, DepartmentMember>())
const candidateLoading = ref(false)
const candidateError = ref('')
let scopeController: AbortController | null = null
let listController: AbortController | null = null
let candidateController: AbortController | null = null
let contextVersion = 0

const selectedDepartment = computed(() => departments.value.find(department => String(department.id) === departmentID.value))
const canAdd = computed(() => scopeReady.value && selectedDepartment.value?.status === 'active')
const departmentLabels = computed(() => new Map(departments.value.map(department => [
  department.id,
  `${formatOrganizationUsageOrganization(department.organization_key, t('admin.organizationUsage.organizations.other'))} · ${department.name}`
])))
const departmentOptions = computed(() => [
  { value: 'all', label: t('admin.departments.allDepartments') },
  ...departments.value.map(department => ({ value: String(department.id), label: departmentLabels.value.get(department.id)! }))
])
const statusOptions = computed(() => [
  { value: '', label: t('admin.departments.allStatuses') },
  { value: 'active', label: t('admin.departments.active') },
  { value: 'disabled', label: t('admin.departments.inactive') }
])
const columns = computed<Column[]>(() => [
  { key: 'email', label: t('admin.departments.email') },
  { key: 'username', label: t('admin.departments.memberManagement.username') },
  { key: 'department_name', label: t('admin.departments.department') },
  { key: 'department_note', label: t('admin.departments.departmentNote') },
  { key: 'status', label: t('admin.departments.status') },
  { key: 'actions', label: t('admin.departments.actions') }
])
const candidateColumns = computed<Column[]>(() => [
  { key: 'select', label: '' },
  { key: 'email', label: t('admin.departments.email') },
  { key: 'username', label: t('admin.departments.memberManagement.username') },
  { key: 'status', label: t('admin.departments.status') }
])
const profileError = computed(() => {
  if (new TextEncoder().encode(form.username.trim()).length > 100) return t('admin.departments.memberManagement.usernameLimit')
  if (!auth.isAdmin && Array.from(form.department_note.trim()).length > 1000) return t('admin.departments.memberManagement.noteLimit')
  return ''
})
const profileChanged = computed(() => editing.value && (
  form.username.trim() !== editing.value.username || (!auth.isAdmin && form.department_note.trim() !== editing.value.department_note)
))
const actionTitle = computed(() => t(`admin.departments.memberManagement.${pendingAction.value?.kind === 'remove' ? 'confirmRemove' : pendingAction.value?.kind === 'enable' ? 'confirmEnable' : 'confirmDisable'}`))
const actionHint = computed(() => t(`admin.departments.memberManagement.${pendingAction.value?.kind === 'remove' ? 'removeHint' : pendingAction.value?.kind === 'enable' ? 'enableHint' : 'disableHint'}`))

function isScopeDenied(cause: unknown) {
  const code = departmentErrorCode(cause)
  return code === 'DEPARTMENT_SCOPE_DENIED' || code === 'ADMIN_PERMISSION_DENIED' ||
    (departmentErrorStatus(cause) === 403 && code !== 'DEPARTMENT_MEMBER_ENABLE_DENIED')
}

function clearCandidateResults() {
  candidateController?.abort()
  candidateLoading.value = false
  candidates.value = []
  selectedCandidates.value.clear()
  candidateError.value = ''
}

function clearProtectedState() {
  contextVersion++
  listController?.abort()
  clearCandidateResults()
  members.value = []
  total.value = 0
  loading.value = false
  saving.value = false
  editing.value = null
  pendingAction.value = null
  candidateOpen.value = false
  candidateQuery.value = ''
}

async function loadScope(message = '') {
  const preferredDepartment = departmentID.value
  scopeController?.abort()
  scopeController = new AbortController()
  const signal = scopeController.signal
  clearProtectedState()
  departments.value = []
  scopeReady.value = false
  noScope.value = false
  scopeLoading.value = true
  error.value = message
  page.value = 1
  try {
    const scope = await departmentsAPI.managedScope(signal)
    if (signal.aborted) return
    departments.value = scope.departments
    if (!scope.departments.length) { noScope.value = true; return }
    departmentID.value = scope.departments.length === 1 ? String(scope.departments[0]!.id)
      : scope.departments.some(department => String(department.id) === preferredDepartment) ? preferredDepartment
        : scope.departments.some(department => String(department.id) === scope.default_department_id) ? scope.default_department_id : 'all'
    scopeReady.value = true
    scopeLoading.value = false
    // A denied list after this refresh must not recursively refresh scope again.
    await loadMembers(false)
  } catch (cause) {
    if (signal.aborted) return
    noScope.value = isScopeDenied(cause)
    error.value = t(departmentErrorKey(cause))
  } finally { if (!signal.aborted) scopeLoading.value = false }
}

async function loadMembers(allowScopeRefresh = true) {
  if (!scopeReady.value) return
  listController?.abort()
  listController = new AbortController()
  const signal = listController.signal
  loading.value = true
  members.value = []
  total.value = 0
  try {
    const result = await departmentsAPI.managedMembers({ department_id: departmentID.value, status: status.value, q: appliedQuery.value, page: page.value, page_size: pageSize }, signal)
    if (signal.aborted) return
    members.value = result.items
    total.value = result.total
  } catch (cause) {
    if (signal.aborted) return
    const message = t(departmentErrorKey(cause))
    if (isScopeDenied(cause)) {
      if (allowScopeRefresh) await loadScope(message)
      else {
        clearProtectedState()
        scopeReady.value = false
        departments.value = []
        noScope.value = true
        error.value = message
      }
    } else error.value = message
  } finally { if (!signal.aborted) loading.value = false }
}

function changeFilter() {
  clearProtectedState()
  page.value = 1
  error.value = ''
  void loadMembers()
}
function searchMembers() {
  if (saving.value || !scopeReady.value) return
  appliedQuery.value = query.value.trim()
  changeFilter()
}
function changePage(value: number) {
  if (saving.value || value === page.value) return
  clearProtectedState()
  page.value = value
  error.value = ''
  void loadMembers()
}

function openEditor(member: ManagedDepartmentMember) {
  if (saving.value || !member.manageable) return
  editing.value = { ...member }
  form.username = member.username
  form.department_note = member.department_note
  error.value = ''
}
function closeEditor() { if (!saving.value) editing.value = null }
function closeAction() { if (!saving.value) pendingAction.value = null }
function closeCandidates() {
  if (saving.value) return
  clearCandidateResults()
  candidateOpen.value = false
  candidateQuery.value = ''
}

async function handleWriteError(cause: unknown) {
  const message = t(departmentErrorKey(cause))
  error.value = message
  selectedCandidates.value.clear()
  if (isScopeDenied(cause)) { await loadScope(message); return }
  const code = departmentErrorCode(cause)
  if (code === 'DEPARTMENT_CONFLICT' || code === 'DEPARTMENT_MEMBER_ENABLE_DENIED') {
    editing.value = null
    pendingAction.value = null
    candidateOpen.value = false
    clearCandidateResults()
    await loadMembers()
  }
}

async function updateMember(member: ManagedDepartmentMember, fields: Pick<ManagedMemberUpdateInput, 'username' | 'department_note' | 'status'>) {
  if (saving.value) return
  saving.value = true
  error.value = ''
  const version = contextVersion
  try {
    await departmentsAPI.updateManagedMember(member.id, {
      expected_department_id: member.department_id,
      expected_department_version: member.department_version,
      ...fields
    })
    if (version !== contextVersion) return
    editing.value = null
    pendingAction.value = null
    app.showSuccess(t('admin.departments.memberManagement.updated'))
    await loadMembers()
  } catch (cause) {
    if (version === contextVersion) await handleWriteError(cause)
  } finally { if (version === contextVersion) saving.value = false }
}

async function saveProfile() {
  if (!editing.value || saving.value || profileError.value || !profileChanged.value) return
  const fields: Pick<ManagedMemberUpdateInput, 'username' | 'department_note'> = {}
  if (form.username.trim() !== editing.value.username) fields.username = form.username.trim()
  if (!auth.isAdmin && form.department_note.trim() !== editing.value.department_note) fields.department_note = form.department_note.trim()
  await updateMember(editing.value, fields)
}

function openAction(member: ManagedDepartmentMember, kind: 'disable' | 'enable' | 'remove') {
  if (saving.value || !member.manageable || (kind === 'enable' && !member.can_enable)) return
  pendingAction.value = { kind, member: { ...member } }
  error.value = ''
}

async function assignMembers(target: number | null, changes: DepartmentMemberChange[]) {
  if (saving.value || !changes.length) return
  saving.value = true
  error.value = ''
  const version = contextVersion
  try {
    const result = await departmentsAPI.managedAssign(target, changes)
    if (version !== contextVersion) return
    pendingAction.value = null
    candidateOpen.value = false
    clearCandidateResults()
    app.showSuccess(t('admin.departments.moved', { count: result.updated_count }))
    await loadMembers()
  } catch (cause) {
    if (version === contextVersion) await handleWriteError(cause)
  } finally { if (version === contextVersion) saving.value = false }
}

async function confirmAction() {
  const action = pendingAction.value
  if (!action || saving.value) return
  if (action.kind === 'remove') {
    await assignMembers(null, [{ user_id: action.member.id, expected_department_id: action.member.department_id, expected_department_version: action.member.department_version }])
  } else await updateMember(action.member, { status: action.kind === 'enable' ? 'active' : 'disabled' })
}

function openCandidates() {
  if (!canAdd.value || saving.value) return
  error.value = ''
  clearCandidateResults()
  candidateQuery.value = ''
  candidateOpen.value = true
}

async function searchCandidates() {
  if (!candidateOpen.value || !selectedDepartment.value || !canAdd.value || saving.value) return
  clearCandidateResults()
  const search = candidateQuery.value.trim()
  if (Array.from(search).length < 2) { candidateError.value = t('admin.departments.memberManagement.candidateMinimum'); return }
  candidateController = new AbortController()
  const signal = candidateController.signal
  candidateLoading.value = true
  try {
    const result = await departmentsAPI.managedCandidates(selectedDepartment.value.id, search, signal)
    if (signal.aborted) return
    candidates.value = result.slice(0, 20)
  } catch (cause) {
    if (signal.aborted) return
    if (isScopeDenied(cause)) await loadScope(t(departmentErrorKey(cause)))
    else candidateError.value = t(departmentErrorKey(cause))
  } finally { if (!signal.aborted) candidateLoading.value = false }
}

function toggleCandidate(member: DepartmentMember) {
  if (saving.value || !candidates.value.some(candidate => candidate.id === member.id)) return
  if (selectedCandidates.value.has(member.id)) selectedCandidates.value.delete(member.id)
  else selectedCandidates.value.set(member.id, { ...member })
}

async function addSelected() {
  if (!canAdd.value || !selectedDepartment.value || saving.value || candidateLoading.value) return
  await assignMembers(selectedDepartment.value.id, [...selectedCandidates.value.values()].map(member => ({
    user_id: member.id, expected_department_id: member.department_id, expected_department_version: member.department_version
  })))
}

watch(candidateQuery, clearCandidateResults, { flush: 'sync' })
onMounted(() => { void loadScope() })
onUnmounted(() => {
  contextVersion++
  scopeController?.abort()
  listController?.abort()
  candidateController?.abort()
})
</script>
