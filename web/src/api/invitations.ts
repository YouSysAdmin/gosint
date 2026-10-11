import api from './client'
import type { InvitationLookup, Member } from './types'

// lookupInvitation says what a link is for. Public, the token is the
// claim: 404 for an unknown or taken one, 410 for an expired one.
export async function lookupInvitation(token: string): Promise<InvitationLookup> {
  return (await api.get<InvitationLookup>(`/invitations/${token}`)).data
}

// acceptInvitation makes the signed-in account a member.
export async function acceptInvitation(
  token: string,
): Promise<{ investigation_id: string; member: Member }> {
  return (
    await api.post<{ investigation_id: string; member: Member }>(`/invitations/${token}/accept`)
  ).data
}
