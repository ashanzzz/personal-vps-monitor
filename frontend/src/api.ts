import axios from 'axios'
import { API_BASE } from './config'

const http = axios.create({ baseURL: API_BASE })

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
  akile_token: string
  refresh_interval: number
  vps_enabled: boolean
  ls_enabled: boolean
  secondhand_enabled: boolean
  watched_plans: string[]
  tg_bot_token: string
  tg_chat_id: string
  tg_notify_new_listing: boolean
  tg_notify_restock: boolean
  tg_notify_delisted: boolean
  tg_notify_price_change: boolean
  tg_notify_purchased: boolean
  auto_buy_enabled: boolean
  auto_buy_plans: string[]
  auto_buy_max_price: number
  auto_buy_cycle: number
  ls_regions: string[]
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
