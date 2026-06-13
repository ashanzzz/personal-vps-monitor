/* API base – empty means same origin (proxied through Vite dev server) */
export const API_BASE = import.meta.env.VITE_API_BASE ?? ''

/* WebSocket URL – auto-detect ws:// or wss:// based on page protocol */
export function wsUrl(): string {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const base = import.meta.env.VITE_API_BASE
    ? import.meta.env.VITE_API_BASE.replace(/^https?/, proto)
    : `${proto}://${location.host}`
  return `${base}/ws`
}
