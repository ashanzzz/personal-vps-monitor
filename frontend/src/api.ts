import axios from 'axios'
import { API_BASE } from './config'

const TOKEN_KEY = 'akile_token'

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? ''
}
export function setToken(t: string): void {
  localStorage.setItem(TOKEN_KEY, t)
}
export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

// Called when a 401 is seen — wired up by App to bounce back to login.
let onUnauthorized: (() => void) | null = null
export function setUnauthorizedHandler(fn: () => void): void {
  onUnauthorized = fn
}

const http = axios.create({ baseURL: API_BASE })

// Attach bearer token to every request.
http.interceptors.request.use(cfg => {
  const t = getToken()
  if (t) cfg.headers.Authorization = ['Bearer', t].join(' ')
  return cfg
})

// On 401, clear the token and notify the app (skip the login call itself).
http.interceptors.response.use(
  r => r,
  err => {
    if (err?.response?.status === 401) {
      const url: string = err.config?.url ?? ''
      if (!url.includes('/api/auth/login') && !url.includes('/api/auth/setup')) {
        clearToken()
        onUnauthorized?.()
      }
    }
    return Promise.reject(err)
  }
)

export interface OfficialMatch {
  plan_id: string
  name: string
  area: string
  price_monthly: number
  available: boolean
  price_delta: number   // official - secondhand (fen); + means official pricier
  cheaper: 'official' | 'secondhand' | 'equal'
  buy_url: string
}

export interface Machine {
  id: number
  category: 'vps' | 'ls' | 'secondhand'
  plan_id: string
  area: string
  node: string
  name: string
  stock: number
  price_monthly: number
  price_yearly: number | null
  cpu: number
  memory_mb: number
  disk_gb: number
  bandwidth_mbps: number
  traffic_gb: number
  ipv4: number
  available: boolean
  ip_status: '' | 'normal' | 'gfw'
  official_match: OfficialMatch | null
  updated_at: string
}

export interface AlertEvent {
  id: number
  category: string
  plan_id: string
  name: string
  area: string
  event_type: string
  old_value: string | null
  new_value: string | null
  notified: boolean
  created_at: string
}

export interface PriceHistory {
  plan_id: string
  price: number
  cycle: number
  fetched_at: string
}

export interface AppSettings {
  // API & connection
  akile_token: string
  refresh_interval: number
  auto_refresh_token: boolean
  refresh_margin_hours: number
  request_timeout: number
  request_retries: number
  // Monitor scope
  vps_enabled: boolean
  ls_enabled: boolean
  secondhand_enabled: boolean
  ls_regions: string[]
  // Filters
  watched_plans: string[]
  filter_areas: string[]
  filter_name_include: string
  filter_name_exclude: string
  filter_max_price: number
  filter_min_cpu: number
  filter_min_memory_mb: number
  filter_min_traffic_gb: number
  // Telegram
  tg_bot_token: string
  tg_chat_id: string
  tg_notify_new_listing: boolean
  tg_notify_restock: boolean
  tg_notify_delisted: boolean
  tg_notify_price_change: boolean
  tg_notify_purchased: boolean
  notify_scope: 'all' | 'watched'
  dnd_enabled: boolean
  dnd_start: string
  dnd_end: string
  merge_interval_minutes: number
  // Webhook
  webhook_enabled: boolean
  webhook_url: string
  webhook_notify_new_listing: boolean
  webhook_notify_restock: boolean
  webhook_notify_delisted: boolean
  webhook_notify_price_change: boolean
  webhook_notify_purchased: boolean
  // Auto-buy
  auto_buy_enabled: boolean
  auto_buy_plans: string[]
  auto_buy_max_price: number
  auto_buy_cycle: number
  auto_buy_max_per_run: number
  auto_buy_daily_limit: number
  // System
  event_retention_days: number
  timezone: string
}

export interface TokenStatus {
  configured: boolean
  exp: number
  seconds_left: number
  expires_at: string | null
  auto_refresh: boolean
}

export const api = {
  getMachines: (params?: { category?: string; available_only?: boolean }) =>
    http.get<Machine[]>('/api/machines', { params }).then(r => r.data),

  getEvents: (params?: { limit?: number; category?: string; event_type?: string }) =>
    http.get<AlertEvent[]>('/api/events', { params }).then(r => r.data),

  getPriceHistory: (plan_id?: string) =>
    http.get<PriceHistory[]>('/api/price-history', { params: plan_id ? { plan_id } : {} }).then(r => r.data),

  getSettings: () =>
    http.get<AppSettings>('/api/settings').then(r => r.data),

  saveSettings: (data: Partial<AppSettings>) =>
    http.post<AppSettings>('/api/settings', data).then(r => r.data),

  triggerPoll: () =>
    http.post('/api/poll/trigger').then(r => r.data),

  buyMachine: (plan_id: string) =>
    http.post(`/api/buy/${plan_id}`).then(r => r.data),

  getTokenStatus: () =>
    http.get<TokenStatus>('/api/token/status').then(r => r.data),

  refreshToken: () =>
    http.post<{ status: string }>('/api/token/refresh').then(r => r.data),

  // --- Auth ---
  authStatus: () =>
    http.get<{ initialized: boolean }>('/api/auth/status').then(r => r.data),

  login: (username: string, password: string) =>
    http.post<{ token: string; username: string }>('/api/auth/login', { username, password }).then(r => r.data),

  setupAdmin: (username: string, password: string) =>
    http.post<{ token: string; username: string }>('/api/auth/setup', { username, password }).then(r => r.data),

  changePassword: (old_password: string, new_password: string) =>
    http.post<{ status: string }>('/api/auth/change-password', { old_password, new_password }).then(r => r.data),
}

export function fmtPrice(fen: number | null | undefined): string {
  if (fen == null) return '—'
  return `¥${(fen / 100).toFixed(2)}`
}

export function fmtMemory(mb: number): string {
  if (mb >= 1024) return `${mb / 1024}GB`
  return `${mb}MB`
}

export const EVENT_LABELS: Record<string, { label: string; color: string }> = {
  new_listing:  { label: '新上架',   color: 'blue' },
  restock:      { label: '已补货',   color: 'green' },
  out_of_stock: { label: '已售罄',   color: 'orange' },
  delisted:     { label: '已下架',   color: 'red' },
  price_change: { label: '价格变动', color: 'purple' },
  purchased:    { label: '购买成功', color: 'cyan' },
}

export const CAT_LABELS: Record<string, string> = {
  vps: '新机VPS',
  ls: '轻量机',
  secondhand: '二手机',
}
