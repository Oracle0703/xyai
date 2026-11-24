<template>
  <section class="card overflow-hidden" :data-testid="`gpt-iq-column-${groupKey}`">
    <header class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
      <div class="flex min-w-0 items-center gap-2.5">
        <!-- 分组标识色与额度区一致，只配合分组名使用，不承担判定含义。 -->
        <span class="h-2.5 w-2.5 shrink-0 rounded-full" :class="groupKey === 'xunyou' ? 'bg-sky-500 dark:bg-sky-600' : 'bg-violet-500'" />
        <h3 class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ title }}</h3>
        <span class="badge badge-gray shrink-0">{{ t('gptIQTest.count', { count: items.length }) }}</span>
      </div>
      <div v-if="verdictCounts.length" class="flex flex-wrap gap-1" data-testid="gpt-iq-column-counts">
        <span v-for="item in verdictCounts" :key="item.verdict" class="badge" :class="gptIQVerdictStyle(item.verdict).badge">
          {{ gptIQVerdictLabel(item.verdict, t, te) }} {{ item.count }}
        </span>
      </div>
    </header>

    <div v-if="!items.length" class="px-5 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('gptIQTest.emptyGroup') }}
    </div>
    <ul v-else class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,12rem),1fr))] gap-3 p-4">
      <GPTIQTestCard
        v-for="item in items"
        :key="item.id"
        :card="item"
        :cases="cases"
        :today="today"
        :now="now"
        :current-revisions="currentRevisions"
        @open="emit('open', $event)"
      />
    </ul>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import GPTIQTestCard from '@/components/user/GPTIQTestCard.vue'
import type { GPTQuotaGroupKey } from '@/api/gptQuotaDisplay'
import type { GPTIQPublicCase, GPTIQUserCard, GPTIQVerdict } from '@/api/gptIQTest'
import { gptIQVerdictLabel, gptIQVerdictStyle } from '@/utils/gptIQTest'

const props = defineProps<{
  groupKey: GPTQuotaGroupKey
  title: string
  items: GPTIQUserCard[]
  cases: GPTIQPublicCase[]
  today: string
  now: number
  currentRevisions?: ReadonlyMap<number, number> | null
}>()

const emit = defineEmits<{ (e: 'open', card: GPTIQUserCard): void }>()

const { t, te } = useI18n()

// 列头只汇总需要关注的判定，正常账号不单独计数。
const ATTENTION: GPTIQVerdict[] = ['suspect', 'failed', 'insufficient']

const verdictCounts = computed(() =>
  ATTENTION.map((verdict) => ({ verdict, count: props.items.filter((item) => item.verdict === verdict).length })).filter((item) => item.count > 0),
)
</script>
