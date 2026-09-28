<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="card p-4 sm:p-6">
          <div class="flex flex-wrap items-end justify-between gap-4">
            <div class="flex flex-1 flex-wrap items-end gap-4">
              <div class="w-full sm:w-auto sm:min-w-[220px]">
                <label class="input-label">{{ t('userSubscriptions.selfReset.eventsEmail') }}</label>
                <input v-model.trim="filters.email" type="text" class="input" @keyup.enter="search" />
              </div>
              <div class="w-full sm:w-auto sm:min-w-[160px]">
                <label class="input-label">{{ t('userSubscriptions.selfReset.eventsOrganization') }}</label>
                <Select v-model="filters.organization" :options="organizationOptions" @change="search" />
              </div>
              <div class="w-full sm:w-auto">
                <label class="input-label">{{ t('userSubscriptions.selfReset.eventsStartDate') }}</label>
                <input v-model="filters.start_date" type="date" class="input" @change="search" />
              </div>
              <div class="w-full sm:w-auto">
                <label class="input-label">{{ t('userSubscriptions.selfReset.eventsEndDate') }}</label>
                <input v-model="filters.end_date" type="date" class="input" @change="search" />
              </div>
            </div>
            <div class="flex flex-wrap items-center gap-3">
              <button type="button" class="btn btn-primary" :disabled="loading" @click="search">{{ t('common.search') }}</button>
              <button type="button" class="btn btn-secondary" :disabled="loading" @click="resetFilters">{{ t('common.reset') }}</button>
            </div>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="events" :loading="loading" row-key="id">
          <template #cell-created_at="{ value }">
            <span class="whitespace-nowrap">{{ formatDateTime(value) }}</span>
          </template>
          <template #cell-user="{ row }">
            <div class="font-medium text-gray-900 dark:text-white">{{ row.user_email || `#${row.user_id}` }}</div>
            <div class="text-xs text-gray-400">{{ t(`userSubscriptions.selfReset.${row.organization}`) }}</div>
          </template>
          <template #cell-subscription="{ row }">
            <div>#{{ row.subscription_id }}</div>
            <div class="text-xs text-gray-400">{{ row.group_name || `#${row.group_id}` }}</div>
          </template>
          <template #cell-count="{ row }">{{ row.used_count }} / {{ row.daily_limit }}</template>
          <template #cell-daily_usage_usd_before="{ value }">${{ Number(value).toFixed(2) }}</template>
          <template #empty>
            <p class="py-8 text-center text-sm text-gray-500">{{ t('userSubscriptions.selfReset.eventsEmpty') }}</p>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="total > 0"
          :total="total"
          :page="page"
          :page-size="pageSize"
          @update:page="onPageChange"
          @update:pageSize="onPageSizeChange"
        />
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import type { Column } from '@/components/common/types'
import { listSelfResetEvents, type SelfResetEvent } from '@/api/subscriptionSelfReset'
import { formatDateTime } from '@/utils/format'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const events = ref<SelfResetEvent[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const filters = reactive({ email: '', organization: '', start_date: '', end_date: '' })

const organizationOptions = computed(() => [
  { value: '', label: t('userSubscriptions.selfReset.eventsAllOrganizations') },
  ...(['xunyou', 'wsdashi', 'other'] as const).map((org) => ({ value: org, label: t(`userSubscriptions.selfReset.${org}`) }))
])

const columns = computed<Column[]>(() => [
  { key: 'created_at', label: t('userSubscriptions.selfReset.eventsTime') },
  { key: 'user', label: t('userSubscriptions.selfReset.eventsUser') },
  { key: 'subscription', label: t('userSubscriptions.selfReset.eventsSubscription') },
  { key: 'quota_date', label: t('userSubscriptions.selfReset.eventsQuotaDate') },
  { key: 'count', label: t('userSubscriptions.selfReset.eventsCount') },
  { key: 'daily_usage_usd_before', label: t('userSubscriptions.selfReset.eventsUsageBefore') }
])

async function load() {
  loading.value = true
  try {
    const query = Object.fromEntries(Object.entries(filters).filter(([, value]) => value !== ''))
    const res = await listSelfResetEvents(page.value, pageSize.value, query)
    events.value = res.items
    total.value = res.total
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('common.error'))
  } finally {
    loading.value = false
  }
}

function search() {
  page.value = 1
  load()
}

function resetFilters() {
  Object.assign(filters, { email: '', organization: '', start_date: '', end_date: '' })
  search()
}

function onPageChange(value: number) {
  page.value = value
  load()
}

function onPageSizeChange(value: number) {
  pageSize.value = value
  search()
}

onMounted(load)
</script>
