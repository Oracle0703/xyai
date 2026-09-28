import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { getSelfResetStatus, resetSubscriptionDaily, type SelfResetStatus } from '@/api/subscriptionSelfReset'

interface Operation { id: number; date: string; key?: string }
interface PendingKey { date: string; key: string }

export function useSubscriptionSelfReset(refreshSubscriptions: () => Promise<void>, invalidateSubscriptions: () => void) {
  const { t, te } = useI18n()
  const auth = useAuthStore()
  const app = useAppStore()
  const store = useSubscriptionStore()
  const storageKey = `subscription-self-reset:${auth.user?.id}`
  const status = ref<SelfResetStatus | null>(null)
  const statusError = ref(false)
  const submitting = ref(false)
  const operation = ref<Operation | null>(null)
  const dialogOpen = ref(false)
  const requestError = ref('')
  const retryBlocked = ref(false)
  let generation = 0
  let disposed = false
  let globalStale = false
  let dayTimer: ReturnType<typeof setTimeout> | undefined
  let retryTimer: ReturnType<typeof setTimeout> | undefined

  // Submitted keys whose outcome is unknown, per subscription. With limits above 1 and new usage after a
  // lost success, a new key would reset again, so closing the dialog or reloading must keep the original key.
  const pending = new Map<number, PendingKey>()
  try {
    const saved: unknown = JSON.parse(sessionStorage.getItem(storageKey) || '[]')
    for (const [id, entry] of Array.isArray(saved) ? saved as [number, PendingKey][] : []) {
      if (Number.isSafeInteger(id) && id > 0 && typeof entry?.date === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(entry.date) && typeof entry.key === 'string' && entry.key) {
        pending.set(id, { date: entry.date, key: entry.key })
      }
    }
  } catch { /* Unreadable storage only loses recovery across reloads. */ }

  function remember(id: number, entry?: PendingKey) {
    if (entry) pending.set(id, entry)
    else pending.delete(id)
    try {
      if (pending.size) sessionStorage.setItem(storageKey, JSON.stringify([...pending]))
      else sessionStorage.removeItem(storageKey)
    } catch { /* Memory still covers this page. */ }
  }

  const items = computed(() => new Map(status.value?.subscriptions.map(item => [item.subscription_id, item]) ?? []))
  const dialogDisabled = computed(() => submitting.value || retryBlocked.value)

  // The last status stays visible while reloading; the list reports its own failures.
  // Global summaries are only reloaded after a write, and retried here until that succeeds.
  async function refresh() {
    if (disposed) return false
    const current = ++generation
    statusError.value = false
    clearTimeout(dayTimer)
    const retryGlobal = globalStale
    const [next, , global] = await Promise.allSettled([getSelfResetStatus(), refreshSubscriptions(), retryGlobal ? store.fetchActiveSubscriptions(true) : undefined])
    if (retryGlobal && global.status === 'fulfilled') globalStale = false
    if (current !== generation || disposed) return false
    if (next.status === 'rejected') {
      status.value = null
      statusError.value = true
      return false
    }
    status.value = next.value
    const until = Date.parse(next.value.next_reset_at) - Date.parse(next.value.server_now)
    if (Number.isFinite(until)) dayTimer = setTimeout(() => { void refresh() }, Math.max(1000, until + 100))
    return !globalStale
  }

  function canReset(id: number) {
    return !submitting.value && !operation.value && !!items.value.get(id)?.can_reset
  }

  function open(id: number) {
    if (!canReset(id) || !status.value) return
    const date = status.value.quota_date
    const saved = pending.get(id)
    // An earlier day's operation either committed on that day or can no longer reset today.
    if (saved && saved.date !== date) remember(id)
    const key = saved?.date === date ? saved.key : undefined
    operation.value = { id, date, key }
    requestError.value = key ? t('userSubscriptions.selfReset.resumePending') : ''
    dialogOpen.value = true
  }

  function close() {
    if (submitting.value) return
    dialogOpen.value = false
    const submitted = !!operation.value?.key
    operation.value = null
    if (submitted) void refresh()
  }

  async function confirm() {
    const current = operation.value
    if (!current || dialogDisabled.value) return
    // getRandomValues also works on HTTP deployments (randomUUID does not).
    current.key ??= Array.from(crypto.getRandomValues(new Uint32Array(4)), n => n.toString(16)).join('-')
    remember(current.id, { date: current.date, key: current.key })
    submitting.value = true
    generation++
    invalidateSubscriptions()
    requestError.value = ''
    try {
      await resetSubscriptionDaily(current.id, current.date, current.key)
      remember(current.id)
      operation.value = null
      dialogOpen.value = false
      app.showSuccess(t('userSubscriptions.selfReset.success'))
      globalStale = true
      if (disposed) void store.fetchActiveSubscriptions(true).catch(() => {})
      else if (!await refresh()) app.showError(t('userSubscriptions.selfReset.refreshFailed'))
    } catch (error: unknown) {
      const failure = error as { status?: number; reason?: string; message?: string; retryAfter?: string }
      const reason = failure.reason || ''
      const retryable = !failure.status || [401, 408, 429].includes(failure.status) || failure.status >= 500 || ['IDEMPOTENCY_IN_PROGRESS', 'IDEMPOTENCY_RETRY_BACKOFF'].includes(reason)
      if (retryable) {
        requestError.value = t('userSubscriptions.selfReset.retrySame')
        const seconds = Number(failure.retryAfter)
        if (Number.isFinite(seconds) && seconds > 0) {
          retryBlocked.value = true
          clearTimeout(retryTimer)
          retryTimer = setTimeout(() => { retryBlocked.value = false }, Math.min(seconds * 1000, 2_147_000_000))
        }
      } else {
        remember(current.id)
        operation.value = null
        dialogOpen.value = false
        const aliases: Record<string, string> = { DAY_CHANGED: 'dayChanged', LIMIT_REACHED: 'DAILY_LIMIT_REACHED', DISABLED: 'POLICY_DISABLED' }
        const code = reason.replace(/^SELF_RESET_/, '')
        const messageKey = `userSubscriptions.selfReset.${aliases[code] || code}`
        app.showError(te(messageKey) ? t(messageKey) : failure.message || t('userSubscriptions.selfReset.requestFailed'))
        await refresh()
      }
    } finally { submitting.value = false }
  }

  function onVisible() { if (document.visibilityState === 'visible' && !submitting.value) void refresh() }
  document.addEventListener('visibilitychange', onVisible)
  onBeforeUnmount(() => {
    disposed = true
    generation++
    clearTimeout(dayTimer)
    clearTimeout(retryTimer)
    document.removeEventListener('visibilitychange', onVisible)
  })
  return { status, statusError, items, operation, dialogOpen, dialogDisabled, requestError, canReset, open, close, confirm, refresh }
}
