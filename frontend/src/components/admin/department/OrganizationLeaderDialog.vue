<template>
  <BaseDialog :show="show" :title="dialogTitle" width="wide" :show-close-button="!saving" :close-on-escape="!saving" @close="close">
    <div class="space-y-4">
      <template v-if="view === 'list'">
        <div class="flex flex-wrap items-end gap-2">
          <label v-if="organizations.length > 1" class="w-40 space-y-1"><span class="text-sm">{{ t('admin.departments.organization') }}</span><Select v-model="organization" :options="organizationOptions" :searchable="false" @change="loadLeaders" /></label>
          <form class="flex min-w-0 flex-1 gap-2" @submit.prevent="loadLeaders">
            <input v-model="query" class="input min-w-0 flex-1" :placeholder="t('admin.departments.organizationLeaders.search')" :aria-label="t('admin.departments.organizationLeaders.search')" />
            <button class="btn btn-secondary" :disabled="loading">{{ t('admin.departments.search') }}</button>
          </form>
          <button class="btn btn-primary" data-testid="appoint-leader" @click="openCandidates">{{ t('admin.departments.organizationLeaders.appoint') }}</button>
        </div>
        <DataTable :columns="leaderColumns" :data="leaders" :loading="loading">
          <template #cell-departments="{ row }">{{ row.departments.map(departmentLabel).join(', ') || '—' }}</template>
          <template #cell-permissions="{ row }">{{ row.managed ? capabilityLabels(row.permissions) : t('admin.departments.organizationLeaders.protectedLabel') }}</template>
          <template #cell-actions="{ row }">
            <button v-if="row.managed" class="text-primary-600 hover:underline" :data-testid="`edit-leader-${row.id}`" :disabled="loading" @click="editLeader(row)">{{ t('admin.departments.organizationLeaders.edit') }}</button>
            <span v-else class="text-gray-500">{{ t('admin.departments.memberManagement.readonly') }}</span>
          </template>
          <template #empty><p class="p-6 text-center text-gray-500">{{ t('admin.departments.organizationLeaders.noLeaders') }}</p></template>
        </DataTable>
      </template>

      <template v-else-if="view === 'candidates'">
        <p class="text-sm text-gray-500">{{ t('admin.departments.organizationLeaders.candidateHint') }}</p>
        <form class="flex gap-2" @submit.prevent="searchCandidates">
          <input v-model="candidateQuery" data-testid="leader-candidate-query" class="input min-w-0 flex-1" :placeholder="t('admin.departments.organizationLeaders.search')" :aria-label="t('admin.departments.organizationLeaders.search')" />
          <button class="btn btn-secondary" :disabled="loading || Array.from(candidateQuery.trim()).length < 2">{{ t('admin.departments.search') }}</button>
        </form>
        <DataTable :columns="candidateColumns" :data="candidates" :loading="loading">
          <template #cell-department_name="{ row }">{{ row.department_name || t('admin.departments.unassigned') }}</template>
          <template #cell-actions="{ row }"><button class="text-primary-600 hover:underline" :data-testid="`choose-candidate-${row.id}`" @click="chooseCandidate(row)">{{ t('admin.departments.organizationLeaders.appoint') }}</button></template>
          <template #empty><p class="p-6 text-center text-gray-500">{{ t('admin.departments.organizationLeaders.noCandidates') }}</p></template>
        </DataTable>
      </template>

      <template v-else-if="target">
        <p class="break-all font-medium">{{ target.email }}</p>
        <p v-if="target.appoint" class="text-sm text-amber-700 dark:text-amber-300">{{ t('admin.departments.organizationLeaders.appointNotice') }}</p>
        <fieldset class="space-y-2">
          <legend class="text-sm font-medium">{{ t('admin.departments.organizationLeaders.capabilities') }}</legend>
          <label class="flex items-center gap-2"><input v-model="report" type="checkbox" data-testid="leader-report" :disabled="saving" />{{ t('admin.departments.reportPermission') }}</label>
          <label class="flex items-center gap-2"><input v-model="resetQuota" type="checkbox" data-testid="leader-reset" :disabled="saving" />{{ t('admin.departments.resetPermission') }}</label>
          <label class="flex items-center gap-2"><input v-model="manageMembers" type="checkbox" data-testid="leader-members" :disabled="saving" />{{ t('admin.departments.manageMembersPermission') }}</label>
        </fieldset>
        <fieldset v-for="group in departmentGroups" :key="group.organization" class="space-y-2">
          <legend class="text-sm font-medium">{{ t('admin.departments.organizationLeaders.departments') }} · {{ organizationLabel(group.organization) }}</legend>
          <label v-for="department in group.departments" :key="department.id" class="flex items-center gap-2">
            <input type="checkbox" :data-testid="`leader-department-${department.id}`" :checked="selected.has(department.id)"
              :disabled="saving || (department.status !== 'active' && !original.has(department.id))" @change="toggleDepartment(department.id)" />
            {{ department.name }}<span v-if="department.status !== 'active'" class="text-xs text-gray-500">（{{ t('admin.departments.inactive') }}）</span>
          </label>
        </fieldset>
        <p class="text-xs text-gray-500">{{ t('admin.departments.organizationLeaders.inactiveHint') }}</p>
        <div class="rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-700" data-testid="leader-changes">
          <p class="font-medium">{{ t('admin.departments.organizationLeaders.changes') }}</p>
          <p v-if="added.length">{{ t('admin.departments.organizationLeaders.added', { departments: added.join(', ') }) }}</p>
          <p v-if="removed.length">{{ t('admin.departments.organizationLeaders.removed', { departments: removed.join(', ') }) }}</p>
          <p v-if="!added.length && !removed.length">{{ t('admin.departments.organizationLeaders.noChanges') }}</p>
        </div>
        <div v-if="confirmingRevoke" class="rounded-lg bg-red-50 p-3 text-sm text-red-800 dark:bg-red-900/20 dark:text-red-200">
          <p class="font-medium">{{ t('admin.departments.organizationLeaders.confirmRevoke') }}</p>
          <p>{{ t('admin.departments.organizationLeaders.revokeHint') }}</p>
        </div>
      </template>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    </div>
    <template #footer>
      <button class="btn btn-secondary" :disabled="saving" @click="view === 'list' ? close() : backToList()">{{ t('admin.departments.cancel') }}</button>
      <template v-if="view === 'edit' && target">
        <button v-if="!target.appoint" class="btn btn-danger" data-testid="revoke-leader" :disabled="saving" @click="revoke">{{ confirmingRevoke ? t('admin.departments.memberManagement.confirm') : t('admin.departments.organizationLeaders.revoke') }}</button>
        <button class="btn btn-primary" data-testid="save-leader" :disabled="!canSave" @click="save">{{ t('admin.departments.save') }}</button>
      </template>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import Select from '@/components/common/Select.vue'
import type { Column } from '@/components/common/types'
import type { AdminPermission } from '@/types'
import {
  departmentsAPI, type Department, type OrganizationKey, type OrganizationLeader,
  type OrganizationLeaderCandidate, type OrganizationLeaderDepartment
} from '@/api/admin/departments'
import { departmentErrorKey } from '@/utils/departmentErrors'
import { formatOrganizationUsageOrganization } from '@/utils/organizations'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ show: boolean; organizations: OrganizationKey[]; departments: Department[] }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const app = useAppStore()
const view = ref<'list' | 'candidates' | 'edit'>('list')
const organization = ref<'all' | OrganizationKey>('all')
const query = ref('')
const leaders = ref<OrganizationLeader[]>([])
const candidateQuery = ref('')
const candidates = ref<OrganizationLeaderCandidate[]>([])
const target = ref<{ id: number; email: string; version: string; appoint: boolean } | null>(null)
const original = ref(new Set<number>())
const selected = ref(new Set<number>())
const report = ref(true)
const resetQuota = ref(true)
const manageMembers = ref(false)
const confirmingRevoke = ref(false)
const loading = ref(false)
const saving = ref(false)
const error = ref('')
let controller: AbortController | null = null

const organizationLabel = (key: OrganizationKey) => formatOrganizationUsageOrganization(key, t('admin.organizationUsage.organizations.other'))
const departmentLabel = (department: OrganizationLeaderDepartment) => props.organizations.length > 1 ? `${organizationLabel(department.organization_key)} · ${department.name}` : department.name
const organizationOptions = computed(() => [{ value: 'all', label: t('admin.departments.allDepartments') }, ...props.organizations.map(value => ({ value, label: organizationLabel(value) }))])
const dialogTitle = computed(() => t(view.value === 'list' ? 'admin.departments.organizationLeaders.title' : view.value === 'candidates' ? 'admin.departments.organizationLeaders.appoint' : 'admin.departments.organizationLeaders.edit'))
const leaderColumns = computed<Column[]>(() => [
  { key: 'email', label: t('admin.departments.email') },
  { key: 'departments', label: t('admin.departments.organizationLeaders.departments') },
  { key: 'permissions', label: t('admin.departments.organizationLeaders.capabilities') },
  { key: 'actions', label: t('admin.departments.actions') }
])
const candidateColumns = computed<Column[]>(() => [
  { key: 'email', label: t('admin.departments.email') },
  { key: 'username', label: t('admin.departments.memberManagement.username') },
  { key: 'department_name', label: t('admin.departments.department') },
  { key: 'actions', label: t('admin.departments.actions') }
])
const departmentGroups = computed(() => props.organizations.map(key => ({ organization: key, departments: props.departments.filter(d => d.organization_key === key) })))
const departmentNames = computed(() => new Map(props.departments.map(d => [d.id, props.organizations.length > 1 ? `${organizationLabel(d.organization_key)} · ${d.name}` : d.name])))
const added = computed(() => [...selected.value].filter(id => !original.value.has(id)).map(id => departmentNames.value.get(id) ?? String(id)))
const removed = computed(() => [...original.value].filter(id => !selected.value.has(id)).map(id => departmentNames.value.get(id) ?? String(id)))
const capabilityCount = computed(() => [report.value, resetQuota.value, manageMembers.value].filter(Boolean).length)
const canSave = computed(() => !!target.value && !saving.value && selected.value.size > 0 && capabilityCount.value > 0)

function capabilityLabels(permissions: AdminPermission[]) {
  const labels: string[] = []
  if (permissions.includes('admin.organization_usage')) labels.push(t('admin.departments.reportPermission'))
  if (permissions.includes('admin.department_subscriptions')) labels.push(t('admin.departments.resetPermission'))
  if (permissions.includes('admin.department_members')) labels.push(t('admin.departments.manageMembersPermission'))
  return labels.join(', ') || '—'
}

function begin() {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  error.value = ''
  return controller.signal
}

async function loadLeaders() {
  view.value = 'list'
  const signal = begin()
  try {
    const result = await departmentsAPI.organizationLeaders({ organization: organization.value, q: query.value.trim(), page: 1, page_size: 100 }, signal)
    if (!signal.aborted) leaders.value = result.items
  } catch (cause) {
    if (!signal.aborted) { leaders.value = []; error.value = t(departmentErrorKey(cause)) }
  } finally { if (!signal.aborted) loading.value = false }
}

function openCandidates() {
  view.value = 'candidates'
  candidateQuery.value = ''
  candidates.value = []
  error.value = ''
}

async function searchCandidates() {
  const search = candidateQuery.value.trim()
  if (Array.from(search).length < 2) return
  const signal = begin()
  try {
    const result = await departmentsAPI.organizationLeaderCandidates(organization.value, search, signal)
    if (!signal.aborted) candidates.value = result
  } catch (cause) {
    if (!signal.aborted) { candidates.value = []; error.value = t(departmentErrorKey(cause)) }
  } finally { if (!signal.aborted) loading.value = false }
}

function startEdit(next: { id: number; email: string; version: string; appoint: boolean }, departments: number[], permissions: AdminPermission[]) {
  target.value = next
  original.value = new Set(departments)
  selected.value = new Set(departments)
  report.value = next.appoint || permissions.includes('admin.organization_usage')
  resetQuota.value = next.appoint || permissions.includes('admin.department_subscriptions')
  manageMembers.value = permissions.includes('admin.department_members')
  confirmingRevoke.value = false
  view.value = 'edit'
}

function chooseCandidate(candidate: OrganizationLeaderCandidate) {
  startEdit({ id: candidate.id, email: candidate.email, version: candidate.version, appoint: true }, [], [])
}

// The edit snapshot comes from the detail endpoint: the list only projects the
// selected organization, and saving replaces every grant (review R3).
async function editLeader(leader: OrganizationLeader) {
  const signal = begin()
  try {
    const detail = await departmentsAPI.organizationLeader(leader.id, signal)
    if (signal.aborted) return
    startEdit({ id: detail.id, email: detail.email, version: detail.version ?? '', appoint: false }, detail.departments.map(d => d.id), detail.permissions)
  } catch (cause) {
    if (!signal.aborted) { error.value = t(departmentErrorKey(cause)); await loadLeaders() }
  } finally { if (!signal.aborted) loading.value = false }
}

function toggleDepartment(id: number) {
  const next = new Set(selected.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selected.value = next
  confirmingRevoke.value = false
}

async function submit(departmentIDs: number[], revoking: boolean) {
  if (!target.value || saving.value) return
  saving.value = true
  error.value = ''
  try {
    await departmentsAPI.updateOrganizationLeader(target.value.id, {
      department_ids: departmentIDs,
      report: !revoking && report.value,
      reset_quota: !revoking && resetQuota.value,
      manage_members: !revoking && manageMembers.value,
      expected_version: target.value.version
    })
    app.showSuccess(t(revoking ? 'admin.departments.organizationLeaders.revoked' : 'admin.departments.organizationLeaders.saved'))
    emit('saved')
    target.value = null
    await loadLeaders()
  } catch (cause) {
    const message = t(departmentErrorKey(cause))
    target.value = null
    await loadLeaders()
    error.value = message
  } finally { saving.value = false }
}

function save() {
  if (canSave.value) void submit([...selected.value].sort((a, b) => a - b), false)
}

function revoke() {
  if (!confirmingRevoke.value) { confirmingRevoke.value = true; return }
  void submit([], true)
}

function backToList() {
  target.value = null
  void loadLeaders()
}

function close() { if (!saving.value) emit('close') }

watch(() => props.show, show => {
  controller?.abort()
  target.value = null
  query.value = ''
  organization.value = 'all'
  if (show) void loadLeaders()
}, { immediate: true })
onUnmounted(() => { controller?.abort() })
</script>
