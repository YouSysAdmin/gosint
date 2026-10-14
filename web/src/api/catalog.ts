import api, { readBlobError } from './client'
import { seg } from './path'
import { multipart, type Download } from './sketches'
import { fileNameOf } from '@/composables/download'
import type {
  ConsoleSettings,
  Detection,
  Transform,
  TransformTemplate,
  EntityType,
  Flow,
  FlowDefinition,
  LlmModel,
  TypeCategory,
  VaultKey,
} from './types'

let settings: Promise<ConsoleSettings> | null = null

// consoleSettings is fetched once per page load.
export function consoleSettings(): Promise<ConsoleSettings> {
  settings ??= api.get<ConsoleSettings>('/console').then((r) => r.data)
  settings.catch(() => (settings = null))
  return settings
}

// The language models a person may pick for the assistant.
export async function listModels(): Promise<LlmModel[]> {
  return (await api.get<{ models: LlmModel[] }>('/models')).data.models
}

export async function listTypes() {
  return (await api.get<{ categories: TypeCategory[]; types: EntityType[] }>('/types')).data
}

export async function detect(values: string[]): Promise<Detection[]> {
  return (await api.post<{ results: Detection[] }>('/types/detect', { values })).data.results
}

export async function listTransforms(type?: string): Promise<Transform[]> {
  return (await api.get<{ transforms: Transform[] }>('/transforms', { params: { type } })).data
    .transforms
}

export async function listKeys(): Promise<VaultKey[]> {
  return (await api.get<{ keys: VaultKey[] }>('/secrets')).data.keys
}

export async function putKey(name: string, value: string): Promise<VaultKey> {
  return (await api.post<{ key: VaultKey }>('/secrets', { name, value })).data.key
}

export async function deleteKey(id: string): Promise<void> {
  await api.delete(`/secrets/${seg(id)}`)
}

export async function listTemplates(): Promise<TransformTemplate[]> {
  return (await api.get<{ templates: TransformTemplate[] }>('/transform-templates')).data.templates
}

export async function getTemplate(id: string): Promise<TransformTemplate> {
  return (await api.get<{ template: TransformTemplate }>(`/transform-templates/${seg(id)}`)).data
    .template
}

export async function saveTemplate(id: string | null, source: string, isPublic: boolean) {
  const body = { source, is_public: isPublic }
  const res = id
    ? await api.put<{ template: TransformTemplate }>(`/transform-templates/${seg(id)}`, body)
    : await api.post<{ template: TransformTemplate }>('/transform-templates', body)
  return res.data.template
}

export async function deleteTemplate(id: string): Promise<void> {
  await api.delete(`/transform-templates/${seg(id)}`)
}

export interface TemplateTest {
  answer: { status_code: number; body: string } | null
  outputs: { label: string; properties: Record<string, unknown>; error: string }[]
  error: string
}

export async function testTemplate(id: string, value: string): Promise<TemplateTest> {
  return (await api.post<TemplateTest>(`/transform-templates/${seg(id)}/test`, { value })).data
}

export async function listFlows(type?: string): Promise<Flow[]> {
  return (await api.get<{ flows: Flow[] }>('/flows', { params: { type } })).data.flows
}

export async function getFlow(id: string): Promise<Flow> {
  return (await api.get<{ flow: Flow }>(`/flows/${seg(id)}`)).data.flow
}

export async function saveFlow(
  id: string | null,
  name: string,
  description: string,
  definition: FlowDefinition,
) {
  const body = { name, description, definition }
  const res = id
    ? await api.put<{ flow: Flow }>(`/flows/${seg(id)}`, body)
    : await api.post<{ flow: Flow }>('/flows', body)
  return res.data.flow
}

export async function deleteFlow(id: string): Promise<void> {
  await api.delete(`/flows/${seg(id)}`)
}

export async function checkFlow(definition: FlowDefinition) {
  return (
    await api.post<{
      steps: { id: string; after: string; transform: string; input: string; output: string }[]
    }>('/flows/check', { definition })
  ).data.steps
}

// Flows and templates move as files: a flow as JSON, several in one,
// a template as its YAML, several with --- between them.
async function fetchFile(url: string, fallback: string): Promise<Download> {
  try {
    const r = await api.get<Blob>(url, { responseType: 'blob' })
    return { blob: r.data, name: fileNameOf(r.headers['content-disposition'], fallback) }
  } catch (err) {
    throw await readBlobError(err)
  }
}

export function exportFlow(id: string): Promise<Download> {
  return fetchFile(`/flows/${seg(id)}/export`, 'flow.json')
}

export function exportFlows(): Promise<Download> {
  return fetchFile('/flows/export', 'flows.json')
}

export interface FileImport {
  imported: number
  skipped: string[]
}

export async function importFlows(file: File): Promise<FileImport> {
  const form = new FormData()
  form.append('file', file)
  return (await api.post<FileImport>('/flows/import', form, multipart)).data
}

export function exportTemplate(id: string): Promise<Download> {
  return fetchFile(`/transform-templates/${seg(id)}/export`, 'template.yaml')
}

export function exportTemplates(): Promise<Download> {
  return fetchFile('/transform-templates/export', 'transforms.yaml')
}

export async function importTemplates(file: File): Promise<FileImport> {
  const form = new FormData()
  form.append('file', file)
  return (await api.post<FileImport>('/transform-templates/import', form, multipart)).data
}

// An entity found across the sketches the account may read.
export interface SearchHit {
  id: string
  type: string
  label: string
  sketch_id: string
  sketch_title: string
  investigation_id: string
  investigation_name: string
}

// searchEntities looks across every readable sketch: by a piece of the
// label, or for the entity itself by type and key, one sketch left out.
export async function searchEntities(params: {
  q?: string
  type?: string
  key?: string
  exclude?: string
}): Promise<SearchHit[]> {
  return (await api.get<{ hits: SearchHit[] }>('/search', { params })).data.hits
}

// investigationEntities finds the entities of one investigation's
// sketches by a piece of the label, the first ones by label when q is
// empty. The analysis editor's mentions ask it as the person types.
export async function investigationEntities(id: string, q: string): Promise<SearchHit[]> {
  return (
    await api.get<{ hits: SearchHit[] }>(`/investigations/${seg(id)}/entities`, { params: { q } })
  ).data.hits
}
