// A CSV file written in the browser from what a table shows, so a
// view can be taken to a spreadsheet without a server round trip. A
// value that is not text is written as JSON, every cell quoted. A cell
// a spreadsheet would read as a formula (a leading =, +, -, @, tab or
// return) gets a quote in front: a label a crawler brought back must
// open as text, not run.
import { saveBlob } from './download'

const formula = /^[=+\-@\t\r]/

export function toCSV(columns: string[], rows: unknown[][]): string {
  const cell = (v: unknown): string => {
    let s = v === null || v === undefined ? '' : typeof v === 'string' ? v : JSON.stringify(v)
    if (formula.test(s)) s = "'" + s
    return '"' + s.replace(/"/g, '""') + '"'
  }
  return [columns, ...rows].map((r) => r.map(cell).join(',')).join('\r\n') + '\r\n'
}

export function downloadCSV(name: string, columns: string[], rows: unknown[][]) {
  saveBlob(new Blob(['\ufeff' + toCSV(columns, rows)], { type: 'text/csv;charset=utf-8' }), name)
}
