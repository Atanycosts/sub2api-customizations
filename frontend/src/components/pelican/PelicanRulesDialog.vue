<template>
  <BaseDialog
    :show="show"
    :title="t('pelican.rules.title')"
    width="wide"
    close-on-click-outside
    @close="$emit('close')"
  >
    <div class="space-y-4 text-sm leading-6 text-gray-600 dark:text-dark-200">
      <p>{{ t('pelican.page.description') }}</p>
      <p>{{ t('pelican.disclosure') }}</p>
      <p>
        {{ t('pelican.page.readOnly') }} ·
        {{ t('pelican.page.promptVersion', { version: promptVersion }) }}
      </p>
      <section>
        <h4 class="mb-2 font-semibold text-gray-900 dark:text-white">
          {{ t('pelican.rules.prompt') }}
        </h4>
        <pre
          v-if="prompt"
          class="max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-xl bg-gray-50 p-4 text-xs leading-6 dark:bg-dark-900"
          >{{ prompt }}</pre>
        <p v-else>{{ t('pelican.rules.promptUnavailable') }}</p>
      </section>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
withDefaults(defineProps<{ show: boolean; prompt?: string; promptVersion?: string }>(), {
  prompt: '',
  promptVersion: 'v1'
})
defineEmits<{ close: [] }>()
const { t } = useI18n()
</script>
