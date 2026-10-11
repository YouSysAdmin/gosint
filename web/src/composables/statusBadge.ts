// One place decides what a status looks like, per kind of thing.

export type StatusScope = 'scan' | 'log' | 'role'

const scopes: Record<StatusScope, Record<string, string>> = {
  scan: {
    pending: 'badge-neutral',
    running: 'badge-info',
    completed: 'badge-success',
    failed: 'badge-danger',
    cancelled: 'badge-warning',
  },
  log: {
    debug: 'badge-neutral',
    info: 'badge-info',
    running: 'badge-info',
    pending: 'badge-neutral',
    success: 'badge-success',
    completed: 'badge-success',
    graph_append: 'badge-secondary',
    warning: 'badge-warning',
    failed: 'badge-danger',
  },
  role: {
    owner: 'badge-success',
    admin: 'badge-info',
    editor: 'badge-secondary',
    viewer: 'badge-neutral',
  },
}

export function statusBadgeClass(status: string, scope: StatusScope): string {
  return scopes[scope][status] ?? 'badge-neutral'
}
