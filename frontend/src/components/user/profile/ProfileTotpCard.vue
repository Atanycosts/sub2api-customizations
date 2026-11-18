<template>
  <section class="profile-security-row">
    <span class="profile-feature-icon"><Icon name="shield" size="md" /></span>
    <div class="profile-row-copy">
      <div class="profile-row-title">
        <h3>{{ t('profile.totp.title') }}</h3>
        <span
          v-if="status && !loading && !statusError"
          :class="['badge', status.enabled ? 'badge-success' : 'badge-gray']"
        >
          {{
            !status.feature_enabled
              ? t('profile.totp.featureDisabled')
              : status.enabled
                ? t('profile.totp.enabled')
                : t('profile.totp.notEnabled')
          }}
        </span>
      </div>
      <p v-if="loading">{{ t('common.loading') }}</p>
      <p v-else-if="statusError" role="alert">
        {{ t('profile.totp.statusLoadFailed') }}
      </p>
      <p v-else-if="status && !status.feature_enabled">
        {{ t('profile.totp.featureDisabledHint') }}
      </p>
      <p v-else-if="status?.enabled && status.enabled_at">
        {{ t('profile.totp.enabledAt') }}: {{ formatDate(status.enabled_at) }}
      </p>
      <p v-else>{{ t('profile.totp.description') }}</p>
    </div>
    <button
      v-if="statusError"
      type="button"
      class="btn btn-secondary btn-sm"
      @click="loadStatus"
    >
      {{ t('common.refresh') }}
    </button>
    <button
      v-else-if="!loading && status?.feature_enabled"
      type="button"
      class="btn btn-secondary btn-sm"
      @click="
        status.enabled ? (showDisableDialog = true) : (showSetupModal = true)
      "
    >
      {{
        status.enabled ? t('profile.totp.disable') : t('profile.totp.enable')
      }}
    </button>
    <TotpSetupModal
      v-if="showSetupModal"
      @close="showSetupModal = false"
      @success="handleSetupSuccess"
    />
    <TotpDisableDialog
      v-if="showDisableDialog"
      @close="showDisableDialog = false"
      @success="handleDisableSuccess"
    />
  </section>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { totpAPI } from '@/api'
import Icon from '@/components/icons/Icon.vue'
import type { TotpStatus } from '@/types'
import TotpSetupModal from './TotpSetupModal.vue'
import TotpDisableDialog from './TotpDisableDialog.vue'

const { t } = useI18n()

const loading = ref(true)
const status = ref<TotpStatus | null>(null)
const statusError = ref(false)
const showSetupModal = ref(false)
const showDisableDialog = ref(false)

const loadStatus = async () => {
  loading.value = true
  statusError.value = false
  try {
    status.value = await totpAPI.getStatus()
  } catch {
    statusError.value = true
  } finally {
    loading.value = false
  }
}

const handleSetupSuccess = () => {
  showSetupModal.value = false
  loadStatus()
}

const handleDisableSuccess = () => {
  showDisableDialog.value = false
  loadStatus()
}

const formatDate = (timestamp: number) => {
  // Backend returns Unix timestamp in seconds, convert to milliseconds
  const date = new Date(timestamp * 1000)
  return date.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  })
}

onMounted(() => {
  loadStatus()
})
</script>
