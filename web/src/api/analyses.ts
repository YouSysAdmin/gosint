import api from './client'
import { seg } from './path'
import type { Analysis } from './types'

const base = (investigationId: string) => `/investigations/${seg(investigationId)}/analyses`

export async function listAnalyses(investigationId: string): Promise<Analysis[]> {
  return (await api.get<{ analyses: Analysis[] }>(base(investigationId))).data.analyses
}

export async function getAnalysis(investigationId: string, id: string): Promise<Analysis> {
  return (await api.get<{ analysis: Analysis }>(`${base(investigationId)}/${seg(id)}`)).data
    .analysis
}

export async function createAnalysis(
  investigationId: string,
  title: string,
  content: unknown = {},
): Promise<Analysis> {
  return (await api.post<{ analysis: Analysis }>(base(investigationId), { title, content })).data
    .analysis
}

export async function saveAnalysis(
  investigationId: string,
  id: string,
  p: { title?: string; content?: unknown },
): Promise<Analysis> {
  return (await api.put<{ analysis: Analysis }>(`${base(investigationId)}/${seg(id)}`, p)).data
    .analysis
}

export async function deleteAnalysis(investigationId: string, id: string): Promise<void> {
  await api.delete(`${base(investigationId)}/${seg(id)}`)
}
