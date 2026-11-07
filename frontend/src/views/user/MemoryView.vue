<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-5" :aria-busy="loading">
      <div class="flex items-center justify-between gap-3">
        <h1 class="text-xl font-semibold text-ink">{{ t('accountMemory.title') }}</h1>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="load">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          {{ t('common.refresh') }}
        </button>
      </div>

      <div v-if="error" class="card flex flex-wrap items-center justify-between gap-3 p-5" role="alert">
        <p class="text-sm text-red-700 dark:text-red-400">{{ t('accountMemory.loadFailed') }}</p>
        <button type="button" class="btn btn-secondary btn-sm" @click="load">{{ t('common.tryAgain') }}</button>
      </div>
      <div v-else-if="loading" class="card p-6" role="status">
        <span class="text-sm text-ink-muted">{{ t('common.loading') }}</span>
      </div>
      <template v-else-if="overview">
        <section class="card p-5 sm:p-6" aria-labelledby="memory-status-title">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-primary-50 p-2.5 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400">
              <Icon name="database" size="lg" />
            </div>
            <div class="min-w-0">
              <h2 id="memory-status-title" class="font-semibold text-ink">{{ t(`accountMemory.${overview.state}`) }}</h2>
              <p class="mt-1 text-sm text-ink-muted">{{ statusHint }}</p>
            </div>
          </div>
          <p v-if="overview.state === 'active'" class="mt-5 border-t border-gray-200 pt-4 font-mono text-xs text-ink-muted dark:border-dark-700">
            {{ t('accountMemory.coverage') }}
          </p>
        </section>

        <template v-if="overview.state === 'active'">
          <dl class="grid grid-cols-3 gap-2 sm:gap-4">
            <div v-for="metric in metrics" :key="metric.key" class="card min-w-0 p-3 sm:p-5">
              <dt class="text-xs text-ink-muted sm:text-sm">{{ t(`accountMemory.${metric.key}`) }}</dt>
              <dd class="mt-2 font-mono text-2xl font-semibold text-ink" :class="{ 'text-amber-700 dark:text-amber-400': metric.key === 'attention' && metric.value > 0 }">
                {{ metric.value.toLocaleString() }}
              </dd>
            </div>
          </dl>

          <section class="card overflow-hidden" aria-labelledby="memory-recent-title">
            <div class="flex items-center justify-between gap-3 border-b border-gray-200 px-5 py-4 dark:border-dark-700">
              <h2 id="memory-recent-title" class="font-semibold text-ink">{{ t('accountMemory.recent') }}</h2>
              <span class="text-xs text-ink-muted">{{ t('accountMemory.recentLimit') }}</span>
            </div>
            <p v-if="overview.recent.length === 0" class="px-5 py-12 text-center text-sm text-ink-muted">{{ t('accountMemory.empty') }}</p>
            <ul v-else class="divide-y divide-gray-100 dark:divide-dark-800">
              <li v-for="event in overview.recent" :key="event.event_id" class="flex items-center justify-between gap-3 px-5 py-4">
                <div class="min-w-0">
                  <p class="truncate font-mono text-xs text-ink" :title="event.event_id">{{ event.event_id }}</p>
                  <time class="mt-1 block text-xs text-ink-muted" :datetime="event.updated_at" :title="t('accountMemory.updated')">{{ formatDate(event.updated_at) }}</time>
                </div>
                <span class="shrink-0 rounded-md px-2 py-1 text-xs font-medium" :class="stateClass(event.state)">
                  {{ t(`accountMemory.states.${event.state}`) }}
                </span>
              </li>
            </ul>
          </section>
        </template>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { memoryAPI, type AccountMemoryOverview, type MemoryArchiveState } from '@/api/memory'
import { Icon } from '@/components/icons'
import AppLayout from '@/components/layout/AppLayout.vue'
import { formatDate } from '@/utils/format'

const { t } = useI18n()
const overview = ref<AccountMemoryOverview | null>(null)
const loading = ref(true)
const error = ref(false)
let controller: AbortController | undefined

const statusHint = computed(() => {
  switch (overview.value?.state) {
    case 'unconfigured': return t('accountMemory.unconfiguredHint')
    case 'suspended': return t('accountMemory.suspendedHint')
    default: return t('accountMemory.accountScope')
  }
})
const metrics = computed(() => overview.value ? [
  { key: 'archived', value: overview.value.archived },
  { key: 'pending', value: overview.value.pending },
  { key: 'attention', value: overview.value.attention }
] : [])

function stateClass(state: MemoryArchiveState): string {
  if (state === 'completed') return 'bg-primary-50 text-primary-800 dark:bg-primary-900/30 dark:text-primary-300'
  if (state === 'failed' || state === 'needs_reconciliation') return 'bg-amber-50 text-amber-800 dark:bg-amber-900/25 dark:text-amber-300'
  return 'bg-gray-100 text-gray-700 dark:bg-dark-800 dark:text-dark-200'
}

async function load() {
  controller?.abort()
  const request = new AbortController()
  controller = request
  loading.value = true
  error.value = false
  try {
    const result = await memoryAPI.overview({ signal: request.signal })
    if (!request.signal.aborted) overview.value = result
  } catch {
    if (!request.signal.aborted) error.value = true
  } finally {
    if (!request.signal.aborted) loading.value = false
  }
}

onMounted(load)
onBeforeUnmount(() => controller?.abort())
</script>
