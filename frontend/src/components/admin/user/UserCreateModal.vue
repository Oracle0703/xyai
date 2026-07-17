<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.createUser')"
    width="normal"
    @close="$emit('close')"
  >
    <form id="create-user-form" @submit.prevent="submit" class="space-y-5">
      <div>
        <label class="input-label">{{ t('admin.users.email') }}</label>
        <input v-model="form.email" type="email" required class="input" :placeholder="t('admin.users.enterEmail')" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.password') }}</label>
        <div class="flex gap-2">
          <div class="relative flex-1">
            <input v-model="form.password" type="text" required class="input pr-10" :placeholder="t('admin.users.enterPassword')" />
          </div>
          <button type="button" @click="generateRandomPassword" class="btn btn-secondary px-3">
            <Icon name="refresh" size="md" />
          </button>
        </div>
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.username') }}</label>
        <input v-model="form.username" type="text" class="input" :placeholder="t('admin.users.enterUsername')" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.form.roleLabel') }}</label>
        <select v-model="form.role" class="input" data-testid="role-select">
          <option value="user">{{ t('admin.users.roles.user') }}</option>
          <option value="sub_admin">{{ t('admin.users.roles.sub_admin') }}</option>
          <option value="admin">{{ t('admin.users.roles.admin') }}</option>
        </select>
      </div>
      <fieldset v-if="form.role === 'sub_admin'" class="space-y-2">
        <legend class="input-label">{{ t('admin.users.form.permissionsLabel') }}</legend>
        <p class="input-hint">{{ t('admin.users.form.permissionsHint') }}</p>
        <p v-if="permissionsLoading" class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('common.loading') }}
        </p>
        <p v-else-if="permissionsLoadFailed" class="text-sm text-red-600 dark:text-red-400">
          {{ t('admin.users.form.permissionsLoadFailed') }}
        </p>
        <label
          v-for="permission in adminPermissionOptions"
          :key="permission.code"
          class="flex items-start gap-3 rounded border border-gray-200 p-3 dark:border-dark-600"
        >
          <input
            v-model="form.admin_permissions"
            type="checkbox"
            :value="permission.code"
            :data-admin-permission="permission.code"
            class="mt-1"
          />
          <span>
            <span class="block text-sm font-medium">{{ t(permission.labelKey) }}</span>
            <span class="block text-xs text-gray-500 dark:text-gray-400">{{ t(permission.descriptionKey) }}</span>
          </span>
        </label>
      </fieldset>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('admin.users.columns.balance') }}</label>
          <input v-model="form.balance" type="number" step="any" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.users.columns.concurrency') }}</label>
          <input v-model.number="form.concurrency" type="number" class="input" />
        </div>
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.form.rpmLimit') }}</label>
        <input
          v-model.number="form.rpm_limit"
          type="number"
          min="0"
          step="1"
          class="input"
          :placeholder="t('admin.users.form.rpmLimitPlaceholder')"
        />
        <p class="input-hint">{{ t('admin.users.form.rpmLimitHint') }}</p>
      </div>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" type="button" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="create-user-form" :disabled="loading" class="btn btn-primary">
          {{ loading ? t('admin.users.creating') : t('common.create') }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <!-- 创建管理员账号时后端要求 step-up 2FA，弹出 TOTP 验证后自动重试 -->
  <TotpStepUpDialog :controller="stepUp" />
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'; import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useStepUp, isStepUpBlocked, isStepUpCancelled, stepUpBlockReason } from '@/composables/useStepUp'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import type { AdminPermission, UserRole } from '@/types'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits(['close', 'success']); const { t } = useI18n()
const appStore = useAppStore()

const adminPermissionOptions = ref<Array<{
  code: AdminPermission
  labelKey: string
  descriptionKey: string
}>>([])
const permissionsLoading = ref(false)
const permissionsLoadFailed = ref(false)

const loadPermissionCatalog = async () => {
  permissionsLoading.value = true
  permissionsLoadFailed.value = false
  try {
    const catalog = await adminAPI.users.getPermissionCatalog()
    adminPermissionOptions.value = catalog.map((item) => ({
      code: item.code,
      labelKey: `admin.users.permissions.${item.code}.label`,
      descriptionKey: `admin.users.permissions.${item.code}.description`,
    }))
  } catch {
    adminPermissionOptions.value = []
    permissionsLoadFailed.value = true
    appStore.showError(t('admin.users.form.permissionsLoadFailed'))
  } finally {
    permissionsLoading.value = false
  }
}

const form = reactive({ email: '', password: '', username: '', notes: '', role: 'user' as UserRole, admin_permissions: [] as AdminPermission[], balance: '', concurrency: 1, rpm_limit: 0 })

const stepUp = useStepUp()
const loading = ref(false)

const submit = async () => {
  if (loading.value) return
  loading.value = true
  try {
    const { balance: rawBalance, admin_permissions: requestedPermissions, ...rest } = { ...form }
    const balance = String(rawBalance).trim()
    const payload: typeof rest & { admin_permissions: AdminPermission[]; balance?: number } = {
      ...rest,
      admin_permissions: rest.role === 'sub_admin' ? [...requestedPermissions] : [],
    }
    if (balance !== '') {
      payload.balance = Number(balance)
    }
    // 创建管理员属敏感操作：后端返回 STEP_UP_REQUIRED 时弹 TOTP 验证并重试
    await stepUp.run(() => adminAPI.users.create(payload))
    appStore.showSuccess(t('admin.users.userCreated'))
    emit('success'); emit('close')
  } catch (e: any) {
    if (isStepUpCancelled(e)) {
      // 用户主动取消二次验证：静默返回，表单保持打开。
    } else if (isStepUpBlocked(e)) {
      appStore.showError(
        stepUpBlockReason(e) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN'
          ? t('stepUp.adminApiKeyForbidden')
          : t('stepUp.notEnabled')
      )
    } else {
      appStore.showError(e?.message || t('admin.users.failedToCreate'))
    }
  } finally { loading.value = false }
}

watch(() => props.show, (v) => {
  if (v) {
    Object.assign(form, { email: '', password: '', username: '', notes: '', role: 'user', admin_permissions: [], balance: '', concurrency: 1, rpm_limit: 0 })
    void loadPermissionCatalog()
  }
}, { immediate: true })

const generateRandomPassword = () => {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%^&*'
  let p = ''; for (let i = 0; i < 16; i++) p += chars.charAt(Math.floor(Math.random() * chars.length))
  form.password = p
}
</script>
