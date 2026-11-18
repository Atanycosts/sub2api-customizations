<template>
  <section class="profile-notifications">
    <div class="profile-panel-heading">
      <h2>{{ t('profile.balanceNotify.title') }}</h2>
      <p>{{ t('profile.balanceNotify.description') }}</p>
    </div>
    <div class="profile-notification-grid">
      <div class="profile-notification-rules">
        <label class="profile-notify-toggle">
          <span>{{ t('profile.balanceNotify.enabled') }}</span>
          <input
            v-model="notifyEnabled"
            type="checkbox"
            class="profile-switch"
            :disabled="savingToggle"
            @change="handleToggle"
          />
        </label>
        <form
          class="profile-threshold-form"
          @submit.prevent="handleThresholdUpdate"
        >
          <label for="profile-notify-threshold" class="input-label">
            {{ t('profile.balanceNotify.threshold') }}
          </label>
          <div class="profile-inline-input">
            <span>$</span>
            <input
              id="profile-notify-threshold"
              v-model.number="customThreshold"
              type="number"
              min="0"
              step="0.01"
              class="input"
              :disabled="!notifyEnabled"
              :placeholder="
                systemDefaultThreshold > 0
                  ? String(systemDefaultThreshold)
                  : t('profile.balanceNotify.thresholdPlaceholder')
              "
            />
            <button
              type="submit"
              :disabled="savingThreshold || !notifyEnabled"
              class="btn btn-primary btn-sm"
            >
              {{ savingThreshold ? t('common.saving') : t('common.save') }}
            </button>
          </div>
          <p class="profile-threshold-hint">
            {{ t('profile.balanceNotify.thresholdHint') }}
            <span v-if="systemDefaultThreshold > 0">
              · ${{ systemDefaultThreshold }}
            </span>
          </p>
        </form>
      </div>
      <div class="profile-recipients">
        <div class="profile-recipient-heading">
          <h3>
            {{ t('profile.balanceNotify.extraEmails') }}
            <span>{{ emailEntries.length }} / {{ maxTotalEmails }}</span>
          </h3>
          <button
            type="button"
            data-testid="notify-email-add"
            class="profile-link"
            :disabled="!canAddMore || !notifyEnabled"
            @click="openEmailDialog()"
          >
            <Icon name="plus" size="sm" />
            {{ t('profile.balanceNotify.addEmail') }}
          </button>
        </div>
        <p v-if="!emailEntries.length" class="profile-recipient-empty">
          {{ t('profile.balanceNotify.extraEmailsHint') }}
        </p>
        <div
          v-for="entry in emailEntries"
          :key="entry.email"
          class="profile-notify-email"
          data-testid="notify-email-row"
        >
          <label class="profile-email-toggle">
            <span class="sr-only">
              {{ t('profile.balanceNotify.enabled') }}: {{ entry.email }}
            </span>
            <input
              type="checkbox"
              class="profile-switch profile-switch-small"
              :checked="!entry.disabled"
              :disabled="!notifyEnabled || !!updatingEmail"
              @change="handleEmailToggle(entry, $event)"
            />
          </label>
          <div class="profile-row-copy">
            <p class="profile-email-address">{{ entry.email }}</p>
            <span
              :class="
                entry.verified
                  ? 'text-green-600 dark:text-green-400'
                  : 'text-amber-600 dark:text-amber-400'
              "
            >
              {{
                entry.verified
                  ? t('profile.balanceNotify.verified')
                  : t('profile.balanceNotify.unverified')
              }}
            </span>
          </div>
          <button
            v-if="!entry.verified"
            type="button"
            class="profile-link"
            :disabled="!notifyEnabled"
            @click="openEmailDialog(entry.email)"
          >
            {{ t('profile.balanceNotify.verify') }}
          </button>
          <button
            type="button"
            class="profile-link text-red-600 dark:text-red-400"
            :disabled="!notifyEnabled || !!updatingEmail"
            @click="handleRemoveEmail(entry.email)"
          >
            {{ t('profile.balanceNotify.removeEmail') }}
          </button>
        </div>
      </div>
    </div>
    <BaseDialog
      :show="showEmailDialog"
      :title="
        verifyingSaved
          ? t('profile.balanceNotify.verify')
          : t('profile.balanceNotify.addEmail')
      "
      width="narrow"
      variant="profile"
      :close-on-escape="!verifying"
      :show-close-button="!verifying"
      @close="showEmailDialog = false"
    >
      <form class="profile-dialog-form" @submit.prevent="verifyEmail">
        <label class="profile-field">
          <span>{{ t('profile.email') }}</span>
          <input
            v-model.trim="emailDraft"
            type="email"
            class="input"
            required
            :disabled="verifyingSaved || sendingCode || verifying"
            :placeholder="t('profile.balanceNotify.emailPlaceholder')"
            @input="code = ''"
          />
        </label>
        <label class="profile-field">
          <span>{{ t('profile.totp.emailCode') }}</span>
          <div class="profile-inline-input">
            <input
              v-model="code"
              type="text"
              inputmode="numeric"
              maxlength="6"
              class="input"
              :placeholder="t('profile.balanceNotify.codePlaceholder')"
              :disabled="verifying"
            />
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="sendingCode || verifying || cooldown > 0"
              @click="sendCode"
            >
              {{
                cooldown > 0
                  ? `${cooldown}s`
                  : sendingCode
                    ? t('common.sending')
                    : t('profile.balanceNotify.sendCode')
              }}
            </button>
          </div>
        </label>
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('profile.balanceNotify.extraEmailsHint') }}
        </p>
        <div class="profile-dialog-actions">
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="verifying"
            @click="showEmailDialog = false"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            class="btn btn-primary"
            :disabled="code.length !== 6 || verifying"
          >
            {{
              verifying
                ? t('common.verifying')
                : t('profile.balanceNotify.verify')
            }}
          </button>
        </div>
      </form>
    </BaseDialog>
  </section>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { userAPI } from '@/api'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { NotifyEmailEntry } from '@/types'

const maxTotalEmails = 3
const props = defineProps<{
  enabled: boolean
  threshold: number | null
  extraEmails: NotifyEmailEntry[]
  systemDefaultThreshold: number
  userEmail: string
}>()
const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const notifyEnabled = ref(props.enabled)
const customThreshold = ref<number | string | null>(props.threshold)
let savedThreshold = props.threshold
const emailEntries = ref([...props.extraEmails])
const savingToggle = ref(false)
const savingThreshold = ref(false)
const updatingEmail = ref('')
const showEmailDialog = ref(false)
const verifyingSaved = ref(false)
const emailDraft = ref('')
const code = ref('')
const sendingCode = ref(false)
const verifying = ref(false)
const cooldown = ref(0)
let codeRecipient = ''
let cooldownTimer: ReturnType<typeof setInterval> | undefined
let disposed = false
const canAddMore = computed(() => emailEntries.value.length < maxTotalEmails)

watch(
  () => props.enabled,
  (value) => {
    notifyEnabled.value = value
  }
)
watch(
  () => props.threshold,
  (value) => {
    if (customThreshold.value === savedThreshold) customThreshold.value = value
    savedThreshold = value
  }
)
watch(
  () => props.extraEmails,
  (value) => {
    emailEntries.value = [...value]
  }
)
onUnmounted(() => {
  disposed = true
  clearInterval(cooldownTimer)
})

function showError(error: unknown) {
  appStore.showError(extractApiErrorMessage(error, t('common.error')))
}
async function handleToggle() {
  savingToggle.value = true
  try {
    authStore.user = await userAPI.updateProfile({
      balance_notify_enabled: notifyEnabled.value
    })
  } catch (error) {
    notifyEnabled.value = !notifyEnabled.value
    showError(error)
  } finally {
    savingToggle.value = false
  }
}
async function handleThresholdUpdate() {
  savingThreshold.value = true
  const draft = customThreshold.value
  try {
    const updated = await userAPI.updateProfile({
      balance_notify_threshold: Number(draft) > 0 ? Number(draft) : 0
    })
    if (customThreshold.value === draft)
      customThreshold.value = updated.balance_notify_threshold
    savedThreshold = updated.balance_notify_threshold
    authStore.user = updated
    appStore.showSuccess(t('common.saved'))
  } catch (error) {
    showError(error)
  } finally {
    savingThreshold.value = false
  }
}
async function handleEmailToggle(entry: NotifyEmailEntry, event: Event) {
  updatingEmail.value = entry.email
  try {
    const updated = await userAPI.toggleNotifyEmail(
      entry.email,
      !entry.disabled
    )
    authStore.user = updated
    emailEntries.value = [...updated.balance_notify_extra_emails]
  } catch (error) {
    const input = event.target as HTMLInputElement
    input.checked = !entry.disabled
    showError(error)
  } finally {
    updatingEmail.value = ''
  }
}
async function refreshEmails() {
  const updated = await userAPI.getProfile()
  authStore.user = updated
  emailEntries.value = [...updated.balance_notify_extra_emails]
}
async function handleRemoveEmail(email: string) {
  updatingEmail.value = email
  try {
    await userAPI.removeNotifyEmail(email)
    await refreshEmails()
    appStore.showSuccess(t('profile.balanceNotify.removeSuccess'))
  } catch (error) {
    showError(error)
  } finally {
    updatingEmail.value = ''
  }
}
function openEmailDialog(savedEmail?: string) {
  verifyingSaved.value = !!savedEmail
  if (savedEmail || !emailDraft.value)
    emailDraft.value =
      savedEmail || (emailEntries.value.length === 0 ? props.userEmail : '')
  if (emailDraft.value !== codeRecipient) code.value = ''
  showEmailDialog.value = true
}
async function sendCode() {
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailDraft.value)) {
    appStore.showError(t('auth.invalidEmail'))
    return
  }
  if (
    !verifyingSaved.value &&
    emailEntries.value.some(
      (entry) => entry.email.toLowerCase() === emailDraft.value.toLowerCase()
    )
  ) {
    appStore.showError(t('profile.balanceNotify.emailDuplicate'))
    return
  }
  sendingCode.value = true
  const recipient = emailDraft.value
  try {
    await userAPI.sendNotifyEmailCode(recipient)
    if (disposed) return
    codeRecipient = recipient
    cooldown.value = 60
    clearInterval(cooldownTimer)
    cooldownTimer = setInterval(() => {
      if (--cooldown.value <= 0) clearInterval(cooldownTimer)
    }, 1000)
    appStore.showSuccess(t('profile.balanceNotify.codeSent'))
  } catch (error) {
    showError(error)
  } finally {
    sendingCode.value = false
  }
}
async function verifyEmail() {
  verifying.value = true
  try {
    await userAPI.verifyNotifyEmail(emailDraft.value, code.value)
    await refreshEmails()
    showEmailDialog.value = false
    emailDraft.value = ''
    code.value = ''
    cooldown.value = 0
    clearInterval(cooldownTimer)
    appStore.showSuccess(t('profile.balanceNotify.verifySuccess'))
  } catch (error) {
    showError(error)
  } finally {
    verifying.value = false
  }
}
</script>
