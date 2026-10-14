import api, { readBlobError } from './client'
import { seg } from './path'
import { fileNameOf } from '@/composables/download'
import type { Graph, GraphEdge, GraphNode, ImportResult, LogEntry, Scan, Sketch } from './types'

const base = (id: string) => `/sketches/${seg(id)}`

export async function listSketches(): Promise<Sketch[]> {
  return (await api.get<{ sketches: Sketch[] }>('/sketches')).data.sketches
}

export async function getSketch(id: string): Promise<Sketch> {
  return (await api.get<{ sketch: Sketch }>(base(id))).data.sketch
}

export async function updateSketch(
  id: string,
  p: Partial<Pick<Sketch, 'title' | 'description' | 'status'>>,
) {
  return (await api.put<{ sketch: Sketch }>(base(id), p)).data.sketch
}

export async function deleteSketch(id: string): Promise<void> {
  await api.delete(base(id))
}

export async function getGraph(id: string): Promise<Graph> {
  return (await api.get<Graph>(`${base(id)}/graph`)).data
}

export interface NodeInput {
  type: string
  label?: string
  properties?: Record<string, unknown>
  color?: string
  icon?: string
  shape?: string
  size?: number
  x?: number
  y?: number
}

export async function addNode(id: string, n: NodeInput): Promise<GraphNode> {
  return (await api.post<{ node: GraphNode }>(`${base(id)}/nodes`, n)).data.node
}

export async function updateNode(
  id: string,
  nodeId: string,
  p: {
    label?: string
    properties?: Record<string, unknown>
    color?: string
    icon?: string
    image?: string
    flag?: string
    shape?: string
    size?: number | null
    metadata?: Record<string, unknown>
  },
): Promise<GraphNode> {
  return (await api.patch<{ node: GraphNode }>(`${base(id)}/nodes/${seg(nodeId)}`, p)).data.node
}

export async function deleteNodes(
  id: string,
  ids: string[],
  withDependents = false,
): Promise<number> {
  return (
    await api.post<{ count: number }>(`${base(id)}/nodes/delete`, {
      ids,
      with_dependents: withDependents,
    })
  ).data.count
}

// dependents lists the nodes a delete with dependents would also take.
export async function dependents(id: string, ids: string[]): Promise<string[]> {
  return (await api.post<{ ids: string[] }>(`${base(id)}/nodes/dependents`, { ids })).data.ids
}

export async function clearSketch(id: string): Promise<number> {
  return (await api.post<{ count: number }>(`${base(id)}/clear`)).data.count
}

// What Undo and Redo would do next, '' for nothing.
export interface Changes {
  undo: string
  redo: string
}

export async function getChanges(id: string): Promise<Changes> {
  return (await api.get<Changes>(`${base(id)}/changes`)).data
}

export interface Reverted extends Changes {
  reverted: string
}

export async function undo(id: string): Promise<Reverted> {
  return (await api.post<Reverted>(`${base(id)}/undo`)).data
}

export async function redo(id: string): Promise<Reverted> {
  return (await api.post<Reverted>(`${base(id)}/redo`)).data
}

export async function mergeNodes(
  id: string,
  nodeIds: string[],
  keepId: string,
): Promise<GraphNode> {
  return (
    await api.post<{ node: GraphNode }>(`${base(id)}/nodes/merge`, {
      node_ids: nodeIds,
      keep_id: keepId,
    })
  ).data.node
}

export async function setPositions(
  id: string,
  positions: { node_id: string; x: number; y: number }[],
) {
  await api.put(`${base(id)}/nodes/positions`, { positions })
}

export async function addEdge(id: string, source: string, target: string, label: string) {
  return (await api.post<{ edge: GraphEdge }>(`${base(id)}/edges`, { source, target, label })).data
    .edge
}

export async function updateEdge(id: string, edgeId: string, label: string): Promise<GraphEdge> {
  return (await api.patch<{ edge: GraphEdge }>(`${base(id)}/edges/${seg(edgeId)}`, { label })).data
    .edge
}

export async function deleteEdges(id: string, ids: string[]): Promise<number> {
  return (await api.post<{ count: number }>(`${base(id)}/edges/delete`, { ids })).data.count
}

// exportSketch downloads the sketch as one file, sealed under the
// passphrase when one is given.
export async function exportSketch(id: string, passphrase = ''): Promise<Download> {
  return download(`${base(id)}/export`, { passphrase })
}

// importSketchFile merges an export file into the sketch, the
// passphrase opening a sealed one.
export async function importSketchFile(id: string, file: File, passphrase = '') {
  return (
    await api.post<ImportResult>(`${base(id)}/import/file`, exportForm(file, passphrase), multipart)
  ).data
}

// Download is a file the server answered with its name.
export interface Download {
  blob: Blob
  name: string
}

// download posts and keeps the answer as a file. An error answered to a
// blob request is a blob too, so it is read back into the usual shape.
export async function download(url: string, body: unknown): Promise<Download> {
  try {
    const r = await api.post<Blob>(url, body, { responseType: 'blob' })
    return { blob: r.data, name: fileNameOf(r.headers['content-disposition'], 'export.json') }
  } catch (err) {
    throw await readBlobError(err)
  }
}

// multipart overrides the client's JSON content type, the browser
// then sets the boundary itself.
export const multipart = { headers: { 'Content-Type': 'multipart/form-data' } }

// exportForm is the multipart an import takes: the file and the
// passphrase of a sealed one.
export function exportForm(file: File, passphrase: string): FormData {
  const form = new FormData()
  form.append('file', file)
  if (passphrase) form.append('passphrase', passphrase)
  return form
}

// importGraph sends an export back: nodes, edges and the files the
// export carried base64.
export async function importGraph(
  id: string,
  nodes: unknown[],
  edges: unknown[] = [],
  files: unknown[] = [],
) {
  return (await api.post<ImportResult>(`${base(id)}/import`, { nodes, edges, files })).data
}

// A CSV or XLSX file: previewed for its columns, then imported by the
// mapping the person drew.
export interface TablePreview {
  columns: string[]
  rows: string[][]
  total: number
  guesses: { column: string; type: string }[]
}

export interface TableMapping {
  entities: { type: string; fields: Record<string, string> }[]
  links: { from: number; to: number; label: string }[]
}

export async function previewTable(id: string, file: File): Promise<TablePreview> {
  const form = new FormData()
  form.append('file', file)
  return (await api.post<TablePreview>(`${base(id)}/import/table/preview`, form, multipart)).data
}

export async function importTable(id: string, file: File, mapping: TableMapping) {
  const form = new FormData()
  form.append('file', file)
  form.append('mapping', JSON.stringify(mapping))
  return (await api.post<ImportResult>(`${base(id)}/import/table`, form, multipart)).data
}

// An nmap XML report (nmap -oX) travels as it is.
export async function importNmap(id: string, xml: string) {
  return (
    await api.post<ImportResult>(`${base(id)}/import/nmap`, xml, {
      headers: { 'Content-Type': 'application/xml' },
    })
  ).data
}

// A nuclei report (-jsonl or -json-export) travels as it is.
export async function importNuclei(id: string, report: string) {
  return (
    await api.post<ImportResult>(`${base(id)}/import/nuclei`, report, {
      headers: { 'Content-Type': 'application/x-ndjson' },
      transformRequest: [(d) => d],
    })
  ).data
}

export async function listScans(id: string): Promise<Scan[]> {
  return (await api.get<{ scans: Scan[] }>(`${base(id)}/scans`)).data.scans
}

export async function cancelScan(id: string, scanId: string): Promise<Scan> {
  return (await api.post<{ scan: Scan }>(`${base(id)}/scans/${seg(scanId)}/cancel`)).data.scan
}

export async function listLogs(id: string, limit = 300): Promise<LogEntry[]> {
  return (await api.get<{ logs: LogEntry[] }>(`${base(id)}/logs`, { params: { limit } })).data.logs
}

export async function clearLogs(id: string): Promise<void> {
  await api.delete(`${base(id)}/logs`)
}

export async function launchTransform(
  id: string,
  name: string,
  nodeIds: string[],
  params?: Record<string, string>,
) {
  return (
    await api.post<{ scan: Scan }>(`${base(id)}/transforms/${seg(name)}/launch`, {
      node_ids: nodeIds,
      params,
    })
  ).data.scan
}

export async function launchTemplate(
  id: string,
  templateId: string,
  nodeIds: string[],
  params?: Record<string, string>,
) {
  return (
    await api.post<{ scan: Scan }>(`${base(id)}/templates/${seg(templateId)}/launch`, {
      node_ids: nodeIds,
      params,
    })
  ).data.scan
}

export async function launchFlow(id: string, flowId: string, nodeIds: string[]) {
  return (
    await api.post<{ scan: Scan }>(`${base(id)}/flows/${seg(flowId)}/launch`, { node_ids: nodeIds })
  ).data.scan
}

// openEvents opens the sketch's live stream. The session cookie goes
// along, so nothing travels in the URL.
// GraphChange says a member changed the graph by hand, and how.
export interface GraphChange {
  by: string
  name: string
  kind: string
}

export function openEvents(
  id: string,
  on: { log?: (e: LogEntry) => void; scan?: (s: Scan) => void; graph?: (g: GraphChange) => void },
): EventSource {
  const es = new EventSource(`/api/sketches/${seg(id)}/events`, { withCredentials: true })
  es.addEventListener('log', (ev) => on.log?.(JSON.parse((ev as MessageEvent).data)))
  es.addEventListener('scan', (ev) => on.scan?.(JSON.parse((ev as MessageEvent).data)))
  es.addEventListener('graph', (ev) => on.graph?.(JSON.parse((ev as MessageEvent).data)))
  return es
}

// A flow run again every so often, on a selection or on every entity
// of its input type, with the last run summed up.
export interface Schedule {
  id: string
  sketch_id: string
  owner_id: string
  flow_id: string
  flow_name: string
  node_ids: string[]
  every_seconds: number
  enabled: boolean
  last_run_at: string | null
  last_scan_id: string
  prev_scan_id: string
  last_error: string
  summary: { scan_id: string; status: string; new: number; seen: number; missing: number } | null
}

export async function listSchedules(id: string): Promise<Schedule[]> {
  return (await api.get<{ schedules: Schedule[] }>(`${base(id)}/schedules`)).data.schedules
}

export async function createSchedule(
  id: string,
  p: { flow_id: string; node_ids: string[]; every_seconds: number },
): Promise<Schedule> {
  return (await api.post<{ schedule: Schedule }>(`${base(id)}/schedules`, p)).data.schedule
}

export async function updateSchedule(
  id: string,
  scheduleId: string,
  p: { every_seconds: number; enabled?: boolean; node_ids?: string[] },
): Promise<Schedule> {
  return (await api.patch<{ schedule: Schedule }>(`${base(id)}/schedules/${seg(scheduleId)}`, p))
    .data.schedule
}

export async function deleteSchedule(id: string, scheduleId: string): Promise<void> {
  await api.delete(`${base(id)}/schedules/${seg(scheduleId)}`)
}

export async function runSchedule(id: string, scheduleId: string): Promise<Scan> {
  return (await api.post<{ scan: Scan }>(`${base(id)}/schedules/${seg(scheduleId)}/run`)).data.scan
}
