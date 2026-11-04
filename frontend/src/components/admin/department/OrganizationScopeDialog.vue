<template>
  <BaseDialog :show="show" :title="t('admin.departments.organizationScope.configure')" :show-close-button="!saving" :close-on-escape="!saving" @close="emit('close')">
    <div class="space-y-4">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.departments.organizationScope.description') }}</p>
      <p v-if="loading" class="text-sm text-gray-500">{{ t('common.loading') }}</p>
      <div v-if="access && !loading" class="space-y-3">
        <label v-for="organization in ORGANIZATION_LEADER_ORGANIZATIONS" :key="organization" class="flex items-center gap-2">
          <input v-model="selected" type="checkbox" :value="organization" :data-testid="`organization-scope-${organization}`" :disabled="saving" />
          {{ organizationLabel(organization) }}
        </label>
        <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.departments.organizationScope.bundleHint') }}</p>
        <label v-if="selected.length && access.department_ids.length" class="flex items-start gap-2 rounded-lg bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-900/20 dark:text-amber-200">
          <input v-model="replaceDepartments" type="checkbox" data-testid="organization-scope-replace-departments" :disabled="saving" class="mt-0.5" />
          {{ t('admin.departments.organizationScope.replaceDepartments', { count: access.department_ids.length }) }}
        </label>
        <label v-if="selected.length && hasGlobalSubscriptions" class="flex items-start gap-2 rounded-lg bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-900/20 dark:text-amber-200">
          <input v-model="replaceGlobal" type="checkbox" data-testid="organization-scope-replace-global" :disabled="saving" class="mt-0.5" />
          {{ t('admin.departments.organizationScope.replaceGlobal') }}
        </label>
        <p v-if="!selected.length && access.organizations.length" class="text-sm text-amber-700 dark:text-amber-300">{{ t('admin.departments.organizationScope.revokeHint') }}</p>
      </div>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    </div>
    <template #footer>
      <button class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('admin.departments.cancel') }}</button>
      <button class="btn btn-primary" data-testid="save-organization-scope" :disabled="!canSave" @click="save">{{ t('admin.departments.save') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { departmentsAPI, ORGANIZATION_LEADER_ORGANIZATIONS, type DepartmentAccess, type OrganizationKey } from '@/api/admin/departments'
import { departmentErrorKey } from '@/utils/departmentErrors'
import { formatOrganizationUsageOrganization } from '@/utils/organizations'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ show: boolean; userId: number | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const app = useAppStore()
const access = ref<DepartmentAccess | null>(null)
const selected = ref<OrganizationKey[]>([])
const replaceDepartments = ref(false)
const replaceGlobal = ref(false)
const loading = ref(false)
const saving = ref(false)
const error = ref('')
let controller: AbortController | null = null

const organizationLabel = (key: OrganizationKey) => formatOrganizationUsageOrganization(key, t('admin.organizationUsage.organizations.other'))
const hasGlobalSubscriptions = computed(() => access.value?.permissions.includes('admin.subscriptions') ?? false)
const unchanged = computed(() => !!access.value && [...selected.value].sort().join(',') === [...access.value.organizations].sort().join(','))
// Converting a department leader or a global subscription holder needs an explicit confirmation.
const canSave = computed(() => !!access.value && !loading.value && !saving.value && !unchanged.value &&
  (!selected.value.length || ((!access.value.department_ids.length || replaceDepartments.value) && (!hasGlobalSubscriptions.value || replaceGlobal.value))))

async function load() {
  controller?.abort()
  access.value = null
  error.value = ''
  replaceDepartments.value = false
  replaceGlobal.value = false
  if (!props.show || props.userId === null) { loading.value = false; return }
  controller = new AbortController()
  const signal = controller.signal
  loading.value = true
  try {
    const result = await departmentsAPI.getOrganizationAccess(props.userId, signal)
    if (signal.aborted) return
    access.value = result
    selected.value = [...result.organizations]
  } catch (cause) {
    if (!signal.aborted) error.value = t(departmentErrorKey(cause))
  } finally { if (!signal.aborted) loading.value = false }
}

async function save() {
  if (!canSave.value || !access.value || props.userId === null) return
  saving.value = true
  error.value = ''
  try {
    await departmentsAPI.setOrganizationAccess(props.userId, {
      organizations: [...selected.value].sort(),
      expected_version: access.value.version,
      replace_department_grants: replaceDepartments.value,
      replace_global_subscriptions: replaceGlobal.value
    })
    app.showSuccess(t('admin.departments.organizationScope.saved'))
    emit('saved')
    emit('close')
  } catch (cause) {
    const message = t(departmentErrorKey(cause))
    await load()
    error.value = message
  } finally { saving.value = false }
}

watch(() => [props.show, props.userId], load, { immediate: true })
onUnmounted(() => { controller?.abort() })
</script>
