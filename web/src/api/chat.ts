import api from './client'
import type { ApplyResult, Chat, ChatMessage, ProposedChange } from './types'

const base = (sketchId: string) => `/sketches/${sketchId}/chats`

export async function listChats(sketchId: string): Promise<Chat[]> {
  return (await api.get<{ chats: Chat[] }>(base(sketchId))).data.chats
}

export async function createChat(sketchId: string, title = ''): Promise<Chat> {
  return (await api.post<{ chat: Chat }>(base(sketchId), { title })).data.chat
}

export async function deleteChat(sketchId: string, chatId: string): Promise<void> {
  await api.delete(`${base(sketchId)}/${chatId}`)
}

export async function listMessages(sketchId: string, chatId: string): Promise<ChatMessage[]> {
  return (await api.get<{ messages: ChatMessage[] }>(`${base(sketchId)}/${chatId}/messages`)).data
    .messages
}

export async function applyChanges(
  sketchId: string,
  chatId: string,
  changes: ProposedChange[],
): Promise<ApplyResult> {
  return (await api.post<ApplyResult>(`${base(sketchId)}/${chatId}/apply`, { changes })).data
}

// BLOCK_FENCE opens the fenced block an answer carries its proposal in.
export const BLOCK_FENCE = '```gosint-changes'

// splitProposal takes the proposal block out of an answer.
export function splitProposal(content: string): { text: string; changes: ProposedChange[] } {
  const start = content.indexOf(BLOCK_FENCE)
  if (start < 0) return { text: content, changes: [] }
  const rest = content.slice(start + BLOCK_FENCE.length)
  const end = rest.indexOf('```')
  const body = (end < 0 ? rest : rest.slice(0, end)).trim()
  try {
    const parsed = JSON.parse(body) as { changes?: ProposedChange[] }
    const after = end < 0 ? '' : rest.slice(end + 3)
    const joined = content.slice(0, start).replace(/\n+$/, '') + '\n\n' + after.replace(/^\n+/, '')
    return { text: joined.trim(), changes: parsed.changes ?? [] }
  } catch {
    return { text: content, changes: [] }
  }
}

export interface StreamHandlers {
  token: (text: string) => void
  done: (message: ChatMessage) => void

  // thinking is the model's reasoning as it happens, before the answer.
  thinking?: (text: string) => void

  // drafting is a fragment of a tool call being spelled: progress only.
  drafting?: (text: string) => void

  // action is a lookup or a run the model asked for, as it starts.
  action?: (text: string) => void

  // progress is how a run the model started is going, replacing the
  // last action line.
  progress?: (text: string) => void
}

// streamChat asks and reads the answer as it is written. The answer is
// a POST that streams, which EventSource cannot do, so the frames are
// read off fetch's body by hand. Resolves once the stream ends, rejects
// with the server's message when it refuses or fails.
export async function streamChat(
  sketchId: string,
  chatId: string,
  prompt: string,
  nodeIds: string[],
  on: StreamHandlers,
  signal?: AbortSignal,
  withChildren = false,
  modelId = '',
): Promise<void> {
  const res = await fetch(`/api${base(sketchId)}/${chatId}/messages`, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      prompt,
      node_ids: nodeIds,
      with_children: withChildren,
      model_id: modelId,
    }),
    signal,
  })
  if (!res.ok || !res.body) {
    let msg = res.statusText
    try {
      msg = ((await res.json()) as { error?: string }).error ?? msg
    } catch {
      // not JSON
    }
    throw new Error(msg)
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let failure = ''

  const frame = (chunk: string) => {
    let event = ''
    const data: string[] = []
    for (const line of chunk.split('\n')) {
      if (line.startsWith('event:')) event = line.slice(6).trim()
      else if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''))
    }
    if (!data.length) return
    // One frame that does not parse is skipped, the stream goes on.
    let payload
    try {
      payload = JSON.parse(data.join('\n'))
    } catch {
      return
    }
    if (event === 'token') on.token(payload.text)
    else if (event === 'thinking') on.thinking?.(payload.text)
    else if (event === 'drafting') on.drafting?.(payload.text)
    else if (event === 'action') on.action?.(payload.text)
    else if (event === 'progress') on.progress?.(payload.text)
    else if (event === 'done') on.done(payload.message)
    else if (event === 'error') failure = payload.error
  }

  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let at: number
    while ((at = buffer.indexOf('\n\n')) >= 0) {
      frame(buffer.slice(0, at))
      buffer = buffer.slice(at + 2)
    }
  }
  if (buffer.trim()) frame(buffer)
  if (failure) throw new Error(failure)
}

export async function generateTemplate(
  prompt: string,
  inputType: string,
  outputType: string,
  modelId = '',
): Promise<{ source: string; problem: string }> {
  return (
    await api.post<{ source: string; problem: string }>('/transform-templates/generate', {
      prompt,
      input_type: inputType,
      output_type: outputType,
      model_id: modelId,
    })
  ).data
}
