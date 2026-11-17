<template>
  <dl class="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
    <div class="col-span-2 min-w-0">
      <dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.fields.group') }}</dt>
      <dd class="mt-0.5 truncate font-medium text-gray-900 dark:text-white">{{ run.group_name }}</dd>
    </div>
    <div class="col-span-2 min-w-0">
      <dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.fields.model') }}</dt>
      <dd class="mt-0.5 break-words font-mono text-sm font-semibold text-gray-900 dark:text-white">
        {{ run.model }}
      </dd>
    </div>
    <div>
      <dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.fields.effort') }}</dt>
      <dd class="mt-0.5 font-medium text-gray-900 dark:text-white">
        {{ run.effort || t('pelican.effort.unspecified') }}
      </dd>
    </div>
    <div>
      <dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.fields.protocol') }}</dt>
      <dd class="mt-0.5 font-medium text-gray-900 dark:text-white">
        {{ t(`pelican.protocol.${run.protocol}`) }}
      </dd>
    </div>
    <div>
      <dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.fields.promptVersion') }}</dt>
      <dd class="mt-0.5 font-mono font-medium text-gray-900 dark:text-white">
        {{ run.prompt_version }}
      </dd>
    </div>
    <div>
      <dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.fields.outputCap') }}</dt>
      <dd class="mt-0.5 font-medium tabular-nums text-gray-900 dark:text-white">
        {{ t('pelican.fields.tokens', { count: run.max_output_tokens }) }}
      </dd>
    </div>
  </dl>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { PelicanPublicRun } from '@/types/pelican'

defineProps<{ run: PelicanPublicRun }>()
const { t } = useI18n()
</script>
