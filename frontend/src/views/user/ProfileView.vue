<template>
  <AppLayout viewport>
    <div data-testid="profile-shell" class="profile-shell">
      <ProfileInfoCard
        ref="overview"
        :user="user"
        :oidc-provider-name="oidcOAuthProviderName"
      />
      <section class="profile-workspace">
        <nav
          class="profile-tabs"
          role="tablist"
          :aria-label="t('profile.title')"
        >
          <button
            v-for="tab in tabs"
            :id="`profile-tab-${tab.key}`"
            :key="tab.key"
            type="button"
            role="tab"
            :aria-selected="selectedTab === tab.key"
            :aria-controls="`profile-panel-${tab.key}`"
            @click="selectedTab = tab.key"
            @keydown="onTabKeydown($event, tab.key)"
          >
            <Icon :name="tab.icon" size="sm" />
            <span>{{ tab.label }}</span>
          </button>
        </nav>
        <div class="profile-panel-area">
          <section
            id="profile-panel-basics"
            v-show="selectedTab === 'basics'"
            role="tabpanel"
            aria-labelledby="profile-tab-basics"
            class="profile-panel"
          >
            <div class="profile-panel-heading">
              <h2>{{ t('profile.tabs.basics') }}</h2>
              <p>{{ t('profile.basicsDescription') }}</p>
            </div>
            <div class="profile-basics-grid">
              <div class="profile-avatar-column">
                <ProfileAvatarCard :user="user" embedded />
                <div
                  v-if="overview?.sourceHints.length"
                  class="profile-source-hints"
                >
                  <p v-for="hint in overview.sourceHints" :key="hint.key">
                    <Icon name="link" size="sm" />
                    {{ hint.text }}
                  </p>
                </div>
              </div>
              <ProfileEditForm
                :initial-username="user?.username || ''"
                embedded
              >
                <div class="profile-primary-email">
                  <span>{{ t('profile.email') }}</span>
                  <div>
                    <p v-if="displayEmail">{{ displayEmail }}</p>
                    <span
                      :class="[
                        'badge',
                        emailBound ? 'badge-success' : 'badge-gray'
                      ]"
                    >
                      {{
                        emailBound
                          ? t('profile.authBindings.status.bound')
                          : t('profile.authBindings.status.notBound')
                      }}
                    </span>
                  </div>
                  <button
                    type="button"
                    class="profile-link"
                    @click="bindings?.manageEmail()"
                  >
                    {{
                      emailBound
                        ? t('profile.authBindings.confirmEmailReplaceAction')
                        : t('profile.authBindings.confirmEmailBindAction')
                    }}
                    <Icon name="arrowRight" size="sm" />
                  </button>
                </div>
              </ProfileEditForm>
            </div>
          </section>
          <section
            id="profile-panel-bindings"
            v-show="selectedTab === 'bindings'"
            role="tabpanel"
            aria-labelledby="profile-tab-bindings"
            class="profile-panel"
          >
            <ProfileIdentityBindingsSection
              ref="bindings"
              :user="user"
              :linuxdo-enabled="linuxdoOAuthEnabled"
              :dingtalk-enabled="dingtalkOAuthEnabled"
              :oidc-enabled="oidcOAuthEnabled"
              :oidc-provider-name="oidcOAuthProviderName"
              :wechat-enabled="wechatOAuthEnabled"
              :wechat-open-enabled="wechatOAuthOpenEnabled"
              :wechat-mp-enabled="wechatOAuthMPEnabled"
            />
          </section>
          <section
            id="profile-panel-security"
            v-show="selectedTab === 'security'"
            role="tabpanel"
            aria-labelledby="profile-tab-security"
            class="profile-panel"
          >
            <div class="profile-panel-heading">
              <h2>{{ t('profile.securityTitle') }}</h2>
              <p>{{ t('profile.securityDescription') }}</p>
            </div>
            <div class="profile-security-list">
              <section class="profile-security-row">
                <span class="profile-feature-icon">
                  <Icon name="lock" size="md" />
                </span>
                <div class="profile-row-copy">
                  <h3>{{ t('profile.changePassword') }}</h3>
                  <p>{{ t('profile.passwordHint') }}</p>
                </div>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  @click="showPasswordDialog = true"
                >
                  {{ t('profile.changePassword') }}
                </button>
              </section>
              <ProfileTotpCard />
              <ProfilePasskeyCard :enabled="passkeyEnabled" />
            </div>
          </section>
          <section
            v-if="user && balanceLowNotifyEnabled"
            id="profile-panel-notifications"
            v-show="selectedTab === 'notifications'"
            role="tabpanel"
            aria-labelledby="profile-tab-notifications"
            class="profile-panel"
          >
            <ProfileBalanceNotifyCard
              :enabled="user.balance_notify_enabled ?? true"
              :threshold="user.balance_notify_threshold"
              :extra-emails="user.balance_notify_extra_emails ?? []"
              :system-default-threshold="systemDefaultThreshold"
              :user-email="displayEmail"
            />
          </section>
        </div>
      </section>
      <footer class="profile-footer">
        <span>
          <Icon name="lock" size="sm" />
          {{ t('profile.title') }}
        </span>
        <button
          v-if="contactInfo"
          type="button"
          class="profile-link"
          @click="showContactDialog = true"
        >
          {{ t('common.contactSupport') }}
        </button>
      </footer>
      <ProfilePasswordForm
        dialog
        :show="showPasswordDialog"
        @close="showPasswordDialog = false"
        @success="showPasswordDialog = false"
      />
      <BaseDialog
        :show="showContactDialog"
        :title="t('common.contactSupport')"
        width="narrow"
        variant="profile"
        @close="showContactDialog = false"
      >
        <p class="break-words text-sm">{{ contactInfo }}</p>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ProfileInfoCard from '@/components/user/profile/ProfileInfoCard.vue'
import ProfileAvatarCard from '@/components/user/profile/ProfileAvatarCard.vue'
import ProfileEditForm from '@/components/user/profile/ProfileEditForm.vue'
import ProfileIdentityBindingsSection from '@/components/user/profile/ProfileIdentityBindingsSection.vue'
import ProfilePasswordForm from '@/components/user/profile/ProfilePasswordForm.vue'
import ProfileBalanceNotifyCard from '@/components/user/profile/ProfileBalanceNotifyCard.vue'
import ProfileTotpCard from '@/components/user/profile/ProfileTotpCard.vue'
import ProfilePasskeyCard from '@/components/user/profile/ProfilePasskeyCard.vue'
import { isWeChatWebOAuthEnabled } from '@/api/auth'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { isProfileIdentityBound, profileEmail } from '@/utils/profileIdentity'
import '@/styles/profile.css'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const user = computed(() => authStore.user)
const displayEmail = computed(() => profileEmail(user.value))
const emailBound = computed(() => isProfileIdentityBound(user.value, 'email'))
const overview = ref<InstanceType<typeof ProfileInfoCard> | null>(null)
const bindings = ref<InstanceType<
  typeof ProfileIdentityBindingsSection
> | null>(null)
const showPasswordDialog = ref(false)
const showContactDialog = ref(false)
type ProfileTab = 'basics' | 'bindings' | 'security' | 'notifications'
const selectedTab = ref<ProfileTab>('basics')
const tabs = computed(() => [
  {
    key: 'basics' as const,
    label: t('profile.tabs.basics'),
    icon: 'user' as const
  },
  {
    key: 'bindings' as const,
    label: t('profile.tabs.bindings'),
    icon: 'link' as const
  },
  {
    key: 'security' as const,
    label: t('profile.securityTitle'),
    icon: 'shield' as const
  },
  ...(balanceLowNotifyEnabled.value
    ? [
        {
          key: 'notifications' as const,
          label: t('profile.tabs.notifications'),
          icon: 'bell' as const
        }
      ]
    : [])
])
function onTabKeydown(event: KeyboardEvent, key: ProfileTab) {
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const index = tabs.value.findIndex((tab) => tab.key === key)
  const next =
    event.key === 'Home'
      ? 0
      : event.key === 'End'
        ? tabs.value.length - 1
        : (index + (event.key === 'ArrowRight' ? 1 : -1) + tabs.value.length) %
          tabs.value.length
  selectedTab.value = tabs.value[next]!.key
  document.getElementById(`profile-tab-${selectedTab.value}`)?.focus()
}

const contactInfo = ref('')
const balanceLowNotifyEnabled = ref(false)
const systemDefaultThreshold = ref(0)
const linuxdoOAuthEnabled = ref(false)
const dingtalkOAuthEnabled = ref(false)
const wechatOAuthEnabled = ref(false)
const wechatOAuthOpenEnabled = ref<boolean | undefined>(undefined)
const wechatOAuthMPEnabled = ref<boolean | undefined>(undefined)
const oidcOAuthEnabled = ref(false)
const oidcOAuthProviderName = ref('OIDC')
const passkeyEnabled = ref(false)
watch(balanceLowNotifyEnabled, (enabled) => {
  if (!enabled && selectedTab.value === 'notifications')
    selectedTab.value = 'basics'
})

onMounted(async () => {
  const profileRefresh = authStore.refreshUser().catch((error) => {
    console.error('Failed to refresh profile:', error)
  })
  const settingsLoad = appStore
    .fetchPublicSettings()
    .then((settings) => {
      if (!settings) return
      contactInfo.value = settings.contact_info || ''
      balanceLowNotifyEnabled.value =
        settings.balance_low_notify_enabled ?? false
      systemDefaultThreshold.value = settings.balance_low_notify_threshold ?? 0
      linuxdoOAuthEnabled.value = settings.linuxdo_oauth_enabled ?? false
      dingtalkOAuthEnabled.value = settings.dingtalk_oauth_enabled ?? false
      wechatOAuthEnabled.value = isWeChatWebOAuthEnabled(settings)
      wechatOAuthOpenEnabled.value =
        typeof settings.wechat_oauth_open_enabled === 'boolean'
          ? settings.wechat_oauth_open_enabled
          : undefined
      wechatOAuthMPEnabled.value =
        typeof settings.wechat_oauth_mp_enabled === 'boolean'
          ? settings.wechat_oauth_mp_enabled
          : undefined
      oidcOAuthEnabled.value = settings.oidc_oauth_enabled ?? false
      oidcOAuthProviderName.value = settings.oidc_oauth_provider_name || 'OIDC'
      passkeyEnabled.value = settings.passkey_enabled === true
    })
    .catch((error) => {
      console.error('Failed to load settings:', error)
    })
  await Promise.all([profileRefresh, settingsLoad])
})
</script>
