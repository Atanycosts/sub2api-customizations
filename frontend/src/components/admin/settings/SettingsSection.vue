<template>
  <section :id="id" class="card settings-section-anchor" tabindex="-1">
    <h2>
      <button
        type="button"
        class="flex min-h-16 w-full items-center gap-3 px-6 py-4 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
        :aria-expanded="isExpanded"
        :aria-controls="`${id}-content`"
        @click="emit('update:expanded', !isExpanded)"
      >
        <span class="min-w-0 flex-1 text-base font-semibold text-gray-900 dark:text-white">{{ title }}</span>
        <span class="shrink-0 rounded-md bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500 dark:bg-dark-700 dark:text-gray-400">
          {{ independent ? t('admin.settings.navigation.independentSave') : t('admin.settings.navigation.pageSave') }}
        </span>
        <Icon name="chevronDown" size="sm" :class="['shrink-0 text-gray-400 transition-transform', isExpanded && 'rotate-180']" />
      </button>
    </h2>
    <div :id="`${id}-content`" v-show="isExpanded">
      <p v-if="description" class="border-b border-gray-100 px-6 pb-4 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">{{ description }}</p>
      <slot />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{
  id: string
  title: string
  description?: string
  independent?: boolean
  expanded?: boolean
  defaultExpanded?: boolean
}>(), {
  description: '',
  independent: false,
  expanded: undefined,
  defaultExpanded: true,
})
const emit = defineEmits<{ 'update:expanded': [value: boolean] }>()
const { t } = useI18n()
const isExpanded = computed(() => props.expanded ?? props.defaultExpanded)
</script>

<style scoped>
.settings-section-anchor {
  scroll-margin-top: 12rem;
}

@media (max-width: 639px) {
  .settings-section-anchor > h2 > button {
    padding-inline: 1rem;
  }
}
</style>
