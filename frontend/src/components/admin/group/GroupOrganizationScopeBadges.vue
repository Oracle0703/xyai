<template>
  <div class="flex flex-wrap items-center gap-1" data-testid="group-organization-badges">
    <span v-if="status === 'error'" class="text-xs text-amber-600 dark:text-amber-400">
      {{ t('admin.groupOrganizationScope.unknown') }}
    </span>
    <span v-else-if="status === 'loading'" class="text-xs text-gray-400 dark:text-gray-500">
      {{ t('admin.groupOrganizationScope.loading') }}
    </span>
    <template v-else-if="keys.length">
      <span
        v-for="key in keys"
        :key="key"
        class="inline-block rounded-full bg-sky-100 px-2 py-0.5 text-xs font-medium text-sky-700 dark:bg-sky-900/30 dark:text-sky-300"
      >
        {{ formatOrganizationUsageOrganization(key) }}
      </span>
    </template>
    <span v-else class="text-xs text-gray-400 dark:text-gray-500">
      {{ t('admin.groupOrganizationScope.unscoped') }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { GroupOrganizationKey } from '@/api/admin/groupOrganizationScopes'
import { formatOrganizationUsageOrganization } from '@/utils/organizations'

// An unknown scope (load failed or not finished) is never shown as "no
// organization" (K24).
withDefaults(defineProps<{ keys: readonly GroupOrganizationKey[]; status?: 'ready' | 'loading' | 'error' }>(), { status: 'ready' })
const { t } = useI18n()
</script>
