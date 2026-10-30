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
  score: number
  star_rating: number
  price_recommendation: { recommend: string; reason: string; savings_pct: number | null } | null
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

// One spec dimension's range (min/max). 0 on a side = unbounded there.
// memory range is in MB (matches the stored field); others map 1:1.
export interface SpecRange {
  min: number
  max: number
}
export type SpecDimension = 'cpu' | 'memory' | 'disk' | 'bandwidth' | 'traffic'
export type RuleSpec = Record<SpecDimension, SpecRange>

export interface BuyRule {
  id: string
  enabled: boolean
  label: string
  tags_include: string[]        // match ANY substring in name/node; [] = any
  spec: RuleSpec                // cpu/memory/disk/bandwidth/traffic min-max
  price_type: 'monthly' | 'yearly'
  price_max: number             // fen; cap on chosen-cycle renewal price; 0 = no limit
  sale_price_max: number        // fen; cap on sale price; 0 = no limit
  max_buy: number               // per-rule buy cap; 0 = unlimited
}

export interface PageBuyConfig {
  enabled: boolean
  refresh_interval: number     // seconds
  max_total_per_run: number
  max_total_daily: number
  max_price: number            // fen; legacy page cap, retained for back-compat
  cycle: number
  on_limit_reached: 'stop' | 'keep_watching'
  rules: BuyRule[]
}

// Per-page hardware scoring weights: {dimension -> weight}. score = Σ(value×weight).
// Positive = more is better; negative (price/latency) = less is better.
// latency is skipped when a machine has no measured value. Mirrors backend
// scoring.DEFAULT_SCORE_RULES.
export type ScoreDimension =
  | 'cpu' | 'memory' | 'disk' | 'bandwidth' | 'traffic'
  | 'price_monthly' | 'sale_price' | 'latency'
export type ScoreRules = Record<ScoreDimension, number>

export interface AppSettings {
  // API & connection
  akile_token: string
  refresh_interval: number
  auto_refresh_token: boolean
  refresh_margin_min_seconds: number
  refresh_margin_max_seconds: number
  request_timeout: number
  request_retries: number
  account_refresh_interval: number
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
  filter_min_score: number
  filter_min_star: number
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
  // Per-page auto-buy
  cloud_buy: PageBuyConfig
  ls_buy: PageBuyConfig
  secondhand_buy: PageBuyConfig
  plan_label_map: Record<string, string>
  // Per-page table column widths ({col key -> width px}); {} = code defaults
  col_widths_cloud: Record<string, number>
  col_widths_ls: Record<string, number>
  col_widths_secondhand: Record<string, number>
  // Per-page hardware scoring rules ({dimension -> weight}); drives the 评分 column
  score_rules_cloud: ScoreRules
  score_rules_ls: ScoreRules
  score_rules_secondhand: ScoreRules
  // Scoring weights
  score_w_cpu: number
  score_w_mem: number
  score_w_disk: number
  score_w_bw: number
  score_w_traffic: number
  // Logging
  log_level: 'INFO' | 'DEBUG'
  auto_checkin_enabled: boolean
  acck_auto_checkin_enabled?: boolean
  checkin_mode?: 'fixed' | 'random'
  checkin_fixed_time?: string
  checkin_time_start: string
  checkin_time_end: string
  random_jitter_enabled: boolean
  // System
  event_retention_days: number
  store_last_refresh: string | null
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

  refreshStore: (page?: string, limit?: number) =>
    http.post<{ status: string; page: string; limit: number }>('/api/store/refresh', { page, limit }).then(r => r.data),

  buyMachine: (plan_id: string) =>
    http.post(`/api/buy/${plan_id}`).then(r => r.data),

  getTokenStatus: () =>
    http.get<TokenStatus>('/api/token/status').then(r => r.data),

  refreshToken: () =>
    http.post<{ status: string }>('/api/token/refresh').then(r => r.data),

  akileLogin: (username: string, password: string, ga_code?: string) =>
    http.post<TokenStatus & { need_2fa?: boolean; message?: string }>('/api/akile-login', { username, password, ga_code: ga_code || '' }).then(r => r.data),

  // --- Auth ---
  authStatus: () =>
    http.get<{ initialized: boolean }>('/api/auth/status').then(r => r.data),

  login: (username: string, password: string) =>
    http.post<{ token: string; username: string }>('/api/auth/login', { username, password }).then(r => r.data),

  setupAdmin: (username: string, password: string) =>
    http.post<{ token: string; username: string }>('/api/auth/setup', { username, password }).then(r => r.data),

  changePassword: (old_password: string, new_password: string) =>
    http.post<{ status: string }>('/api/auth/change-password', { old_password, new_password }).then(r => r.data),

  // --- Version & logs ---
  getVersion: () =>
    http.get<VersionInfo>('/api/version').then(r => r.data),

  getAppLog: (lines = 500) =>
    http.get<{ lines: number; content: string }>('/api/logs/app', { params: { lines } }).then(r => r.data),

  // --- My account ---
  getAccount: () =>
    http.get<AccountInfo>('/api/account').then(r => r.data),

  refreshAccount: () =>
    http.post<AccountInfo>('/api/account/refresh').then(r => r.data),

  // --- Checkin ---
  getCheckinStatus: () =>
    http.get<{
      today: string
      config: {
        mode: 'fixed' | 'random'
        fixed_time: string
        time_start: string
        time_end: string
        akile_enabled: boolean
        acck_enabled: boolean
      }
      status: {
        akile: { checked_in: boolean; status: string; last_log: any }
        acck: { checked_in: boolean; status: string; last_log: any }
      }
      history: Array<{
        id: number
        platform: string
        checkin_date: string
        status: string
        message: string
        created_at: string | null
      }>
    }>('/api/checkin/status').then(r => r.data),

  checkin: () =>
    http.post<{ status_code: number; status_msg: string; ak_coin_earned?: number }>('/api/checkin').then(r => r.data),

  // --- ACCK Platform ---
  getAcckAccount: () =>
    http.get<AcckAccountInfo>('/api/acck/account').then(r => r.data),

  refreshAcckAccount: () =>
    http.post<AcckAccountInfo>('/api/acck/account/refresh').then(r => r.data),

  checkinAcck: () =>
    http.post<Record<string, unknown>>('/api/acck/checkin').then(r => r.data),

  getAcckServers: () =>
    http.get<AcckServer[]>('/api/acck/servers').then(r => r.data),

  getAcckShop: () =>
    http.get<{ shop: Record<string, unknown>; logs: Array<Record<string, unknown>> }>('/api/acck/shop').then(r => r.data),

  acckBuyPreview: (payload: Record<string, unknown>) =>
    http.post<Record<string, unknown>>('/api/acck/buy-preview', payload).then(r => r.data),

  // --- Dual-Rate Refresh Control ---
  getRefreshStatus: () =>
    http.get<RefreshState>('/api/unified/refresh/status').then(r => r.data),

  startBurstRefresh: (duration = 300, interval = 2) =>
    http.post<RefreshState>('/api/unified/refresh/burst', { duration, interval }).then(r => r.data),

  stopBurstRefresh: () =>
    http.post<RefreshState>('/api/unified/refresh/slow').then(r => r.data),

  // --- Unified Dual-Platform Management ---
  getUnifiedServers: () =>
    http.get<{ total: number; servers: UnifiedServer[]; counts: Record<string, number> }>('/api/unified/servers').then(r => r.data),

  updateServerAutoRenew: (platform: string, server_id: number, auto_renew: boolean) =>
    http.post<{ platform: string; server_id: number; auto_renew: boolean; status_text?: string; result: any }>('/api/unified/servers/auto-renew', {
      platform,
      server_id,
      auto_renew,
    }).then(r => r.data),

  getServersAutoRenewStatus: () =>
    http.get<{
      total: number
      enabled_count: number
      disabled_count: number
      servers: any[]
      mechanism_docs: any
    }>('/api/unified/servers/auto-renew-status').then(r => r.data),

  renewServer: (platform: string, server_id: number, cycle = 1, auto_pay = false) =>
    http.post<{ platform: string; server_id: number; order?: any; paid?: boolean; pay_result?: any }>('/api/unified/servers/renew', {
      platform,
      server_id,
      cycle,
      auto_pay,
    }).then(r => r.data),

  getUnifiedBilling: (page = 1, page_size = 20) =>
    http.get<UnifiedBillingData>('/api/unified/billing', { params: { page, page_size } }).then(r => r.data),

  payUnifiedOrder: (platform: string, order_id?: any, order_no?: string, method = 'balance') =>
    http.post('/api/unified/billing/pay', { platform, order_id, order_no, method }).then(r => r.data),

  getUnifiedStore: () =>
    http.get<{ total: number; packages: StorePackage[]; akile_store: any; acck_store: any }>('/api/unified/store').then(r => r.data),

  buyStorePackage: (platform: string, plan_id: any, order_data: any[] = [], safe_mode = true) =>
    http.post('/api/unified/store/buy', { platform, plan_id, order_data, safe_mode }).then(r => r.data),

  getUnifiedSecondhand: (page = 1, page_size = 50) =>
    http.get<{ total: number; items: SecondhandItem[]; counts: Record<string, number> }>('/api/unified/secondhand', { params: { page, page_size } }).then(r => r.data),

  buySecondhandItem: (platform: string, listing_id: number | string, price_cents = 0, safe_mode = true, captcha = '', force_buy_depleted = false) =>
    http.post('/api/unified/secondhand/buy', { platform, listing_id, price_cents, safe_mode, captcha, force_buy_depleted }).then(r => r.data),
}

export interface RefreshState {
  mode: 'slow' | 'burst'
  current_interval: number
  slow_interval: number
  burst_interval: number
  burst_duration: number
  remaining_seconds: number
  expires_at: number | null
  last_poll_at: number | null
  total_bursts: number
}

export interface UnifiedServer {
  uid: string
  platform: 'akile' | 'acck'
  id: number
  server_id: string
  name: string
  node_name: string
  area_name: string
  ipv4: string
  ipv6: string
  cpu: number
  memory: number
  disk: number
  bandwidth: number
  flow: number
  used_flow: number
  due_time: number
  auto_renew: boolean
  price?: number
  price_cents?: number
  price_cny: number
  cycle: number
  cycle_text?: string
  status: number
  system_name?: string
  raw?: Record<string, unknown>
}

export interface UnifiedBillingData {
  akile: {
    orders: any[]
    money_logs: any[]
    pending_invoices: any[]
  }
  acck: {
    orders: any[]
    shop_logs: any[]
  }
}

export interface RouteAnalysis {
  tier: 'S' | 'A' | 'B' | 'C'
  tier_label: string
  is_optimized: boolean
  routes: string[]
  primary_route: string
  native_ip: boolean
  ip_clean: boolean
  ip_status: string
  ip_status_text: string
}

export interface TrafficAnalysis {
  total_gb: number
  used_gb: number
  remaining_gb: number
  remaining_ratio_percent: number
  is_depleted: boolean
  is_low: boolean
  status: string
  status_text: string
  badge_tone: string
  reset_price_cny: number
}

export interface PricingAnalysis {
  ask_price_cny: number
  renew_price_cny: number
  reset_traffic_fee_cny: number
  ip_repair_fee_cny?: number
  real_total_cost_cny: number
  intrinsic_value_cny: number
}

export interface SnipingVerdict {
  is_snipable: boolean
  score: number
  reasons: string[]
  verdict_text: string
}

export interface CostEffectiveness {
  score: number
  grade: string
  grade_label: string
  badge_color: string
  points_per_yuan?: number
  points_per_usd?: number
  cost_per_point_cny?: number
  monthly_points_per_yuan?: number
  monthly_points_per_usd?: number
  breakdown: {
    network_score: number
    ltv_score: number
    traffic_score: number
    price_score: number
  }
  reasons: string[]
}

export interface StorePackage {
  uid: string
  platform: 'akile' | 'acck'
  id: number | string
  name: string
  area: string
  node: string
  cpu: number
  memory: number
  disk: number
  bandwidth: number
  flow: number
  price_cny: number
  cycle: number
  monthly_cny: number
  stock: number
  cost_effectiveness?: CostEffectiveness
  score?: number
  grade_label?: string
  badge_color?: string
  points_per_yuan?: number
  points_per_usd?: number
  reasons?: string[]
  raw?: Record<string, unknown>
}

export interface SecondhandItem {
  uid: string
  platform: 'akile' | 'acck'
  id: number | string
  title: string
  area: string
  node: string
  price_cents: number
  price_cny: number
  renew_price_cny?: number
  bandwidth: number
  flow: number
  due_time: number
  seller: string
  intro?: string
  route?: RouteAnalysis
  traffic?: TrafficAnalysis
  pricing_analysis?: PricingAnalysis
  cost_effectiveness?: CostEffectiveness
  sniping?: SnipingVerdict
  score?: number
  grade_label?: string
  badge_color?: string
  points_per_yuan?: number
  points_per_usd?: number
  reasons?: string[]
  raw?: Record<string, unknown>
}

export interface AcckProfile {
  username: string
  email: string
  role: string
  points: number | null
  coins?: number | null
  telegram_id?: string | number | null
  created_at?: string | null
  status?: string
  last_checkin_time?: number | null
  checked_in_today?: boolean
}

export interface AcckServer {
  id?: string | number
  name?: string
  ip?: string
  node?: string
  status?: string
  expired_at?: string
  [key: string]: unknown
}

export interface AcckAccountInfo {
  configured: boolean
  platform: string
  profile: AcckProfile | null
  servers: AcckServer[]
  shop?: Record<string, unknown> | null
  errors: string[]
  updated_at: string | null
}

export interface AccountProfile {
  username: string
  email: string
  role: string
  money: number | null        // balance in fen
  telegram_id: number | null
  is_real_name: boolean
  is_ga: boolean
  last_checkin_time: number | null
  ak_coin: number | null
}

export interface MyServer {
  [key: string]: unknown
}

export interface AccountInfo {
  configured: boolean
  profile: AccountProfile | null
  servers: MyServer[]
  errors: string[]
  updated_at: string | null
}

export interface VersionInfo {
  version: string
  git_sha: string
  build_time: string
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
