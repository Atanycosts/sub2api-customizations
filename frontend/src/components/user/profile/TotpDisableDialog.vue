<template>
  <BaseDialog
    :show="true"
    :title="t('profile.totp.disableTitle')"
    width="narrow"
    variant="profile"
    :close-on-escape="!loading"
    :show-close-button="!loading"
    @close="$emit('close')"
  >
    <p class="profile-totp-instruction">
      {{ t('profile.totp.disableWarning') }}
    </p>
    <p v-if="methodLoading">{{ t('common.loading') }}</p>
    <form v-else class="profile-dialog-form" @submit.prevent="handleDisable">
      <label v-if="verificationMethod === 'email'" class="profile-field">
        <span>{{ t('profile.totp.emailCode') }}</span>
        <div class="profile-inline-input">
          <input
            v-model="form.emailCode"
            type="text"
            maxlength="6"
            inputmode="numeric"
            class="input"
            :placeholder="t('profile.totp.enterEmailCode')"
          />
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="sendingCode || codeCooldown > 0"
            @click="handleSendCode"
          >
            {{
              codeCooldown > 0
                ? `${codeCooldown}s`
                : sendingCode
                  ? t('common.sending')
                  : t('profile.totp.sendCode')
            }}
          </button>
        </div>
      </label>
      <label v-else class="profile-field">
        <span>{{ t('profile.currentPassword') }}</span>
        <input
          id="password"
          v-model="form.password"
          type="password"
          autocomplete="current-password"
          class="input"
          :placeholder="t('profile.totp.enterPassword')"
        />
      </label>
      <div class="profile-dialog-actions">
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="loading"
          @click="$emit('close')"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          class="btn btn-danger"
          :disabled="loading || !canSubmit"
        >
          {{
            loading ? t('common.processing') : t('profile.totp.confirmDisable')
          }}
        </button>
      </div>
    </form>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { totpAPI } from '@/api'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { extractApiErrorMessage } from '@/utils/apiError'

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

const methodLoading = ref(true)
const verificationMethod = ref<'email' | 'password'>('password')
const loading = ref(false)
const sendingCode = ref(false)
let disposed = false
const codeCooldown = ref(0)
const cooldownTimer = ref<ReturnType<typeof setInterval> | null>(null)
const form = ref({
  emailCode: '',
  password: ''
})

const canSubmit = computed(() => {
  if (verificationMethod.value === 'email') {
    return form.value.emailCode.length === 6
  }
  return form.value.password.length > 0
})

const loadVerificationMethod = async () => {
  methodLoading.value = true
  try {
    const method = await totpAPI.getVerificationMethod()
    verificationMethod.value = method.method
  } catch (err: any) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
    emit('close')
  } finally {
    methodLoading.value = false
  }
}

const handleSendCode = async () => {
  sendingCode.value = true
  try {
    await totpAPI.sendVerifyCode()
    if (disposed) return
    appStore.showSuccess(t('profile.totp.codeSent'))
    // Start cooldown
    codeCooldown.value = 60
    if (cooldownTimer.value) {
      clearInterval(cooldownTimer.value)
      cooldownTimer.value = null
    }
    cooldownTimer.value = setInterval(() => {
      codeCooldown.value--
      if (codeCooldown.value <= 0) {
        if (cooldownTimer.value) {
          clearInterval(cooldownTimer.value)
          cooldownTimer.value = null
        }
      }
    }, 1000)
  } catch (err: any) {
    appStore.showError(
      extractApiErrorMessage(err, t('profile.totp.sendCodeFailed'))
    )
  } finally {
    sendingCode.value = false
  }
}

const handleDisable = async () => {
  if (!canSubmit.value) return

  loading.value = true

  try {
    const request =
      verificationMethod.value === 'email'
        ? { email_code: form.value.emailCode }
        : { password: form.value.password }

    await totpAPI.disable(request)
    appStore.showSuccess(t('profile.totp.disableSuccess'))
    emit('success')
  } catch (err: any) {
    appStore.showError(
      extractApiErrorMessage(err, t('profile.totp.disableFailed'))
    )
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadVerificationMethod()
})

onUnmounted(() => {
  disposed = true
  if (cooldownTimer.value) {
    clearInterval(cooldownTimer.value)
    cooldownTimer.value = null
  }
})
</script>
