<template>
  <BaseDialog :show="show" :title="t('admin.departments.assignDepartment')" width="wide" :show-close-button="!saving" :close-on-escape="!saving" @close="emit('close')">
    <div class="space-y-4">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.departments.memberHint') }}</p>
      <p v-if="mixedOrganizations" class="text-sm text-amber-700 dark:text-amber-300">{{ t('admin.departments.sameOrganization') }}</p>
      <label class="block space-y-1"><span>{{ t('admin.departments.targetDepartment') }}</span><Select v-model="targetID" :options="options" :disabled="loading || saving" /></label>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <DataTable :columns="columns" :data="members" :loading="loading"><template #cell-department_name="{ row }">{{ row.department_name || t('admin.departments.unassigned') }}</template></DataTable>
    </div>
    <template #footer><button class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('admin.departments.cancel') }}</button><button class="btn btn-primary" :disabled="saving || loading || !members.length || members.length !== userIds.length || targetID === ''" @click="save">{{ t('admin.departments.confirmMembers') }}</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { departmentErrorKey } from '@/utils/departmentErrors'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import DataTable from '@/components/common/DataTable.vue'
import type { Column } from '@/components/common/types'
import { departmentsAPI, type Department, type DepartmentMember } from '@/api/admin/departments'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ show: boolean; userIds: number[] }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const app = useAppStore()
const members = ref<DepartmentMember[]>([])
const departments = ref<Department[]>([])
const targetID = ref('')
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const organizations = computed(() => [...new Set(members.value.map(member => member.organization))])
const mixedOrganizations = computed(() => organizations.value.length > 1)
let controller: AbortController | null = null
const options = computed(() => [
  { value: '', label: t('admin.departments.targetDepartment') },
  { value: 'unassigned', label: t('admin.departments.clearDepartment') },
  ...departments.value.map(department => ({ value: String(department.id), label: department.name }))
])
const columns = computed<Column[]>(() => [{ key: 'email', label: t('admin.departments.email') }, { key: 'department_name', label: t('admin.departments.currentDepartment') }])

async function load() {
  controller?.abort()
  controller = new AbortController()
  const signal = controller.signal
  members.value = []
  departments.value = []
  targetID.value = ''
  error.value = ''
  loading.value = false
  if (!props.show) return
  if (!props.userIds.length || props.userIds.length > 200) { error.value = t('admin.departments.maxSelection'); return }
  loading.value = true
  try {
    const result = await departmentsAPI.members({ user_ids: props.userIds.join(','), page: 1, page_size: 200 }, signal)
    if (signal.aborted) return
    if (result.items.length !== props.userIds.length) throw new Error('members changed')
    members.value = result.items
    if (organizations.value.length === 1) {
      const options: Department[] = []
      let page = 1
      while (true) {
        const directory = await departmentsAPI.list({ organization: organizations.value[0], status: 'active', page, page_size: 200 }, signal)
        if (signal.aborted) return
        options.push(...directory.items)
        if (options.length >= directory.total || !directory.items.length) break
        if (++page > 100) throw new Error('department directory too large')
      }
      departments.value = options
    }
  } catch {
    if (!signal.aborted) { members.value = []; error.value = t('admin.departments.conflict') }
  } finally { if (!signal.aborted) loading.value = false }
}
async function save() {
  if (saving.value || loading.value || !targetID.value || members.value.length !== props.userIds.length) return
  const target = targetID.value === 'unassigned' ? null : Number(targetID.value)
  if (target !== null && !departments.value.some(department => department.id === target)) return
  saving.value = true
  try {
    const result = await departmentsAPI.assign(target, members.value.map(member => ({ user_id: member.id, expected_department_id: member.department_id, expected_department_version: member.department_version })))
    app.showSuccess(t('admin.departments.moved', { count: result.updated_count }))
    emit('saved')
    emit('close')
  } catch (cause) {
    await load()
    error.value = t(departmentErrorKey(cause))
  } finally { saving.value = false }
}
watch(() => [props.show, props.userIds.join(',')], load, { immediate: true })
onUnmounted(() => { controller?.abort() })
</script>
