<template>
  <BaseDialog
    :show="true"
    :title="t('profile.totp.setupTitle')"
    width="narrow"
    variant="profile"
    :close-on-escape="!setupLoading && !verifying"
    :show-close-button="!setupLoading && !verifying"
    @close="$emit('close')"
  >
    <div v-if="step === 0" class="profile-totp-identity">
      <p class="profile-totp-instruction">
        {{
          verificationMethod === 'email'
            ? t('profile.totp.verifyEmailFirst')
            : t('profile.totp.verifyPasswordFirst')
        }}
      </p>
      <p v-if="methodLoading">{{ t('common.loading') }}</p>
      <form
        v-else
        class="profile-dialog-form"
        @submit.prevent="handleVerifyAndSetup"
      >
        <label v-if="verificationMethod === 'email'" class="profile-field">
          <span>{{ t('profile.totp.emailCode') }}</span>
          <div class="profile-inline-input">
            <input
              v-model="verifyForm.emailCode"
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
            v-model="verifyForm.password"
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
            :disabled="setupLoading"
            @click="$emit('close')"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            class="btn btn-primary"
            :disabled="!canProceedFromVerify || setupLoading"
          >
            {{ setupLoading ? t('common.loading') : t('common.next') }}
          </button>
        </div>
      </form>
    </div>
    <form
      v-else-if="setupData"
      class="profile-totp-confirm"
      @submit.prevent="handleVerify"
    >
      <p class="profile-totp-instruction">{{ t('profile.totp.setupStep1') }}</p>
      <img
        v-if="qrCodeDataUrl"
        :src="qrCodeDataUrl"
        alt="QR Code"
        class="profile-totp-qr"
      />
      <div class="profile-totp-secret">
        <p>{{ t('profile.totp.manualEntry') }}</p>
        <div>
          <code>{{ setupData.secret }}</code>
          <button
            type="button"
            class="profile-icon-button"
            :aria-label="t('common.copy')"
            @click="copySecret"
          >
            <Icon name="copy" size="sm" />
          </button>
        </div>
      </div>
      <label class="profile-totp-code-label">
        {{ t('profile.totp.enterCode') }}
      </label>
      <div class="profile-totp-code">
        <input
          v-for="(_, index) in 6"
          :key="index"
          :ref="(el) => setInputRef(el, index)"
          :value="code[index]"
          type="text"
          maxlength="1"
          inputmode="numeric"
          pattern="[0-9]"
          :aria-label="t('profile.totp.codeDigit', { digit: index + 1 })"
          @input="handleCodeInput($event, index)"
          @keydown="handleKeydown($event, index)"
          @paste="handlePaste"
        />
      </div>
      <div class="profile-dialog-actions">
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="verifying"
          @click="$emit('close')"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          class="btn btn-primary"
          :disabled="verifying || code.join('').length !== 6"
        >
          {{ verifying ? t('common.verifying') : t('profile.totp.verify') }}
        </button>
      </div>
    </form>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { totpAPI } from '@/api'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { TotpSetupResponse } from '@/types'
import QRCode from 'qrcode'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

// Verify identity before generating a secret, then scan and confirm on the same screen.
const step = ref(0)
const methodLoading = ref(true)
const verificationMethod = ref<'email' | 'password'>('password')
const verifyForm = ref({ emailCode: '', password: '' })
const sendingCode = ref(false)
let disposed = false
const codeCooldown = ref(0)
const cooldownTimer = ref<ReturnType<typeof setInterval> | null>(null)

const setupLoading = ref(false)
const setupData = ref<TotpSetupResponse | null>(null)
const verifying = ref(false)
const code = ref<string[]>(['', '', '', '', '', ''])
const inputRefs = ref<(HTMLInputElement | null)[]>([])
const qrCodeDataUrl = ref('')

const canProceedFromVerify = computed(() => {
  if (verificationMethod.value === 'email') {
    return verifyForm.value.emailCode.length === 6
  }
  return verifyForm.value.password.length > 0
})

// Generate QR code as base64 when setupData changes
watch(
  () => setupData.value?.qr_code_url,
  async (url) => {
    if (url) {
      try {
        qrCodeDataUrl.value = await QRCode.toDataURL(url, {
          width: 200,
          margin: 2,
          color: {
            dark: '#000000',
            light: '#ffffff'
          }
        })
      } catch (err) {
        console.error('Failed to generate QR code:', err)
      }
    }
  },
  { immediate: true }
)

const setInputRef = (el: any, index: number) => {
  inputRefs.value[index] = el as HTMLInputElement | null
}

const handleCodeInput = (event: Event, index: number) => {
  const input = event.target as HTMLInputElement
  const value = input.value.replace(/[^0-9]/g, '')
  code.value[index] = value

  if (value && index < 5) {
    nextTick(() => {
      inputRefs.value[index + 1]?.focus()
    })
  }
}

const handleKeydown = (event: KeyboardEvent, index: number) => {
  if (event.key === 'Backspace') {
    const input = event.target as HTMLInputElement
    // If current cell is empty and not the first, move to previous cell
    if (!input.value && index > 0) {
      event.preventDefault()
      inputRefs.value[index - 1]?.focus()
    }
    // Otherwise, let the browser handle the backspace naturally
    // The input event will sync code.value via handleCodeInput
  }
}

const handlePaste = (event: ClipboardEvent) => {
  event.preventDefault()
  const pastedData = event.clipboardData?.getData('text') || ''
  const digits = pastedData
    .replace(/[^0-9]/g, '')
    .slice(0, 6)
    .split('')

  // Update both the ref and the input elements
  digits.forEach((digit, index) => {
    code.value[index] = digit
    if (inputRefs.value[index]) {
      inputRefs.value[index]!.value = digit
    }
  })

  // Clear remaining inputs if pasted less than 6 digits
  for (let i = digits.length; i < 6; i++) {
    code.value[i] = ''
    if (inputRefs.value[i]) {
      inputRefs.value[i]!.value = ''
    }
  }

  const focusIndex = Math.min(digits.length, 5)
  nextTick(() => {
    inputRefs.value[focusIndex]?.focus()
  })
}

const copySecret = async () => {
  if (setupData.value) {
    try {
      await navigator.clipboard.writeText(setupData.value.secret)
      appStore.showSuccess(t('common.copied'))
    } catch {
      appStore.showError(t('common.copyFailed'))
    }
  }
}

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

const handleVerifyAndSetup = async () => {
  setupLoading.value = true

  try {
    const request =
      verificationMethod.value === 'email'
        ? { email_code: verifyForm.value.emailCode }
        : { password: verifyForm.value.password }

    setupData.value = await totpAPI.initiateSetup(request)
    step.value = 1
  } catch (err: any) {
    appStore.showError(
      extractApiErrorMessage(err, t('profile.totp.setupFailed'))
    )
  } finally {
    setupLoading.value = false
  }
}

const handleVerify = async () => {
  const totpCode = code.value.join('')
  if (totpCode.length !== 6 || !setupData.value) return

  verifying.value = true

  try {
    await totpAPI.enable({
      totp_code: totpCode,
      setup_token: setupData.value.setup_token
    })
    appStore.showSuccess(t('profile.totp.enableSuccess'))
    emit('success')
  } catch (err: any) {
    appStore.showError(
      extractApiErrorMessage(err, t('profile.totp.verifyFailed'))
    )
    code.value = ['', '', '', '', '', '']
    nextTick(() => {
      inputRefs.value[0]?.focus()
    })
  } finally {
    verifying.value = false
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
