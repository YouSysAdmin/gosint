import api from './client'
import { seg } from './path'
import type { AuthInfo, LinkedIdentity, Passkey, Profile, UserSession } from './types'

export async function me(): Promise<Profile> {
  return (await api.get<{ user: Profile }>('/auth/me')).data.user
}

// LoginResult is the account, or the second factor still owed. invite
// is the invitation an identity provider sign-in started from.
export interface LoginResult {
  user?: Profile
  second_factor?: 'totp'
  invite?: string
}

export async function login(email: string, password: string): Promise<LoginResult> {
  return (await api.post<LoginResult>('/auth/login', { email, password })).data
}

// loginSecondFactor finishes a sign-in with an authenticator or a
// recovery code.
export async function loginSecondFactor(code: string): Promise<LoginResult> {
  return (await api.post<LoginResult>('/auth/login/2fa', { code })).data
}

export async function register(email: string, password: string, invite = ''): Promise<Profile> {
  const body: Record<string, string> = { email, password }
  if (invite) body.invite = invite
  return (await api.post<{ user: Profile }>('/auth/register', body)).data.user
}

export async function logout(): Promise<void> {
  await api.post('/auth/logout')
}

// info is what the sign-in page may offer, public.
export async function info(): Promise<AuthInfo> {
  return (await api.get<AuthInfo>('/auth/info')).data
}

export async function updateMe(
  p: Partial<Pick<Profile, 'first_name' | 'last_name' | 'avatar_url'>>,
) {
  return (await api.put<{ user: Profile }>('/auth/me', p)).data.user
}

export async function searchUsers(q: string): Promise<Profile[]> {
  return (await api.get<{ users: Profile[] }>('/users/search', { params: { q } })).data.users
}

// Password.
export async function resetRequest(email: string): Promise<string> {
  return (await api.post<{ message: string }>('/auth/password-reset/request', { email })).data
    .message
}

export async function resetConfirm(token: string, password: string): Promise<string> {
  return (await api.post<{ message: string }>('/auth/password-reset/confirm', { token, password }))
    .data.message
}

export async function changePassword(current: string, password: string): Promise<string> {
  return (
    await api.post<{ message: string }>('/auth/password', {
      current_password: current,
      password,
    })
  ).data.message
}

// Two-factor authentication.
export async function totpSetup(
  password: string,
): Promise<{ secret: string; otpauth_url: string }> {
  return (await api.post<{ secret: string; otpauth_url: string }>('/auth/2fa/setup', { password }))
    .data
}

export async function totpEnable(
  code: string,
): Promise<{ totp_enabled: boolean; recovery_codes?: string[] }> {
  return (
    await api.post<{ totp_enabled: boolean; recovery_codes?: string[] }>('/auth/2fa/enable', {
      code,
    })
  ).data
}

export async function totpDisable(code: string): Promise<void> {
  await api.post('/auth/2fa/disable', { code })
}

export async function recoveryCodesStatus(): Promise<number> {
  return (await api.get<{ remaining: number }>('/auth/2fa/recovery-codes')).data.remaining
}

export async function recoveryCodesRegenerate(password: string): Promise<string[]> {
  return (await api.post<{ codes: string[] }>('/auth/2fa/recovery-codes', { password })).data.codes
}

// Passkeys.
export async function passkeys(): Promise<Passkey[]> {
  return (await api.get<{ passkeys: Passkey[] }>('/auth/passkeys')).data.passkeys
}

export async function passkeyRegisterBegin(password: string) {
  return (
    await api.post<{ publicKey: Record<string, unknown> }>('/auth/passkeys/register/begin', {
      password,
    })
  ).data
}

export async function passkeyRegisterFinish(name: string, attestation: unknown): Promise<Passkey> {
  return (
    await api.post<{ passkey: Passkey }>('/auth/passkeys/register/finish', attestation, {
      params: { name },
    })
  ).data.passkey
}

export async function passkeyRename(id: string, name: string): Promise<void> {
  await api.put(`/auth/passkeys/${seg(id)}`, { name })
}

export async function passkeyDelete(id: string, password: string): Promise<void> {
  await api.post(`/auth/passkeys/${seg(id)}/delete`, { password })
}

export async function passkeyLoginBegin() {
  return (await api.post<{ publicKey: Record<string, unknown> }>('/auth/passkeys/login/begin')).data
}

export async function passkeyLoginFinish(assertion: unknown): Promise<Profile> {
  return (await api.post<{ user: Profile }>('/auth/passkeys/login/finish', assertion)).data.user
}

// Sessions.
export async function sessions(): Promise<UserSession[]> {
  return (await api.get<{ sessions: UserSession[] }>('/auth/sessions')).data.sessions
}

export async function revokeSession(id: string): Promise<void> {
  await api.delete(`/auth/sessions/${seg(id)}`)
}

export async function revokeOtherSessions(): Promise<number> {
  return (await api.post<{ revoked: number }>('/auth/sessions/revoke-others')).data.revoked
}

// identities lists the identity providers linked to this account.
export async function identities(): Promise<LinkedIdentity[]> {
  return (await api.get<{ identities: LinkedIdentity[] }>('/auth/identities')).data.identities
}

// linkProvider starts linking a provider and answers where to send the
// browser, the provider's sign-in page.
export async function linkProvider(slug: string): Promise<string> {
  return (await api.post<{ redirect_url: string }>(`/auth/oauth/${seg(slug)}/link`)).data
    .redirect_url
}

export async function unlinkIdentity(id: string): Promise<void> {
  await api.delete(`/auth/identities/${seg(id)}`)
}
