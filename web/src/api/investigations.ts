import api from './client'
import { download, exportForm, multipart, type Download } from './sketches'
import type {
  ImportResult,
  Invitation,
  Investigation,
  InvestigationImportResult,
  Member,
  Role,
  Sketch,
} from './types'

export async function listInvestigations(): Promise<Investigation[]> {
  return (await api.get<{ investigations: Investigation[] }>('/investigations')).data.investigations
}

export async function getInvestigation(id: string) {
  return (
    await api.get<{ investigation: Investigation; sketches: Sketch[] }>(`/investigations/${id}`)
  ).data
}

export async function createInvestigation(name: string, description = ''): Promise<Investigation> {
  return (
    await api.post<{ investigation: Investigation }>('/investigations', { name, description })
  ).data.investigation
}

export async function updateInvestigation(
  id: string,
  p: Partial<Pick<Investigation, 'name' | 'description' | 'status'>>,
): Promise<Investigation> {
  return (await api.put<{ investigation: Investigation }>(`/investigations/${id}`, p)).data
    .investigation
}

export async function deleteInvestigation(id: string): Promise<void> {
  await api.delete(`/investigations/${id}`)
}

export async function listMembers(id: string): Promise<Member[]> {
  return (await api.get<{ members: Member[] }>(`/investigations/${id}/members`)).data.members
}

export async function addMember(id: string, email: string, role: Role): Promise<Member> {
  return (await api.post<{ member: Member }>(`/investigations/${id}/members`, { email, role })).data
    .member
}

export async function setMemberRole(id: string, userId: string, role: Role): Promise<Member> {
  return (await api.put<{ member: Member }>(`/investigations/${id}/members/${userId}`, { role }))
    .data.member
}

export async function removeMember(id: string, userId: string): Promise<void> {
  await api.delete(`/investigations/${id}/members/${userId}`)
}

export async function listInvitations(id: string): Promise<Invitation[]> {
  return (await api.get<{ invitations: Invitation[] }>(`/investigations/${id}/invitations`)).data
    .invitations
}

// createInvitation answers the link once, the token is not kept in the
// clear anywhere else.
export async function createInvitation(
  id: string,
  email: string,
  role: Role,
): Promise<{ invitation: Invitation; link: string; emailed: boolean }> {
  return (
    await api.post<{ invitation: Invitation; link: string; emailed: boolean }>(
      `/investigations/${id}/invitations`,
      { email, role },
    )
  ).data
}

export async function revokeInvitation(id: string, invitationId: string): Promise<void> {
  await api.delete(`/investigations/${id}/invitations/${invitationId}`)
}

// transferOwner hands the investigation to a member, owner only.
export async function transferOwner(id: string, userId: string): Promise<void> {
  await api.put(`/investigations/${id}/owner`, { user_id: userId })
}

// exportInvestigation downloads the investigation with every sketch and
// analysis as one file, sealed under the passphrase when one is given.
export async function exportInvestigation(id: string, passphrase = ''): Promise<Download> {
  return download(`/investigations/${id}/export`, { passphrase })
}

// importSketchInto makes a new sketch of an export file under the
// investigation.
export async function importSketchInto(id: string, file: File, passphrase = '') {
  return (
    await api.post<{ sketch: Sketch; result: ImportResult }>(
      `/investigations/${id}/sketches/import`,
      exportForm(file, passphrase),
      multipart,
    )
  ).data
}

// importInvestigation makes a new investigation of an export file.
export async function importInvestigation(file: File, passphrase = '') {
  return (
    await api.post<InvestigationImportResult>(
      '/investigations/import',
      exportForm(file, passphrase),
      multipart,
    )
  ).data
}

export async function createSketch(investigationId: string, title: string, description = '') {
  return (
    await api.post<{ sketch: Sketch }>(`/investigations/${investigationId}/sketches`, {
      title,
      description,
    })
  ).data.sketch
}
