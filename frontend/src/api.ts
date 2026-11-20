import axios from 'axios'

// 始终使用相对路径，纯天然适配单一端口部署
const http = axios.create({
  baseURL: '',
  timeout: 15000,
})

let authToken = localStorage.getItem('vps_token') || ''

export function setToken(token: string) {
  authToken = token
  if (token) {
    localStorage.setItem('vps_token', token)
  } else {
    localStorage.removeItem('vps_token')
  }
}

export function getToken(): string {
  return authToken
}

export function clearToken() {
  authToken = ''
  localStorage.removeItem('vps_token')
}

// 请求拦截器注入 Token
http.interceptors.request.use((cfg) => {
  if (authToken) {
    cfg.headers.Authorization = `Bearer ${authToken}`
  }
  return cfg
})

// 响应拦截器处理 401
let onUnauthorized: (() => void) | null = null
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

http.interceptors.response.use(
  (res) => res,
  (err) => {
    // 只有在管理端主系统的会话失效或未授权时才触发全局登出，绝不能因第三方供应商 Token 报错而误踢出管理员
    const isMainAuthFailure =
      err.response?.status === 401 &&
      ((err.config?.url?.includes('/api/auth/') && !err.config?.url?.includes('/api/auth/login')) ||
        err.response?.data?.error === 'unauthorized')
    if (isMainAuthFailure) {
      clearToken()
      onUnauthorized?.()
    }
    return Promise.reject(err)
  }
)

// ==================== 接口定义 ====================

export interface Instance {
  provider_id: 'lazycat' | 'akile' | 'acck'
  id: string
  name: string
  ip: string
  ipv4?: string
  ipv6?: string
  ssh_user?: string
  ssh_password?: string
  ssh_port?: number
  status: string
  region: string
  category?: string
  cpu?: string
  memory?: string
  traffic_used?: string
  traffic_total?: string
  traffic_percent?: number
  traffic_reset_date?: string
  traffic_reset_price?: number
  auto_reset?: boolean
  user_balance?: number
  root_password?: string
  renew_price?: number
  auto_renew?: boolean
  port_count?: number
  port_limit?: number
  expire_date?: string
  is_stale?: boolean
  stale_reason?: string
  last_synced_at?: string
  // 前端探测状态
  rtt?: number
  songzhong?: boolean
  probing?: boolean
}

export interface PortMapping {
  id: number
  protocol: string
  private_port: number
  public_port: number
  remark: string
}

export interface PortMappingDetail {
  total: number
  host_public_ip?: string
  ssh_port?: number
  port_limit?: number
  ports: PortMapping[]
}

export interface LazyCatInstanceDetail {
  id: number
  name: string
  status: string
  packageName: string
  planName: string
  host_public_ip: string
  nat_public_ip: string
  port_limit: number
  port_mappings: PortMapping[]
  expires_at: string
}

export interface FleetNode {
  uuid: string
  name: string
  region: string
  is_mother: boolean
  is_residential: boolean
  status: string
  cpu_percent: number
  load_avg: number
  ram_used_gb: number
  ram_total_gb: number
  ram_percent: number
  speed_down_kb: number
  speed_up_kb: number
  uptime_days: number
}

export interface EgressNode {
  id: number
  name: string
  country_code: string
  flag_emoji: string
  private_port: number
  outbound_tag: string
  protocol: string
  flow: string
  sni: string
  public_port?: number
  vless_url?: string
}

export interface MarketItem {
  provider_id: string
  id: string
  name: string
  price: number
  renewal_price?: number
  discount_percent?: number
  reset_price?: number
  platform_fee?: number
  flow_used?: string
  cycle?: number
  cycle_name?: string
  monthly_price?: number
  seller_username?: string
  seller_user_id?: number
  location: string
  node_name?: string
  bandwidth?: string
  port_limit?: number
  config_desc?: string
  cpu?: string
  memory?: string
  disk?: string
  traffic_total?: string
  detail?: string
  ip_check_detail?: string
  due_time?: string
  sla?: string
  created_at?: string
  bargain_tag?: string
  diff_percent?: number
  can_buy?: boolean
}

export interface StoreProductItem {
  id: string
  provider_id: string
  name: string
  location: string
  bandwidth: string
  config_desc: string
  cpu?: string
  memory?: string
  disk?: string
  traffic_total?: string
  price: string
  in_stock: boolean
}

export interface MatchedItemPreview {
  id: string
  provider: string
  title: string
  price: number
  renewal_price?: number
  discount_percent?: number
  reset_price?: number
  platform_fee?: number
  flow_used?: string
  cycle?: number
  cycle_name?: string
  monthly_price?: number
  seller_username?: string
  seller_user_id?: number
  location?: string
  node_name?: string
  bandwidth?: string
  port_limit?: number
  config_desc?: string
  cpu?: string
  memory?: string
  disk?: string
  traffic_total?: string
  detail?: string
  ip_check_detail?: string
  due_time?: string
  sla?: string
  can_buy: boolean
  matched_at: string
}

export interface PurchasePlan {
  id: string
  name: string
  provider: 'akile' | 'acck' | 'lazycat' | 'all'
  market_type: 'secondhand' | 'new_stock' | 'all'
  region_keyword?: string
  max_price?: number
  max_renewal_price?: number
  cycle_filter?: 'all' | 'yearly' | 'monthly' | 'other'
  min_discount_percent?: number
  min_port_count?: number
  min_bandwidth?: number
  keyword_filter?: string
  safe_mode: boolean
  auto_buy: boolean
  max_buy_count: number
  current_bought_count: number
  is_active: boolean
  created_at: string
  last_triggered_at?: string
  match_count: number
  last_match_item?: string
  matched_items?: MatchedItemPreview[]
}

export interface SchedulerStatus {
  mode: 'idle' | 'steady' | 'burst'
  active_plans_count: number
  current_interval_sec: number
  slow_interval_sec: number
  burst_interval_sec: number
  burst_remaining_seconds: number
  last_poll_at: string
  total_matched: number
}

export interface MarketStats {
  total_tracked: number
  active_items: number
  avg_price: number
  avg_duration_sec: number
  by_location: Record<string, number>
  hot_models: string[]
}

export interface CheckinResult {
  success: boolean
  message: string
  reward?: string
  website?: string
  account?: string
}

export interface AuditEntry {
  id: string
  timestamp: string
  source: string // "ai_mcp" | "daemon" | "cli" | "rest_api" | "security"
  action: string
  provider: string
  params?: any
  status: 'SUCCESS' | 'FAILED'
  message: string
  duration_ms?: number
}

export interface UnraidContainerStatus {
  name: string
  target: string
  online: boolean
  detail?: string
  latency_ms: number
}

export interface UnraidStatusReport {
  timestamp: string
  containers: UnraidContainerStatus[]
  credentials_found: string[]
}

export interface ProviderHealthDetail {
  provider_id: 'akile' | 'lazycat' | 'acck'
  configured: boolean
  token_valid: boolean
  is_expired: boolean
  expires_at?: string
  seconds_left: number
  has_credentials: boolean
  username_masked?: string
  auto_refresh: boolean
  last_synced_at?: string
  today_checkin?: {
    status: string
    last_attempt_at?: string
    success_at?: string
    retry_count?: number
    next_retry_at?: string
    message?: string
    reward?: string
    account?: string
  }
}

export interface VaultCipherSummary {
  id: string
  name: string
  username?: string
  username_masked: string
  has_password: boolean
  has_totp: boolean
  notes?: string
}

export interface VendorHealthSummary {
  providers: Record<string, ProviderHealthDetail>
  refreshed_at: string
}

export interface TokensConfig {
  akile_configured: boolean
  akile_token_masked: string
  akile_expires_at?: string
  akile_is_expired?: boolean
  akile_seconds_left?: number
  akile_has_credentials?: boolean
  akile_auto_refresh?: boolean

  acck_configured: boolean
  acck_token_masked: string
  acck_expires_at?: string
  acck_is_expired?: boolean
  acck_seconds_left?: number
  acck_has_credentials?: boolean

  lazycat_configured: boolean
  lazycat_token_masked: string
  lazycat_expires_at?: string
  lazycat_is_expired?: boolean
  lazycat_seconds_left?: number
  lazycat_has_credentials?: boolean
  lazycat_auto_refresh?: boolean

  proxy_url: string
}

// ==================== API 客户端方法 ====================

export const api = {
  // 认证体系
  authStatus: () => http.get<{ initialized: boolean }>('/api/auth/status').then(r => r.data),
  login: (username: string, password: string) =>
    http.post<{ token: string; username: string }>('/api/auth/login', { username, password }).then(r => r.data),
  changePassword: (old_password: string, new_password: string) =>
    http.post<{ status: string }>('/api/auth/change-password', { old_password, new_password }).then(r => r.data),

  // 厂商 API 凭证与 Token 配置
  getTokens: () => http.get<TokensConfig>('/api/settings/tokens').then(r => r.data),
  saveTokens: (payload: { akile_token?: string; acck_token?: string; lazycat_token?: string; proxy_url?: string }) =>
    http.post<{ status: string; message: string }>('/api/settings/tokens', payload).then(r => r.data),

  // 厂商交互式一键登录与免密自动续期
  akileLogin: (payload: { username: string; password: string; ga_code?: string; save_credentials?: boolean }) =>
    http.post<{ status: string; message: string; need_2fa?: boolean; token?: string; expires_at?: string }>('/api/akile/login', payload).then(r => r.data),
  akileRefreshToken: () =>
    http.post<{ status: string; message: string; token: string; expires_at: string }>('/api/akile/refresh-token').then(r => r.data),
  lazycatLogin: (payload: { username: string; password: string; ga_code?: string; save_credentials?: boolean }) =>
    http.post<{ status: string; message: string; need_2fa?: boolean; token?: string; expires_at?: string }>('/api/lazycat/login', payload).then(r => r.data),
  acckLogin: (token: string) =>
    http.post<{ status: string; message: string; token: string; expires_at?: string }>('/api/acck/login', { token }).then(r => r.data),

  // 全网聚合资产与打卡
  getInstances: (provider?: string, refresh?: boolean) =>
    http.get<{
      total: number
      instances: Instance[]
      last_synced_at?: string
      next_scheduled_sync?: string
      provider_status?: Record<string, { status: string; message?: string; last_attempt_at: string }>
      refreshed_at?: string
      provider_refresh_times?: Record<string, string>
      provider_errors?: Record<string, string>
    }>('/api/instances', { params: { provider, refresh: refresh ? 'true' : undefined } }).then(r => r.data),
  getProviders: () => http.get<Array<{ id: string; name: string }>>('/api/providers').then(r => r.data),
  checkinAll: () => http.post<Record<string, CheckinResult>>('/api/checkin').then(r => r.data),
  checkinProvider: (providerId: string) => http.post<CheckinResult>(`/api/checkin/${providerId}`).then(r => r.data),

  // 二手与新机集市
  getMarket: (provider?: string) =>
    http.get<{ total: number; items: MarketItem[]; refreshed_at?: string; provider_refresh_times?: Record<string, string>; provider_errors?: Record<string, string> }>('/api/market', { params: { provider } }).then(r => r.data),
  getMarketStats: () => http.get<MarketStats>('/api/market/stats').then(r => r.data),
  getStoreProducts: (provider?: string) =>
    http.get<{ total: number; products: StoreProductItem[]; refreshed_at?: string; provider_refresh_times?: Record<string, string> }>('/api/store', { params: { provider } }).then(r => r.data),
  getRefreshTimes: () => http.get<Record<string, string>>('/api/system/refresh-times').then(r => r.data),

  // 购买计划与智能狙击 (Purchase Plans)
  getPlans: () => http.get<{ total: number; plans: PurchasePlan[] }>('/api/plans').then(r => r.data),
  createPlan: (plan: Partial<PurchasePlan>) => http.post<PurchasePlan>('/api/plans', plan).then(r => r.data),
  updatePlan: (id: string, plan: Partial<PurchasePlan>) => http.put<PurchasePlan>(`/api/plans/${id}`, plan).then(r => r.data),
  deletePlan: (id: string) => http.delete(`/api/plans/${id}`).then(r => r.data),
  togglePlan: (id: string) => http.post<PurchasePlan>(`/api/plans/${id}/toggle`).then(r => r.data),
  buyMarketItemDirect: (providerId: string, itemId: string) =>
    http.post<{ status: string; message: string }>('/api/plans/buy-item', { provider_id: providerId, item_id: itemId }).then(r => r.data),

  // 巡检调度器控制
  getSchedulerStatus: () => http.get<SchedulerStatus>('/api/scheduler/status').then(r => r.data),
  triggerBurst: () => http.post<{ status: string; message: string }>('/api/scheduler/burst').then(r => r.data),

  // 懒猫云特化与深度管理
  getLazyCatInstances: () => http.get<{ total: number; instances: Instance[] }>('/api/lazycat/instances').then(r => r.data),
  getLazyCatPorts: (instanceId?: string) =>
    http.get<PortMappingDetail>('/api/lazycat/ports', { params: { instance_id: instanceId } }).then(r => r.data),
  addLazyCatPort: (instanceId: string, privatePort: number, protocol: string, remark?: string) =>
    http.post<{ status: string; public_port: number; private_port: number; protocol: string; remark: string }>('/api/lazycat/ports', { instance_id: instanceId, private_port: privatePort, protocol, remark }).then(r => r.data),
  deleteLazyCatPort: (instanceId: string, mappingId: number) =>
    http.delete<{ status: string; message: string }>(`/api/lazycat/ports/${mappingId}`, { params: { instance_id: instanceId } }).then(r => r.data),
  getLazyCatInstanceDetail: (instanceId: string) =>
    http.get<LazyCatInstanceDetail>('/api/lazycat/detail', { params: { instance_id: instanceId } }).then(r => r.data),
  lazycatExecCommand: (instanceId: string, command: string, timeoutSec?: number) =>
    http.post<{ status: string; instance_id: string; output: string }>('/api/lazycat/terminal/exec', { instance_id: instanceId, command, timeout_sec: timeoutSec }).then(r => r.data),
  lazycatGetFleetProbe: (refresh?: boolean) =>
    http.get<{ total: number; nodes: FleetNode[]; refreshed_at: string }>('/api/lazycat/fleet-probe', { params: { refresh: refresh ? 'true' : 'false' } }).then(r => r.data),
  lazycatGetEgressNodes: (instanceId: string, refresh?: boolean) =>
    http.get<{ total: number; instance_id: string; nodes: EgressNode[]; refreshed_at: string }>('/api/lazycat/container/egress-nodes', { params: { instance_id: instanceId, refresh: refresh ? 'true' : undefined } }).then(r => r.data),
  syncLazyCatEgress: () => http.post<{ status: string; message: string }>('/api/lazycat/sync').then(r => r.data),

  // 网关联动 (Sing-box / OpenClash)
  getGatewayProxies: () => http.get<any>('/api/gateway/proxies').then(r => r.data),
  selectGatewayNode: (group_name: string, target_node: string) =>
    http.put<{ status: string; group: string; selected: string }>('/api/gateway/select', { group_name, target_node }).then(r => r.data),
  testGatewayDelay: (node_name: string, test_url = 'http://www.gstatic.com/generate_204', timeout_ms = 5000) =>
    http.post<{ name: string; delay: number; error?: string }>('/api/gateway/delay', { node_name, test_url, timeout_ms }).then(r => r.data),

  // 行为审计日志
  getAuditLogs: (params?: { days?: number; limit?: number; source?: string; provider?: string; status?: string }) =>
    http.get<{ total: number; entries: AuditEntry[] }>('/api/audit', { params }).then(r => r.data),

  // Unraid 生态探测与自动连接状态
  getUnraidStatus: () => http.get<UnraidStatusReport>('/api/system/unraid-status').then(r => r.data),

  // 极速秒级鉴权健康与各服务独立状态
  getVendorHealthSummary: () => http.get<VendorHealthSummary>('/api/vendor/health-summary').then(r => r.data),
  vendorQuickLogin: (provider: string, gaCode?: string) =>
    http.post<{ status: string; message: string; need_2fa?: boolean; token?: string; expires_at?: string }>(`/api/vendor/quick-login/${provider}`, { ga_code: gaCode }).then(r => r.data),
  vendorSingleSync: (provider: string) =>
    http.post<{ status: string; provider_id: string; instances: Instance[]; count: number; last_synced_at: string }>(`/api/vendor/sync/${provider}`).then(r => r.data),
  saveVendorCredentials: (data: { provider: string; username: string; password: string; auto_refresh: boolean }) =>
    http.post<{ status: string; message: string }>('/api/vendor/credentials/save', data).then(r => r.data),
  vendorVaultSync: () =>
    http.post<{ status: string; message: string; matched_count: number; items: any[]; credentials: any }>('/api/vendor/vault/sync').then(r => r.data),
  vendorVaultListCiphers: (data?: { server?: string; username?: string; password?: string }) =>
    http.post<{ status: string; ciphers: VaultCipherSummary[]; count: number }>('/api/vendor/vault/ciphers', data || {}).then(r => r.data),
  vendorVaultBind: (data: { provider: string; cipher_id: string; server?: string; username?: string; password?: string }) =>
    http.post<{ status: string; message: string; token: string; expires_at: string; username_masked: string }>('/api/vendor/vault/bind', data).then(r => r.data),
  vendorTestAndSave: (data: { provider: string; username: string; password: string; totp_secret?: string; auto_refresh?: boolean }) =>
    http.post<{ status: string; message: string; token: string; expires_at: string; username_masked: string }>('/api/vendor/test-and-save', data).then(r => r.data),
}
