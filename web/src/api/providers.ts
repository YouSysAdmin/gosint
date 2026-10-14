import api from './client'
import { seg } from './path'
import type { OAuthProvider, OAuthProviderInput, OAuthTestResult } from './types'

// The identity providers an administrator configures.

export async function listProviders(): Promise<OAuthProvider[]> {
  return (await api.get<{ providers: OAuthProvider[] }>('/admin/oauth-providers')).data.providers
}

export async function createProvider(p: OAuthProviderInput): Promise<OAuthProvider> {
  return (await api.post<{ provider: OAuthProvider }>('/admin/oauth-providers', p)).data.provider
}

export async function updateProvider(id: string, p: OAuthProviderInput): Promise<OAuthProvider> {
  return (await api.patch<{ provider: OAuthProvider }>(`/admin/oauth-providers/${seg(id)}`, p)).data
    .provider
}

export async function deleteProvider(id: string): Promise<void> {
  await api.delete(`/admin/oauth-providers/${seg(id)}`)
}

export async function testProvider(id: string): Promise<OAuthTestResult> {
  return (await api.post<{ test: OAuthTestResult }>(`/admin/oauth-providers/${seg(id)}/test`)).data
    .test
}
