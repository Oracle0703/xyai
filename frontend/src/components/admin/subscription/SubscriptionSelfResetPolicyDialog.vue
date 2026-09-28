<template>
  <BaseDialog :show="true" :title="t('userSubscriptions.selfReset.policyTitle')" width="normal" @close="close">
    <p class="mb-4 text-sm text-gray-500">{{ t('userSubscriptions.selfReset.policyHint') }}</p>
    <p v-if="loading">{{ t('common.loading') }}</p>
    <p v-if="error" role="alert" class="mb-3 text-sm text-red-600">{{ error }}</p>
    <form v-if="limits" id="self-reset-policy" class="space-y-4" @submit.prevent="save">
      <label class="flex items-center justify-between gap-4">
        <span>{{ t('userSubscriptions.selfReset.rollout') }}</span>
        <select v-model="rollout" :disabled="saving" class="input w-40">
          <option value="off">{{ t('userSubscriptions.selfReset.rolloutOff') }}</option>
          <option value="admin">{{ t('userSubscriptions.selfReset.rolloutAdmin') }}</option>
          <option value="all">{{ t('userSubscriptions.selfReset.rolloutAll') }}</option>
        </select>
      </label>
      <label v-for="org in organizations" :key="org" class="flex items-center justify-between gap-4">
        <span>{{ t(`userSubscriptions.selfReset.${org}`) }}</span>
        <input v-model.number="limits[org]" :aria-label="t(`userSubscriptions.selfReset.${org}`)" type="number" min="0" max="100" step="1" required :disabled="saving" class="input w-28" />
      </label>
    </form>
    <template #footer>
      <button class="btn btn-secondary" :disabled="saving" @click="close">{{ t('common.cancel') }}</button>
      <button v-if="loadFailed && !loading" class="btn btn-secondary" :disabled="saving" @click="load">{{ t('userSubscriptions.selfReset.retry') }}</button>
      <button type="submit" form="self-reset-policy" class="btn btn-primary" :disabled="loading || saving || !limits">{{ t('common.save') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { getSelfResetPolicy, setSelfResetPolicy, type SelfResetPolicy } from '@/api/subscriptionSelfReset'
import { useAppStore } from '@/stores/app'

type Limits = SelfResetPolicy['daily_limit_by_organization']
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const app = useAppStore()
const organizations = ['xunyou', 'wsdashi', 'other'] as const
const limits = ref<Partial<Limits> | null>(null)
const rollout = ref<SelfResetPolicy['rollout']>('admin')
const loading = ref(false)
const loadFailed = ref(false)
const saving = ref(false)
const error = ref('')
function close() { if (!saving.value) emit('close') }
async function load() {
  loading.value = true
  loadFailed.value = false
  error.value = ''
  try { const policy = await getSelfResetPolicy(); limits.value = { ...policy.daily_limit_by_organization }; rollout.value = policy.rollout || 'admin' }
  catch {
    // PUT does not need the stored value, so a missing or corrupt policy can be repaired from empty inputs.
    limits.value = {}
    loadFailed.value = true
    error.value = t('userSubscriptions.selfReset.policyLoadFailed')
  }
  finally { loading.value = false }
}
async function save() {
  const values = limits.value
  if (!values || saving.value) return
  if (organizations.some(org => !Number.isInteger(values[org]) || values[org]! < 0 || values[org]! > 100)) {
    error.value = t('userSubscriptions.selfReset.policyInvalid')
    return
  }
  saving.value = true
  error.value = ''
  try {
    await setSelfResetPolicy({ daily_limit_by_organization: values as Limits, rollout: rollout.value })
    app.showSuccess(t('userSubscriptions.selfReset.policySaved'))
    emit('close')
  } catch { error.value = t('userSubscriptions.selfReset.policySaveFailed') }
  finally { saving.value = false }
}
onMounted(load)
</script>
