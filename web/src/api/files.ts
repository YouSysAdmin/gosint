import api from './client'
import type { AttachedFile, MailMessage } from './types'

const base = (sketchId: string) => `/sketches/${sketchId}`

export async function listFiles(sketchId: string, nodeId: string): Promise<AttachedFile[]> {
  return (await api.get<{ files: AttachedFile[] }>(`${base(sketchId)}/nodes/${nodeId}/files`)).data
    .files
}

// uploadFile sends one file as multipart, the browser sets the boundary.
export async function uploadFile(
  sketchId: string,
  nodeId: string,
  file: File,
): Promise<AttachedFile> {
  const form = new FormData()
  form.append('file', file, file.name)
  return (
    await api.post<{ file: AttachedFile }>(`${base(sketchId)}/nodes/${nodeId}/files`, form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  ).data.file
}

export async function deleteFile(sketchId: string, fileId: string): Promise<void> {
  await api.delete(`${base(sketchId)}/files/${fileId}`)
}

// getMail parses an attached .eml on the server.
export async function getMail(sketchId: string, fileId: string): Promise<MailMessage> {
  return (await api.get<{ mail: MailMessage }>(`${base(sketchId)}/files/${fileId}/mail`)).data.mail
}
