<template>
  <BaseDialog
    :show="show"
    :title="plan ? t('pelican.admin.plans.editTitle') : t('pelican.admin.plans.createTitle')"
    width="wide"
    close-on-click-outside
    @close="$emit('close')"
  >
    <form class="space-y-5" @submit.prevent="submit">
      <div class="rounded-2xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/40">
        <div class="flex items-center justify-between gap-3">
          <div>
            <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('pelican.admin.plans.scheduleEnabled') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.plans.scheduleEnabledHint') }}</p>
          </div>
          <Toggle v-model="form.enabled" />
        </div>
        <div class="mt-4 flex items-center justify-between gap-3 border-t border-gray-200 pt-4 dark:border-dark-700">
          <div>
            <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('pelican.admin.plans.published') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.plans.publishedHint') }}</p>
          </div>
          <Toggle v-model="form.published" />
        </div>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <label class="space-y-1.5 sm:col-span-2">
          <span class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('pelican.fields.group') }}</span>
          <Select
            v-model="form.group_id"
            :options="groupOptions"
            :placeholder="t('pelican.admin.plans.selectGroup')"
            searchable
            @change="loadModels"
          />
          <span class="block text-xs text-gray-500 dark:text-dark-300">
            {{ t('pelican.admin.plans.groupScopeHint') }}
          </span>
        </label>
        <label class="space-y-1.5 sm:col-span-2">
          <span class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('pelican.fields.model') }}</span>
          <Select
            v-model="form.model"
            :options="modelOptions"
            :placeholder="t('pelican.admin.plans.selectModel')"
            :loading="loadingModels"
            :disabled="!form.group_id"
            searchable
            creatable
          />
        </label>
        <label class="space-y-1.5">
          <span class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('pelican.fields.effort') }}</span>
          <Select v-model="form.effort" :options="effortOptions" :placeholder="t('pelican.effort.unspecified')" />
        </label>
        <label class="space-y-1.5">
          <span class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('pelican.fields.protocol') }}</span>
          <Select v-model="form.protocol" :options="protocolOptions" />
        </label>
        <label class="space-y-1.5">
          <span class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('pelican.admin.plans.interval') }}</span>
          <input v-model.number="form.interval_minutes" type="number" min="30" max="10080" required class="input" />
          <span class="block text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.plans.intervalHint') }}</span>
        </label>
        <label class="space-y-1.5">
          <span class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('pelican.fields.outputCap') }}</span>
          <input v-model.number="form.max_output_tokens" type="number" min="64" max="16384" required class="input" />
          <span class="block text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.plans.outputHint') }}</span>
        </label>
        <label class="space-y-1.5">
          <span class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('pelican.admin.plans.budget') }}</span>
          <input v-model.number="form.budget_usd" type="number" min="0.000001" max="100" step="any" required class="input" />
          <span class="block text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.plans.budgetHint') }}</span>
        </label>
        <div class="space-y-1.5">
          <span class="text-sm font-medium text-gray-700 dark:text-dark-100">{{ t('pelican.fields.promptVersion') }}</span>
          <div class="input flex items-center bg-gray-50 font-mono text-gray-600 dark:bg-dark-900/40 dark:text-dark-200">
            pelican-bicycle-v1
          </div>
          <span class="block text-xs text-gray-500 dark:text-dark-300">{{ t('pelican.admin.plans.promptFixed') }}</span>
        </div>
      </div>

      <p class="rounded-2xl border border-amber-200 bg-amber-50 p-3 text-xs leading-5 text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">
        {{ t('pelican.admin.budgetNotice') }}
      </p>

      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="$emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button type="submit" class="btn btn-primary" :disabled="saving || !valid">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </form>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { adminAPI } from '@/api/admin'
import type { AdminGroup } from '@/types'
import type {
  PelicanEffort,
  PelicanPlan,
  PelicanPlanInput,
  PelicanProtocol,
} from '@/types/pelican'

const props = defineProps<{
  show: boolean
  plan: PelicanPlan | null
  groups: AdminGroup[]
  saving: boolean
}>()

const emit = defineEmits<{
  close: []
  save: [input: PelicanPlanInput]
}>()

const { t } = useI18n()
const loadingModels = ref(false)
const modelOptions = ref<{ value: string; label: string }[]>([])
let modelRequestSequence = 0
const emptyForm = (): PelicanPlanInput => ({
  group_id: 0,
  model: '',
  effort: '',
  protocol: 'chat',
  enabled: true,
  published: false,
  interval_minutes: 1440,
  max_output_tokens: 4096,
  budget_usd: 1,
})
const form = reactive<PelicanPlanInput>(emptyForm())
const groupOptions = computed(() =>
  props.groups.map((group) => ({ value: group.id, label: `${group.name} · ${group.platform}` })),
)
const effortOptions: { value: PelicanEffort; label: string }[] = [
  { value: '', label: t('pelican.effort.unspecified') },
  { value: 'none', label: 'none' },
  { value: 'minimal', label: 'minimal' },
  { value: 'low', label: 'low' },
  { value: 'medium', label: 'medium' },
  { value: 'high', label: 'high' },
  { value: 'xhigh', label: 'xhigh' },
  { value: 'max', label: 'max' },
]
const protocolOptions: { value: PelicanProtocol; label: string }[] = [
  { value: 'chat', label: t('pelican.protocol.chat') },
  { value: 'responses', label: t('pelican.protocol.responses') },
]
const valid = computed(() =>
  form.group_id > 0
  && form.model.trim().length > 0
  && Number.isInteger(form.interval_minutes)
  && form.interval_minutes >= 30
  && form.interval_minutes <= 10080
  && Number.isInteger(form.max_output_tokens)
  && form.max_output_tokens >= 64
  && form.max_output_tokens <= 16384
  && Number.isFinite(form.budget_usd)
  && form.budget_usd > 0
  && form.budget_usd <= 100,
)

async function loadModels() {
  const requestID = ++modelRequestSequence
  const group = props.groups.find((item) => item.id === form.group_id)
  if (!group) {
    modelOptions.value = []
    return
  }
  loadingModels.value = true
  try {
    const models = group.model_allowlist?.enabled && group.model_allowlist.models.length
      ? group.model_allowlist.models
      : await adminAPI.groups.getModelAllowlistCandidates(group.id, group.platform)
    if (requestID !== modelRequestSequence) return
    modelOptions.value = models.map((model) => ({ value: model, label: model }))
  } finally {
    if (requestID === modelRequestSequence) loadingModels.value = false
  }
}

function reset() {
  Object.assign(form, props.plan ? {
    group_id: props.plan.group_id,
    model: props.plan.model,
    effort: props.plan.effort,
    protocol: props.plan.protocol,
    enabled: props.plan.enabled,
    published: props.plan.published,
    interval_minutes: props.plan.interval_minutes,
    max_output_tokens: props.plan.max_output_tokens,
    budget_usd: props.plan.budget_usd,
  } : emptyForm())
}

function submit() {
  if (!valid.value || props.saving) return
  emit('save', { ...form, model: form.model.trim() })
}

watch(
  () => props.show,
  async (open) => {
    if (!open) return
    reset()
    if (form.group_id) await loadModels()
  },
  { immediate: true },
)
</script>
