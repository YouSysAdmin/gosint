import api from './client'
import type {
  AdminUser,
  CustomType,
  FileStore,
  FileStoreInput,
  Investigation,
  Job,
  LlmModelAdmin,
  LlmModelInput,
  MailSettings,
  MailSettingsInput,
  ToolSetting,
  WorkerSettings,
} from './types'

// The admin panel: accounts of the install.

export async function listUsers(
  q = '',
  limit = 200,
): Promise<{ users: AdminUser[]; total: number }> {
  return (
    await api.get<{ users: AdminUser[]; total: number }>('/admin/users', { params: { q, limit } })
  ).data
}

export async function getUser(id: string): Promise<{ user: AdminUser; owns: Investigation[] }> {
  return (await api.get<{ user: AdminUser; owns: Investigation[] }>(`/admin/users/${id}`)).data
}

export async function createUser(
  email: string,
  password: string,
  admin: boolean,
): Promise<AdminUser> {
  return (await api.post<{ user: AdminUser }>('/admin/users', { email, password, admin })).data.user
}

export async function patchUser(
  id: string,
  p: { admin?: boolean; disabled?: boolean; password?: string },
): Promise<AdminUser> {
  return (await api.patch<{ user: AdminUser }>(`/admin/users/${id}`, p)).data.user
}

export async function deleteUser(id: string): Promise<void> {
  await api.delete(`/admin/users/${id}`)
}

export async function resetLink(id: string): Promise<{ link: string; emailed: boolean }> {
  return (await api.post<{ link: string; emailed: boolean }>(`/admin/users/${id}/reset-link`)).data
}

export async function resetTOTP(id: string): Promise<void> {
  await api.delete(`/admin/users/${id}/2fa`)
}

export async function resetPasskeys(id: string): Promise<number> {
  return (await api.delete<{ removed: number }>(`/admin/users/${id}/passkeys`)).data.removed
}

export async function revokeSessions(id: string): Promise<number> {
  return (await api.post<{ revoked: number }>(`/admin/users/${id}/revoke-sessions`)).data.revoked
}

// Release is whether GitHub has a newer stable release than this build.
export interface Release {
  current: string
  latest?: string
  url?: string
  available: boolean
}

export async function latestRelease(): Promise<Release> {
  return (await api.get<Release>('/admin/release')).data
}

// The maintenance jobs of the node answering, and one run on request.
export async function listJobs(): Promise<{ jobs: Job[] }> {
  return (await api.get<{ jobs: Job[] }>('/admin/jobs')).data
}

export async function runJob(name: string): Promise<{ jobs: Job[] }> {
  return (await api.post<{ jobs: Job[] }>(`/admin/jobs/${encodeURIComponent(name)}/run`)).data
}

// The entity types declared as data.
export async function listCustomTypes(): Promise<CustomType[]> {
  return (await api.get<{ types: CustomType[] }>('/admin/types')).data.types
}

export type CustomTypeInput = Omit<CustomType, 'id' | 'created_at'>

export async function saveCustomType(id: string | null, t: CustomTypeInput): Promise<CustomType> {
  const r = id
    ? await api.put<{ type: CustomType }>(`/admin/types/${id}`, t)
    : await api.post<{ type: CustomType }>('/admin/types', t)
  return r.data.type
}

export async function deleteCustomType(id: string): Promise<void> {
  await api.delete(`/admin/types/${id}`)
}

// The language models the assistant may use.
export async function listModels(): Promise<LlmModelAdmin[]> {
  return (await api.get<{ models: LlmModelAdmin[] }>('/admin/models')).data.models
}

export async function saveModel(id: string | null, m: LlmModelInput): Promise<LlmModelAdmin> {
  const r = id
    ? await api.patch<{ model: LlmModelAdmin }>(`/admin/models/${id}`, m)
    : await api.post<{ model: LlmModelAdmin }>('/admin/models', m)
  return r.data.model
}

export async function deleteModel(id: string): Promise<void> {
  await api.delete(`/admin/models/${id}`)
}

// The relay the server sends its own mail through.
export async function getMail(): Promise<MailSettings> {
  return (await api.get<{ mail: MailSettings }>('/admin/settings/mail')).data.mail
}

export async function saveMail(m: MailSettingsInput): Promise<MailSettings> {
  return (await api.put<{ mail: MailSettings }>('/admin/settings/mail', m)).data.mail
}

export async function testMail(): Promise<string> {
  return (await api.post<{ sent_to: string }>('/admin/settings/mail/test')).data.sent_to
}

// The worker's settings, and where downloads go when no directory is
// set.
export async function getWorker(): Promise<{ worker: WorkerSettings; default_cache_dir: string }> {
  return (
    await api.get<{ worker: WorkerSettings; default_cache_dir: string }>('/admin/settings/worker')
  ).data
}

export async function saveWorker(w: WorkerSettings): Promise<WorkerSettings> {
  return (await api.put<{ worker: WorkerSettings }>('/admin/settings/worker', w)).data.worker
}

// The tools the transforms use, with where this node finds them.
export async function getTools(): Promise<ToolSetting[]> {
  return (await api.get<{ tools: ToolSetting[] }>('/admin/settings/tools')).data.tools
}

export async function saveTools(tools: Record<string, ToolInput>): Promise<ToolSetting[]> {
  return (await api.put<{ tools: ToolSetting[] }>('/admin/settings/tools', { tools })).data.tools
}

// Where the bytes of attached files live, and the limits.
export async function getFileStore(): Promise<FileStore> {
  return (await api.get<{ files: FileStore }>('/admin/settings/files')).data.files
}

export async function saveFileStore(f: FileStoreInput): Promise<FileStore> {
  return (await api.put<{ files: FileStore }>('/admin/settings/files', f)).data.files
}
