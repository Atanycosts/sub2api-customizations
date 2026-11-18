<template>
  <nav
    class="flex min-h-9 flex-wrap items-center justify-between gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-dark-300"
    :aria-label="t('pelican.pagination.label')"
  >
    <span aria-live="polite">{{ t('pelican.pagination.range', { start, end, total }) }}</span>
    <div class="flex items-center gap-2">
      <button
        type="button"
        class="btn btn-secondary btn-sm"
        :disabled="page <= 1"
        @click="$emit('update:page', page - 1)"
      >
        {{ t('pelican.pagination.previous') }}
      </button>
      <span class="tabular-nums">{{ page }} / {{ pages }}</span>
      <button
        type="button"
        class="btn btn-secondary btn-sm"
        :disabled="page >= pages"
        @click="$emit('update:page', page + 1)"
      >
        {{ t('pelican.pagination.next') }}
      </button>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
const props = defineProps<{ page: number; total: number; pageSize: number }>()
defineEmits<{ 'update:page': [page: number] }>()
const { t } = useI18n()
const pages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
const start = computed(() => (props.total ? (props.page - 1) * props.pageSize + 1 : 0))
const end = computed(() => Math.min(props.page * props.pageSize, props.total))
</script>
