<template>
  <BaseDialog :show="true" :title="t('userSubscriptions.selfReset.policyTitle')" width="normal" @close="close">
    <p class="mb-4 text-sm text-gray-500">{{ t('userSubscriptions.selfReset.policyHint') }}</p>
    <p v-if="loading">{{ t('common.loading') }}</p>
    <p v-if="notice" role="status" class="mb-3 text-sm text-gray-600">{{ notice }}</p>
    <p v-if="error" role="alert" class="mb-3 text-sm text-red-600">{{ error }}</p>
    <form v-if="limits" id="self-reset-policy" class="space-y-4" @submit.prevent="save">
      <label class="flex items-center justify-between gap-4">
        <span>{{ t('userSubscriptions.selfReset.rollout') }}</span>
        <select v-model="rollout" :disabled="editingBlocked" class="input w-40">
          <option value="off">{{ t('userSubscriptions.selfReset.rolloutOff') }}</option>
          <option value="admin">{{ t('userSubscriptions.selfReset.rolloutAdmin') }}</option>
          <option value="all">{{ t('userSubscriptions.selfReset.rolloutAll') }}</option>
        </select>
      </label>
      <p v-if="closingRollout" class="text-sm text-amber-600">{{ t('userSubscriptions.selfReset.closeOnlyHint') }}</p>
      <section v-for="org in organizations" :key="org" :data-testid="`self-reset-card-${org}`" class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span class="font-medium">{{ cardTitle(org) }}</span>
          <label class="flex items-center gap-2 text-sm">
            <span class="text-gray-500">{{ t('userSubscriptions.selfReset.dailyLimitLabel') }}</span>
            <input v-model.number="limits[org]" :aria-label="`${t(`userSubscriptions.selfReset.${org}`)} ${t('userSubscriptions.selfReset.dailyLimitLabel')}`" type="number" min="0" max="100" step="1" :disabled="editingBlocked" class="input w-24" />
          </label>
        </div>
        <p v-if="limits[org] === 0" :data-testid="`self-reset-zero-${org}`" class="mt-1 text-xs text-amber-600">{{ t('userSubscriptions.selfReset.limitZeroHint') }}</p>
        <template v-if="groupsByOrganization">
          <div class="mb-2 mt-3 flex items-center justify-between text-xs text-gray-500">
            <span :data-testid="`self-reset-count-${org}`">{{ t('userSubscriptions.selfReset.groupsSelected', { selected: groupsByOrganization[org].filter(group => group.enabled).length, total: groupsByOrganization[org].length }) }}</span>
            <span v-if="groupsByOrganization[org].length" class="flex gap-3">
              <button type="button" :data-testid="`self-reset-select-all-${org}`" class="text-primary-600 hover:underline disabled:opacity-50" :disabled="editingBlocked" @click="setAll(org, true)">{{ t('common.selectAll') }}</button>
              <button type="button" :data-testid="`self-reset-clear-${org}`" class="text-primary-600 hover:underline disabled:opacity-50" :disabled="editingBlocked" @click="setAll(org, false)">{{ t('userSubscriptions.selfReset.groupsClearAll') }}</button>
            </span>
          </div>
          <div v-if="groupsByOrganization[org].length" class="max-h-48 space-y-2 overflow-auto">
            <label v-for="group in groupsByOrganization[org]" :key="group.id" :data-testid="`self-reset-group-${org}-${group.id}`" class="flex items-center gap-2 text-sm">
              <input v-model="group.enabled" type="checkbox" :disabled="editingBlocked" />
              <span class="min-w-0 flex-1 truncate">{{ group.name }} <span class="text-xs text-gray-400">#{{ group.id }}</span></span>
              <span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ group.platform }}</span>
              <span v-if="hasDailyLimit(group)" class="whitespace-nowrap text-xs text-gray-500">{{ t('userSubscriptions.selfReset.groupDailyLimit', { amount: `$${group.daily_limit_usd}` }) }}</span>
              <span v-else class="whitespace-nowrap text-xs text-amber-600">{{ t('userSubscriptions.selfReset.groupNoDailyLimit') }}</span>
              <span v-if="group.status !== 'active'" class="whitespace-nowrap text-xs text-gray-500">{{ t('userSubscriptions.selfReset.groupInactive') }}</span>
            </label>
          </div>
          <p v-else class="text-sm text-gray-500">{{ t(org === 'other' ? 'userSubscriptions.selfReset.groupsEmptyOther' : 'userSubscriptions.selfReset.groupsEmptyScoped') }}</p>
        </template>
        <p v-else-if="groupsLoadFailed" class="mt-3 text-sm text-red-600">{{ t('userSubscriptions.selfReset.groupsLoadFailed') }}</p>
      </section>
    </form>
    <template #footer>
      <button class="btn btn-secondary" :disabled="saving" @click="close">{{ t('common.cancel') }}</button>
      <button v-if="needsRetry && !loading" data-testid="self-reset-retry" class="btn btn-secondary" :disabled="saving" @click="retry">{{ t('userSubscriptions.selfReset.retry') }}</button>
      <button v-if="!loading && (revision === null || pending || !initialPolicy || initialPolicy.rollout === 'off')" data-testid="self-reset-close-rollout" class="btn btn-secondary" :disabled="saving" @click="closeRollout">{{ t('userSubscriptions.selfReset.closeOnly') }}</button>
      <button type="submit" form="self-reset-policy" class="btn btn-primary" :disabled="editingBlocked || !limits">{{ closingRollout ? t('userSubscriptions.selfReset.closeOnly') : t('common.save') }}</button>
    </template>
  </BaseDialog>
</template>

<script lang="ts">
// The marker is written before the request, so refreshing during a save can also recover.
// Keep an in-page copy for reopening the dialog when sessionStorage is unavailable.
type PendingWrite = {
  revision?: number
  close?: true
  closeRevision?: number
  closeRejectedInvalidPolicy?: true
  conflict?: 'revision' | 'groups'
  requiredPart?: 'policy' | 'groups'
}
const pendingByAdmin = new Map<string, PendingWrite>()
</script>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import {
  closeSelfResetRollout, getSelfResetConfig, saveSelfResetConfig,
  type SelfResetConfig, type SelfResetConfigUpdate, type SelfResetGroup, type SelfResetGroupChanges,
  type SelfResetGroupsByOrganization, type SelfResetOrganization, type SelfResetPolicy
} from '@/api/subscriptionSelfReset'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const app = useAppStore()
const storageKey = `subscription-self-reset-config:${useAuthStore().user?.id}`
const organizations = ['xunyou', 'wsdashi', 'other'] as const
const revision = ref<number | null>(null)
const limits = ref<Partial<Record<SelfResetOrganization, number | ''>> | null>(null)
const rollout = ref<SelfResetPolicy['rollout']>('admin')
const initialPolicy = ref<SelfResetPolicy | null>(null)
const groupsByOrganization = ref<SelfResetGroupsByOrganization | null>(null)
const initialGroups = ref<Partial<Record<SelfResetOrganization, number[]>>>({})
const loading = ref(false)
const saving = ref(false)
const pending = ref<PendingWrite | null>(restorePending())
const error = ref('')
const notice = ref('')
const groupsLoadFailed = computed(() => revision.value !== null && !groupsByOrganization.value)
const needsRetry = computed(() => revision.value === null || !initialPolicy.value || groupsLoadFailed.value || !!pending.value)
const editingBlocked = computed(() => loading.value || saving.value || revision.value === null || !!pending.value)
const closingRollout = computed(() => rollout.value === 'off' && !!initialPolicy.value && initialPolicy.value.rollout !== 'off')
let disposed = false

function restorePending(): PendingWrite | null {
  const inMemory = pendingByAdmin.get(storageKey)
  if (inMemory) return inMemory
  try {
    const data = JSON.parse(sessionStorage.getItem(storageKey) || 'null')
    if (data && (Number.isSafeInteger(data.revision) && data.revision >= 1 || data.close === true || data.conflict === 'revision' || data.conflict === 'groups')) return data
  } catch { /* Storage may be unavailable. */ }
  return null
}

function remember(value: PendingWrite | null) {
  pending.value = value
  if (value) pendingByAdmin.set(storageKey, value)
  else pendingByAdmin.delete(storageKey)
  try {
    if (value) sessionStorage.setItem(storageKey, JSON.stringify(value))
    else sessionStorage.removeItem(storageKey)
  } catch { /* Retain the marker for this page in memory. */ }
}

function close() { if (!saving.value) emit('close') }
function cardTitle(org: SelfResetOrganization) {
  return org === 'other' ? t('userSubscriptions.selfReset.otherGroupsTitle') : t(`userSubscriptions.selfReset.${org}`)
}
function hasDailyLimit(group: SelfResetGroup) { return group.daily_limit_usd != null && group.daily_limit_usd > 0 }
function setAll(org: SelfResetOrganization, enabled: boolean) {
  groupsByOrganization.value?.[org].forEach(group => { group.enabled = enabled })
}

function groupChanges() {
  const changes: Partial<Record<SelfResetOrganization, Required<SelfResetGroupChanges>>> = {}
  if (!groupsByOrganization.value) return changes
  for (const org of organizations) {
    const current = new Set(groupsByOrganization.value[org].filter(group => group.enabled).map(group => group.id))
    const previous = new Set(initialGroups.value[org] || [])
    const enable = [...current].filter(id => !previous.has(id))
    const disable = [...previous].filter(id => !current.has(id))
    if (enable.length || disable.length) changes[org] = { enable, disable }
  }
  return changes
}
function policyChanged() {
  const values = limits.value || {}
  const policy = initialPolicy.value
  if (!policy) return rollout.value !== 'admin' || organizations.some(org => values[org] !== undefined && values[org] !== '')
  return rollout.value !== policy.rollout || organizations.some(org => values[org] !== policy.daily_limit_by_organization[org])
}
function limitsValid() {
  return organizations.every(org => {
    const value = limits.value?.[org]
    return typeof value === 'number' && Number.isInteger(value) && value >= 0 && value <= 100
  })
}

// Only a retry of a partially loaded form may retain edits, and only at the same revision.
// Reapply actual group edits to the fresh candidate list because ownership changes independently.
function applySnapshot(config: SelfResetConfig, retryPartial: boolean) {
  const keepEdits = retryPartial && revision.value === config.revision
  const keepPolicy = keepEdits && !!config.policy && !!initialPolicy.value && policyChanged()
  const edits = keepEdits ? groupChanges() : {}
  revision.value = config.revision
  initialPolicy.value = config.policy
  if (!keepPolicy) {
    limits.value = { ...config.policy?.daily_limit_by_organization }
    rollout.value = config.policy?.rollout || 'admin'
  }
  groupsByOrganization.value = config.groups_by_organization
  initialGroups.value = {}
  if (config.groups_by_organization) {
    for (const org of organizations) {
      const groups = config.groups_by_organization[org]
      initialGroups.value[org] = groups.filter(group => group.enabled).map(group => group.id)
      for (const group of groups) {
        if (edits[org]?.enable.includes(group.id)) group.enabled = true
        if (edits[org]?.disable.includes(group.id)) group.enabled = false
      }
    }
  }
  if (!config.policy) error.value = t('userSubscriptions.selfReset.policyLoadFailed')
}

async function readConfig(retryPartial = false, minimumRevision = 1) {
  loading.value = true
  error.value = ''
  try {
    const config = await getSelfResetConfig()
    if (disposed) return
    if (!Number.isSafeInteger(config.revision) || config.revision < minimumRevision) throw new Error('Missing or stale configuration revision')
    if (retryPartial && revision.value !== null && revision.value !== config.revision) {
      notice.value = t('userSubscriptions.selfReset.revisionConflict')
    }
    applySnapshot(config, retryPartial)
    return config
  } catch {
    if (!disposed) {
      revision.value = null
      limits.value = null
      initialPolicy.value = null
      groupsByOrganization.value = null
      error.value = t('userSubscriptions.selfReset.configLoadFailed')
    }
  } finally {
    if (!disposed) loading.value = false
  }
}

function failureInfo(cause: unknown) {
  const failure = cause as { status?: number; reason?: string; code?: string } | null
  return { status: failure?.status ?? 0, reason: failure?.reason || failure?.code }
}
function unknownOutcome(status: number) { return status === 0 || status === 408 || status >= 500 }

function missingRequiredPart(operation: PendingWrite, config: SelfResetConfig) {
  if (operation.revision === undefined && !operation.conflict) return false
  return (!config.policy && operation.requiredPart !== 'groups' && !operation.closeRejectedInvalidPolicy) ||
    (!config.groups_by_organization && operation.requiredPart !== 'policy')
}

function ordinaryPending(operation: PendingWrite | null) {
  if (operation?.revision === undefined && !operation?.conflict) return null
  return { ...operation, close: undefined, closeRevision: undefined, closeRejectedInvalidPolicy: undefined }
}

function closedAtNewerRevision(config: SelfResetConfig, prior: number | undefined) {
  return config.policy?.rollout === 'off' && prior !== undefined && Number.isSafeInteger(prior) &&
    prior >= 1 && config.revision > prior
}

async function reconcile() {
  const operation = pending.value
  if (!operation || disposed) return
  saving.value = true
  error.value = ''
  notice.value = operation.conflict
    ? t(operation.conflict === 'groups' ? 'userSubscriptions.selfReset.groupConflict' : 'userSubscriptions.selfReset.revisionConflict')
    : t('userSubscriptions.selfReset.confirmingSave')
  try {
    let confirmedFenceRevision: number | undefined
    if (operation.revision !== undefined && !operation.conflict) {
      try {
        const result = await saveSelfResetConfig({ expected_revision: operation.revision })
        if (!Number.isSafeInteger(result?.revision) || result.revision <= operation.revision) throw new Error('Unconfirmed revision')
        confirmedFenceRevision = result.revision
      } catch (cause) {
        const { status, reason } = failureInfo(cause)
        if (status !== 409 || reason !== 'SELF_RESET_REVISION_CONFLICT') throw cause
      }
      if (disposed) return
    }
    const minimumRevision = operation.revision === undefined ? 1 : operation.revision + (operation.conflict === 'groups' ? 0 : 1)
    const config = await readConfig(false, minimumRevision)
    if (disposed) return
    if (!config) {
      if (!operation.conflict) notice.value = t('userSubscriptions.selfReset.saveOutcomeUnknown')
      return
    }
    // Even a newer off snapshot may predate the close click. Require an acknowledged write
    // from this recovery, or let the administrator explicitly retry the unconditional close.
    if (operation.close && (confirmedFenceRevision !== config.revision || !closedAtNewerRevision(config, operation.closeRevision))) {
      notice.value = ''
      error.value = t('userSubscriptions.selfReset.closeOutcomeUnknown')
      return
    }
    if (missingRequiredPart(operation, config)) {
      error.value = t('userSubscriptions.selfReset.snapshotIncomplete')
      return
    }
    remember(null)
    if (operation.closeRejectedInvalidPolicy && !config.policy) {
      notice.value = ''
      error.value = t('userSubscriptions.selfReset.closePolicyInvalid')
    } else if (operation.close || !operation.conflict) {
      notice.value = t(operation.close ? 'userSubscriptions.selfReset.closedSnapshot' : 'userSubscriptions.selfReset.reviewSnapshot')
    }
  } catch {
    if (!disposed) {
      notice.value = ''
      error.value = t('userSubscriptions.selfReset.saveOutcomeUnknown')
    }
  } finally {
    if (!disposed) saving.value = false
  }
}

async function closeRollout() {
  if (saving.value || loading.value) return
  const previous = pending.value
  const closeRevision = revision.value ?? undefined
  remember({ ...previous, close: true, closeRevision })
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const result = await closeSelfResetRollout()
    if (!Number.isSafeInteger(result?.revision) || result.revision <= (closeRevision ?? 0)) throw new Error('Unconfirmed revision')
    if (disposed) return
    // Close fences earlier writes, but their group changes may have committed before it.
    const ordinary = ordinaryPending(previous)
    remember(ordinary)
    notice.value = t('userSubscriptions.selfReset.rolloutClosed')
    const config = await readConfig(false, result.revision)
    if (disposed || !config) return
    if (ordinary && missingRequiredPart(ordinary, config)) {
      error.value = t('userSubscriptions.selfReset.snapshotIncomplete')
      return
    }
    remember(null)
    if (config.policy && config.policy.rollout !== 'off') notice.value = t('userSubscriptions.selfReset.closeChangedAgain')
  } catch (cause) {
    if (disposed) return
    const { status, reason } = failureInfo(cause)
    if (status === 409 && reason === 'SELF_RESET_CLOSE_POLICY_INVALID') {
      // This attempt rolled back. Older ordinary writes must still finish reconciliation.
      const ordinary = ordinaryPending(previous)
      if (ordinary) {
        remember({ ...ordinary, closeRejectedInvalidPolicy: true })
        await reconcile()
      } else {
        remember(null)
        const config = await readConfig()
        if (!disposed && config) error.value = t('userSubscriptions.selfReset.closePolicyInvalid')
      }
    } else if (unknownOutcome(status)) {
      await reconcile()
    } else {
      remember(previous)
      error.value = t('userSubscriptions.selfReset.closeFailed')
    }
  } finally {
    if (!disposed) saving.value = false
  }
}

async function save() {
  if (editingBlocked.value || revision.value === null || !limits.value) return
  if (closingRollout.value) return closeRollout()
  const changes = groupChanges()
  const hasGroups = Object.keys(changes).length > 0
  const hasPolicy = policyChanged()
  if ((hasPolicy || !initialPolicy.value && !hasGroups) && !limitsValid()) {
    error.value = t('userSubscriptions.selfReset.policyInvalid')
    return
  }
  if (!hasPolicy && !hasGroups) return close()
  const update: SelfResetConfigUpdate = { expected_revision: revision.value }
  if (hasGroups) update.changes_by_organization = changes
  if (hasPolicy) {
    update.policy = {
      daily_limit_by_organization: { ...limits.value } as SelfResetPolicy['daily_limit_by_organization'],
      rollout: rollout.value
    }
  }
  // A degraded snapshot can still repair a policy or edit groups. A normal snapshot needs both
  // parts again before editing resumes; a previously unreadable, untouched part can stay degraded.
  const tracked: PendingWrite = { revision: update.expected_revision }
  if (!initialPolicy.value && !hasPolicy) tracked.requiredPart = 'groups'
  if (!groupsByOrganization.value && !hasGroups) tracked.requiredPart = 'policy'
  remember(tracked)
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const result = await saveSelfResetConfig(update)
    if (!Number.isSafeInteger(result?.revision) || result.revision <= update.expected_revision) throw new Error('Unconfirmed revision')
    if (disposed) return
    remember(null)
    app.showSuccess(t('userSubscriptions.selfReset.policySaved'))
    emit('close')
  } catch (cause) {
    if (disposed) return
    const { status, reason } = failureInfo(cause)
    if (unknownOutcome(status)) return await reconcile()
    if (status === 409 && (reason === 'SELF_RESET_REVISION_CONFLICT' || reason === 'SELF_RESET_GROUP_CONFLICT')) {
      remember({ ...tracked, conflict: reason === 'SELF_RESET_GROUP_CONFLICT' ? 'groups' : 'revision' })
      await reconcile()
    } else {
      remember(null)
      error.value = t('userSubscriptions.selfReset.policySaveFailed')
    }
  } finally {
    if (!disposed) saving.value = false
  }
}

function retry() {
  if (saving.value || loading.value) return
  if (pending.value) void reconcile()
  else void readConfig(true)
}
onMounted(() => {
  if (pending.value) void reconcile()
  else void readConfig()
})
onBeforeUnmount(() => { disposed = true })
</script>
