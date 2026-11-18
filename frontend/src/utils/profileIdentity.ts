import type { User, UserAuthBindingStatus, UserAuthProvider } from '@/types'

export function isProfileIdentityBound(
  user: User | null | undefined,
  provider: UserAuthProvider
): boolean {
  const direct = user?.[`${provider}_bound` as keyof User]
  if (typeof direct === 'boolean') return direct

  const binding: boolean | UserAuthBindingStatus | undefined =
    user?.auth_bindings?.[provider] ?? user?.identity_bindings?.[provider]
  if (typeof binding === 'boolean') return binding
  if (!binding) return false
  if (typeof binding.bound === 'boolean') return binding.bound
  return Boolean(
    binding.provider_subject || binding.issuer || binding.provider_key
  )
}

export function profileEmail(user: User | null | undefined): string {
  const email = user?.email?.trim() || ''
  return email.endsWith('.invalid') && !isProfileIdentityBound(user, 'email')
    ? ''
    : email
}
