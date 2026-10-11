import api from './client'

// The seal: the server holds no key until the unseal key unwraps it.

export interface SealStatus {
  initialized: boolean
  sealed: boolean
  min_unseal_key: number
}

export async function sealStatus(): Promise<SealStatus> {
  return (await api.get<SealStatus>('/sys/seal-status')).data
}

export async function initialize(unsealKey: string): Promise<SealStatus> {
  return (await api.post<SealStatus>('/sys/init', { unseal_key: unsealKey })).data
}

export async function unseal(unsealKey: string): Promise<SealStatus> {
  return (await api.post<SealStatus>('/sys/unseal', { unseal_key: unsealKey })).data
}

export async function seal(): Promise<SealStatus> {
  return (await api.post<SealStatus>('/admin/seal')).data
}
