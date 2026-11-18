<template>
  <section class="profile-security-row">
    <span class="profile-feature-icon"><Icon name="key" size="md" /></span>
    <div class="profile-row-copy">
      <div class="profile-row-title">
        <h3>{{ t('profile.passkey.title') }}</h3>
        <span v-if="enabled && !loading && !loadError" class="badge badge-gray">
          {{ t('profile.passkey.count', { count: credentials.length }) }}
        </span>
      </div>
      <p v-if="!enabled">{{ t('profile.passkey.featureDisabled') }}</p>
      <p v-else-if="!supported">{{ t('profile.passkey.unsupported') }}</p>
      <p v-else>{{ t('profile.passkey.description') }}</p>
    </div>
    <button
      v-if="enabled"
      type="button"
      class="btn btn-secondary btn-sm"
      @click="dialogMode = 'list'"
    >
      {{ t('profile.manageShort') }}
    </button>
    <BaseDialog
      :show="dialogMode !== null"
      :title="dialogTitle"
      width="narrow"
      variant="profile"
      :close-on-escape="!busy"
      :show-close-button="!busy"
      @close="closeDialog"
    >
      <template v-if="dialogMode === 'list'">
        <div class="profile-passkey-toolbar">
          <p>{{ t('profile.passkey.count', { count: credentials.length }) }}</p>
          <button
            v-if="enabled && supported"
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="busy"
            @click="dialogMode = 'add'"
          >
            {{ t('profile.passkey.add') }}
          </button>
        </div>
        <p v-if="loading" class="profile-list-empty">
          {{ t('common.loading') }}
        </p>
        <div v-else-if="loadError" class="profile-passkey-toolbar" role="alert">
          <p>{{ t('profile.passkey.loadFailed') }}</p>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            @click="loadCredentials"
          >
            {{ t('common.refresh') }}
          </button>
        </div>
        <p v-else-if="!credentials.length" class="profile-list-empty">
          {{ t('profile.passkey.empty') }}
        </p>
        <div
          v-for="credential in visibleCredentials"
          v-else
          :key="credential.id"
          class="profile-passkey-row"
          data-testid="passkey-row"
        >
          <div class="profile-row-copy">
            <div class="profile-row-title">
              <h3>{{ credential.name }}</h3>
              <span v-if="credential.backup" class="badge badge-success">
                {{ t('profile.passkey.synced') }}
              </span>
            </div>
            <p>
              {{
                t('profile.passkey.createdAt', {
                  date: formatDate(credential.created_at)
                })
              }}
            </p>
            <p v-if="credential.last_used_at">
              {{
                t('profile.passkey.lastUsed', {
                  date: formatDate(credential.last_used_at)
                })
              }}
            </p>
          </div>
          <button
            type="button"
            class="profile-link"
            :disabled="busy"
            @click="editPasskey(credential)"
          >
            {{ t('common.edit') }}
          </button>
        </div>
        <div v-if="pageCount > 1" class="profile-list-pager">
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="page === 1"
            :aria-label="t('pagination.previous')"
            @click="page--"
          >
            <Icon name="chevronLeft" size="sm" />
          </button>
          <span>{{ page }} / {{ pageCount }}</span>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="page === pageCount"
            :aria-label="t('pagination.next')"
            @click="page++"
          >
            <Icon name="chevronRight" size="sm" />
          </button>
        </div>
      </template>
      <form
        v-else-if="dialogMode === 'add'"
        class="profile-dialog-form"
        @submit.prevent="addPasskey"
      >
        <label class="profile-field">
          <span>{{ t('profile.passkey.name') }}</span>
          <input
            id="passkey-name"
            v-model="newName"
            class="input"
            maxlength="100"
            :placeholder="t('profile.passkey.namePlaceholder')"
          />
        </label>
        <label class="profile-field">
          <span>{{ t('profile.currentPassword') }}</span>
          <input
            id="passkey-add-password"
            v-model="newPassword"
            type="password"
            autocomplete="current-password"
            class="input"
            :placeholder="t('profile.passkey.passwordPlaceholder')"
          />
        </label>
        <div class="profile-dialog-actions">
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="busy"
            @click="cancelAdd"
          >
            {{ t('common.back') }}
          </button>
          <button
            type="submit"
            class="btn btn-primary"
            :disabled="busy || !newPassword"
          >
            {{ busy ? t('common.processing') : t('profile.passkey.continue') }}
          </button>
        </div>
      </form>
      <form
        v-else-if="dialogMode === 'edit'"
        class="profile-dialog-form"
        @submit.prevent="renamePasskey"
      >
        <label class="profile-field">
          <span>{{ t('profile.passkey.name') }}</span>
          <input v-model="editName" class="input" maxlength="100" required />
        </label>
        <div class="profile-dialog-actions">
          <button
            type="button"
            class="profile-link text-red-600 dark:text-red-400"
            :disabled="busy"
            @click="deletePasskey(editTarget!)"
          >
            {{ t('common.delete') }}
          </button>
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="busy"
            @click="dialogMode = 'list'"
          >
            {{ t('common.back') }}
          </button>
          <button
            type="submit"
            class="btn btn-primary"
            :disabled="busy || !editName.trim()"
          >
            {{ t('common.save') }}
          </button>
        </div>
      </form>
      <form
        v-else-if="dialogMode === 'delete'"
        class="profile-dialog-form"
        @submit.prevent="confirmDelete"
      >
        <p>
          {{ t('profile.passkey.deleteConfirm', { name: deleteTarget?.name }) }}
        </p>
        <label class="profile-field">
          <span>{{ t('profile.currentPassword') }}</span>
          <input
            id="passkey-delete-password"
            v-model="deletePassword"
            type="password"
            autocomplete="current-password"
            class="input"
            :placeholder="t('profile.passkey.passwordPlaceholder')"
          />
        </label>
        <div class="profile-dialog-actions">
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="busy"
            @click="closeDeleteDialog"
          >
            {{ t('common.back') }}
          </button>
          <button
            type="submit"
            class="btn btn-danger"
            :disabled="busy || !deletePassword"
          >
            {{ busy ? t('common.processing') : t('common.delete') }}
          </button>
        </div>
      </form>
    </BaseDialog>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useWindowSize } from '@vueuse/core'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useI18n } from 'vue-i18n'
import { passkeyAPI, type PasskeyCredentialSummary } from '@/api'
import { Icon } from '@/components/icons'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ enabled: boolean }>()

const { t } = useI18n()
const appStore = useAppStore()
const supported = passkeyAPI.isSupported()
const loading = ref(false)
const loadError = ref(false)
const busy = ref(false)
const credentials = ref<PasskeyCredentialSummary[]>([])
const dialogMode = ref<'list' | 'add' | 'edit' | 'delete' | null>(null)
const editTarget = ref<PasskeyCredentialSummary | null>(null)
const editName = ref('')
const page = ref(1)
const { width } = useWindowSize()
const pageSize = computed(() => (width.value < 640 ? 3 : 4))
const pageCount = computed(() =>
  Math.max(1, Math.ceil(credentials.value.length / pageSize.value))
)
const visibleCredentials = computed(() =>
  credentials.value.slice(
    (page.value - 1) * pageSize.value,
    page.value * pageSize.value
  )
)
watch(pageCount, (count) => {
  page.value = Math.min(page.value, count)
})
const dialogTitle = computed(() =>
  dialogMode.value === 'add'
    ? t('profile.passkey.add')
    : dialogMode.value === 'edit'
      ? t('common.edit')
      : dialogMode.value === 'delete'
        ? t('profile.passkey.deleteTitle')
        : t('profile.passkey.manage')
)
const newName = ref('')
const newPassword = ref('')
const deleteTarget = ref<PasskeyCredentialSummary | null>(null)
const deletePassword = ref('')

// apiClient 拦截器把错误规范化为 { code, reason, message }；
// 透出后端消息（如密码错误），否则回退到通用文案。
function extractErrorMessage(error: unknown, fallback: string): string {
  const message = (error as { message?: string }).message
  return typeof message === 'string' && message.length > 0 ? message : fallback
}

async function loadCredentials(): Promise<void> {
  if (!props.enabled) {
    credentials.value = []
    return
  }
  loading.value = true
  loadError.value = false
  try {
    credentials.value = await passkeyAPI.list()
  } catch (error) {
    // 字符串错误码在 reason 字段（code 是数字状态码）；
    // 设置变更竞态下后端仍可能返回 PASSKEY_DISABLED，静默处理
    const reason = (error as { reason?: string }).reason
    if (reason !== 'PASSKEY_DISABLED') {
      loadError.value = true
      appStore.showError(t('profile.passkey.loadFailed'))
    }
  } finally {
    loading.value = false
  }
}

async function addPasskey(): Promise<void> {
  if (newPassword.value.length === 0) return
  busy.value = true
  try {
    await passkeyAPI.register(newName.value.trim(), newPassword.value)
    appStore.showSuccess(t('profile.passkey.added'))
    cancelAdd()
    await loadCredentials()
    page.value = pageCount.value
  } catch (error) {
    if (!(error instanceof DOMException && error.name === 'NotAllowedError')) {
      appStore.showError(
        extractErrorMessage(error, t('profile.passkey.addFailed'))
      )
    }
  } finally {
    busy.value = false
  }
}

function cancelAdd(): void {
  dialogMode.value = 'list'
  newName.value = ''
  newPassword.value = ''
}

function editPasskey(credential: PasskeyCredentialSummary): void {
  editTarget.value = credential
  editName.value = credential.name
  dialogMode.value = 'edit'
}

async function renamePasskey(): Promise<void> {
  const credential = editTarget.value
  const name = editName.value.trim()
  if (!credential || !name) return
  busy.value = true
  try {
    await passkeyAPI.rename(credential.id, name)
    credential.name = name
    dialogMode.value = 'list'
    appStore.showSuccess(t('profile.passkey.renamed'))
  } catch {
    appStore.showError(t('profile.passkey.renameFailed'))
  } finally {
    busy.value = false
  }
}

function closeDialog(): void {
  if (busy.value) return
  dialogMode.value = null
  newPassword.value = ''
  deletePassword.value = ''
}

function deletePasskey(credential: PasskeyCredentialSummary): void {
  deleteTarget.value = credential
  deletePassword.value = ''
  dialogMode.value = 'delete'
}

function closeDeleteDialog(): void {
  dialogMode.value = 'list'
  deleteTarget.value = null
  deletePassword.value = ''
}

async function confirmDelete(): Promise<void> {
  const credential = deleteTarget.value
  if (!credential || deletePassword.value.length === 0) return
  busy.value = true
  try {
    await passkeyAPI.remove(credential.id, deletePassword.value)
    credentials.value = credentials.value.filter(
      (item) => item.id !== credential.id
    )
    appStore.showSuccess(t('profile.passkey.deleted'))
    closeDeleteDialog()
  } catch (error) {
    // 密码错误等失败保持对话框打开，允许重试
    appStore.showError(
      extractErrorMessage(error, t('profile.passkey.deleteFailed'))
    )
  } finally {
    busy.value = false
  }
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  }).format(new Date(value))
}

watch(
  () => props.enabled,
  () => {
    void loadCredentials()
  },
  { immediate: true }
)
</script>
