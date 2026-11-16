<template>
  <section data-testid="profile-overview-hero" class="profile-overview">
    <div class="profile-overview-identity">
      <div class="profile-overview-avatar">
        <img v-if="avatarUrl" :src="avatarUrl" :alt="displayName" />
        <span v-else>{{ avatarInitial }}</span>
      </div>
      <div class="min-w-0">
        <div class="profile-overview-name">
          <h2>{{ displayName }}</h2>
          <span
            :class="[
              'badge',
              user?.role === 'admin' ? 'badge-primary' : 'badge-gray'
            ]"
          >
            {{
              user?.role === 'admin'
                ? t('profile.administrator')
                : t('profile.user')
            }}
          </span>
          <span
            :class="[
              'badge',
              user?.status === 'active' ? 'badge-success' : 'badge-danger'
            ]"
          >
            {{
              user?.status === 'active'
                ? t('common.active')
                : t('common.disabled')
            }}
          </span>
        </div>
        <p v-if="primaryEmailDisplay" class="profile-overview-email">
          {{ primaryEmailDisplay }}
        </p>
      </div>
    </div>
    <dl class="profile-overview-metrics">
      <div data-testid="profile-overview-metric-balance">
        <dt>{{ t('profile.accountBalance') }}</dt>
        <dd>{{ formatCurrency(user?.balance || 0) }}</dd>
      </div>
      <div data-testid="profile-overview-metric-concurrency">
        <dt>{{ t('profile.concurrencyLimit') }}</dt>
        <dd>{{ user?.concurrency || 0 }}</dd>
      </div>
      <div data-testid="profile-overview-metric-member-since">
        <dt>{{ t('profile.memberSince') }}</dt>
        <dd>{{ memberSinceLabel }}</dd>
      </div>
    </dl>
    <button
      type="button"
      class="profile-link profile-overview-details"
      @click="showDetails = true"
    >
      {{ t('profile.accountDetails') }}
    </button>
    <BaseDialog
      :show="showDetails"
      :title="t('profile.accountDetails')"
      width="narrow"
      variant="profile"
      @close="showDetails = false"
    >
      <dl class="profile-account-details">
        <div>
          <dt>{{ t('profile.role') }}</dt>
          <dd>
            {{
              user?.role === 'admin'
                ? t('profile.administrator')
                : t('profile.user')
            }}
          </dd>
        </div>
        <div>
          <dt>{{ t('profile.status') }}</dt>
          <dd>
            {{
              user?.status === 'active'
                ? t('common.active')
                : t('common.disabled')
            }}
          </dd>
        </div>
        <div>
          <dt>{{ t('profile.accountBalance') }}</dt>
          <dd>{{ formatCurrency(user?.balance || 0) }}</dd>
        </div>
        <div>
          <dt>{{ t('profile.concurrencyLimit') }}</dt>
          <dd>{{ user?.concurrency || 0 }}</dd>
        </div>
        <div>
          <dt>{{ t('profile.memberSince') }}</dt>
          <dd>{{ memberSinceLabel }}</dd>
        </div>
      </dl>
      <div v-if="sourceHints.length" class="profile-source-details">
        <p v-for="hint in sourceHints" :key="hint.key">
          <Icon name="link" size="sm" />
          {{ hint.text }}
        </p>
      </div>
    </BaseDialog>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { profileEmail } from '@/utils/profileIdentity'
import type { User, UserAuthProvider, UserProfileSourceContext } from '@/types'

const props = withDefaults(
  defineProps<{
    user: User | null
    oidcProviderName?: string
  }>(),
  {
    oidcProviderName: 'OIDC'
  }
)

const { t } = useI18n()
const showDetails = ref(false)

const avatarUrl = computed(() => props.user?.avatar_url?.trim() || '')
const displayName = computed(
  () =>
    props.user?.username?.trim() ||
    props.user?.email?.trim() ||
    t('profile.user')
)
const primaryEmailDisplay = computed(() => profileEmail(props.user))
const avatarInitial = computed(
  () => displayName.value.charAt(0).toUpperCase() || 'U'
)
const memberSinceLabel = computed(() => {
  const raw = props.user?.created_at?.trim()
  if (!raw) {
    return '-'
  }

  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) {
    return '-'
  }

  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short'
  }).format(date)
})

const providerLabels = computed<Record<UserAuthProvider, string>>(() => ({
  email: t('profile.authBindings.providers.email'),
  linuxdo: t('profile.authBindings.providers.linuxdo'),
  dingtalk: t('profile.authBindings.providers.dingtalk'),
  oidc: t('profile.authBindings.providers.oidc', {
    providerName: props.oidcProviderName
  }),
  wechat: t('profile.authBindings.providers.wechat'),
  github: 'GitHub',
  google: 'Google'
}))

function formatCurrency(value: number): string {
  return `$${value.toFixed(2)}`
}

function normalizeProvider(value: string): UserAuthProvider | null {
  const normalized = value.trim().toLowerCase()
  if (
    normalized === 'email' ||
    normalized === 'linuxdo' ||
    normalized === 'dingtalk' ||
    normalized === 'wechat' ||
    normalized === 'github' ||
    normalized === 'google'
  ) {
    return normalized
  }
  if (
    normalized === 'oidc' ||
    normalized.startsWith('oidc:') ||
    normalized.startsWith('oidc/')
  ) {
    return 'oidc'
  }
  return null
}

function readObjectString(
  source: Record<string, unknown>,
  ...keys: string[]
): string {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
  }
  return ''
}

function resolveThirdPartySource(
  rawSource: string | UserProfileSourceContext | null | undefined
): { provider: UserAuthProvider; label: string } | null {
  if (!rawSource) {
    return null
  }

  if (typeof rawSource === 'string') {
    const provider = normalizeProvider(rawSource)
    if (!provider || provider === 'email') {
      return null
    }
    return {
      provider,
      label: providerLabels.value[provider]
    }
  }

  const sourceRecord = rawSource as Record<string, unknown>
  const provider = normalizeProvider(
    readObjectString(
      sourceRecord,
      'provider',
      'source',
      'provider_type',
      'auth_provider'
    )
  )
  if (!provider || provider === 'email') {
    return null
  }

  const explicitLabel = readObjectString(
    sourceRecord,
    'provider_label',
    'label',
    'provider_name',
    'providerName'
  )

  return {
    provider,
    label: explicitLabel || providerLabels.value[provider]
  }
}

const sourceHints = computed(() => {
  const currentUser = props.user
  if (!currentUser) {
    return []
  }

  const hints: Array<{ key: string; text: string }> = []
  const avatarSource = resolveThirdPartySource(
    currentUser.profile_sources?.avatar ?? currentUser.avatar_source
  )
  const usernameSource = resolveThirdPartySource(
    currentUser.profile_sources?.username ??
      currentUser.profile_sources?.display_name ??
      currentUser.profile_sources?.nickname ??
      currentUser.display_name_source ??
      currentUser.username_source ??
      currentUser.nickname_source
  )

  if (avatarSource) {
    hints.push({
      key: 'avatar',
      text: t('profile.authBindings.source.avatar', {
        providerName: avatarSource.label
      })
    })
  }

  if (usernameSource) {
    hints.push({
      key: 'username',
      text: t('profile.authBindings.source.username', {
        providerName: usernameSource.label
      })
    })
  }

  return hints
})
defineExpose({ sourceHints })
</script>
