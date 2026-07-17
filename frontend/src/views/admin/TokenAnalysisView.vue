<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="card p-4">
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-8">
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('common.startDate') }}</span>
            <input v-model="filters.start_date" type="date" class="input h-9 text-sm" @change="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('common.endDate') }}</span>
            <input v-model="filters.end_date" type="date" class="input h-9 text-sm" @change="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">User ID</span>
            <input v-model.number="filters.user_id" type="number" min="1" class="input h-9 text-sm" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.tokenAnalysis.userEmail') }}</span>
            <input v-model.trim="filters.user_email" type="text" class="input h-9 text-sm" :placeholder="t('admin.tokenAnalysis.userEmailHint')" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">API Key ID</span>
            <input v-model.number="filters.api_key_id" type="number" min="1" class="input h-9 text-sm" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Account ID</span>
            <input v-model.number="filters.account_id" type="number" min="1" class="input h-9 text-sm" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Group ID</span>
            <input v-model.number="filters.group_id" type="number" min="1" class="input h-9 text-sm" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Model</span>
            <input v-model.trim="filters.model" class="input h-9 text-sm" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.tokenAnalysis.project') }}</span>
            <input v-model.trim="filters.project" class="input h-9 text-sm" :placeholder="t('admin.tokenAnalysis.projectFilterHint')" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.tokenAnalysis.riskMin') }}</span>
            <select v-model.number="filters.risk_min" class="input h-9 text-sm" @change="reloadAll">
              <option :value="0">0</option>
              <option :value="20">20</option>
              <option :value="40">40</option>
              <option :value="60">60</option>
            </select>
          </label>
          <label class="flex items-end gap-2 pb-2 text-sm text-gray-600 dark:text-gray-300">
            <input v-model="filters.include_unmatched" type="checkbox" class="h-4 w-4 rounded border-gray-300" @change="reloadAll" />
            <span>{{ t('admin.tokenAnalysis.includeUnmatched') }}</span>
          </label>
          <div class="flex items-end gap-2">
            <button type="button" class="btn btn-secondary h-9 flex-1 text-sm" @click="reloadAll">
              {{ t('common.refresh') }}
            </button>
            <button v-if="canTriggerIndex" type="button" class="btn btn-primary h-9 flex-1 text-sm" :disabled="indexing" @click="triggerIndex">
              {{ indexing ? t('common.loading') : t('admin.tokenAnalysis.indexNow') }}
            </button>
          </div>
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3 lg:grid-cols-6">
        <div v-for="card in summaryCards" :key="card.label" class="card p-4">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ card.label }}</div>
          <div class="mt-2 truncate text-xl font-semibold text-gray-900 dark:text-white" :title="card.title">{{ card.value }}</div>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div class="card p-4">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.matchQuality') }}</h2>
          <div class="mt-3 grid grid-cols-3 gap-3 text-sm">
            <div>
              <div class="text-xs text-gray-500">{{ t('admin.tokenAnalysis.matched') }}</div>
              <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ formatNumber(summary?.matched_requests ?? 0) }}</div>
            </div>
            <div>
              <div class="text-xs text-gray-500">{{ t('admin.tokenAnalysis.unmatched') }}</div>
              <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ formatNumber(summary?.unmatched_requests ?? 0) }}</div>
            </div>
            <div>
              <div class="text-xs text-gray-500">{{ t('admin.tokenAnalysis.unmatchedRate') }}</div>
              <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ percent(summary?.unmatched_rate ?? 0) }}</div>
            </div>
          </div>
        </div>

        <div class="card p-4 lg:col-span-2">
          <div class="mb-3 flex items-center justify-between gap-3">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.riskReasons') }}</h2>
            <span class="text-xs text-gray-500">{{ t('admin.tokenAnalysis.riskRate') }} {{ percent(summary?.risk_request_rate ?? 0) }}</span>
          </div>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="reason in summary?.risk_reasons || []"
              :key="reason.code"
              type="button"
              class="rounded border px-2.5 py-1 text-xs"
              :class="filters.risk_reason === reason.code ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'border-gray-200 bg-white text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300'"
              :title="riskDesc(reason.code) || reason.code"
              @click="toggleRiskReason(reason.code)"
            >
              {{ riskLabel(reason.code) }} · {{ formatNumber(reason.count) }}
            </button>
            <span v-if="!(summary?.risk_reasons || []).length" class="text-sm text-gray-500">{{ t('common.noData') }}</span>
          </div>
        </div>
      </div>

      <div class="card p-4">
        <button type="button" class="mb-1 flex w-full items-center justify-between text-left" @click="showRiskLegend = !showRiskLegend">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.riskLegend') }}</h2>
          <span class="text-xs text-gray-400">{{ showRiskLegend ? '▾' : '▸' }}</span>
        </button>
        <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.tokenAnalysis.riskLegendHint') }}</p>
        <div v-if="showRiskLegend" class="grid grid-cols-1 gap-x-6 gap-y-2.5 md:grid-cols-2">
          <div v-for="code in RISK_CODES" :key="code" class="flex items-start gap-2">
            <span class="mt-0.5 shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
              {{ riskLabel(code) }}
            </span>
            <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ riskDesc(code) }}</p>
          </div>
        </div>
      </div>

      <div class="card p-4">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.tokenAnalysis.indexStatus') }}:
            <span class="font-medium text-gray-800 dark:text-gray-100">{{ indexStatusText }}</span>
          </div>
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.tokenAnalysis.processed') }} {{ indexStatus?.processed_rows ?? 0 }} / {{ t('admin.tokenAnalysis.failed') }} {{ indexStatus?.failed_rows ?? 0 }}
          </div>
        </div>
        <div v-if="indexStatus?.files?.length" class="mt-3 overflow-x-auto">
          <table class="table text-xs">
            <thead>
              <tr>
                <th>{{ t('admin.tokenAnalysis.file') }}</th>
                <th>{{ t('admin.tokenAnalysis.offset') }}</th>
                <th>{{ t('admin.tokenAnalysis.processed') }}</th>
                <th>{{ t('admin.tokenAnalysis.failed') }}</th>
                <th>{{ t('admin.tokenAnalysis.updated') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="file in indexStatus.files.slice(0, 5)" :key="file.source_file">
                <td class="max-w-[320px] truncate">{{ file.source_file }}</td>
                <td>{{ formatNumber(file.last_offset) }}</td>
                <td>{{ formatNumber(file.processed_rows) }}</td>
                <td>{{ formatNumber(file.failed_rows) }}</td>
                <td class="whitespace-nowrap">{{ formatTimeCN(file.updated_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="card p-4">
        <div class="mb-1 flex items-center justify-between">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.archiveFiles') }}</h2>
          <span class="text-xs text-gray-500">{{ archiveFiles.length }}</span>
        </div>
        <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.tokenAnalysis.archiveFilesHint') }}</p>
        <div class="overflow-x-auto">
          <table class="table text-xs">
            <thead>
              <tr>
                <th>{{ t('admin.tokenAnalysis.file') }}</th>
                <th>{{ t('admin.tokenAnalysis.fileSize') }}</th>
                <th>{{ t('admin.tokenAnalysis.indexProgress') }}</th>
                <th>{{ t('admin.tokenAnalysis.processed') }} / {{ t('admin.tokenAnalysis.failed') }}</th>
                <th>{{ t('admin.tokenAnalysis.fileStatus') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="file in archiveFiles" :key="file.name">
                <td class="font-medium">{{ file.name }}</td>
                <td>{{ formatBytes(file.size_bytes) }}</td>
                <td :title="archiveFileProgressDetail(file)">{{ archiveFileProgress(file) }}</td>
                <td>
                  {{ formatNumber(file.processed_rows) }} / {{ formatNumber(file.failed_rows) }}
                  <span v-if="archiveFileFullyRead(file)" class="text-gray-400">
                    · {{ t('admin.tokenAnalysis.requestRowsTotal', { n: formatNumber(file.processed_rows + file.failed_rows) }) }}
                  </span>
                </td>
                <td>
                  <span
                    class="inline-flex rounded-full px-2 py-0.5 text-xs font-semibold"
                    :class="archiveFileStatusClass(file.status)"
                    :title="t(`admin.tokenAnalysis.archiveFileStatusHint.${file.status}`)"
                  >
                    {{ t(`admin.tokenAnalysis.archiveFileStatus.${file.status}`) }}
                  </span>
                  <div v-if="file.last_error" class="mt-1 max-w-[280px] truncate text-xs text-red-500" :title="file.last_error">{{ file.last_error }}</div>
                </td>
              </tr>
              <tr v-if="archiveFiles.length === 0">
                <td colspan="5" class="py-4 text-center text-gray-500">{{ t('common.noData') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="card p-4">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.projectRanking') }}</h2>
          <span class="text-xs text-gray-500">{{ projectsPagination.total }}</span>
        </div>
        <div class="overflow-x-auto">
          <table class="table text-sm">
            <thead>
              <tr>
                <th>{{ t('admin.tokenAnalysis.project') }}</th>
                <th>{{ t('admin.tokenAnalysis.user') }}</th>
                <th>
                  <button type="button" class="inline-flex items-center gap-0.5 hover:text-primary-600" @click="changeProjectSort('request_count')">
                    {{ t('admin.tokenAnalysis.requests') }}
                    <span :class="projectSort === 'request_count' ? 'text-primary-600' : 'text-gray-400'">{{ projectSortIcon('request_count') }}</span>
                  </button>
                </th>
                <th>
                  <button type="button" class="inline-flex items-center gap-0.5 hover:text-primary-600" @click="changeProjectSort('total_tokens')">
                    {{ t('admin.tokenAnalysis.tokens') }}
                    <span :class="projectSort === 'total_tokens' ? 'text-primary-600' : 'text-gray-400'">{{ projectSortIcon('total_tokens') }}</span>
                  </button>
                </th>
                <th>{{ t('admin.tokenAnalysis.inputOutput') }}</th>
                <th>{{ t('admin.tokenAnalysis.cacheTokens') }}</th>
                <th>{{ t('admin.tokenAnalysis.outputInputRatio') }}</th>
                <th>
                  <button type="button" class="inline-flex items-center gap-0.5 hover:text-primary-600" @click="changeProjectSort('last_event_time')">
                    {{ t('admin.tokenAnalysis.lastActive') }}
                    <span :class="projectSort === 'last_event_time' ? 'text-primary-600' : 'text-gray-400'">{{ projectSortIcon('last_event_time') }}</span>
                  </button>
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in projects" :key="`${row.project}-${row.user_id || 0}`">
                <td>
                  <button
                    type="button"
                    class="max-w-[200px] truncate font-medium hover:text-primary-600"
                    :class="row.project === 'unattributed' ? 'italic text-gray-400' : ''"
                    :title="row.project"
                    @click="toggleProjectFilter(row.project)"
                  >
                    {{ row.project === 'unattributed' ? t('admin.tokenAnalysis.unattributed') : row.project }}
                  </button>
                </td>
                <td>
                  <div class="max-w-[180px] truncate">{{ row.user_email || '-' }}</div>
                </td>
                <td>
                  <div>{{ formatNumber(row.request_count) }}</div>
                  <div class="text-xs text-gray-500">{{ t('admin.tokenAnalysis.matched') }} {{ formatNumber(row.matched_request_count) }}</div>
                </td>
                <td class="font-medium" :title="formatNumber(row.total_tokens)">{{ formatCompactNumber(row.total_tokens) }}</td>
                <td class="text-xs text-gray-500" :title="`${formatNumber(row.input_tokens)} / ${formatNumber(row.output_tokens)}`">
                  {{ formatCompactNumber(row.input_tokens) }} / {{ formatCompactNumber(row.output_tokens) }}
                </td>
                <td class="text-xs text-gray-500" :title="formatNumber(row.cache_read_tokens + row.cache_creation_tokens)">
                  {{ formatCompactNumber(row.cache_read_tokens + row.cache_creation_tokens) }}
                </td>
                <td
                  class="text-xs font-semibold"
                  :class="projectOutputInputRatioClass(row)"
                  :data-test="projectOutputInputRatioDataTest(row)"
                  :title="projectOutputInputRatioTitle(row)"
                >
                  {{ formatProjectOutputInputRatio(row) }}
                </td>
                <td class="whitespace-nowrap text-xs text-gray-500">{{ formatTimeCN(row.last_event_time) }}</td>
              </tr>
              <tr v-if="!projectsLoading && projects.length === 0">
                <td colspan="8" class="py-8 text-center text-gray-500">{{ t('common.noData') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          v-if="projectsPagination.total > 0"
          :page="projectsPagination.page"
          :page-size="projectsPagination.page_size"
          :total="projectsPagination.total"
          class="mt-3"
          @update:page="changeProjectPage"
          @update:pageSize="changeProjectPageSize"
        />
      </div>

      <div class="grid grid-cols-1 gap-4 xl:grid-cols-3">
        <div class="card p-4 xl:col-span-1">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.userRanking') }}</h2>
            <span class="text-xs text-gray-500">{{ usersPagination.total }}</span>
          </div>
          <div class="overflow-x-auto">
            <table class="table text-sm">
              <thead>
                <tr>
                  <th class="w-8"></th>
                  <th class="w-10">#</th>
                  <th>{{ t('admin.tokenAnalysis.user') }}</th>
                  <th>{{ t('admin.tokenAnalysis.tokens') }}</th>
                  <th>{{ t('admin.tokenAnalysis.cost') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(user, index) in users" :key="user.user_id || 0">
                  <td class="w-8">
                    <input
                      v-if="user.user_id"
                      data-user-trend-select
                      type="checkbox"
                      class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                      :checked="isTrendUserSelected(user.user_id)"
                      :disabled="trendUserSelectionDisabled(user.user_id)"
                      :aria-label="t('admin.tokenAnalysis.selectUserTrend', { email: user.user_email || user.user_id })"
                      @change="toggleTrendUser(user)"
                    />
                    <span v-else class="text-gray-300">-</span>
                  </td>
                  <td class="tabular-nums text-gray-500">{{ userRankingRank(index) }}</td>
                  <td>
                    <div class="max-w-[180px] truncate font-medium">{{ user.user_email || '-' }}</div>
                  </td>
                  <td class="tabular-nums" :title="formatNumber(user.total_tokens)">{{ formatUserRankingTokens(user.total_tokens) }}</td>
                  <td class="tabular-nums" :title="formatCost(user.actual_cost)">{{ formatUserRankingCost(user.actual_cost) }}</td>
                </tr>
                <tr v-if="!usersLoading && users.length === 0">
                  <td colspan="5" class="py-8 text-center text-gray-500">{{ t('common.noData') }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <Pagination
            v-if="usersPagination.total > 0"
            :page="usersPagination.page"
            :page-size="usersPagination.page_size"
            :total="usersPagination.total"
            class="mt-3"
            @update:page="changeUserPage"
            @update:pageSize="changeUserPageSize"
          />
        </div>

        <div class="card p-4 xl:col-span-2">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.requestDetails') }}</h2>
            <div class="flex items-center gap-2">
              <select v-model="requestSort" class="input h-8 w-auto text-xs" @change="changeRequestSort">
                <option value="event_time">{{ t('admin.tokenAnalysis.sortByTime') }}</option>
                <option value="risk_score">{{ t('admin.tokenAnalysis.sortByRisk') }}</option>
              </select>
              <span class="text-xs text-gray-500">{{ requestsPagination.total }}</span>
            </div>
          </div>
          <div class="overflow-x-auto">
            <table class="table text-sm">
              <thead>
                <tr>
                  <th>{{ t('admin.tokenAnalysis.time') }}</th>
                  <th>{{ t('admin.tokenAnalysis.user') }}</th>
                  <th>Model</th>
                  <th>{{ t('admin.tokenAnalysis.project') }}</th>
                  <th>{{ t('admin.tokenAnalysis.usage') }}</th>
                  <th>{{ t('admin.tokenAnalysis.quality') }}</th>
                  <th>{{ t('admin.tokenAnalysis.preview') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in requests" :key="item.id" class="cursor-pointer" @click="selectRequest(item)">
                  <td class="whitespace-nowrap text-xs text-gray-600 dark:text-gray-300">{{ formatDateTime(item.event_time) }}</td>
                  <td>
                    <div class="max-w-[160px] truncate font-medium">{{ item.user_email || '-' }}</div>
                    <div class="text-xs text-gray-500">{{ item.api_key_name || '-' }}</div>
                  </td>
                  <td>
                    <div class="max-w-[160px] truncate">{{ item.model }}</div>
                    <div class="text-xs text-gray-500">{{ item.endpoint }}</div>
                  </td>
                  <td>
                    <div class="max-w-[150px] truncate">{{ item.client_project || t('admin.tokenAnalysis.unattributed') }}</div>
                    <div class="max-w-[150px] truncate text-xs text-gray-500">{{ item.client_branch || '-' }}</div>
                  </td>
                  <td>
                    <div>{{ formatNumber(item.input_tokens + item.output_tokens + item.cache_read_tokens + item.cache_creation_tokens) }}</div>
                    <div class="text-xs text-gray-500">{{ formatCost(item.actual_cost) }}</div>
                  </td>
                  <td>
                    <span v-if="item.quality_score !== undefined && item.quality_score !== null" class="inline-flex min-w-10 justify-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-semibold text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
                      {{ item.quality_score }}
                    </span>
                    <span v-else-if="item.has_input" class="text-xs text-gray-400">{{ t('admin.tokenAnalysis.qualityNotEvaluated') }}</span>
                    <span v-else class="text-xs text-gray-400">-</span>
                  </td>
                  <td>
                    <div class="max-w-[300px] truncate">{{ item.last_user_preview }}</div>
                    <div class="mt-1 flex flex-wrap items-center gap-1">
                      <span
                        v-if="(item.duplicate_count ?? 0) > 1"
                        class="rounded-full bg-purple-100 px-1.5 py-0.5 text-xs font-semibold text-purple-700 dark:bg-purple-900/30 dark:text-purple-300"
                        :title="t('admin.tokenAnalysis.duplicateHint')"
                      >
                        ×{{ item.duplicate_count }}
                      </span>
                      <span
                        v-if="item.request_body_truncated"
                        class="rounded-full bg-amber-100 px-1.5 py-0.5 text-xs font-semibold text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
                        :title="t('admin.tokenAnalysis.bodyTruncatedHint')"
                      >
                        {{ t('admin.tokenAnalysis.bodyTruncated') }}
                      </span>
                      <span v-if="item.risk_score > 0" class="inline-flex min-w-8 justify-center rounded-full px-1.5 py-0.5 text-xs font-semibold" :class="riskClass(item.risk_score)">
                        {{ item.risk_score }}
                      </span>
                      <span
                        v-for="reason in item.risk_reasons || []"
                        :key="reason.code"
                        class="cursor-help rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                        :title="riskDesc(reason.code) || reason.message || reason.code"
                      >
                        {{ riskLabel(reason.code) }}
                      </span>
                    </div>
                  </td>
                </tr>
                <tr v-if="!requestsLoading && requests.length === 0">
                  <td colspan="7" class="py-8 text-center text-gray-500">{{ t('common.noData') }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <Pagination
            v-if="requestsPagination.total > 0"
            :page="requestsPagination.page"
            :page-size="requestsPagination.page_size"
            :total="requestsPagination.total"
            class="mt-3"
            @update:page="changeRequestPage"
            @update:pageSize="changeRequestPageSize"
          />
        </div>
      </div>

      <div v-if="selectedTrendUsers.length > 0" class="card p-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.usageTrend') }}</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.tokenAnalysis.selectedUsersCount', { count: selectedTrendUsers.length }) }}
            </p>
          </div>
          <div class="inline-flex overflow-hidden rounded-md border border-gray-200 dark:border-dark-700">
            <button
              type="button"
              class="h-8 px-3 text-xs font-medium"
              :class="userTrendGranularity === 'day' ? 'bg-primary-600 text-white' : 'bg-white text-gray-600 hover:bg-gray-50 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700'"
              @click="setUserTrendGranularity('day')"
            >
              {{ t('admin.tokenAnalysis.trendDay') }}
            </button>
            <button
              type="button"
              class="h-8 border-l border-gray-200 px-3 text-xs font-medium dark:border-dark-700"
              :class="userTrendGranularity === 'hour' ? 'bg-primary-600 text-white' : 'bg-white text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700'"
              :disabled="!canUseHourlyTrend"
              :title="canUseHourlyTrend ? '' : t('admin.tokenAnalysis.trendHourSingleDayHint')"
              @click="setUserTrendGranularity('hour')"
            >
              {{ t('admin.tokenAnalysis.trendHour') }}
            </button>
          </div>
        </div>

        <div v-if="userTrendLoading" class="mt-4 flex h-72 items-center justify-center text-sm text-gray-500 md:h-80">
          {{ t('common.loading') }}
        </div>
        <div v-else-if="userTrendError" class="mt-4 flex h-72 flex-col items-center justify-center gap-3 text-center md:h-80">
          <p class="text-sm text-red-600 dark:text-red-400">{{ t('admin.tokenAnalysis.trendLoadFailed') }}</p>
          <button type="button" class="btn btn-secondary btn-sm" @click="loadSelectedUserTrend">
            {{ t('admin.tokenAnalysis.trendRetry') }}
          </button>
        </div>
        <div v-else class="mt-4">
          <p v-if="userTrendPoints.length === 0" class="mb-2 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.tokenAnalysis.trendNoUsage') }}
          </p>
          <div class="h-72 md:h-80">
            <Line :data="selectedUserTrendChartData" :options="selectedUserTrendChartOptions" />
          </div>
        </div>
      </div>
    </div>

    <div v-if="selectedRequest" class="fixed inset-0 z-40 bg-black/20" @click="selectedRequest = null"></div>
    <aside v-if="selectedRequest" class="fixed right-0 top-0 z-50 h-full w-full max-w-xl overflow-y-auto border-l border-gray-200 bg-white p-5 shadow-xl dark:border-dark-700 dark:bg-dark-900">
      <div class="mb-4 flex items-start justify-between gap-3">
        <div>
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ selectedRequest.archive_id }}</h2>
          <p class="text-sm text-gray-500">{{ selectedRequest.event_time }}</p>
        </div>
        <button type="button" class="btn btn-ghost btn-sm" @click="selectedRequest = null">{{ t('common.close') }}</button>
      </div>
      <div v-if="extractedUserRequest" class="mb-5">
        <div class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.userRequest') }}</div>
        <div class="max-h-72 overflow-y-auto whitespace-pre-wrap break-words rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm text-gray-800 dark:border-blue-900/50 dark:bg-blue-950/30 dark:text-gray-100">{{ extractedUserRequest }}</div>
        <div class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.tokenAnalysis.userRequestHint') }}</div>
      </div>
      <dl class="grid grid-cols-2 gap-3 text-sm">
        <template v-for="field in detailFields" :key="field.label">
          <dt class="text-gray-500">{{ field.label }}</dt>
          <dd class="min-w-0 break-words font-medium text-gray-900 dark:text-gray-100">{{ field.value }}</dd>
        </template>
      </dl>
      <div class="mt-5">
        <div class="mb-2 flex flex-wrap items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
          <span>{{ t('admin.tokenAnalysis.inputFull') }}</span>
          <span v-if="requestInput?.truncated" class="rounded bg-amber-100 px-1.5 py-0.5 text-xs font-normal text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
            {{ t('admin.tokenAnalysis.inputTruncatedNote') }}
          </span>
          <span v-if="requestInput" class="text-xs font-normal text-gray-500">{{ formatNumber(requestInput.chars) }} {{ t('admin.tokenAnalysis.chars') }}</span>
        </div>
        <div v-if="requestInputLoading" class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800">
          {{ t('common.loading') }}
        </div>
        <div v-else-if="requestInput" class="max-h-96 overflow-y-auto whitespace-pre-wrap break-words rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200">{{ requestInput.content }}</div>
        <div v-else class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200">
          {{ selectedRequest.last_user_preview || t('admin.tokenAnalysis.noInputStored') }}
        </div>
        <div
          v-if="requestInput?.content_sha256"
          class="mt-1.5 break-all font-mono text-xs text-gray-400 dark:text-gray-500"
          :title="t('admin.tokenAnalysis.contentHashHint')"
        >
          {{ t('admin.tokenAnalysis.contentHash') }}: {{ requestInput.content_sha256 }}
        </div>
      </div>
      <div class="mt-5">
        <div class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.quality') }}</div>
        <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-700 dark:bg-dark-800">
          <template v-if="requestInput && requestInput.quality_score !== undefined && requestInput.quality_score !== null">
            <div class="flex items-center gap-2">
              <span class="inline-flex min-w-10 justify-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-semibold text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
                {{ requestInput.quality_score }}
              </span>
              <span v-if="requestInput.quality_version" class="text-xs text-gray-500">{{ requestInput.quality_version }}</span>
              <span v-if="requestInput.evaluated_at" class="text-xs text-gray-500">{{ formatDateTime(requestInput.evaluated_at) }}</span>
            </div>
            <pre v-if="requestInput.quality_findings && Object.keys(requestInput.quality_findings).length > 0" class="mt-2 max-h-48 overflow-y-auto whitespace-pre-wrap break-words text-xs text-gray-600 dark:text-gray-300">{{ JSON.stringify(requestInput.quality_findings, null, 2) }}</pre>
          </template>
          <span v-else class="text-gray-500">{{ t('admin.tokenAnalysis.qualityNotEvaluated') }}</span>
        </div>
      </div>
    </aside>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  type ChartData,
  type ChartOptions
} from 'chart.js'
import { Line } from 'vue-chartjs'
import { adminAPI } from '@/api/admin'
import type {
  TokenAnalysisArchiveFile,
  TokenAnalysisArchiveFileStatus,
  TokenAnalysisIndexStatus,
  TokenAnalysisProjectUsage,
  TokenAnalysisQueryParams,
  TokenAnalysisRequestInput,
  TokenAnalysisRequestItem,
  TokenAnalysisSummary,
  TokenAnalysisUserUsage
} from '@/api/admin/tokenAnalysis'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { UserUsageTrendPoint } from '@/types'
import { formatBytes, formatCompactNumber, formatDateTime } from '@/utils/format'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend)

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const canTriggerIndex = computed(() => authStore.isAdmin)

const today = new Date().toISOString().slice(0, 10)
const filters = reactive<TokenAnalysisQueryParams>({
  start_date: today,
  end_date: today,
  timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
  // 请求明细默认展示全部请求(风险排查时再调高风险分过滤)。
  risk_min: 0,
  include_unmatched: false
})

const summary = ref<TokenAnalysisSummary | null>(null)
const users = ref<TokenAnalysisUserUsage[]>([])
const projects = ref<TokenAnalysisProjectUsage[]>([])
const requests = ref<TokenAnalysisRequestItem[]>([])
const indexStatus = ref<TokenAnalysisIndexStatus | null>(null)
const archiveFiles = ref<TokenAnalysisArchiveFile[]>([])
const selectedRequest = ref<TokenAnalysisRequestItem | null>(null)
const requestSort = ref<'event_time' | 'risk_score'>('event_time')
// 风险说明图例常驻展示(可折叠), 顺序与 ScoreTokenAnalysisRisk 中的扣分权重一致。
const showRiskLegend = ref(true)
const RISK_CODES = [
  'huge_input_tiny_output',
  'repeat_uncached_body',
  'low_cache_hit_large_input',
  'rapid_similar_requests',
  'oversized_system_prompt',
  'tool_heavy_short_output',
  'large_tool_history',
  'giant_tool_output'
] as const
// 项目排行支持按请求数/token/最近活动服务端排序(列表是分页的, 前端排序无意义)。
type ProjectSortField = 'request_count' | 'total_tokens' | 'last_event_time'
const projectSort = ref<ProjectSortField>('total_tokens')
const projectSortOrder = ref<'asc' | 'desc'>('desc')
const requestInput = ref<TokenAnalysisRequestInput | null>(null)
const requestInputLoading = ref(false)
const usersLoading = ref(false)
const projectsLoading = ref(false)
const requestsLoading = ref(false)
const indexing = ref(false)

const usersPagination = reactive({ page: 1, page_size: 20, total: 0 })
const projectsPagination = reactive({ page: 1, page_size: 20, total: 0 })
const requestsPagination = reactive({ page: 1, page_size: 20, total: 0 })
const PROJECT_OUTPUT_INPUT_HEALTHY_RATIO = 0.05
const MAX_SELECTED_TREND_USERS = 5
const USER_TREND_COLORS = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6']

type TrendGranularity = 'day' | 'hour'
type SelectedTrendUser = { user_id: number; email: string }

const selectedTrendUsers = ref<SelectedTrendUser[]>([])
const userTrendPoints = ref<UserUsageTrendPoint[]>([])
const userTrendGranularity = ref<TrendGranularity>('day')
const userTrendLoading = ref(false)
const userTrendError = ref(false)
let userTrendLoadSeq = 0

const cleanFilters = computed(() => {
  const out: TokenAnalysisQueryParams = {}
  for (const [key, value] of Object.entries(filters)) {
    if (value !== '' && value !== undefined && value !== null) {
      (out as Record<string, unknown>)[key] = value
    }
  }
  return out
})

const summaryCards = computed(() => [
  {
    label: t('admin.tokenAnalysis.summary.totalRequests'),
    value: formatRequestMetric(summary.value?.total_requests ?? 0),
    title: formatNumber(summary.value?.total_requests ?? 0)
  },
  // 同期计费请求数(usage_logs)与归档覆盖率: 直观呈现归档样本与真实
  // 计费请求量的差距, 避免把"已归档请求数"误读为全量请求数。
  {
    label: t('admin.tokenAnalysis.summary.billedRequests'),
    value: formatRequestMetric(summary.value?.billed_requests ?? 0),
    title: formatNumber(summary.value?.billed_requests ?? 0)
  },
  {
    label: t('admin.tokenAnalysis.summary.archiveCoverage'),
    value: percent(summary.value?.archive_coverage ?? 0),
    title: percent(summary.value?.archive_coverage ?? 0)
  },
  {
    label: t('admin.tokenAnalysis.summary.totalTokens'),
    value: formatTokenMetric(summary.value?.total_tokens ?? 0),
    title: formatNumber(summary.value?.total_tokens ?? 0)
  },
  {
    label: t('admin.tokenAnalysis.summary.inputTokens'),
    value: formatTokenMetric(summary.value?.total_input_tokens ?? 0),
    title: formatNumber(summary.value?.total_input_tokens ?? 0)
  },
  {
    label: t('admin.tokenAnalysis.summary.outputTokens'),
    value: formatTokenMetric(summary.value?.total_output_tokens ?? 0),
    title: formatNumber(summary.value?.total_output_tokens ?? 0)
  },
  {
    label: t('admin.tokenAnalysis.summary.totalCost'),
    value: formatCost(summary.value?.total_actual_cost ?? 0),
    title: formatCost(summary.value?.total_actual_cost ?? 0)
  },
  {
    label: t('admin.tokenAnalysis.summary.cacheRead'),
    value: formatTokenMetric(summary.value?.cache_read_tokens ?? 0),
    title: formatNumber(summary.value?.cache_read_tokens ?? 0)
  },
  {
    label: t('admin.tokenAnalysis.summary.cacheHitRate'),
    value: percent(summary.value?.cache_hit_rate ?? 0),
    title: percent(summary.value?.cache_hit_rate ?? 0)
  },
  {
    label: t('admin.tokenAnalysis.summary.riskyRequests'),
    value: formatRequestMetric(summary.value?.risky_requests ?? 0),
    title: formatNumber(summary.value?.risky_requests ?? 0)
  },
  {
    label: t('admin.tokenAnalysis.summary.riskyCost'),
    value: formatCost(summary.value?.risky_cost ?? 0),
    title: formatCost(summary.value?.risky_cost ?? 0)
  }
])

const canUseHourlyTrend = computed(() => {
  return Boolean(filters.start_date && filters.end_date && filters.start_date === filters.end_date)
})

function buildTrendLabels(startDate: string, endDate: string, granularity: TrendGranularity): string[] {
  if (!startDate || !endDate) return []
  if (granularity === 'hour') {
    if (startDate !== endDate) return []
    return Array.from({ length: 24 }, (_, hour) => `${startDate} ${String(hour).padStart(2, '0')}:00`)
  }

  const labels: string[] = []
  const cursor = new Date(`${startDate}T00:00:00Z`)
  const end = new Date(`${endDate}T00:00:00Z`)
  if (Number.isNaN(cursor.getTime()) || Number.isNaN(end.getTime()) || cursor > end) return labels
  while (cursor <= end) {
    labels.push(cursor.toISOString().slice(0, 10))
    cursor.setUTCDate(cursor.getUTCDate() + 1)
  }
  return labels
}

const selectedUserTrendChartData = computed<ChartData<'line'>>(() => {
  const labels = buildTrendLabels(
    filters.start_date || '',
    filters.end_date || '',
    userTrendGranularity.value
  )
  const values = new Map<string, number>()
  for (const point of userTrendPoints.value) {
    values.set(`${point.user_id}/${point.date}`, point.tokens)
  }
  return {
    labels,
    datasets: selectedTrendUsers.value.map((user, index) => {
      const color = USER_TREND_COLORS[index % USER_TREND_COLORS.length]
      return {
        label: user.email || `User ${user.user_id}`,
        data: labels.map((label) => values.get(`${user.user_id}/${label}`) ?? 0),
        borderColor: color,
        backgroundColor: `${color}20`,
        borderWidth: 2,
        pointRadius: 2,
        pointHoverRadius: 4,
        fill: false,
        tension: 0.3
      }
    })
  }
})

const selectedUserTrendChartOptions: ChartOptions<'line'> = {
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index'
  },
  plugins: {
    legend: {
      position: 'bottom',
      labels: {
        usePointStyle: true,
        boxWidth: 8
      }
    }
  },
  scales: {
    x: {
      grid: { display: false }
    },
    y: {
      beginAtZero: true,
      ticks: {
        callback: (value) => formatCompactNumber(Number(value))
      }
    }
  }
}

const indexStatusText = computed(() => {
  if (indexStatus.value?.running) return t('admin.tokenAnalysis.indexRunning')
  if (indexStatus.value?.last_error) return indexStatus.value.last_error
  return formatTimeCN(indexStatus.value?.updated_at)
})

const extractedUserRequest = computed(() => extractUserRequestTag(requestInput.value?.content || ''))

function extractUserRequestTag(content: string): string {
  const match = content.match(/<userRequest>([\s\S]*?)<\/userRequest>/i)
  return match?.[1]?.trim() || ''
}

const detailFields = computed(() => {
  const item = selectedRequest.value
  if (!item) return []
  return [
    { label: 'Endpoint', value: item.endpoint },
    { label: 'Model', value: item.model },
    { label: 'Match', value: String(item.match_confidence) },
    { label: 'Messages', value: String(item.message_count ?? 0) },
    { label: 'System chars', value: formatNumber(item.system_chars ?? 0) },
    { label: 'User chars', value: formatNumber(item.user_chars ?? 0) },
    { label: 'Tools', value: String(item.tools_count ?? 0) },
    { label: 'Images', value: String(item.image_count ?? 0) },
    { label: 'Input tokens', value: formatNumber(item.input_tokens ?? 0) },
    { label: 'Output tokens', value: formatNumber(item.output_tokens ?? 0) },
    { label: 'Cache read', value: formatNumber(item.cache_read_tokens ?? 0) },
    { label: 'Cache creation', value: formatNumber(item.cache_creation_tokens ?? 0) },
    { label: 'Cost', value: formatCost(item.actual_cost ?? 0) },
    { label: t('admin.tokenAnalysis.project'), value: item.client_project || '-' },
    { label: t('admin.tokenAnalysis.workdir'), value: item.client_workdir || '-' },
    { label: t('admin.tokenAnalysis.branch'), value: item.client_branch || '-' },
    { label: t('admin.tokenAnalysis.attributionSource'), value: item.attribution_source || '-' },
    { label: t('admin.tokenAnalysis.duplicateCount'), value: formatNumber(item.duplicate_count ?? 1) },
    { label: 'Body size', value: formatBytes(item.request_body_size ?? 0) },
    { label: t('admin.tokenAnalysis.bodyTruncated'), value: item.request_body_truncated ? t('common.yes') : t('common.no') }
  ]
})

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(Math.round(value || 0))
}

function finiteNumber(value: number): number {
  const number = Number(value || 0)
  return Number.isFinite(number) ? number : 0
}

function formatTokenMetric(value: number): string {
  const number = finiteNumber(value)
  const abs = Math.abs(number)
  if (abs >= 1_000_000_000) return `${(number / 1_000_000_000).toFixed(1)}B`
  if (abs >= 1_000_000) return `${(number / 1_000_000).toFixed(1)}M`
  return formatNumber(number)
}

function formatRequestMetric(value: number): string {
  const number = finiteNumber(value)
  const abs = Math.abs(number)
  if (abs >= 1_000_000_000) return `${(number / 1_000_000_000).toFixed(1)}B`
  if (abs >= 1_000_000) return `${(number / 1_000_000).toFixed(1)}M`
  if (abs >= 1_000) return `${(number / 1_000).toFixed(1)}K`
  return formatNumber(number)
}

// 时间列固定东八区展示(团队所在时区), 形如 2026-06-10 16:23:45;
// sv-SE locale 天然输出 YYYY-MM-DD HH:mm:ss, 不带 ISO 的 T 和时区尾巴。
function formatTimeCN(value?: string | null): string {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return new Intl.DateTimeFormat('sv-SE', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  }).format(d)
}

function formatCost(value: number): string {
  return `$${Number(value || 0).toFixed(4)}`
}

// 用户排行专用: Token 仅用 M/B(不用 K), 费用达到 1000 后使用 K。
function formatUserRankingTokens(value: number): string {
  return formatTokenMetric(value)
}

function formatUserRankingCost(value: number): string {
  const number = finiteNumber(value)
  if (Math.abs(number) >= 1_000) return `$${(number / 1_000).toFixed(1)}K`
  return formatCost(number)
}

function userRankingRank(index: number): number {
  return (usersPagination.page - 1) * usersPagination.page_size + index + 1
}

function isTrendUserSelected(userID?: number): boolean {
  return Boolean(userID && selectedTrendUsers.value.some((user) => user.user_id === userID))
}

function trendUserSelectionDisabled(userID?: number): boolean {
  if (!userID) return true
  return selectedTrendUsers.value.length >= MAX_SELECTED_TREND_USERS && !isTrendUserSelected(userID)
}

function toggleTrendUser(user: TokenAnalysisUserUsage) {
  const userID = Number(user.user_id || 0)
  if (userID <= 0) return
  const selectedIndex = selectedTrendUsers.value.findIndex((item) => item.user_id === userID)
  if (selectedIndex >= 0) {
    selectedTrendUsers.value.splice(selectedIndex, 1)
  } else if (selectedTrendUsers.value.length < MAX_SELECTED_TREND_USERS) {
    selectedTrendUsers.value.push({
      user_id: userID,
      email: user.user_email || `User ${userID}`
    })
  }
  void loadSelectedUserTrend()
}

function setUserTrendGranularity(granularity: TrendGranularity) {
  if (granularity === 'hour' && !canUseHourlyTrend.value) return
  if (userTrendGranularity.value === granularity) return
  userTrendGranularity.value = granularity
  void loadSelectedUserTrend()
}

async function loadSelectedUserTrend() {
  const seq = ++userTrendLoadSeq
  userTrendPoints.value = []
  userTrendError.value = false
  if (selectedTrendUsers.value.length === 0) {
    userTrendLoading.value = false
    return
  }
  if (!filters.start_date || !filters.end_date) {
    userTrendLoading.value = false
    userTrendError.value = true
    return
  }
  if (userTrendGranularity.value === 'hour' && !canUseHourlyTrend.value) {
    userTrendGranularity.value = 'day'
  }

  userTrendLoading.value = true
  try {
    const response = await adminAPI.dashboard.getUserUsageTrend({
      user_ids: selectedTrendUsers.value.map((user) => user.user_id),
      start_date: filters.start_date,
      end_date: filters.end_date,
      granularity: userTrendGranularity.value
    })
    if (seq !== userTrendLoadSeq) return
    userTrendPoints.value = response.trend || []
  } catch {
    if (seq !== userTrendLoadSeq) return
    userTrendError.value = true
  } finally {
    if (seq === userTrendLoadSeq) userTrendLoading.value = false
  }
}

function percent(value: number): string {
  return `${((value || 0) * 100).toFixed(1)}%`
}

function projectOutputInputRatio(row: TokenAnalysisProjectUsage): number | null {
  if (!row.input_tokens || row.input_tokens <= 0) return null
  return (row.output_tokens || 0) / row.input_tokens
}

function formatProjectOutputInputRatio(row: TokenAnalysisProjectUsage): string {
  const ratio = projectOutputInputRatio(row)
  if (ratio === null) return '-'
  return percent(ratio)
}

function projectOutputInputRatioClass(row: TokenAnalysisProjectUsage): string {
  const ratio = projectOutputInputRatio(row)
  if (ratio === null) return 'text-gray-400'
  if (ratio >= PROJECT_OUTPUT_INPUT_HEALTHY_RATIO) return 'text-emerald-600 dark:text-emerald-400'
  return 'text-red-600 dark:text-red-400'
}

function projectOutputInputRatioDataTest(row: TokenAnalysisProjectUsage): string {
  const ratio = projectOutputInputRatio(row)
  if (ratio === null) return 'project-io-ratio-empty'
  return ratio >= PROJECT_OUTPUT_INPUT_HEALTHY_RATIO ? 'project-io-ratio-healthy' : 'project-io-ratio-low'
}

function projectOutputInputRatioTitle(row: TokenAnalysisProjectUsage): string {
  const ratio = projectOutputInputRatio(row)
  if (ratio === null) return t('admin.tokenAnalysis.outputInputRatioUnavailable')
  return `${t('admin.tokenAnalysis.outputInputRatio')}: ${percent(ratio)} (${t('admin.tokenAnalysis.outputInputRatioThreshold')}: ${percent(PROJECT_OUTPUT_INPUT_HEALTHY_RATIO)})`
}

function riskClass(score: number): string {
  if (score >= 60) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (score >= 30) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
}

// 风险原因 code 映射中文名称/解释(见 i18n riskCodes)。缺失映射时回退展示原始
// code, 保证后端新增风险类型而前端文案未补时仍可读, 不会显示成空白。
function riskLabel(code: string): string {
  const key = `admin.tokenAnalysis.riskCodes.${code}.label`
  const label = t(key)
  return label === key ? code : label
}

function riskDesc(code: string): string {
  const key = `admin.tokenAnalysis.riskCodes.${code}.desc`
  const desc = t(key)
  return desc === key ? '' : desc
}

async function loadSummary() {
  summary.value = await adminAPI.tokenAnalysis.getSummary(cleanFilters.value)
}

async function loadUsers() {
  usersLoading.value = true
  try {
    const result = await adminAPI.tokenAnalysis.listUsers({
      ...cleanFilters.value,
      page: usersPagination.page,
      page_size: usersPagination.page_size
    })
    users.value = result.items
    usersPagination.total = result.total
    usersPagination.page = result.page
    usersPagination.page_size = result.page_size
  } finally {
    usersLoading.value = false
  }
}

async function loadProjects() {
  projectsLoading.value = true
  try {
    const result = await adminAPI.tokenAnalysis.listProjects({
      ...cleanFilters.value,
      page: projectsPagination.page,
      page_size: projectsPagination.page_size,
      sort_by: projectSort.value,
      sort_order: projectSortOrder.value
    })
    projects.value = result.items
    projectsPagination.total = result.total
    projectsPagination.page = result.page
    projectsPagination.page_size = result.page_size
  } finally {
    projectsLoading.value = false
  }
}

async function loadRequests() {
  requestsLoading.value = true
  try {
    const result = await adminAPI.tokenAnalysis.listRequests({
      ...cleanFilters.value,
      page: requestsPagination.page,
      page_size: requestsPagination.page_size,
      sort_by: requestSort.value,
      sort_order: 'desc'
    })
    requests.value = result.items
    requestsPagination.total = result.total
    requestsPagination.page = result.page
    requestsPagination.page_size = result.page_size
  } finally {
    requestsLoading.value = false
  }
}

function changeRequestSort() {
  requestsPagination.page = 1
  void loadRequests()
}

// 点已选中列翻转方向, 点新列重置为降序。
function changeProjectSort(field: ProjectSortField) {
  if (projectSort.value === field) {
    projectSortOrder.value = projectSortOrder.value === 'desc' ? 'asc' : 'desc'
  } else {
    projectSort.value = field
    projectSortOrder.value = 'desc'
  }
  projectsPagination.page = 1
  void loadProjects()
}

function projectSortIcon(field: ProjectSortField): string {
  if (projectSort.value !== field) return '↕'
  return projectSortOrder.value === 'desc' ? '↓' : '↑'
}

// 点行打开抽屉并懒加载净输入全文(列表只带 300 字预览)。
function selectRequest(item: TokenAnalysisRequestItem) {
  selectedRequest.value = item
  requestInput.value = null
  if (!item.has_input) return
  requestInputLoading.value = true
  adminAPI.tokenAnalysis
    .getRequestInput(item.archive_id)
    .then((input) => {
      if (selectedRequest.value?.archive_id === item.archive_id) {
        requestInput.value = input
      }
    })
    .catch(() => {
      // 全文加载失败时抽屉回退展示预览, 不打断查看。
    })
    .finally(() => {
      // 仅当抽屉仍停留在本条记录时才收起 loading,
      // 避免旧请求的 finally 提前关掉新记录的 loading(竞态)。
      if (selectedRequest.value?.archive_id === item.archive_id) {
        requestInputLoading.value = false
      }
    })
}

async function loadIndexStatus() {
  indexStatus.value = await adminAPI.tokenAnalysis.getIndexStatus()
}

async function loadArchiveFiles() {
  archiveFiles.value = await adminAPI.tokenAnalysis.listArchiveFiles()
}

// 文件是否已被索引器读完(字节水位追平文件大小)。"有失败行"状态也意味着
// 已读完 -- 只是其中部分行未入库, 这层语义靠进度列和状态提示显式说出来。
function archiveFileFullyRead(file: TokenAnalysisArchiveFile): boolean {
  return file.status !== 'compressed' && file.size_bytes > 0 && file.indexed_offset >= file.size_bytes
}

// 进度展示为百分比, 直观回答"处理完没有"; 字节明细放 title 悬浮提示。
function archiveFileProgress(file: TokenAnalysisArchiveFile): string {
  if (file.status === 'compressed' || file.size_bytes <= 0) return '-'
  if (archiveFileFullyRead(file)) return `100% · ${t('admin.tokenAnalysis.fullyRead')}`
  return `${(Math.min(file.indexed_offset / file.size_bytes, 1) * 100).toFixed(1)}%`
}

function archiveFileProgressDetail(file: TokenAnalysisArchiveFile): string {
  return `${formatBytes(file.indexed_offset)} / ${formatBytes(file.size_bytes)}`
}

function archiveFileStatusClass(status: TokenAnalysisArchiveFileStatus): string {
  switch (status) {
    case 'deletable':
      return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
    case 'writing':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'indexing':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    case 'attention':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
}

async function reloadAll() {
  usersPagination.page = 1
  requestsPagination.page = 1
  projectsPagination.page = 1
  if (userTrendGranularity.value === 'hour' && !canUseHourlyTrend.value) {
    userTrendGranularity.value = 'day'
  }
  try {
    const loaders = [loadSummary(), loadUsers(), loadProjects(), loadRequests(), loadIndexStatus(), loadArchiveFiles()]
    if (selectedTrendUsers.value.length > 0) {
      loaders.push(loadSelectedUserTrend())
    }
    await Promise.all(loaders)
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

// 索引触发已改异步(后端立即返回 202): 大范围回扫可达分钟级, 同步等待会撞
// axios 30s 超时报"网络错误"。触发后轮询索引状态, running 翻 false 即完成。
const INDEX_POLL_INTERVAL_MS = 3000
let indexPollTimer: ReturnType<typeof setTimeout> | null = null

function stopIndexPolling() {
  if (indexPollTimer !== null) {
    clearTimeout(indexPollTimer)
    indexPollTimer = null
  }
}

function scheduleIndexPoll() {
  stopIndexPolling()
  indexing.value = true
  indexPollTimer = setTimeout(() => void pollIndexStatus(), INDEX_POLL_INTERVAL_MS)
}

async function pollIndexStatus() {
  indexPollTimer = null
  try {
    await Promise.all([loadIndexStatus(), loadArchiveFiles()])
  } catch {
    // 单次轮询失败不终止(可能是瞬时网络抖动), 下一轮重试。
  }
  if (indexStatus.value?.running) {
    scheduleIndexPoll()
    return
  }
  indexing.value = false
  appStore.showSuccess(t('admin.tokenAnalysis.indexDone'))
  void reloadAll()
}

async function triggerIndex() {
  indexing.value = true
  try {
    await adminAPI.tokenAnalysis.triggerIndex({
      start_date: filters.start_date,
      end_date: filters.end_date,
      timezone: filters.timezone
    })
    appStore.showSuccess(t('admin.tokenAnalysis.indexStarted'))
    scheduleIndexPoll()
  } catch (error) {
    const err = error as { reason?: string; message?: string }
    if (err?.reason === 'TOKEN_ANALYSIS_INDEX_RUNNING') {
      // 自动索引或他人触发的轮次在跑: 跟进其进度而非报错收场。
      appStore.showError(t('admin.tokenAnalysis.indexAlreadyRunning'))
      scheduleIndexPoll()
      return
    }
    appStore.showError(err?.message || t('errors.networkError'))
    indexing.value = false
  }
}

function changeRequestPage(page: number) {
  requestsPagination.page = page
  void loadRequests()
}

function changeUserPage(page: number) {
  usersPagination.page = page
  void loadUsers()
}

function changeUserPageSize(pageSize: number) {
  usersPagination.page_size = pageSize
  usersPagination.page = 1
  void loadUsers()
}

function changeRequestPageSize(pageSize: number) {
  requestsPagination.page_size = pageSize
  requestsPagination.page = 1
  void loadRequests()
}

function toggleRiskReason(code: string) {
  filters.risk_reason = filters.risk_reason === code ? undefined : code
  void reloadAll()
}

function changeProjectPage(page: number) {
  projectsPagination.page = page
  void loadProjects()
}

function changeProjectPageSize(pageSize: number) {
  projectsPagination.page_size = pageSize
  projectsPagination.page = 1
  void loadProjects()
}

function toggleProjectFilter(project: string) {
  filters.project = filters.project === project ? undefined : project
  void reloadAll()
}

onMounted(() => {
  void reloadAll().then(() => {
    // 进页面时已有索引在跑(自动索引或他人触发): 直接跟进进度。
    if (indexStatus.value?.running) {
      scheduleIndexPoll()
    }
  })
})

onUnmounted(() => {
  userTrendLoadSeq++
  stopIndexPolling()
})
</script>
