import { useState, useEffect, useCallback, useRef } from 'react'
import {
  ConfigProvider, theme, Layout, Menu, Typography, Button, Badge,
  Space, Tag, Table, Card, Row, Col, Statistic, Input, Select,
  Modal, Form, message, Alert, Tabs, Popconfirm, Drawer, Progress,
  Switch, InputNumber, Tooltip, Dropdown, Avatar
} from 'antd'
import zhCN from 'antd/locale/zh_CN'
import {
  DashboardOutlined, CloudServerOutlined, ThunderboltOutlined,
  RocketOutlined, SafetyCertificateOutlined,
  SettingOutlined, LogoutOutlined, CheckCircleOutlined,
  ReloadOutlined, PlusOutlined, DeleteOutlined, LockOutlined,
  UserOutlined, ClockCircleOutlined, EyeOutlined,
  AimOutlined, PauseCircleOutlined, SyncOutlined,
  EditOutlined, FireOutlined, DesktopOutlined,
  ShoppingOutlined, SwapOutlined, ApiOutlined, CodeOutlined,
  LoginOutlined, KeyOutlined, CopyOutlined
} from '@ant-design/icons'

import { api, setToken, getToken, clearToken, setUnauthorizedHandler } from './api'
import type {
  Instance, PortMapping, MarketItem, AuditEntry, UnraidStatusReport,
  TokensConfig, PurchasePlan, SchedulerStatus, MarketStats, StoreProductItem, MatchedItemPreview,
  VendorHealthSummary
} from './api'
import { LazyCatControlCenterModal } from './components/LazyCatControlCenterModal'
import { ProviderLoginModal } from './components/ProviderLoginModal'
import { fmtBytes } from './lib/format'
import './App.css'

const { Header, Sider, Content } = Layout
const { Text } = Typography

// 格式化时间为 HH:mm:ss 或友好展示
function formatTime(isoStr?: string): string {
  if (!isoStr) return '未同步'
  try {
    const d = new Date(isoStr)
    if (isNaN(d.getTime())) return isoStr
    return d.toLocaleTimeString('zh-CN', { hour12: false })
  } catch {
    return isoStr
  }
}

// 格式化完整时间 YYYY-MM-DD HH:mm:ss
function formatFullTime(isoStr?: string): string {
  if (!isoStr) return '未单独同步'
  try {
    const d = new Date(isoStr)
    if (isNaN(d.getTime())) return isoStr
    const year = d.getFullYear()
    const month = String(d.getMonth() + 1).padStart(2, '0')
    const day = String(d.getDate()).padStart(2, '0')
    const time = d.toLocaleTimeString('zh-CN', { hour12: false })
    return `${year}-${month}-${day} ${time}`
  } catch {
    return isoStr
  }
}

// 统一格式化实例到期时间 (将 LazyCat ISO UTC 时间转换为本地 YYYY-MM-DD HH:mm:ss)
function formatExpireDate(dateStr?: string): string {
  if (!dateStr) return '自动扣费'
  if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}/.test(dateStr)) return dateStr
  try {
    const d = new Date(dateStr)
    if (isNaN(d.getTime())) return dateStr
    const year = d.getFullYear()
    const month = String(d.getMonth() + 1).padStart(2, '0')
    const day = String(d.getDate()).padStart(2, '0')
    const time = d.toLocaleTimeString('zh-CN', { hour12: false })
    return `${year}-${month}-${day} ${time}`
  } catch {
    return dateStr
  }
}

// 格式化到期剩余时间提示
function formatExpiry(expiresAt?: string, isExpired?: boolean, secondsLeft?: number): string {
  if (!expiresAt) return ''
  if (isExpired) return '已过期'
  if (secondsLeft !== undefined) {
    if (secondsLeft <= 0) return '已过期'
    if (secondsLeft < 3600) return `剩余 ${Math.max(1, Math.floor(secondsLeft / 60))} 分钟`
    const hours = Math.floor(secondsLeft / 3600)
    const days = Math.floor(hours / 24)
    if (days > 0) {
      return `剩余 ${days} 天 ${hours % 24} 小时`
    }
    return `剩余 ${hours} 小时`
  }
  return ''
}

// 格式化展示续费价格与周期（年付/月付/季付/半年付），区分年付总价与等效月付
function renderRenewalDisplay(item: {
  renewal_price?: number
  renew_price?: number
  cycle?: number
  cycle_name?: string
  monthly_price?: number
  discount_percent?: number
}) {
  const price = item.renewal_price ?? item.renew_price
  if (!price || price <= 0) {
    return <span style={{ color: '#94a3b8' }}>-</span>
  }

  const cycleMonths = item.cycle || 1
  let cycleName = item.cycle_name
  if (!cycleName) {
    if (cycleMonths >= 12) cycleName = '年付'
    else if (cycleMonths === 6) cycleName = '半年付'
    else if (cycleMonths === 3) cycleName = '季付'
    else cycleName = '月付'
  }

  const isYearly = cycleMonths >= 12
  const isMultiMonth = cycleMonths > 1
  const monthly = item.monthly_price && item.monthly_price > 0
    ? item.monthly_price
    : (isMultiMonth ? Number((price / cycleMonths).toFixed(2)) : price)

  const cycleTagColor = isYearly ? 'purple' : isMultiMonth ? 'cyan' : 'blue'

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 4, flexWrap: 'nowrap' }}>
        <Tag color={cycleTagColor} style={{ fontSize: 10, padding: '0 4px', margin: 0, fontWeight: 600 }}>
          {cycleName}
        </Tag>
        <span style={{ color: '#2563eb', fontWeight: 600, fontSize: 12, fontFamily: 'monospace', whiteSpace: 'nowrap' }}>
          ¥{price.toFixed(2)}
        </span>
      </div>
      {isMultiMonth && (
        <div style={{ fontSize: 11, color: '#64748b', marginTop: 2 }}>
          折合 <span style={{ color: '#059669', fontWeight: 600 }}>¥{monthly.toFixed(2)}</span>/月
        </div>
      )}
      {item.discount_percent && item.discount_percent > 0 ? (
        <div style={{ marginTop: 2 }}>
          <Tag color="red" style={{ fontSize: 10, padding: '0 3px', margin: 0 }}>
            {item.discount_percent / 10}折 ({item.discount_percent}%)
          </Tag>
        </div>
      ) : null}
    </div>
  )
}

// 动态解析流量百分比，杜绝硬编码伪造数据
function parseTrafficPercent(used?: string, total?: string): number {
  if (!used || !total) return 0
  const parseGB = (s: string) => {
    const clean = s.replace(/[^0-9.]/g, '')
    const num = parseFloat(clean) || 0
    if (s.toLowerCase().includes('tb')) return num * 1024
    if (s.toLowerCase().includes('mb')) return num / 1024
    return num
  }
  const u = parseGB(used)
  const t = parseGB(total)
  if (t <= 0) return 0
  const pct = Math.min(100, Math.max(0, Math.round((u / t) * 100)))
  return pct
}

function getTrafficPercent(r: Instance): number {
  if (typeof r.traffic_percent === 'number' && !isNaN(r.traffic_percent) && r.traffic_percent > 0) {
    return Math.min(100, Math.max(0, Number(r.traffic_percent.toFixed(1))))
  }
  return parseTrafficPercent(r.traffic_used, r.traffic_total)
}

function getTrafficColor(pct: number): string {
  if (pct >= 85) return '#ef4444' // 红色: 消耗超 85%
  if (pct >= 70) return '#f59e0b' // 橙色: 消耗 70-85%
  return '#10b981' // 绿色: 消耗低健康
}

// ==================== 登录组件 (白灰黑蓝专业风) ====================
function LoginForm({ onLoginSuccess }: { onLoginSuccess: () => void }) {
  const [loading, setLoading] = useState(false)
  const [form] = Form.useForm()

  const handleFinish = async (values: any) => {
    setLoading(true)
    try {
      const res = await api.login(values.username, values.password)
      setToken(res.token)
      message.success(`欢迎回来，管理员 ${res.username}`)
      onLoginSuccess()
    } catch (err: any) {
      const msg = err.response?.data?.detail || '登录失败，请检查密码'
      message.error(msg)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{
      minHeight: '100vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      background: 'linear-gradient(135deg, #f8fafc 0%, #e2e8f0 100%)',
      padding: 24,
    }}>
      <Card
        bordered={false}
        style={{
          width: '100%',
          maxWidth: 420,
          boxShadow: '0 10px 25px -5px rgba(15, 23, 42, 0.08), 0 8px 10px -6px rgba(15, 23, 42, 0.04)',
          borderRadius: 12,
          padding: '12px 8px',
        }}
      >
        <div style={{ textAlign: 'center', marginBottom: 28 }}>
          <div style={{
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            width: 48,
            height: 48,
            borderRadius: 12,
            background: '#eff6ff',
            color: '#2563eb',
            fontSize: 22,
            marginBottom: 14,
          }}>
            <SafetyCertificateOutlined />
          </div>
          <h2 style={{ margin: 0, fontWeight: 700, color: '#0f172a', fontSize: 20 }}>
            Personal VPS Monitor
          </h2>
          <Text type="secondary" style={{ fontSize: 13, color: '#64748b', display: 'block', marginTop: 4 }}>
            单端口全栈资产中枢 · 智能购买计划 · 安全审计中心
          </Text>
        </div>

        <Form form={form} layout="vertical" onFinish={handleFinish} initialValues={{ username: 'admin' }}>
          <Form.Item name="username" rules={[{ required: true, message: '请输入管理员账号' }]}>
            <Input prefix={<UserOutlined style={{ color: '#94a3b8' }} />} placeholder="管理员账号 (默认 admin)" size="large" />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined style={{ color: '#94a3b8' }} />} placeholder="管理员密码 (默认 admin)" size="large" />
          </Form.Item>
          <Form.Item style={{ marginBottom: 12, marginTop: 24 }}>
            <Button type="primary" htmlType="submit" size="large" block loading={loading} style={{ fontWeight: 600, height: 42 }}>
              安全登录
            </Button>
          </Form.Item>
        </Form>

        <div style={{ textAlign: 'center', marginTop: 12, fontSize: 12, color: '#94a3b8' }}>
          已开启连续 5 次输错密码自动封禁 IP 安全防护
        </div>
      </Card>
    </div>
  )
}

// ==================== 主应用 ====================
export default function App() {
  const [authed, setAuthed] = useState<boolean>(!!getToken())
  const [page, setPage] = useState<string>('overview')
  const [collapsed, setCollapsed] = useState<boolean>(false)

  // 数据状态
  const [instances, setInstances] = useState<Instance[]>([])
  const [marketItems, setMarketItems] = useState<MarketItem[]>([])
  const [storeProducts, setStoreProducts] = useState<StoreProductItem[]>([])
  const [auditLogs, setAuditLogs] = useState<AuditEntry[]>([])
  const [unraidStatus, setUnraidStatus] = useState<UnraidStatusReport | null>(null)
  // 已登录时首帧即置为加载中，避免首屏闪现「0 台 / 尚未同步」的假空态
  const [loading, setLoading] = useState<boolean>(!!getToken())
  // 首次成功加载资产前，表格保持骨架加载态，杜绝「共 0 台」误导
  const [initialLoaded, setInitialLoaded] = useState(false)
  // 二手集市为慢通道，加载期间以占位符代替 0，避免统计数字跳变
  const [marketLoading, setMarketLoading] = useState(true)
  const [checkinLoading, setCheckinLoading] = useState(false)

  // 模块细粒度刷新时间戳与每日资产快照
  const [refreshTimes, setRefreshTimes] = useState<Record<string, string>>({})
  const [dailySyncInfo, setDailySyncInfo] = useState<{ last_synced_at?: string; next_scheduled_sync?: string }>({})

  // 购买计划与智能狙击状态
  const [plans, setPlans] = useState<PurchasePlan[]>([])
  const [schedulerStatus, setSchedulerStatus] = useState<SchedulerStatus | null>(null)
  const [marketStats, setMarketStats] = useState<MarketStats | null>(null)
  const [planModalOpen, setPlanModalOpen] = useState(false)
  const [editingPlan, setEditingPlan] = useState<PurchasePlan | null>(null)
  const [planForm] = Form.useForm()
  const [sniperTab, setSniperTab] = useState<string>('all')

  // VPS 详情 Drawer
  const [detailDrawerOpen, setDetailDrawerOpen] = useState(false)
  const [selectedVps, setSelectedVps] = useState<Instance | null>(null)

  // 单台实例 SSH / Root 凭据按需取用（列表接口已统一脱敏，绝不下发明文）
  const [credentialValue, setCredentialValue] = useState<string>('')
  const [credentialVisible, setCredentialVisible] = useState(false)
  const [credentialLoading, setCredentialLoading] = useState(false)

  const fetchCredential = useCallback(async (id: string) => {
    setCredentialLoading(true)
    try {
      const res = await api.getInstanceCredential(id, true)
      setCredentialValue(res.password || '')
      setCredentialVisible(true)
      if (!res.has_password) {
        message.info('该实例暂未持久化 SSH 密码')
      }
    } catch (e: any) {
      message.error('读取实例凭据失败: ' + (e.response?.data?.error || e.message))
    } finally {
      setCredentialLoading(false)
    }
  }, [])

  useEffect(() => {
    setCredentialValue('')
    setCredentialVisible(false)
  }, [selectedVps?.id])

  useEffect(() => {
    if (!detailDrawerOpen) {
      setCredentialValue('')
      setCredentialVisible(false)
    }
  }, [detailDrawerOpen])

  // 三家服务商二手集市独立搜索关键字
  const [akMarketSearch, setAkMarketSearch] = useState('')
  const [acMarketSearch, setAcMarketSearch] = useState('')
  const [lzMarketSearch, setLzMarketSearch] = useState('')

  // 二手检索过滤通用函数 (大小写不敏感，匹配标题/机器名、节点代码、配置描述与备注)
  const matchMarketSearch = (item: MarketItem, keyword: string) => {
    if (!keyword || !keyword.trim()) return true
    const kw = keyword.trim().toLowerCase()
    const name = (item.name || '').toLowerCase()
    const nodeName = (item.node_name || '').toLowerCase()
    const configDesc = (item.config_desc || '').toLowerCase()
    const bargainTag = (item.bargain_tag || '').toLowerCase()
    const detail = (item.detail || '').toLowerCase()
    return name.includes(kw) || nodeName.includes(kw) || configDesc.includes(kw) || bargainTag.includes(kw) || detail.includes(kw)
  }

  // 懒猫云端口状态
  const [ports, setPorts] = useState<PortMapping[]>([])
  const [portsLoading, setPortsLoading] = useState(false)
  const [selectedInstId, setSelectedInstId] = useState<string>('11433')
  const [portModalOpen, setPortModalOpen] = useState(false)
  const [containerPortModalOpen, setContainerPortModalOpen] = useState(false)
  const [portForm] = Form.useForm()

  // 懒猫云多维控制中心 (WebShell + 端口映射 + Reality矩阵 + 母鸡遥测)
  const [lazycatModalOpen, setLazycatModalOpen] = useState(false)
  const [lazycatTargetInstance, setLazycatTargetInstance] = useState<Instance | null>(null)

  // 厂商 API 凭证管理
  const [tokensConfig, setTokensConfig] = useState<TokensConfig | null>(null)
  const [tokensForm] = Form.useForm()
  const [savingTokens, setSavingTokens] = useState(false)
  const [providerErrors, setProviderErrors] = useState<Record<string, string>>({})

  // 极速秒级鉴权健康与单服务独立同步状态
  const [vendorHealth, setVendorHealth] = useState<VendorHealthSummary | null>(null)
  const [singleSyncing, setSingleSyncing] = useState<string | null>(null)
  const [quickLogging, setQuickLogging] = useState<string | null>(null)
  const [vaultSyncing, setVaultSyncing] = useState(false)

  const syncVendorCheckinFromHealth = useCallback((res: VendorHealthSummary | null) => {
    if (!res?.providers) return
    setVendorCheckinStatus(prev => {
      const next = { ...prev }
      Object.entries(res.providers).forEach(([pid, detail]) => {
        const st = detail.today_checkin?.status
        if (st === 'success' || st === 'already_signed') {
          next[pid] = true
        }
      })
      return next
    })
    setVendorCheckinMsg(prev => {
      const next = { ...prev }
      Object.entries(res.providers).forEach(([pid, detail]) => {
        if (detail.today_checkin?.message) {
          next[pid] = detail.today_checkin.message
        }
      })
      return next
    })
  }, [])

  const loadVendorHealth = useCallback(async () => {
    try {
      const res = await api.getVendorHealthSummary()
      setVendorHealth(res)
      syncVendorCheckinFromHealth(res)
    } catch {
      // 容错不阻断
    }
  }, [syncVendorCheckinFromHealth])

  const handleGlobalVaultSync = async () => {
    try {
      setVaultSyncing(true)
      const res = await api.vendorVaultSync()
      message.success(res.message || `成功从 Vaultwarden 解密提取 ${res.matched_count} 家厂商凭据！`)
      loadVendorHealth()
      loadData()
    } catch (err: any) {
      message.error(err.response?.data?.error || err.message || '从 Vaultwarden 自动同步解密失败')
    } finally {
      setVaultSyncing(false)
    }
  }

  const handleSingleSync = async (provider: string) => {
    try {
      setSingleSyncing(provider)
      const res = await api.vendorSingleSync(provider)
      message.success(`${provider.toUpperCase()} 服务同步完成 (获取 ${res.count} 台)`)
      loadVendorHealth()
      loadData()
    } catch (err: any) {
      message.error(err.response?.data?.error || err.message || `同步 ${provider} 失败`)
    } finally {
      setSingleSyncing(null)
    }
  }

  const handleQuickLogin = async (provider: string) => {
    try {
      setQuickLogging(provider)
      const res = await api.vendorQuickLogin(provider)
      if (res.need_2fa) {
        setLoginTargetProvider(provider as any)
        setLoginModalOpen(true)
        message.info('检测到二次验证，请输入 6 位 2FA 动态码')
        return
      }
      message.success(res.message || `${provider} 免密登录成功！Token 已自动更新生效`)
      loadVendorHealth()
      loadData()
    } catch (err: any) {
      setLoginTargetProvider(provider as any)
      setLoginModalOpen(true)
    } finally {
      setQuickLogging(null)
    }
  }

  // 渲染服务专属独立同步状态栏
  const renderProviderHeader = (providerKey: 'akile' | 'lazycat' | 'acck', providerTitle: string) => {
    const health = vendorHealth?.providers?.[providerKey]
    const syncTime = health?.last_synced_at || refreshTimes?.[providerKey]
    const isSyncing = singleSyncing === providerKey
    const isLogging = quickLogging === providerKey

    const isValid = health?.token_valid && !health?.is_expired
    const checkin = health?.today_checkin
    const checkinSuccess = checkin?.status === 'success' || checkin?.status === 'already_signed'
    const checkinRetrying = checkin?.status === 'retrying' || (checkin?.next_retry_at && checkin?.next_retry_at !== '0001-01-01 00:00:00' && checkin?.next_retry_at !== '')
    const checkinNextRetry = checkin?.next_retry_at
    const checkinErr = checkin?.message
    const checkinTime = checkin?.success_at || checkin?.last_attempt_at

    return (
      <Card
        size="small"
        bordered={false}
        style={{
          background: 'linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%)',
          borderRadius: 10,
          border: '1px solid #e2e8f0',
          boxShadow: '0 1px 3px rgba(0,0,0,0.02)'
        }}
      >
        <Row gutter={[16, 12]} align="middle" justify="space-between">
          <Col xs={24} md={15}>
            <Space size={10} wrap align="center">
              <span style={{ fontSize: 15, fontWeight: 700, color: '#0f172a' }}>
                {providerTitle}
              </span>
              <Tag color={isValid ? 'green' : 'error'} style={{ margin: 0, padding: '2px 8px' }}>
                {isValid ? '● 鉴权有效 (秒级就绪)' : '● 鉴权失效 (自动重登自愈)'}
              </Tag>
              {checkinSuccess ? (
                <Tag color="success" icon={<CheckCircleOutlined />} style={{ margin: 0 }}>
                  今日已打卡 {checkinTime ? `(${formatTime(checkinTime)})` : ''}
                </Tag>
              ) : checkinRetrying ? (
                <Tooltip title={checkinErr ? `重试原因: ${checkinErr}` : '30分钟后自动重试'}>
                  <Tag color="processing" icon={<ClockCircleOutlined />} style={{ margin: 0 }}>
                    打卡重试中 (下次: {checkinNextRetry ? formatTime(checkinNextRetry) : '稍后'})
                  </Tag>
                </Tooltip>
              ) : (
                <Tag color="default" style={{ margin: 0 }}>
                  打卡: 每日 08:00
                </Tag>
              )}
            </Space>
            <div style={{ marginTop: 6, fontSize: 12, color: '#94a3b8' }}>
              独立同步: {syncTime ? formatFullTime(syncTime) : '尚未单独同步 (点击右侧「从官网获取」)'}
            </div>
          </Col>
          <Col xs={24} md={9}>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
              <Button
                size="small"
                type="primary"
                ghost
                icon={<ThunderboltOutlined />}
                loading={isLogging}
                disabled={!health?.has_credentials}
                onClick={() => handleQuickLogin(providerKey)}
                style={{ width: '100%', justifySelf: 'stretch' }}
              >
                一键免密登录
              </Button>
              <Button
                size="small"
                type="primary"
                icon={<SyncOutlined spin={isSyncing} />}
                loading={isSyncing}
                onClick={() => handleSingleSync(providerKey)}
                style={{ width: '100%' }}
              >
                从官网获取
              </Button>
              <Button
                size="small"
                type="dashed"
                icon={<SafetyCertificateOutlined />}
                loading={vaultSyncing}
                onClick={handleGlobalVaultSync}
                style={{ width: '100%' }}
              >
                密码库提取
              </Button>
              <Button
                size="small"
                icon={<SettingOutlined />}
                onClick={() => {
                  setLoginTargetProvider(providerKey)
                  setLoginModalOpen(true)
                }}
                style={{ width: '100%' }}
              >
                凭据管理
              </Button>
            </div>
          </Col>
        </Row>
      </Card>
    )
  }

  // 厂商快捷登录与凭证录入 Modal
  const [loginModalOpen, setLoginModalOpen] = useState(false)
  const [loginTargetProvider, setLoginTargetProvider] = useState<'akile' | 'lazycat' | 'acck'>('akile')

  // 各厂商独立加载与打卡状态
  const [vendorCheckinLoading, setVendorCheckinLoading] = useState<Record<string, boolean>>({})
  const [vendorCheckinStatus, setVendorCheckinStatus] = useState<Record<string, boolean>>({})
  const [vendorCheckinMsg, setVendorCheckinMsg] = useState<Record<string, string>>({})

  const [vendorMarketLoading, setVendorMarketLoading] = useState<Record<string, boolean>>({})
  const [vendorInstancesLoading, setVendorInstancesLoading] = useState<Record<string, boolean>>({})
  const [vendorStoreLoading, setVendorStoreLoading] = useState<Record<string, boolean>>({})

  // 401 监听
  useEffect(() => {
    setUnauthorizedHandler(() => {
      setAuthed(false)
    })
  }, [])

  // 载入计划列表
  const loadPlans = useCallback(async () => {
    try {
      const res = await api.getPlans()
      setPlans(res.plans || [])
    } catch {
      // ignore
    }
  }, [])

  // 载入调度器与市场统计 (仅本地内存，极速0外部API)
  const loadScheduler = useCallback(async () => {
    try {
      const [sched, stats, times] = await Promise.all([
        api.getSchedulerStatus().catch(() => null),
        api.getMarketStats().catch(() => null),
        api.getRefreshTimes().catch(() => ({})),
      ])
      if (sched) setSchedulerStatus(sched)
      if (stats) setMarketStats(stats)
      if (times) setRefreshTimes(prev => ({ ...prev, ...times }))
    } catch {
      // ignore
    }
  }, [])

  // 全量数据载入 (两阶段非阻塞：秒级渲染主资产与健康态，慢接口后台独立渐进更新)
  const loadingDataRef = useRef(false)
  const loadData = useCallback(async () => {
    if (loadingDataRef.current) return
    loadingDataRef.current = true
    setLoading(true)
    try {
      const [
        instRes, auditRes, unraidRes,
        tokensRes, plansRes, schedRes, statsRes, timesRes,
        vendorHealthRes
      ] = await Promise.all([
        api.getInstances().catch(() => ({ total: 0, instances: [], provider_refresh_times: {}, provider_errors: {}, last_synced_at: undefined, next_scheduled_sync: undefined })),
        api.getAuditLogs({ limit: 30 }).catch(() => ({ total: 0, entries: [] })),
        api.getUnraidStatus().catch(() => null),
        api.getTokens().catch(() => null),
        api.getPlans().catch(() => ({ total: 0, plans: [] })),
        api.getSchedulerStatus().catch(() => null),
        api.getMarketStats().catch(() => null),
        api.getRefreshTimes().catch(() => ({})),
        api.getVendorHealthSummary().catch(() => null),
      ])

      if (vendorHealthRes) {
        setVendorHealth(vendorHealthRes)
        syncVendorCheckinFromHealth(vendorHealthRes)
      }

      setInstances(instRes.instances || [])
      setAuditLogs(auditRes.entries || [])
      setPlans(plansRes.plans || [])
      if (instRes.last_synced_at) {
        setDailySyncInfo({
          last_synced_at: instRes.last_synced_at,
          next_scheduled_sync: instRes.next_scheduled_sync,
        })
      }
      if (instRes.provider_errors) {
        setProviderErrors(prev => ({
          ...prev,
          ...(instRes.provider_errors || {}),
        }))
      }
      if (unraidRes) setUnraidStatus(unraidRes)
      if (tokensRes) {
        setTokensConfig(tokensRes)
        tokensForm.setFieldsValue(tokensRes)
      }
      if (schedRes) setSchedulerStatus(schedRes)
      if (statsRes) setMarketStats(statsRes)

      setRefreshTimes(prev => ({
        ...prev,
        ...timesRes,
        ...(instRes.provider_refresh_times || {}),
      }))
      setLoading(false)
      setInitialLoaded(true)

      // 慢通道异步加载二手集市与官方商城 (绝不阻塞首屏资产与健康渲染)
      void Promise.allSettled([
        api.getMarket().then(marketRes => {
          if (Array.isArray(marketRes.items)) {
            setMarketItems(marketRes.items)
          }
          if (marketRes.provider_errors) {
            setProviderErrors(prev => ({ ...prev, ...(marketRes.provider_errors || {}) }))
          }
          if (marketRes.provider_refresh_times) {
            setRefreshTimes(prev => ({ ...prev, ...marketRes.provider_refresh_times }))
          }
        }),
        api.getStoreProducts().then(storeRes => {
          if (Array.isArray(storeRes.products)) {
            setStoreProducts(storeRes.products)
          }
          if (storeRes.provider_refresh_times) {
            setRefreshTimes(prev => ({ ...prev, ...storeRes.provider_refresh_times }))
          }
        }),
      ]).finally(() => {
        loadingDataRef.current = false
        setMarketLoading(false)
      })
    } catch {
      message.error('加载系统数据失败')
      setLoading(false)
      setInitialLoaded(true)
      setMarketLoading(false)
      loadingDataRef.current = false
    }
  }, [tokensForm, syncVendorCheckinFromHealth])

  // 单厂商二手市场局部刷新
  const handleRefreshMarket = async (vendor: string) => {
    setVendorMarketLoading(prev => ({ ...prev, [vendor]: true }))
    try {
      const res = await api.getMarket(vendor)
      setMarketItems(prev => {
        const others = prev.filter(i => i.provider_id !== vendor)
        return [...others, ...(res.items || [])]
      })
      if (res.provider_errors && res.provider_errors[vendor]) {
        setProviderErrors(prev => ({ ...prev, [vendor]: res.provider_errors![vendor] }))
      } else {
        setProviderErrors(prev => {
          const next = { ...prev }
          delete next[vendor]
          return next
        })
      }
      if (res.provider_refresh_times) {
        setRefreshTimes(prev => ({ ...prev, ...res.provider_refresh_times }))
      }
      message.success(`${vendor.toUpperCase()} 二手市场已更新 (${res.items?.length || 0} 台在售)`)
    } catch {
      message.error(`刷新 ${vendor} 二手市场失败`)
    } finally {
      setVendorMarketLoading(prev => ({ ...prev, [vendor]: false }))
    }
  }

  // 主机资产局部或穿透全量刷新
  const handleRefreshInstances = async (vendor?: string, forceRefresh = false) => {
    const key = vendor || 'all'
    setVendorInstancesLoading(prev => ({ ...prev, [key]: true }))
    try {
      const res = await api.getInstances(vendor, forceRefresh)
      if (vendor) {
        setInstances(prev => {
          const others = prev.filter(i => i.provider_id !== vendor)
          return [...others, ...(res.instances || [])]
        })
      } else {
        setInstances(res.instances || [])
      }
      if (res.last_synced_at) {
        setDailySyncInfo({
          last_synced_at: res.last_synced_at,
          next_scheduled_sync: res.next_scheduled_sync,
        })
      }
      if (vendor) {
        if (res.provider_errors && res.provider_errors[vendor]) {
          setProviderErrors(prev => ({ ...prev, [vendor]: res.provider_errors![vendor] }))
        } else {
          setProviderErrors(prev => {
            const next = { ...prev }
            delete next[vendor]
            return next
          })
        }
      }
      if (res.provider_refresh_times) {
        setRefreshTimes(prev => ({ ...prev, ...res.provider_refresh_times }))
      }
      message.success(`${vendor ? vendor.toUpperCase() : '全网'} 主机资产${forceRefresh ? '穿透全量同步' : '同步'}成功 (${res.instances?.length || 0} 台)`)
    } catch {
      message.error(`同步 ${vendor || '全网'} 资产失败`)
    } finally {
      setVendorInstancesLoading(prev => ({ ...prev, [key]: false }))
    }
  }

  // 单厂商商城规格局部刷新
  const handleRefreshStore = async (vendor: string) => {
    setVendorStoreLoading(prev => ({ ...prev, [vendor]: true }))
    try {
      const res = await api.getStoreProducts(vendor)
      setStoreProducts(prev => {
        const others = prev.filter(i => i.provider_id !== vendor)
        return [...others, ...(res.products || [])]
      })
      if (res.provider_refresh_times) {
        setRefreshTimes(prev => ({ ...prev, ...res.provider_refresh_times }))
      }
      message.success(`${vendor.toUpperCase()} 官方商城已更新`)
    } catch {
      message.error(`刷新 ${vendor} 商城失败`)
    } finally {
      setVendorStoreLoading(prev => ({ ...prev, [vendor]: false }))
    }
  }

  // 保存 API Token
  const handleSaveTokens = async (values: any) => {
    setSavingTokens(true)
    try {
      const res = await api.saveTokens(values)
      message.success(res.message || '凭据保存成功并已热加载生效！')
      setProviderErrors({})
      loadData()
    } catch (e: any) {
      message.error('保存凭据失败: ' + (e.response?.data?.error || e.message))
    } finally {
      setSavingTokens(false)
    }
  }

  // 购买计划操作
  const handleTogglePlan = async (id: string) => {
    try {
      const res = await api.togglePlan(id)
      message.success(`计划「${res.name}」已${res.is_active ? '开启巡检' : '停止'}`)
      loadPlans()
      loadScheduler()
    } catch (e: any) {
      message.error('切换计划状态失败: ' + e.message)
    }
  }

  const handleDeletePlan = async (id: string) => {
    try {
      await api.deletePlan(id)
      message.success('计划已删除')
      loadPlans()
      loadScheduler()
    } catch (e: any) {
      message.error('删除计划失败: ' + e.message)
    }
  }

  const handleSavePlan = async () => {
    try {
      const values = await planForm.validateFields()
      values.max_price = Number(values.max_price) || 0
      values.max_renewal_price = Number(values.max_renewal_price) || 0
      values.min_discount_percent = Number(values.min_discount_percent) || 0
      values.min_port_count = Number(values.min_port_count) || 0
      values.min_bandwidth = Number(values.min_bandwidth) || 0
      values.max_buy_count = Number(values.max_buy_count) || 1

      if (editingPlan) {
        await api.updatePlan(editingPlan.id, values)
        message.success('计划更新成功')
      } else {
        await api.createPlan(values)
        message.success('已新建购买盯盘计划')
      }
      setPlanModalOpen(false)
      setEditingPlan(null)
      planForm.resetFields()
      loadPlans()
      loadScheduler()
    } catch (e: any) {
      if (e.message) message.error('保存计划失败: ' + e.message)
    }
  }

  const handleToggleAutoBuy = async (plan: PurchasePlan) => {
    try {
      const nextVal = !plan.auto_buy
      await api.updatePlan(plan.id, { auto_buy: nextVal })
      message.success(`计划「${plan.name}」已切换为【${nextVal ? '实盘秒杀模式 (自动下单)' : '模拟筛选模式 (安全预览)'}】`)
      loadPlans()
    } catch (e: any) {
      message.error('切换下单模式失败: ' + (e.response?.data?.error || e.message))
    }
  }

  const handleDirectBuy = async (providerId: string, itemId: string, title: string) => {
    try {
      const res = await api.buyMarketItemDirect(providerId, itemId)
      message.success(res.message || `购买请求已提交: ${title}`)
      loadPlans()
    } catch (e: any) {
      message.error('购买失败: ' + (e.response?.data?.error || e.message))
    }
  }

  const handleTriggerBurst = async () => {
    try {
      const res = await api.triggerBurst()
      message.success(res.message || '已开启 5 分钟极速突发巡检模式！')
      loadScheduler()
    } catch (e: any) {
      message.error('触发突发模式失败: ' + e.message)
    }
  }

  // 懒猫云端口查询
  const loadLazyCatPorts = useCallback(async (instId = selectedInstId) => {
    setPortsLoading(true)
    try {
      const res = await api.getLazyCatPorts(instId)
      setPorts(res.ports || [])
    } catch {
      // ignore
    } finally {
      setPortsLoading(false)
    }
  }, [selectedInstId])

  // 单厂商签到
  const handleSingleVendorCheckin = async (vendor: string) => {
    setVendorCheckinLoading(prev => ({ ...prev, [vendor]: true }))
    try {
      const res = await api.checkinProvider(vendor)
      if (res.success) {
        message.success(`${vendor.toUpperCase()} 打卡成功: ${res.message}`)
        setVendorCheckinStatus(prev => ({ ...prev, [vendor]: true }))
        setVendorCheckinMsg(prev => ({ ...prev, [vendor]: res.message }))
      } else {
        message.warning(`${vendor.toUpperCase()} 打卡响应: ${res.message}`)
        setVendorCheckinMsg(prev => ({ ...prev, [vendor]: res.message }))
      }
      api.getAuditLogs({ limit: 30 }).then(r => setAuditLogs(r.entries || []))
    } catch (err: any) {
      message.error(`${vendor.toUpperCase()} 打卡请求失败: ` + (err.response?.data?.message || err.message))
    } finally {
      setVendorCheckinLoading(prev => ({ ...prev, [vendor]: false }))
    }
  }

  // 一键全网签到
  const handleCheckinAll = async () => {
    setCheckinLoading(true)
    try {
      const results = await api.checkinAll()
      let successCount = 0
      let totalCount = 0
      const newStatus = { ...vendorCheckinStatus }
      const newMsg = { ...vendorCheckinMsg }

      Object.entries(results).forEach(([provider, res]) => {
        totalCount++
        newMsg[provider] = res.message
        if (res.success) {
          successCount++
          newStatus[provider] = true
        }
      })
      setVendorCheckinStatus(newStatus)
      setVendorCheckinMsg(newMsg)
      message.success(`签到完成！成功 ${successCount}/${totalCount} 家云厂商`)
      api.getAuditLogs({ limit: 30 }).then(r => setAuditLogs(r.entries || []))
    } catch (e: any) {
      message.error('全网签到失败: ' + e.message)
    } finally {
      setCheckinLoading(false)
    }
  }

  // 初始拉取与调度器状态静默定时拉取 (10秒仅拉取本地内存，0外部API损耗)
  useEffect(() => {
    if (authed) {
      loadData()
      const timer = setInterval(() => {
        loadScheduler()
        loadVendorHealth()
      }, 10000)
      return () => clearInterval(timer)
    }
  }, [authed, loadData, loadScheduler, loadVendorHealth])

  if (!authed) {
    return (
      <ConfigProvider locale={zhCN} theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#2563eb',
          colorBgBase: '#ffffff',
          colorTextBase: '#0f172a',
          borderRadius: 8,
          fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
        }
      }}>
        <LoginForm onLoginSuccess={() => {
          setAuthed(true)
        }} />
      </ConfigProvider>
    )
  }

  // 快速通过二手集市创建盯盘规则
  const handleCreatePlanFromMarket = (record: MarketItem) => {
    planForm.setFieldsValue({
      name: `${record.provider_id.toUpperCase()} ${record.name} 限价盯盘`,
      provider: record.provider_id,
      market_type: 'secondhand',
      max_price: record.price,
      region_keyword: record.location,
      safe_mode: true,
      auto_buy: false,
      is_active: true
    })
    setEditingPlan(null)
    setPlanModalOpen(true)
  }

  // VPS 详情 Drawer
  const renderVpsDetailDrawer = () => {
    if (!selectedVps) return null
    return (
      <Drawer
        title={
          <Space>
            <DesktopOutlined style={{ color: '#2563eb' }} />
            <span>主机资产详情 / {selectedVps.name}</span>
          </Space>
        }
        width={560}
        open={detailDrawerOpen}
        onClose={() => setDetailDrawerOpen(false)}
      >
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <Card size="small" title="基础运行状态" bordered={false} style={{ background: '#f8fafc', borderRadius: 8 }}>
            <Row gutter={[16, 12]}>
              <Col span={12}>
                <Text type="secondary">主机标识 ID:</Text>
                <div style={{ fontWeight: 600 }}>{selectedVps.id}</div>
              </Col>
              <Col span={12}>
                <Text type="secondary">所属云厂商:</Text>
                <div>
                  <Tag color="blue">{selectedVps.provider_id.toUpperCase()}</Tag>
                  <Tag color="default">{selectedVps.category || '云主机'}</Tag>
                </div>
              </Col>
              <Col span={12}>
                <Text type="secondary">机房区域:</Text>
                <div style={{ fontWeight: 600 }}>{selectedVps.region || '未知'}</div>
              </Col>
              <Col span={12}>
                <Text type="secondary">网络 IP 地址:</Text>
                <div style={{ fontFamily: 'monospace', fontWeight: 600, color: '#2563eb' }}>{selectedVps.ip || 'NAT 共享出口'}</div>
              </Col>
            </Row>
          </Card>

          {/* 网络地址分配 (IPv4 / IPv6) */}
          <Card size="small" title="网络与 IP 地址分配" bordered={false} style={{ background: '#f8fafc', borderRadius: 8 }}>
            <Row gutter={[16, 12]}>
              <Col span={12}>
                <Text type="secondary">IPv4 公网地址:</Text>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 4 }}>
                  <span style={{ fontFamily: 'monospace', fontWeight: 600, color: '#2563eb' }}>
                    {selectedVps.ipv4 || selectedVps.ip || 'NAT 共享出口'}
                  </span>
                  {(selectedVps.ipv4 || selectedVps.ip) && (
                    <Button
                      size="small"
                      type="text"
                      icon={<CopyOutlined />}
                      onClick={() => {
                        navigator.clipboard.writeText(selectedVps.ipv4 || selectedVps.ip)
                        message.success('IPv4 已复制到剪贴板')
                      }}
                    />
                  )}
                </div>
              </Col>
              <Col span={12}>
                <Text type="secondary">IPv6 公网地址:</Text>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 4 }}>
                  <span style={{ fontFamily: 'monospace', fontWeight: 600, color: selectedVps.ipv6 ? '#7c3aed' : '#94a3b8' }}>
                    {selectedVps.ipv6 || '未分配 IPv6'}
                  </span>
                  {selectedVps.ipv6 && (
                    <Button
                      size="small"
                      type="text"
                      icon={<CopyOutlined />}
                      onClick={() => {
                        navigator.clipboard.writeText(selectedVps.ipv6!)
                        message.success('IPv6 已复制到剪贴板')
                      }}
                    />
                  )}
                </div>
              </Col>
            </Row>
          </Card>

          {/* SSH / Root 凭据与远程管理 */}
          <Card size="small" title="SSH / Root 远程登录凭据" bordered={false} style={{ background: '#f8fafc', borderRadius: 8 }}>
            <Row gutter={[16, 12]}>
              <Col span={8}>
                <Text type="secondary">登录用户名:</Text>
                <div style={{ fontFamily: 'monospace', fontWeight: 600 }}>{selectedVps.ssh_user || 'root'}</div>
              </Col>
              <Col span={8}>
                <Text type="secondary">SSH 端口:</Text>
                <div style={{ fontFamily: 'monospace', fontWeight: 600 }}>
                  {selectedVps.provider_id === 'lazycat' && !selectedVps.ssh_port
                    ? <Tag color="purple">未映射 22 (走 WebShell)</Tag>
                    : (selectedVps.ssh_port || 22)}
                </div>
              </Col>
              <Col span={8}>
                <Text type="secondary">初始 Root 密码:</Text>
                <div style={{ display: 'flex', alignItems: 'center', gap: 4, marginTop: 2 }}>
                  {selectedVps.provider_id === 'lazycat' && !selectedVps.ssh_port ? (
                    <Tag color="purple">WebShell 免密直通</Tag>
                  ) : credentialVisible && credentialValue ? (
                    <>
                      <Text code style={{ fontFamily: 'monospace', fontSize: 12 }}>{credentialValue}</Text>
                      <Button
                        size="small"
                        type="text"
                        icon={<CopyOutlined />}
                        onClick={() => {
                          navigator.clipboard.writeText(credentialValue)
                          message.success('Root 密码已复制')
                        }}
                      />
                    </>
                  ) : (
                    <>
                      <Tag style={{ fontFamily: 'monospace', margin: 0 }}>••••••••</Tag>
                      <Button size="small" type="text" loading={credentialLoading} onClick={() => fetchCredential(selectedVps.id)}>
                        取用
                      </Button>
                    </>
                  )}
                </div>
              </Col>
              {(selectedVps.ipv4 || selectedVps.ip) && (
                <Col span={24}>
                  <Text type="secondary">一键终端连接命令:</Text>
                  {selectedVps.provider_id === 'lazycat' && !selectedVps.ssh_port ? (
                    <div style={{ fontSize: 12, color: '#64748b', marginTop: 4 }}>
                      当前 NAT 容器未映射 22 端口，请点击「管理 / 终端」使用 WebShell PTY 免密进入；如需外部 SSH，请在「NAT 端口映射」中新增私有端口 22。
                    </div>
                  ) : (
                    <div style={{
                      background: '#1e293b',
                      color: '#38bdf8',
                      padding: '6px 12px',
                      borderRadius: 6,
                      fontFamily: 'monospace',
                      fontSize: 12,
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      marginTop: 4
                    }}>
                      <span>ssh -p {selectedVps.ssh_port || 22} {selectedVps.ssh_user || 'root'}@{selectedVps.ipv4 || selectedVps.ip}</span>
                      <Button
                        size="small"
                        type="link"
                        style={{ color: '#38bdf8', padding: 0 }}
                        onClick={() => {
                          const cmd = `ssh -p ${selectedVps.ssh_port || 22} ${selectedVps.ssh_user || 'root'}@${selectedVps.ipv4 || selectedVps.ip}`
                          navigator.clipboard.writeText(cmd)
                          message.success('SSH 命令已复制')
                        }}
                      >
                        复制命令
                      </Button>
                    </div>
                  )}
                </Col>
              )}
            </Row>
          </Card>

          <Card size="small" title="硬件规格详情" bordered={false} style={{ background: '#f8fafc', borderRadius: 8 }}>
            <Row gutter={[16, 12]}>
              <Col span={8}>
                <Statistic title="处理器核心" value={selectedVps.cpu || '1 核'} valueStyle={{ fontSize: 16, fontWeight: 600 }} />
              </Col>
              <Col span={8}>
                <Statistic title="内存容量" value={selectedVps.memory || '1024 MB'} valueStyle={{ fontSize: 16, fontWeight: 600 }} />
              </Col>
              <Col span={8}>
                <Statistic title="网络端口" value={`${selectedVps.port_count || 0} / ${selectedVps.port_limit || 20}`} valueStyle={{ fontSize: 16, fontWeight: 600 }} />
              </Col>
            </Row>
          </Card>

          <Card size="small" title="流量与计费结算周期" bordered={false} style={{ background: '#f8fafc', borderRadius: 8 }}>
            {selectedVps.is_stale && (
              <Alert
                type="warning"
                showIcon
                message="当前资产处于离线缓存状态"
                description={
                  <div>
                    <div style={{ fontSize: 12 }}>失败原因: {selectedVps.stale_reason || '供应商 Token 已过期或未配置'}</div>
                    <Button
                      type="primary"
                      size="small"
                      style={{ marginTop: 8 }}
                      onClick={() => {
                        setDetailDrawerOpen(false)
                        setLoginTargetProvider(selectedVps.provider_id)
                        setLoginModalOpen(true)
                      }}
                    >
                      立即重新登录 {selectedVps.provider_id.toUpperCase()}
                    </Button>
                  </div>
                }
                style={{ marginBottom: 12, borderRadius: 8 }}
              />
            )}
            <div style={{ marginBottom: 12 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
                <Text type="secondary">本月流量使用情况</Text>
                <Text strong>
                  {selectedVps.traffic_used || '0 GB'} / {selectedVps.traffic_total || '1000 GB'}
                  <span style={{ marginLeft: 6, color: getTrafficColor(getTrafficPercent(selectedVps)) }}>
                    ({getTrafficPercent(selectedVps)}%)
                  </span>
                </Text>
              </div>
              <Progress
                percent={getTrafficPercent(selectedVps)}
                strokeColor={getTrafficColor(getTrafficPercent(selectedVps))}
                showInfo={false}
              />
            </div>
            <Row gutter={[16, 12]}>
              <Col span={12}>
                <Text type="secondary">续费周期与金额:</Text>
                <div style={{ marginTop: 4 }}>
                  {renderRenewalDisplay(selectedVps)}
                </div>
              </Col>
              <Col span={12}>
                <Text type="secondary">平台账户余额:</Text>
                <div style={{ fontWeight: 600, color: '#059669', marginTop: 4, fontSize: 15 }}>
                  {selectedVps.user_balance !== undefined && selectedVps.user_balance > 0 ? `¥${selectedVps.user_balance.toFixed(2)}` : '正常状态'}
                </div>
              </Col>
              <Col span={12}>
                <Text type="secondary">流量配额重置日:</Text>
                <div style={{ fontWeight: 600, color: '#2563eb', marginTop: 4 }}>
                  {selectedVps.traffic_reset_date || '自然月月末或账单日重置'}
                </div>
              </Col>
              <Col span={12}>
                <Text type="secondary">流量重置费用:</Text>
                <div style={{ fontWeight: 600, color: '#d97706', marginTop: 4 }}>
                  {selectedVps.traffic_reset_price !== undefined && selectedVps.traffic_reset_price > 0 ? `¥${selectedVps.traffic_reset_price.toFixed(2)}/次` : '标准资费'}
                </div>
              </Col>
              <Col span={12}>
                <Text type="secondary">自动重置流量状态:</Text>
                <div style={{ marginTop: 4 }}>
                  {selectedVps.auto_reset ? <Tag color="green">已开启自动重置</Tag> : <Tag color="default">按需手动重置</Tag>}
                </div>
              </Col>
              <Col span={12}>
                <Text type="secondary">Root 初始/管理密码:</Text>
                <div style={{ marginTop: 4, display: 'flex', alignItems: 'center', gap: 6 }}>
                  {credentialVisible && credentialValue ? (
                    <Text code copyable style={{ fontFamily: 'monospace' }}>{credentialValue}</Text>
                  ) : (
                    <Space size={6}>
                      <Tag style={{ fontFamily: 'monospace', margin: 0 }}>••••••••</Tag>
                      <Button size="small" type="link" loading={credentialLoading} onClick={() => fetchCredential(selectedVps.id)}>
                        按需取用
                      </Button>
                    </Space>
                  )}
                </div>
              </Col>
              <Col span={12}>
                <Text type="secondary">资产到期时间:</Text>
                <div style={{ fontWeight: 600, color: '#d97706', marginTop: 4 }}>
                  {formatExpireDate(selectedVps.expire_date)}
                </div>
              </Col>
              <Col span={12}>
                <Text type="secondary">快照同步时间:</Text>
                <div style={{ fontSize: 12, color: '#64748b', marginTop: 4 }}>
                  {selectedVps.last_synced_at || dailySyncInfo.last_synced_at || '每日 04:00 自动更新'}
                </div>
              </Col>
            </Row>
          </Card>

          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 12, marginTop: 12 }}>
            <Button onClick={() => setDetailDrawerOpen(false)}>关闭详情</Button>
            <Button type="primary" onClick={() => {
              setDetailDrawerOpen(false)
              setPage('plans')
            }}>
              为该机型创建盯盘计划
            </Button>
          </div>
        </Space>
      </Drawer>
    )
  }

  // 页面标题映射
  const pageTitleMap: Record<string, string> = {
    overview: '全网资产看板',
    plans: '智能抢购中心 (1 分钟轮询)',
    akile: 'Akile Cloud 专区',
    acck: 'ACCK 专区',
    lazycat: '懒猫云专区',
    audit: '安全审计中心',
    settings: '系统与凭据配置',
  }

  const pageSubtitleMap: Record<string, string> = {
    overview: '全网 VPS · NAT 容器 · 资产全景矩阵',
    plans: '支持懒猫云 / Akile / ACCK 一手与二手集市；默认模拟筛选安全不扣款，人工核实后开启实盘秒杀',
    akile: '香港 / 美西 / 亚太高性能直连 VPS 资产与集市',
    acck: '三网优化与高带宽二手机器交易流转',
    lazycat: '国内家宽出口 NAT 容器与动态端口映射',
    audit: '过去 30 天自动化与安全审计日志',
    settings: '厂商 API Token、网络代理与安全设置',
  }

  // 渲染主内容区域
  const renderContent = () => {
    switch (page) {
      case 'overview': {
        const counts = {
          lazycat: instances.filter(i => i.provider_id === 'lazycat').length,
          akile: instances.filter(i => i.provider_id === 'akile').length,
          acck: instances.filter(i => i.provider_id === 'acck').length,
        }
        return (
          <Space direction="vertical" size={20} style={{ width: '100%' }}>
            {/* 顶栏宏观概览卡片 */}
            <Row gutter={[16, 16]}>
              <Col xs={24} sm={12} md={6}>
                <Card bordered={false} className="vps-card" style={{ height: '100%' }}>
                  <Statistic
                    title={<span style={{ color: '#64748b' }}>全网纳管总资产</span>}
                    value={instances.length}
                    suffix={<span style={{ fontSize: 13, color: '#94a3b8' }}>台主机/容器</span>}
                    prefix={<CloudServerOutlined style={{ color: '#2563eb', marginRight: 8 }} />}
                    valueStyle={{ fontWeight: 700, color: '#0f172a' }}
                  />
                  <div style={{ marginTop: 8, fontSize: 12, color: '#64748b' }}>
                    懒猫云 {counts.lazycat} · Akile {counts.akile} · ACCK {counts.acck}
                  </div>
                </Card>
              </Col>

              <Col xs={24} sm={12} md={6}>
                <Card bordered={false} className="vps-card" style={{ height: '100%' }}>
                  <Statistic
                    title={<span style={{ color: '#64748b' }}>全网二手流转挂单</span>}
                    value={marketLoading ? '—' : marketItems.length}
                    suffix={<span style={{ fontSize: 13, color: '#94a3b8' }}>{marketLoading ? '同步中' : '台在售'}</span>}
                    prefix={<SwapOutlined style={{ color: '#059669', marginRight: 8 }} />}
                    valueStyle={{ fontWeight: 700, color: '#0f172a' }}
                  />
                  <div style={{ marginTop: 8, fontSize: 12, color: '#64748b' }}>
                    {marketLoading
                      ? '正在拉取懒猫云 / Akile / ACCK 三家集市…'
                      : `懒猫云 ${marketItems.filter(i => i.provider_id === 'lazycat').length} · Akile ${marketItems.filter(i => i.provider_id === 'akile').length} · ACCK ${marketItems.filter(i => i.provider_id === 'acck').length}`}
                  </div>
                </Card>
              </Col>

              <Col xs={24} sm={12} md={6}>
                <Card bordered={false} className="vps-card" style={{ height: '100%' }}>
                  <Statistic
                    title={<span style={{ color: '#64748b' }}>活跃购买盯盘计划</span>}
                    value={plans.filter(p => p.is_active).length}
                    suffix={<span style={{ fontSize: 13, color: '#94a3b8' }}>/ {plans.length} 条</span>}
                    prefix={<AimOutlined style={{ color: '#2563eb', marginRight: 8 }} />}
                    valueStyle={{ fontWeight: 700, color: '#0f172a' }}
                  />
                  <div style={{ marginTop: 8, fontSize: 12, color: '#64748b' }}>
                    模式: {schedulerStatus?.mode === 'idle' ? '休眠节能' : schedulerStatus?.mode === 'burst' ? '极速突发' : '常态巡检 (30s)'}
                  </div>
                </Card>
              </Col>

              <Col xs={24} sm={12} md={6}>
                <Card bordered={false} className="vps-card">
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 14, justifyContent: 'space-between' }}>
                    <span style={{ color: '#64748b', fontSize: 14, lineHeight: '22px' }}>自动化操作中心</span>
                    <Button
                      type="primary"
                      icon={<CheckCircleOutlined />}
                      loading={checkinLoading}
                      onClick={handleCheckinAll}
                      style={{ background: '#059669', borderColor: '#059669', fontWeight: 600, height: 38 }}
                    >
                      一键全网签到
                    </Button>
                    <div style={{ fontSize: 12, color: '#64748b' }}>
                      每日 08:00 自动签到 · 覆盖懒猫云 / Akile / ACCK
                    </div>
                  </div>
                </Card>
              </Col>
            </Row>

            {!loading && instances.length === 0 && (
              <Alert
                message="尚未同步云厂商 API 资产"
                description={
                  <div>
                    系统检测到尚未配置或已过期 Akile、ACCK 或懒猫云的 API Token，因此暂未同步名下主机资产。
                    您可点击下方按钮一键快捷登录或前往<b>「系统与凭据配置」</b>管理凭据，保存后系统将自动秒级热重载拉取。
                    <div style={{ marginTop: 10 }}>
                      <Space wrap>
                        <Button
                          type="primary"
                          size="small"
                          icon={<LoginOutlined />}
                          onClick={() => { setLoginTargetProvider('akile'); setLoginModalOpen(true); }}
                        >
                          登录 Akile (支持2FA/自动续期)
                        </Button>
                        <Button
                          type="primary"
                          size="small"
                          icon={<LoginOutlined />}
                          style={{ background: '#059669', borderColor: '#059669' }}
                          onClick={() => { setLoginTargetProvider('lazycat'); setLoginModalOpen(true); }}
                        >
                          登录 懒猫云 (免密/2FA)
                        </Button>
                        <Button
                          size="small"
                          icon={<LoginOutlined />}
                          onClick={() => { setLoginTargetProvider('acck'); setLoginModalOpen(true); }}
                        >
                          登录 ACCK (免密/2FA)
                        </Button>
                        <Button size="small" onClick={() => setPage('settings')}>
                          凭据与代理高级设置
                        </Button>
                      </Space>
                    </div>
                  </div>
                }
                type="info"
                showIcon
                style={{ borderRadius: 8, borderColor: '#bfdbfe', background: '#eff6ff' }}
              />
            )}

            {/* 统一宽表 */}
            <Card
              title={
                <Space>
                  <DesktopOutlined style={{ color: '#2563eb' }} />
                  <Text strong>全网 VPS / NAT 容器资产全景矩阵</Text>
                </Space>
              }
              extra={
                <Space size={12}>
                  <Text type="secondary" style={{ fontSize: 13 }}>
                    <ClockCircleOutlined style={{ marginRight: 4 }} />
                    快照时间: {dailySyncInfo.last_synced_at || formatTime(refreshTimes['instances:all']) || '每日 04:00 更新'}
                  </Text>
                  <Button size="small" type="primary" ghost icon={<ReloadOutlined spin={loading} />} onClick={() => handleRefreshInstances(undefined, true)}>
                    穿透全量刷新
                  </Button>
                </Space>
              }
              bordered={false}
              className="vps-card"
            >
              {instances.some(i => i.is_stale) && (
                <Alert
                  type="warning"
                  showIcon
                  message="部分云厂商凭据失效，正在展示离线持久化快照数据"
                  description={
                    <Space wrap style={{ marginTop: 4 }}>
                      {Array.from(new Set(instances.filter(i => i.is_stale).map(i => i.provider_id))).map(pid => (
                        <Button
                          key={pid}
                          size="small"
                          type="primary"
                          danger
                          onClick={() => {
                            setLoginTargetProvider(pid as any)
                            setLoginModalOpen(true)
                          }}
                        >
                          立即重新登录 {pid.toUpperCase()}
                        </Button>
                      ))}
                    </Space>
                  }
                  style={{ marginBottom: 16, borderRadius: 8 }}
                />
              )}
              <Table
                dataSource={instances}
                rowKey="id"
                loading={loading || !initialLoaded}
                locale={{ emptyText: initialLoaded ? undefined : ' ' }}
                pagination={{
                  pageSize: 10,
                  showTotal: (total) => `共 ${total} 台全网资产 (动态同步)`,
                  showSizeChanger: true
                }}
                size="middle"
                onRow={(record) => ({
                  onClick: () => {
                    setSelectedVps(record)
                    setDetailDrawerOpen(true)
                  },
                  style: { cursor: 'pointer' }
                })}
                columns={[
                  {
                    title: '所属厂商',
                    dataIndex: 'provider_id',
                    key: 'provider',
                    width: 110,
                    render: (pid: string) => {
                      const colors: Record<string, string> = { lazycat: 'purple', akile: 'blue', acck: 'cyan' }
                      return <Tag color={colors[pid] || 'default'} style={{ fontWeight: 600 }}>{pid.toUpperCase()}</Tag>
                    }
                  },
                  {
                    title: '实例名称 / ID',
                    key: 'name',
                    render: (_, r) => (
                      <div>
                        <div style={{ fontWeight: 600, color: '#0f172a', display: 'flex', alignItems: 'center', gap: 6 }}>
                          <span>{r.name}</span>
                          {r.is_stale && (
                            <Tooltip title={`离线快照: ${r.stale_reason || 'Token 失效需重新登录'}`}>
                              <Tag color="warning" style={{ fontSize: 11, cursor: 'pointer', margin: 0 }} onClick={(e) => {
                                e.stopPropagation()
                                setLoginTargetProvider(r.provider_id)
                                setLoginModalOpen(true)
                              }}>
                                缓存
                              </Tag>
                            </Tooltip>
                          )}
                        </div>
                        <div style={{ fontSize: 12, color: '#94a3b8' }}>ID: {r.id}</div>
                      </div>
                    )
                  },
                  {
                    title: '机房区域',
                    dataIndex: 'region',
                    key: 'region',
                    width: 120,
                    render: (reg) => <Tag color="geekblue">{reg || '默认机房'}</Tag>
                  },
                  {
                    title: '网络 IP (v4/v6)',
                    key: 'ip',
                    width: 200,
                    render: (_, r) => (
                      <div style={{ fontFamily: 'monospace', fontSize: 12 }}>
                        <div><Tag color="blue" style={{ fontSize: 11, padding: '0 4px', marginRight: 4 }}>v4</Tag>{r.ipv4 || r.ip || 'NAT出口'}</div>
                        {r.ipv6 && (
                          <div style={{ marginTop: 2, color: '#7c3aed', display: 'flex', alignItems: 'center', gap: 4, whiteSpace: 'nowrap' }}>
                            <Tag color="purple" style={{ fontSize: 11, padding: '0 4px', margin: 0, flexShrink: 0 }}>v6</Tag>
                            <span style={{ fontSize: 11 }}>{r.ipv6.length > 18 ? r.ipv6.substring(0, 16) + '...' : r.ipv6}</span>
                          </div>
                        )}
                      </div>
                    )
                  },
                  {
                    title: '配置规格',
                    key: 'spec',
                    width: 140,
                    render: (_, r) => (
                      <span style={{ fontSize: 12, color: '#64748b' }}>
                        {r.cpu ? `${r.cpu} / ${r.memory}` : (r.provider_id === 'lazycat' ? '家宽容器' : '标准规格')}
                      </span>
                    )
                  },
                  {
                    title: '月度流量 & 重置资费',
                    key: 'traffic',
                    width: 180,
                    render: (_, r) => {
                      const pct = getTrafficPercent(r)
                      const color = getTrafficColor(pct)
                      return (
                        <div>
                          <div style={{ fontSize: 12, display: 'flex', justifyContent: 'space-between', gap: 6, marginBottom: 2, whiteSpace: 'nowrap' }}>
                            <span style={{ fontWeight: 600 }}>{r.traffic_used || '0 GB'}</span>
                            <span style={{ color: '#64748b' }}>{r.traffic_total || '1000 GB'} ({pct}%)</span>
                          </div>
                          <Progress percent={pct} size="small" showInfo={false} strokeColor={color} />
                          <div style={{ fontSize: 10, color: '#94a3b8', marginTop: 3, display: 'flex', justifyContent: 'space-between', gap: 4 }}>
                            {r.traffic_reset_date ? <span>重置: {r.traffic_reset_date.split(' ')[0]}</span> : <span>月末重置</span>}
                            {r.traffic_reset_price ? <span style={{ color: '#d97706', fontWeight: 600 }}>包: ¥{r.traffic_reset_price.toFixed(2)}</span> : null}
                          </div>
                        </div>
                      )
                    }
                  },
                  {
                    title: '到期 / 续费 / 余额',
                    key: 'expire',
                    width: 185,
                    render: (_, r) => (
                      <div>
                        {renderRenewalDisplay(r)}
                        <div style={{ fontSize: 11, color: '#94a3b8', marginTop: 2, whiteSpace: 'nowrap' }}>{formatExpireDate(r.expire_date)}</div>
                        {r.user_balance !== undefined && r.user_balance > 0 ? (
                          <div style={{ fontSize: 10, color: '#2563eb', marginTop: 1 }}>余额: ¥{r.user_balance.toFixed(2)}</div>
                        ) : null}
                      </div>
                    )
                  },
                  {
                    title: '操作',
                    key: 'action',
                    width: 150,
                    render: (_, r) => (
                      <Space size={4}>
                        {r.provider_id === 'lazycat' && (
                          <Button
                            type="primary"
                            size="small"
                            icon={<CodeOutlined />}
                            style={{ background: '#7c3aed', borderColor: '#7c3aed' }}
                            onClick={(e) => {
                              e.stopPropagation()
                              setLazycatTargetInstance(r)
                              setLazycatModalOpen(true)
                            }}
                          >
                            管理 / 终端
                          </Button>
                        )}
                        <Button
                          type="link"
                          size="small"
                          icon={<EyeOutlined />}
                          onClick={(e) => {
                            e.stopPropagation()
                            setSelectedVps(r)
                            setDetailDrawerOpen(true)
                          }}
                        >
                          详情
                        </Button>
                      </Space>
                    )
                  }
                ]}
              />
            </Card>
          </Space>
        )
      }

      case 'plans': {
        const activeCount = plans.filter(p => p.is_active).length
        const totalMatchedItems = plans.reduce((acc, p) => acc + (p.matched_items?.length || 0), 0)
        const lazycatPlans = plans.filter(p => p.provider === 'lazycat')
        const akilePlans = plans.filter(p => p.provider === 'akile')
        const acckPlans = plans.filter(p => p.provider === 'acck')

        // 收集并去重各平台当前命中的候选挂单
        const extractUniqueCandidates = (providerFilter?: string) => {
          const map = new Map<string, MatchedItemPreview>()
          plans.forEach(plan => {
            if (!providerFilter || providerFilter === 'all' || plan.provider === providerFilter) {
              (plan.matched_items || []).forEach(it => {
                const key = `${it.provider}-${it.id}`
                if (!map.has(key)) map.set(key, it)
              })
            }
          })
          return Array.from(map.values())
        }

        const allMatched = extractUniqueCandidates('all')
        const lazycatMatched = extractUniqueCandidates('lazycat')
        const akileMatched = extractUniqueCandidates('akile')
        const acckMatched = extractUniqueCandidates('acck')

        const handleOpenCreatePlan = (provider: 'all' | 'lazycat' | 'akile' | 'acck' = 'all', presetValues?: Partial<PurchasePlan>) => {
          setEditingPlan(null)
          planForm.resetFields()
          planForm.setFieldsValue({
            provider: provider,
            market_type: 'secondhand',
            safe_mode: true,
            auto_buy: false,
            max_buy_count: 1,
            is_active: true,
            max_price: 50,
            max_renewal_price: 0,
            cycle_filter: 'all',
            min_discount_percent: 0,
            min_port_count: 0,
            min_bandwidth: 0,
            region_keyword: '',
            keyword_filter: '',
            ...presetValues,
          })
          setPlanModalOpen(true)
        }

        // 针对不同服务商个性化渲染候选挂单表格
        const renderMatchedTable = (items: MatchedItemPreview[], providerType: string) => {
          if (!items || items.length === 0) {
            return (
              <div style={{ padding: '16px 20px', color: '#94a3b8', fontStyle: 'italic', background: '#fafafa', borderRadius: 6, textAlign: 'center' }}>
                暂无符合条件的实时候选挂单（抢购引擎每 1 分钟自动扫描匹配二手/一手商城）
              </div>
            )
          }

          if (providerType === 'lazycat') {
            return (
              <Table
                size="small"
                dataSource={items}
                rowKey={(it) => `lz-${it.id}`}
                pagination={false}
                columns={[
                  {
                    title: '挂单名称 / 节点',
                    key: 'title',
                    render: (_, it) => (
                      <div>
                        <div style={{ fontWeight: 600, color: '#0f172a' }}>{it.title}</div>
                        <div style={{ fontSize: 11, color: '#64748b' }}>
                          {it.node_name && <Tag color="purple" style={{ fontSize: 10, padding: '0 4px', marginRight: 4 }}>{it.node_name}</Tag>}
                          {it.location && <span>{it.location}</span>}
                        </div>
                      </div>
                    )
                  },
                  {
                    title: '卖家信息',
                    key: 'seller',
                    width: 130,
                    render: (_, it) => (
                      <div style={{ fontSize: 12 }}>
                        {it.seller_username ? (
                          <div>
                            <span style={{ color: '#0f172a', fontWeight: 500 }}>{it.seller_username}</span>
                            {it.seller_user_id ? <div style={{ fontSize: 11, color: '#94a3b8' }}>UID: {it.seller_user_id}</div> : null}
                          </div>
                        ) : <span style={{ color: '#94a3b8' }}>官方直营</span>}
                      </div>
                    )
                  },
                  {
                    title: '售价 / 续费 / 折扣',
                    key: 'price',
                    width: 175,
                    render: (_, it) => (
                      <div>
                        <div style={{ fontWeight: 700, color: '#ef4444', fontSize: 14 }}>¥{it.price}</div>
                        <div style={{ marginTop: 2 }}>
                          {renderRenewalDisplay(it)}
                        </div>
                      </div>
                    )
                  },
                  {
                    title: 'NAT 端口配额',
                    dataIndex: 'port_limit',
                    key: 'port_limit',
                    width: 110,
                    render: (pl?: number) => <Tag color="purple">{pl ? `${pl} 个端口` : '10 个端口'}</Tag>
                  },
                  {
                    title: '保证率 / SLA',
                    key: 'sla',
                    width: 120,
                    render: (_, it) => <Tag color="green">{it.sla || '99.9%'}</Tag>
                  },
                  {
                    title: '命中时间',
                    dataIndex: 'matched_at',
                    key: 'matched_at',
                    width: 110,
                    render: (t: string) => <span style={{ fontSize: 11, color: '#94a3b8' }}>{formatTime(t)}</span>
                  },
                  {
                    title: '操作',
                    key: 'act',
                    width: 100,
                    render: (_, it) => (
                      <Popconfirm
                        title={`确认以 ¥${it.price} 购买【${it.title}】？`}
                        description="将调用懒猫云 API 从账户余额扣款购买！"
                        onConfirm={() => handleDirectBuy('lazycat', it.id, it.title)}
                        okText="立即购买"
                        cancelText="取消"
                        okButtonProps={{ danger: true }}
                      >
                        <Button size="small" type="primary" danger icon={<ThunderboltOutlined />}>
                          购买
                        </Button>
                      </Popconfirm>
                    )
                  }
                ]}
              />
            )
          }

          if (providerType === 'akile') {
            return (
              <Table
                size="small"
                dataSource={items}
                rowKey={(it) => `ak-${it.id}`}
                pagination={false}
                columns={[
                  {
                    title: '机器型号 / 节点',
                    key: 'title',
                    render: (_, it) => (
                      <div>
                        <div style={{ fontWeight: 600, color: '#0f172a' }}>{it.title}</div>
                        <div style={{ fontSize: 11, color: '#64748b' }}>
                          {it.node_name && <Tag color="blue" style={{ fontSize: 10, padding: '0 4px', marginRight: 4 }}>{it.node_name}</Tag>}
                          {it.location && <Tag color="geekblue" style={{ fontSize: 10, padding: '0 4px' }}>{it.location}</Tag>}
                        </div>
                      </div>
                    )
                  },
                  {
                    title: '转让标价',
                    dataIndex: 'price',
                    key: 'price',
                    width: 110,
                    render: (p: number) => <span style={{ fontWeight: 700, color: '#ef4444', fontSize: 14 }}>¥{p.toFixed(2)}</span>
                  },
                  {
                    title: '续费周期 / 价格',
                    key: 'renewal_price',
                    width: 145,
                    render: (_, it) => renderRenewalDisplay(it)
                  },
                  {
                    title: '流量重置费',
                    key: 'reset_price',
                    width: 120,
                    render: (_, it) => (
                      <div>
                        <span style={{ color: '#d97706', fontWeight: 600 }}>¥{it.reset_price ? it.reset_price.toFixed(2) : '20.00'}/次</span>
                        {it.flow_used ? <div style={{ fontSize: 10, color: '#64748b' }}>已用: {it.flow_used}</div> : null}
                      </div>
                    )
                  },
                  {
                    title: 'IP 状态与三网测速',
                    key: 'ip_status',
                    width: 190,
                    render: (_, it) => {
                      const isNormal = it.detail?.includes('正常')
                      const isBlocked = it.detail?.includes('被墙')
                      return (
                        <div>
                          <div style={{ marginBottom: 2 }}>
                            {isNormal ? (
                              <Tag color="green" style={{ fontWeight: 600 }}>IP正常</Tag>
                            ) : isBlocked ? (
                              <Tag color="red" style={{ fontWeight: 600 }}>IP异常</Tag>
                            ) : (
                              <Tag color="default">{it.detail || '未检测'}</Tag>
                            )}
                          </div>
                          {it.ip_check_detail ? (
                            <Tooltip title={<pre style={{ margin: 0, fontSize: 11 }}>{it.ip_check_detail}</pre>}>
                              <span style={{ fontSize: 11, color: '#64748b', cursor: 'pointer', textDecoration: 'underline dotted' }}>
                                三网: {it.ip_check_detail.split('\n')[1] || '查看测速'}
                              </span>
                            </Tooltip>
                          ) : null}
                        </div>
                      )
                    }
                  },
                  {
                    title: '网络与硬件',
                    key: 'specs',
                    render: (_, it) => (
                      <div style={{ fontSize: 11, color: '#475569' }}>
                        <div>{it.config_desc || '-'}</div>
                        {it.bandwidth && <div>带宽: <b style={{ color: '#059669' }}>{it.bandwidth}</b></div>}
                      </div>
                    )
                  },
                  {
                    title: '到期时间',
                    dataIndex: 'due_time',
                    key: 'due_time',
                    width: 120,
                    render: (dt?: string) => <span style={{ fontSize: 11, color: '#64748b' }}>{dt || '-'}</span>
                  },
                  {
                    title: '命中时间',
                    dataIndex: 'matched_at',
                    key: 'matched_at',
                    width: 100,
                    render: (t: string) => <span style={{ fontSize: 11, color: '#94a3b8' }}>{formatTime(t)}</span>
                  },
                  {
                    title: '操作',
                    key: 'act',
                    width: 90,
                    render: (_, it) => (
                      <Popconfirm
                        title={`确认以 ¥${it.price.toFixed(2)} 秒杀【${it.title}】？`}
                        description="将调用 Akile API 从账户余额扣款购买！"
                        onConfirm={() => handleDirectBuy('akile', it.id, it.title)}
                        okText="秒杀购买"
                        cancelText="取消"
                        okButtonProps={{ danger: true }}
                      >
                        <Button size="small" type="primary" danger icon={<ThunderboltOutlined />}>
                          秒杀
                        </Button>
                      </Popconfirm>
                    )
                  }
                ]}
              />
            )
          }

          if (providerType === 'acck') {
            return (
              <Table
                size="small"
                dataSource={items}
                rowKey={(it) => `ac-${it.id}`}
                pagination={false}
                columns={[
                  {
                    title: '套餐型号 / ID',
                    key: 'title',
                    render: (_, it) => (
                      <div>
                        <div style={{ fontWeight: 600, color: '#0f172a' }}>{it.title}</div>
                        <div style={{ fontSize: 11, color: '#94a3b8' }}>ID: {it.id}</div>
                      </div>
                    )
                  },
                  {
                    title: '优化线路 / 节点特性',
                    dataIndex: 'detail',
                    key: 'detail',
                    render: (detail) => <div style={{ fontSize: 12, color: '#334155' }}>{detail || '三网直连优质线路'}</div>
                  },
                  {
                    title: '机房区域',
                    dataIndex: 'location',
                    key: 'location',
                    width: 90,
                    render: (l) => <Tag color="geekblue">{l || '全球'}</Tag>
                  },
                  {
                    title: '转让标价',
                    dataIndex: 'price',
                    key: 'price',
                    width: 110,
                    render: (p: number) => <span style={{ fontWeight: 700, color: '#ef4444', fontSize: 14 }}>¥{p.toFixed(2)}</span>
                  },
                  {
                    title: '续费周期 / 价格',
                    key: 'renewal_price',
                    width: 145,
                    render: (_, it) => renderRenewalDisplay(it)
                  },
                  {
                    title: '流量重置费',
                    key: 'reset_price',
                    width: 120,
                    render: (_, it) => (
                      <div>
                        <span style={{ color: '#d97706', fontWeight: 600 }}>¥{it.reset_price ? it.reset_price.toFixed(2) : '25.00'}/次</span>
                        {it.flow_used ? <div style={{ fontSize: 10, color: '#64748b' }}>已用: {it.flow_used}</div> : null}
                      </div>
                    )
                  },
                  {
                    title: '到期时间',
                    dataIndex: 'due_time',
                    key: 'due_time',
                    width: 120,
                    render: (dt?: string) => <span style={{ fontSize: 11, color: '#64748b' }}>{dt || '-'}</span>
                  },
                  {
                    title: '命中时间',
                    dataIndex: 'matched_at',
                    key: 'matched_at',
                    width: 100,
                    render: (t: string) => <span style={{ fontSize: 11, color: '#94a3b8' }}>{formatTime(t)}</span>
                  },
                  {
                    title: '操作',
                    key: 'act',
                    width: 90,
                    render: (_, it) => (
                      <Popconfirm
                        title={`确认以 ¥${it.price.toFixed(2)} 秒杀【${it.title}】？`}
                        description="将调用 ACCK API 从账户余额扣款购买！"
                        onConfirm={() => handleDirectBuy('acck', it.id, it.title)}
                        okText="秒杀购买"
                        cancelText="取消"
                        okButtonProps={{ danger: true }}
                      >
                        <Button size="small" type="primary" danger icon={<ThunderboltOutlined />}>
                          秒杀
                        </Button>
                      </Popconfirm>
                    )
                  }
                ]}
              />
            )
          }

          // 默认全网汇总视图
          return (
            <Table
              size="small"
              dataSource={items}
              rowKey={(it) => `${it.provider}-${it.id}`}
              pagination={false}
              columns={[
                {
                  title: '平台',
                  dataIndex: 'provider',
                  key: 'provider',
                  width: 90,
                  render: (p: string) => (
                    <Tag color={p === 'lazycat' ? 'purple' : p === 'akile' ? 'blue' : 'cyan'}>
                      {p.toUpperCase()}
                    </Tag>
                  )
                },
                {
                  title: '挂单名称 / 节点规格',
                  key: 'title',
                  render: (_, it) => (
                    <div>
                      <div style={{ fontWeight: 600, color: '#0f172a' }}>{it.title}</div>
                      <div style={{ fontSize: 11, color: '#64748b' }}>
                        {it.node_name && <Tag color="blue" style={{ fontSize: 10, padding: '0 4px', marginRight: 4 }}>{it.node_name}</Tag>}
                        {it.location && <Tag color="geekblue" style={{ fontSize: 10, padding: '0 4px', marginRight: 4 }}>{it.location}</Tag>}
                        {it.bandwidth && <span>{it.bandwidth}</span>}
                      </div>
                    </div>
                  )
                },
                {
                  title: '卖家 / 线路 / 端口',
                  key: 'special',
                  render: (_, it) => (
                    <div style={{ fontSize: 12 }}>
                      {it.provider === 'lazycat' && (
                        <div>
                          {it.seller_username && <div>{it.seller_username}</div>}
                          {it.port_limit ? <Tag color="purple" style={{ fontSize: 10 }}>{it.port_limit} 端口</Tag> : null}
                        </div>
                      )}
                      {it.provider === 'akile' && (
                        <div>
                          {it.detail?.includes('正常') ? <Tag color="green" style={{ fontSize: 10 }}>IP正常</Tag> : null}
                          {it.detail?.includes('被墙') ? <Tag color="red" style={{ fontSize: 10 }}>IP异常</Tag> : null}
                        </div>
                      )}
                      {it.provider === 'acck' && (
                        <div style={{ fontSize: 11, color: '#475569' }}>{it.detail || '三网直连'}</div>
                      )}
                    </div>
                  )
                },
                {
                  title: '价格 / 续费 / 折扣',
                  key: 'price',
                  width: 175,
                  render: (_, it) => (
                    <div>
                      <div style={{ fontWeight: 700, color: '#ef4444', fontSize: 14 }}>¥{it.price}</div>
                      <div style={{ marginTop: 2 }}>
                        {renderRenewalDisplay(it)}
                      </div>
                    </div>
                  )
                },
                {
                  title: '命中时间',
                  dataIndex: 'matched_at',
                  key: 'matched_at',
                  width: 100,
                  render: (t: string) => <span style={{ fontSize: 11, color: '#94a3b8' }}>{formatTime(t)}</span>
                },
                {
                  title: '操作',
                  key: 'act',
                  width: 90,
                  render: (_, it) => (
                    <Popconfirm
                      title={`确认以 ¥${it.price} 购买【${it.title}】？`}
                      description="将直接调用平台 API 从账户余额扣除！"
                      onConfirm={() => handleDirectBuy(it.provider, it.id, it.title)}
                      okText="立即购买"
                      cancelText="取消"
                      okButtonProps={{ danger: true }}
                    >
                      <Button size="small" type="primary" danger icon={<ThunderboltOutlined />}>
                        购买
                      </Button>
                    </Popconfirm>
                  )
                }
              ]}
            />
          )
        }

        // 统一计划规则表格
        const renderPlansTable = (planList: PurchasePlan[], _currentProvider: string = 'all') => (
          <Table
            dataSource={planList}
            rowKey="id"
            pagination={false}
            size="middle"
            expandable={{
              expandedRowRender: (record: PurchasePlan) => (
                <div style={{ background: '#f8fafc', padding: 12, borderRadius: 8, border: '1px solid #e2e8f0' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                    <Space>
                      <FireOutlined style={{ color: '#ef4444' }} />
                      <Text strong style={{ fontSize: 13 }}>
                        符合【{record.name}】筛选条件候选 ({record.matched_items?.length || 0} 台) - 
                        {record.auto_buy ? ' 实盘秒杀中 (自动下单)' : ' 模拟筛选预览 (安全不扣款)'}
                      </Text>
                    </Space>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      上次匹配: {record.last_triggered_at || '刚刚'} (1m 智能轮询)
                    </Text>
                  </div>
                  {renderMatchedTable(record.matched_items || [], record.provider)}
                </div>
              ),
              rowExpandable: () => true,
              defaultExpandAllRows: true,
            }}
            columns={[
              {
                title: '状态',
                dataIndex: 'is_active',
                key: 'is_active',
                width: 70,
                render: (active: boolean, r) => (
                  <Switch checked={active} size="small" onChange={() => handleTogglePlan(r.id)} />
                )
              },
              {
                title: '计划名称',
                dataIndex: 'name',
                key: 'name',
                render: (name, r) => (
                  <div>
                    <div style={{ fontWeight: 600, color: '#0f172a' }}>{name}</div>
                    <div style={{ fontSize: 11, color: '#94a3b8' }}>ID: {r.id}</div>
                  </div>
                )
              },
              {
                title: '监控目标',
                key: 'target',
                width: 120,
                render: (_, r) => (
                  <Space direction="vertical" size={2}>
                    <Tag color={r.provider === 'akile' ? 'blue' : r.provider === 'acck' ? 'cyan' : r.provider === 'lazycat' ? 'purple' : 'geekblue'}>
                      {r.provider === 'all' ? '全厂商' : r.provider.toUpperCase()}
                    </Tag>
                    <span style={{ fontSize: 11, color: '#64748b' }}>
                      {r.market_type === 'new_stock' ? '官方新机' : r.market_type === 'secondhand' ? '二手集市' : '全量监控'}
                    </span>
                  </Space>
                )
              },
              {
                title: '筛选条件要求',
                key: 'filters',
                render: (_, r) => (
                  <Space wrap size={4}>
                    {r.region_keyword && <Tag color="geekblue">区域: {r.region_keyword}</Tag>}
                    {r.keyword_filter && <Tag color="cyan">关键词: {r.keyword_filter}</Tag>}
                    {r.max_price && r.max_price > 0 ? <Tag color="gold">售价: ≤ ¥{r.max_price}</Tag> : null}
                    {r.cycle_filter && r.cycle_filter !== 'all' ? (
                      <Tag color="purple">
                        周期: {r.cycle_filter === 'yearly' ? '年付' : r.cycle_filter === 'monthly' ? '月付' : '季付/半年付'}
                      </Tag>
                    ) : null}
                    {r.max_renewal_price && r.max_renewal_price > 0 ? <Tag color="orange">月续费: ≤ ¥{r.max_renewal_price}/月</Tag> : null}
                    {r.min_discount_percent && r.min_discount_percent > 0 ? <Tag color="red">折扣: ≤ {r.min_discount_percent}% ({r.min_discount_percent / 10}折)</Tag> : null}
                    {r.min_port_count && r.min_port_count > 0 ? <Tag color="purple">端口: ≥ {r.min_port_count} 个</Tag> : null}
                    {r.min_bandwidth && r.min_bandwidth > 0 ? <Tag color="green">带宽: ≥ {r.min_bandwidth}M</Tag> : null}
                  </Space>
                )
              },
              {
                title: '抢购模式',
                key: 'auto_buy_mode',
                width: 150,
                render: (_, r) => (
                  <div>
                    {r.auto_buy ? (
                      <>
                        <Tag color="red" icon={<ThunderboltOutlined />}>实盘秒杀</Tag>
                        <div style={{ fontSize: 11, color: '#ef4444', marginTop: 2 }}>
                          限额 {r.max_buy_count || 1} · 已购 {r.current_bought_count || 0}
                        </div>
                      </>
                    ) : (
                      <>
                        <Tag color="green" icon={<EyeOutlined />}>模拟筛选</Tag>
                        <div style={{ fontSize: 11, color: '#94a3b8', marginTop: 2 }}>
                          仅匹配展示，不扣款
                        </div>
                      </>
                    )}
                  </div>
                )
              },
              {
                title: '下单开关',
                key: 'auto_buy_toggle',
                width: 90,
                render: (_, r) => r.auto_buy ? (
                  <Switch checked={true} size="small" onChange={() => handleToggleAutoBuy(r)} checkedChildren="实盘" unCheckedChildren="模拟" />
                ) : (
                  <Popconfirm
                    title="确认开启【实盘秒杀】？"
                    description="每1分钟轮询命中符合条件的挂单后，将自动调用平台 API 扣除余额直接购买！"
                    onConfirm={() => handleToggleAutoBuy(r)}
                    okText="开启实盘"
                    cancelText="取消"
                    okButtonProps={{ danger: true }}
                  >
                    <Switch checked={false} size="small" checkedChildren="实盘" unCheckedChildren="模拟" />
                  </Popconfirm>
                )
              },
              {
                title: '命中候选',
                dataIndex: 'match_count',
                key: 'match_count',
                width: 110,
                render: (cnt: number, r) => {
                  const count = r.matched_items?.length || cnt || 0
                  return (
                    <div>
                      <Badge count={count} showZero style={{ backgroundColor: count > 0 ? '#10b981' : '#cbd5e1' }} />
                      {r.last_match_item && (
                        <div style={{ fontSize: 11, color: '#64748b', marginTop: 2 }}>{r.last_match_item}</div>
                      )}
                    </div>
                  )
                }
              },
              {
                title: '操作',
                key: 'action',
                width: 90,
                render: (_, r) => (
                  <Space size={6}>
                    <Button
                      type="text"
                      size="small"
                      icon={<EditOutlined />}
                      onClick={() => {
                        setEditingPlan(r)
                        planForm.setFieldsValue({
                          ...r,
                          cycle_filter: r.cycle_filter || 'all',
                          max_buy_count: r.max_buy_count || 1,
                        })
                        setPlanModalOpen(true)
                      }}
                    />
                    <Popconfirm
                      title="确定删除此条购买计划？"
                      onConfirm={() => handleDeletePlan(r.id)}
                      okText="删除"
                      cancelText="取消"
                    >
                      <Button type="text" danger size="small" icon={<DeleteOutlined />} />
                    </Popconfirm>
                  </Space>
                )
              }
            ]}
          />
        )

        return (
          <Space direction="vertical" size={20} style={{ width: '100%' }}>
            {/* 顶栏宏观监控状态卡片 */}
            <Row gutter={[16, 16]}>
              <Col xs={24} sm={12} md={6}>
                <Card bordered={false} className="vps-card">
                  <Statistic
                    title={<span style={{ color: '#64748b' }}>活跃抢购盯盘计划</span>}
                    value={activeCount}
                    suffix={<span style={{ fontSize: 13, color: '#94a3b8' }}>/ {plans.length} 条已配置</span>}
                    prefix={<AimOutlined style={{ color: '#2563eb', marginRight: 8 }} />}
                    valueStyle={{ fontWeight: 700, color: '#0f172a' }}
                  />
                  <div style={{ marginTop: 8, fontSize: 12, color: '#64748b' }}>
                    {activeCount > 0 ? '抢购引擎严格 1m 轮询中' : '无计划时完全休眠 (0 API 损耗)'}
                  </div>
                </Card>
              </Col>

              <Col xs={24} sm={12} md={6}>
                <Card bordered={false} className="vps-card">
                  <Statistic
                    title={<span style={{ color: '#64748b' }}>抢购引擎状态</span>}
                    value={schedulerStatus?.mode === 'burst' ? '极速突发' : activeCount > 0 ? '1分钟智能巡检' : '休眠节能'}
                    prefix={
                      schedulerStatus?.mode === 'burst' ? (
                        <RocketOutlined style={{ color: '#ef4444', marginRight: 8 }} />
                      ) : activeCount > 0 ? (
                        <SyncOutlined spin style={{ color: '#2563eb', marginRight: 8 }} />
                      ) : (
                        <PauseCircleOutlined style={{ color: '#64748b', marginRight: 8 }} />
                      )
                    }
                    valueStyle={{
                      fontWeight: 700,
                      color: schedulerStatus?.mode === 'burst' ? '#ef4444' : activeCount > 0 ? '#2563eb' : '#64748b'
                    }}
                    suffix={<span style={{ fontSize: 13, color: '#94a3b8' }}>({schedulerStatus?.current_interval_sec || 60}s/次)</span>}
                  />
                  <div style={{ marginTop: 8, fontSize: 12, color: '#64748b' }}>
                    上次轮询: {schedulerStatus?.last_poll_at ? formatTime(schedulerStatus.last_poll_at) : '暂未轮询'}
                  </div>
                </Card>
              </Col>

              <Col xs={24} sm={12} md={6}>
                <Card bordered={false} className="vps-card">
                  <Statistic
                    title={<span style={{ color: '#64748b' }}>当前命中候选 / 历史收录</span>}
                    value={totalMatchedItems}
                    suffix={<span style={{ fontSize: 13, color: '#94a3b8' }}>台待选 / {marketStats?.total_tracked || 0} 挂单</span>}
                    prefix={<FireOutlined style={{ color: '#f59e0b', marginRight: 8 }} />}
                    valueStyle={{ fontWeight: 700, color: '#0f172a' }}
                  />
                  <div style={{ marginTop: 8, fontSize: 12, color: '#64748b' }}>
                    在售 {marketStats?.active_items ?? 0} · 懒猫云 {marketStats?.by_provider_active?.lazycat ?? 0} / Akile {marketStats?.by_provider_active?.akile ?? 0} / ACCK {marketStats?.by_provider_active?.acck ?? 0}
                  </div>
                  <div style={{ marginTop: 2, fontSize: 12, color: '#94a3b8' }}>
                    平均历史价格: ¥{marketStats?.avg_price?.toFixed(1) || '0.0'}
                  </div>
                </Card>
              </Col>

              <Col xs={24} sm={12} md={6}>
                <Card bordered={false} className="vps-card">
                  <div style={{ display: 'flex', flexDirection: 'column', height: '100%', justifyContent: 'space-between' }}>
                    <span style={{ color: '#64748b', fontSize: 14 }}>快速突发控制</span>
                    <Button
                      type="primary"
                      icon={<ThunderboltOutlined />}
                      onClick={handleTriggerBurst}
                      style={{ background: '#7c3aed', borderColor: '#7c3aed', fontWeight: 600, height: 38 }}
                    >
                      {schedulerStatus?.mode === 'burst' ? `突发中 (${schedulerStatus.burst_remaining_seconds}s)` : '一键极速监控 5分'}
                    </Button>
                  </div>
                </Card>
              </Col>
            </Row>

            {/* 智能抢购中心: 外层汇总 + 内部分服务商独立专区 */}
            <Card
              bordered={false}
              className="vps-card"
              bodyStyle={{ padding: '16px 20px 24px' }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
                <Space size={12}>
                  <AimOutlined style={{ color: '#2563eb', fontSize: 20 }} />
                  <div>
                    <Text strong style={{ fontSize: 16, color: '#0f172a' }}>智能抢购中心 (1分钟智能轮询一手/二手集市)</Text>
                    <div style={{ fontSize: 12, color: '#64748b' }}>
                      全网运行中: <b>{activeCount}</b> 条 · 懒猫云: <b>{lazycatPlans.length}</b> 条 · Akile: <b>{akilePlans.length}</b> 条 · ACCK: <b>{acckPlans.length}</b> 条
                    </div>
                  </div>
                </Space>
                <Space>
                  <Button icon={<ReloadOutlined />} onClick={loadPlans}>
                    刷新候选
                  </Button>
                  <Button
                    type="primary"
                    icon={<PlusOutlined />}
                    onClick={() => handleOpenCreatePlan(sniperTab === 'all' ? 'all' : (sniperTab as any))}
                    style={{ background: '#2563eb', borderColor: '#2563eb' }}
                  >
                    新建抢购计划
                  </Button>
                </Space>
              </div>

              <Tabs
                activeKey={sniperTab}
                onChange={setSniperTab}
                type="card"
                items={[
                  {
                    key: 'all',
                    label: `全网汇总总览 (${plans.length})`,
                    children: (
                      <Space direction="vertical" size={16} style={{ width: '100%', marginTop: 8 }}>
                        {renderPlansTable(plans, 'all')}

                        <Card
                          size="small"
                          title={
                            <Space>
                              <FireOutlined style={{ color: '#ef4444' }} />
                              <Text strong>全网实时已命中挂单候选流动池 ({allMatched.length} 台待选)</Text>
                            </Space>
                          }
                          bordered={false}
                          style={{ background: '#f8fafc', borderRadius: 8, border: '1px solid #e2e8f0' }}
                        >
                          {renderMatchedTable(allMatched, 'all')}
                        </Card>
                      </Space>
                    )
                  },
                  {
                    key: 'lazycat',
                    label: `懒猫云专属抢购 (${lazycatPlans.length})`,
                    children: (
                      <Space direction="vertical" size={16} style={{ width: '100%', marginTop: 8 }}>
                        <Alert
                          message={
                            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                              <span><b>懒猫云全球家宽与 NAT 容器抢购专区</b> (支持 3折特惠、端口配额、在线保证率筛选)</span>
                              <Space wrap>
                                <Button
                                  size="small"
                                  onClick={() => handleOpenCreatePlan('lazycat', { name: '懒猫云3折特惠捡漏', max_price: 50, min_discount_percent: 30 })}
                                  style={{ background: '#fef3c7', borderColor: '#f59e0b', color: '#b45309' }}
                                >
                                  3折超值捡漏 (≤¥50)
                                </Button>
                                <Button
                                  size="small"
                                  onClick={() => handleOpenCreatePlan('lazycat', { name: '懒猫云20端口配额捡漏', max_price: 60, min_port_count: 20 })}
                                  style={{ background: '#ede9fe', borderColor: '#8b5cf6', color: '#6d28d9' }}
                                >
                                  20端口特惠机
                                </Button>
                                <Button
                                  size="small"
                                  type="primary"
                                  icon={<PlusOutlined />}
                                  onClick={() => handleOpenCreatePlan('lazycat')}
                                  style={{ background: '#7c3aed', borderColor: '#7c3aed' }}
                                >
                                  新建懒猫云计划
                                </Button>
                              </Space>
                            </div>
                          }
                          type="info"
                          showIcon
                          style={{ borderRadius: 8, background: '#faf5ff', borderColor: '#e9d5ff' }}
                        />

                        {renderPlansTable(lazycatPlans, 'lazycat')}

                        <Card
                          size="small"
                          title={
                            <Space>
                              <FireOutlined style={{ color: '#7c3aed' }} />
                              <Text strong>懒猫云当前匹配候选库存池 ({lazycatMatched.length} 台)</Text>
                            </Space>
                          }
                          bordered={false}
                          style={{ background: '#f8fafc', borderRadius: 8, border: '1px solid #e2e8f0' }}
                        >
                          {renderMatchedTable(lazycatMatched, 'lazycat')}
                        </Card>
                      </Space>
                    )
                  },
                  {
                    key: 'akile',
                    label: `Akile 专属抢购 (${akilePlans.length})`,
                    children: (
                      <Space direction="vertical" size={16} style={{ width: '100%', marginTop: 8 }}>
                        <Alert
                          message={
                            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                              <span><b>Akile PushShop 二手集市精准秒杀专区</b> (真实标价、真实续费、IP正常保障、三网低延迟过滤)</span>
                              <Space wrap>
                                <Button
                                  size="small"
                                  onClick={() => handleOpenCreatePlan('akile', { name: 'Akile IP正常平价机', max_price: 25, keyword_filter: '正常' })}
                                  style={{ background: '#dcfce7', borderColor: '#22c55e', color: '#15803d' }}
                                >
                                  IP正常平价机 (≤¥25)
                                </Button>
                                <Button
                                  size="small"
                                  onClick={() => handleOpenCreatePlan('akile', { name: 'Akile 优质大带宽节点', max_price: 35, min_bandwidth: 500, keyword_filter: '正常' })}
                                  style={{ background: '#e0f2fe', borderColor: '#0284c7', color: '#0369a1' }}
                                >
                                  优质大带宽 (≥500M)
                                </Button>
                                <Button
                                  size="small"
                                  type="primary"
                                  icon={<PlusOutlined />}
                                  onClick={() => handleOpenCreatePlan('akile')}
                                  style={{ background: '#2563eb', borderColor: '#2563eb' }}
                                >
                                  新建 Akile 计划
                                </Button>
                              </Space>
                            </div>
                          }
                          type="info"
                          showIcon
                          style={{ borderRadius: 8, background: '#eff6ff', borderColor: '#bfdbfe' }}
                        />

                        {renderPlansTable(akilePlans, 'akile')}

                        <Card
                          size="small"
                          title={
                            <Space>
                              <FireOutlined style={{ color: '#2563eb' }} />
                              <Text strong>Akile 当前匹配候选库存池 ({akileMatched.length} 台)</Text>
                            </Space>
                          }
                          bordered={false}
                          style={{ background: '#f8fafc', borderRadius: 8, border: '1px solid #e2e8f0' }}
                        >
                          {renderMatchedTable(akileMatched, 'akile')}
                        </Card>
                      </Space>
                    )
                  },
                  {
                    key: 'acck',
                    label: `ACCK 专属抢购 (${acckPlans.length})`,
                    children: (
                      <Space direction="vertical" size={16} style={{ width: '100%', marginTop: 8 }}>
                        <Alert
                          message={
                            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                              <span><b>ACCK 交易所优化线路捡漏专区</b> (三网 4837 / 9929 优质线路、流媒体解锁、原生 IP 穿透)</span>
                              <Space wrap>
                                <Button
                                  size="small"
                                  onClick={() => handleOpenCreatePlan('acck', { name: 'ACCK 三网4837转让捡漏', max_price: 15, keyword_filter: '4837' })}
                                  style={{ background: '#fee2e2', borderColor: '#ef4444', color: '#b91c1c' }}
                                >
                                  三网4837转让 (≤¥15)
                                </Button>
                                <Button
                                  size="small"
                                  onClick={() => handleOpenCreatePlan('acck', { name: 'ACCK 9929原生IP捡漏', max_price: 20, keyword_filter: '9929' })}
                                  style={{ background: '#ffedd5', borderColor: '#f97316', color: '#c2410c' }}
                                >
                                  9929原生IP (≤¥20)
                                </Button>
                                <Button
                                  size="small"
                                  type="primary"
                                  icon={<PlusOutlined />}
                                  onClick={() => handleOpenCreatePlan('acck')}
                                  style={{ background: '#0891b2', borderColor: '#0891b2' }}
                                >
                                  新建 ACCK 计划
                                </Button>
                              </Space>
                            </div>
                          }
                          type="info"
                          showIcon
                          style={{ borderRadius: 8, background: '#ecfeff', borderColor: '#a5f3fc' }}
                        />

                        {renderPlansTable(acckPlans, 'acck')}

                        <Card
                          size="small"
                          title={
                            <Space>
                              <FireOutlined style={{ color: '#0891b2' }} />
                              <Text strong>ACCK 当前匹配候选库存池 ({acckMatched.length} 台)</Text>
                            </Space>
                          }
                          bordered={false}
                          style={{ background: '#f8fafc', borderRadius: 8, border: '1px solid #e2e8f0' }}
                        >
                          {renderMatchedTable(acckMatched, 'acck')}
                        </Card>
                      </Space>
                    )
                  }
                ]}
              />
            </Card>
          </Space>
        )
      }

      case 'akile': {
        const akInstances = instances.filter(i => i.provider_id === 'akile')
        const akMarket = marketItems.filter(i => i.provider_id === 'akile')
        const filteredAkMarket = akMarket.filter(i => matchMarketSearch(i, akMarketSearch))
        const akStore = storeProducts.filter(i => i.provider_id === 'akile')

        return (
          <Space direction="vertical" size={20} style={{ width: '100%' }}>
            {renderProviderHeader('akile', 'Akile Cloud')}
            <Card bordered={false} className="vps-card">
              <Tabs
                defaultActiveKey="1"
                items={[
                  {
                    key: '1',
                    label: `我的主机资产 (${akInstances.length})`,
                    children: (
                      <div>
                        {/* 4列资产与打卡平衡卡片 */}
                        <Card size="small" bordered={false} style={{ background: '#f8fafc', marginBottom: 16, borderRadius: 8, border: '1px solid #e2e8f0' }}>
                          <Row gutter={[16, 16]} align="middle">
                            <Col xs={24} sm={6}>
                              <div style={{ color: '#64748b', fontSize: 13, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                                <span>Akile 凭据状态</span>
                                <Button
                                  type="link"
                                  size="small"
                                  icon={<LoginOutlined />}
                                  style={{ padding: 0, height: 'auto', fontSize: 12 }}
                                  onClick={() => { setLoginTargetProvider('akile'); setLoginModalOpen(true); }}
                                >
                                  一键登录/续期
                                </Button>
                              </div>
                              <div style={{ marginTop: 6, display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
                                {providerErrors['akile'] ? (
                                  <Tooltip title={providerErrors['akile']}>
                                    <Tag color="error">鉴权失效 (401)</Tag>
                                  </Tooltip>
                                ) : tokensConfig?.akile_is_expired ? (
                                  <Tag color="error">Token 已过期</Tag>
                                ) : tokensConfig?.akile_configured ? (
                                  <Tag color="green">Token 有效 ({tokensConfig.akile_token_masked})</Tag>
                                ) : (
                                  <Tag color="warning">未配置 Token</Tag>
                                )}
                                {tokensConfig?.akile_auto_refresh && (
                                  <Tag color="blue">自动续期</Tag>
                                )}
                              </div>
                              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>
                                {tokensConfig?.akile_expires_at ? (
                                  <span>
                                    {tokensConfig.akile_is_expired ? '已过期: ' : '到期: '}
                                    {formatTime(tokensConfig.akile_expires_at)}
                                    {tokensConfig.akile_seconds_left ? ` (${formatExpiry(tokensConfig.akile_expires_at, tokensConfig.akile_is_expired, tokensConfig.akile_seconds_left)})` : ''}
                                  </span>
                                ) : '私有资产鉴权凭据'}
                              </div>
                            </Col>

                            <Col xs={12} sm={6}>
                              <Statistic
                                title={<span style={{ color: '#64748b', fontSize: 13 }}>名下主机实例</span>}
                                value={akInstances.length}
                                suffix={<span style={{ fontSize: 13, color: '#64748b' }}>台在线</span>}
                                valueStyle={{ color: '#0f172a', fontWeight: 700, fontSize: 22 }}
                              />
                              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>
                                同步于: {formatTime(refreshTimes['instances:akile'] || refreshTimes['instances:all'])}
                              </div>
                            </Col>

                            <Col xs={12} sm={6}>
                              <Statistic
                                title={<span style={{ color: '#64748b', fontSize: 13 }}>集市在售挂单</span>}
                                value={akMarket.length}
                                suffix={<span style={{ fontSize: 13, color: '#2563eb', fontWeight: 600 }}>台在售</span>}
                                valueStyle={{ color: '#2563eb', fontWeight: 700, fontSize: 22 }}
                              />
                              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>
                                更新于: {formatTime(refreshTimes['market:akile'])}
                              </div>
                            </Col>

                            <Col xs={12} sm={6}>
                              <div style={{ color: '#64748b', fontSize: 13 }}>每日打卡中心</div>
                              <div style={{ marginTop: 6 }}>
                                <Button
                                  type="primary"
                                  icon={<CheckCircleOutlined />}
                                  loading={vendorCheckinLoading['akile']}
                                  onClick={() => handleSingleVendorCheckin('akile')}
                                  style={{
                                    background: '#2563eb',
                                    borderColor: '#2563eb',
                                    height: 32,
                                    fontWeight: 600,
                                    borderRadius: 6,
                                  }}
                                >
                                  {vendorCheckinStatus['akile'] ? '今日已打卡' : '立即打卡领 AK币'}
                                </Button>
                              </div>
                              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>
                                {vendorCheckinMsg['akile'] ? `反馈: ${vendorCheckinMsg['akile']}` : '每日签到可领 AK 币'}
                              </div>
                            </Col>
                          </Row>
                        </Card>

                        {providerErrors['akile'] && (
                          <Alert
                            type="error"
                            showIcon
                            style={{ marginBottom: 16, borderRadius: 8 }}
                            message="Akile API 鉴权失败：Token 已过期失效"
                            description={
                              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 8 }}>
                                <span>Akile 服务端返回「401 token无效」。由于 Akile Token 有效期较短，当前 Token 已被服务端失效，导致名下主机与二手集市挂单无法同步。请前往凭据配置更新最新 Token。</span>
                                <Button size="small" type="primary" danger onClick={() => setPage('settings')}>
                                  前往凭据配置更新
                                </Button>
                              </div>
                            }
                          />
                        )}

                        {akInstances.length === 0 && !providerErrors['akile'] ? (
                          <Alert
                            message="暂未拉取到 Akile 主机"
                            description={
                              <div>
                                当前未拉取到名下 Akile 主机。若尚未配置 Token，请先前往凭据设置填写；已配置则可点击下方按钮手动同步。
                                <div style={{ marginTop: 8 }}>
                                  <Button size="small" type="primary" loading={vendorInstancesLoading['akile']} onClick={() => handleRefreshInstances('akile')}>
                                    立即同步 Akile 资产
                                  </Button>
                                </div>
                              </div>
                            }
                            type="info"
                            showIcon
                            style={{ borderRadius: 8, background: '#eff6ff', borderColor: '#bfdbfe' }}
                          />
                        ) : (
                          <Table
                            loading={vendorInstancesLoading['akile']}
                            dataSource={akInstances}
                            rowKey="id"
                            pagination={{
                              pageSize: 10,
                              showTotal: (total) => `共 ${total} 台 Akile 主机 (动态同步)`,
                              showSizeChanger: true
                            }}
                            columns={[
                              {
                                title: '实例名称 / ID',
                                key: 'name',
                                render: (_, r) => (
                                  <div>
                                    <div style={{ fontWeight: 600, color: '#0f172a', display: 'flex', alignItems: 'center', gap: 6 }}>
                                      <span>{r.name}</span>
                                      {r.is_stale && (
                                        <Tooltip title={`离线缓存: ${r.stale_reason || 'Token 失效需重新登录'}`}>
                                          <Tag color="warning" style={{ fontSize: 11, cursor: 'pointer', margin: 0 }} onClick={(e) => {
                                            e.stopPropagation()
                                            setLoginTargetProvider('akile')
                                            setLoginModalOpen(true)
                                          }}>
                                            缓存
                                          </Tag>
                                        </Tooltip>
                                      )}
                                    </div>
                                    <div style={{ fontSize: 12, color: '#94a3b8' }}>ID: {r.id}</div>
                                  </div>
                                )
                              },
                              { title: '机房区域', dataIndex: 'region', key: 'region', render: (reg) => <Tag color="blue">{reg || '未知'}</Tag> },
                              {
                                title: '网络 IP (v4/v6)',
                                key: 'ip',
                                render: (_, r) => (
                                  <div style={{ fontFamily: 'monospace', fontSize: 12 }}>
                                    <div><Tag color="blue" style={{ fontSize: 11, padding: '0 4px', marginRight: 4 }}>v4</Tag>{r.ipv4 || r.ip}</div>
                                    {r.ipv6 && (
                                      <div style={{ marginTop: 2, color: '#7c3aed', display: 'flex', alignItems: 'center', gap: 4, whiteSpace: 'nowrap' }}>
                                        <Tag color="purple" style={{ fontSize: 11, padding: '0 4px', margin: 0, flexShrink: 0 }}>v6</Tag>
                                        <span style={{ fontSize: 11 }}>{r.ipv6.length > 18 ? r.ipv6.substring(0, 16) + '...' : r.ipv6}</span>
                                      </div>
                                    )}
                                  </div>
                                )
                              },
                              { title: '规格', key: 'spec', render: (_, r) => <span>{r.cpu} / {r.memory}</span> },
                              {
                                title: '月度流量',
                                key: 'traffic',
                                width: 170,
                                render: (_, r) => {
                                  const pct = getTrafficPercent(r)
                                  const color = getTrafficColor(pct)
                                  return (
                                    <div>
                                      <div style={{ fontSize: 12, display: 'flex', justifyContent: 'space-between', gap: 6, marginBottom: 2, whiteSpace: 'nowrap' }}>
                                        <span style={{ fontWeight: 600 }}>{r.traffic_used || '0 GB'}</span>
                                        <span style={{ color: '#64748b' }}>{r.traffic_total || '1000 GB'} ({pct}%)</span>
                                      </div>
                                      <Progress percent={pct} size="small" showInfo={false} strokeColor={color} />
                                      {r.traffic_reset_date && (
                                        <div style={{ fontSize: 10, color: '#94a3b8', marginTop: 2 }}>
                                          重置日: {r.traffic_reset_date}
                                        </div>
                                      )}
                                    </div>
                                  )
                                }
                              },
                              { title: '续费周期 / 金额', key: 'renew', width: 145, render: (_, r) => renderRenewalDisplay(r) },
                              {
                                title: '操作',
                                key: 'action',
                                render: (_, r) => (
                                  <Button size="small" type="link" onClick={() => { setSelectedVps(r); setDetailDrawerOpen(true); }}>
                                    详情与凭据
                                  </Button>
                                )
                              }
                            ]}
                          />
                        )}
                      </div>
                    )
                  },
                  {
                    key: '2',
                    label: '官方新机商城',
                    children: (
                      <Card
                        size="small"
                        title={
                          <Space>
                            <ShoppingOutlined style={{ color: '#2563eb' }} />
                            <Text strong>Akile 官方直营新机在售规格</Text>
                          </Space>
                        }
                        extra={
                          <Space size={12}>
                            <Text type="secondary" style={{ fontSize: 13 }}>
                              <ClockCircleOutlined style={{ marginRight: 4 }} />
                              库存更新于: {formatTime(refreshTimes['store:akile'] || refreshTimes['store:all'])}
                            </Text>
                            <Button size="small" icon={<ReloadOutlined spin={vendorStoreLoading['akile']} />} onClick={() => handleRefreshStore('akile')}>
                              刷新商城
                            </Button>
                          </Space>
                        }
                        bordered={false}
                      >
                        <Table
                          size="small"
                          dataSource={akStore}
                          rowKey="id"
                          pagination={{
                            defaultPageSize: 50,
                            pageSizeOptions: ['20', '50', '100', '200'],
                            showSizeChanger: true,
                            showTotal: (total, range) => `第 ${range[0]}-${range[1]} 项 · 共 ${total} 款规格`
                          }}
                          columns={[
                            { 
                              title: '套餐型号', 
                              dataIndex: 'name', 
                              key: 'name', 
                              sorter: (a, b) => (a.name || '').localeCompare(b.name || ''),
                              render: (t) => <span style={{ fontWeight: 600 }}>{t}</span> 
                            },
                            { 
                              title: '区域机房', 
                              dataIndex: 'location', 
                              key: 'location', 
                              width: 100, 
                              sorter: (a, b) => (a.location || '').localeCompare(b.location || ''),
                              render: (l) => <Tag color="blue">{l}</Tag> 
                            },
                            {
                              title: '硬件配置',
                              key: 'hardware',
                              width: 140,
                              sorter: (a, b) => ((parseInt(a.cpu || '1') || 1) * 10000 + (parseInt(a.memory || '0') || 0)) - ((parseInt(b.cpu || '1') || 1) * 10000 + (parseInt(b.memory || '0') || 0)),
                              render: (_, r) => (
                                <div>
                                  <Space size={4}>
                                    <Tag color="blue" style={{ margin: 0, fontWeight: 600, fontSize: 11, padding: '0 4px' }}>{r.cpu || '1C'}</Tag>
                                    <Tag color="cyan" style={{ margin: 0, fontWeight: 600, fontSize: 11, padding: '0 4px' }}>{r.memory || '1024M'}</Tag>
                                  </Space>
                                  <div style={{ fontSize: 11, color: '#64748b', marginTop: 1 }}>
                                    {r.disk ? `${r.disk} 磁盘` : (r.config_desc || '-')}
                                  </div>
                                </div>
                              )
                            },
                            {
                              title: '网络配额',
                              key: 'network',
                              width: 150,
                              sorter: (a, b) => (parseInt(a.bandwidth || '0') || 0) - (parseInt(b.bandwidth || '0') || 0),
                              render: (_, r) => (
                                <div>
                                  <div style={{ color: '#059669', fontWeight: 600, fontSize: 12 }}>{r.bandwidth || '1000 Mbps'}</div>
                                  <div style={{ fontSize: 11, color: '#64748b' }}>
                                    {r.traffic_total ? `月流量: ${r.traffic_total}` : '按套餐配额'}
                                  </div>
                                </div>
                              )
                            },
                            { 
                              title: '资费价格', 
                              dataIndex: 'price', 
                              key: 'price', 
                              width: 110, 
                              sorter: (a, b) => (parseFloat(String(a.price).replace(/[^0-9.]/g, '')) || 0) - (parseFloat(String(b.price).replace(/[^0-9.]/g, '')) || 0),
                              render: (p) => <span style={{ color: '#059669', fontWeight: 700, fontFamily: 'monospace' }}>{p}</span> 
                            },
                            {
                              title: '库存状态',
                              dataIndex: 'in_stock',
                              key: 'in_stock',
                              width: 105,
                              sorter: (a, b) => (a.in_stock === b.in_stock ? 0 : a.in_stock ? -1 : 1),
                              render: (inStock: boolean) => inStock ? <Tag color="green" style={{ margin: 0 }}>有货</Tag> : <Tag color="default" style={{ margin: 0 }}>缺货</Tag>
                            },
                            {
                              title: '操作',
                              key: 'action',
                              width: 110,
                              render: (_, r) => (
                                <Button
                                  size="small"
                                  type="link"
                                  icon={<AimOutlined />}
                                  onClick={() => {
                                    planForm.setFieldsValue({
                                      name: `Akile ${r.name} 补货盯盘`,
                                      provider: 'akile',
                                      market_type: 'new_stock',
                                      region_keyword: r.location,
                                      safe_mode: true,
                                      auto_buy: false,
                                      is_active: true
                                    })
                                    setEditingPlan(null)
                                    setPlanModalOpen(true)
                                  }}
                                >
                                  以此规格盯盘
                                </Button>
                              )
                            }
                          ]}
                        />
                      </Card>
                    )
                  },
                  {
                    key: '3',
                    label: `二手转让集市 (${akMarketSearch.trim() ? `${filteredAkMarket.length}/${akMarket.length}` : `${akMarket.length}`})`,
                    children: (
                      <Card
                        size="small"
                        title={
                          <Space>
                            <SwapOutlined style={{ color: '#2563eb' }} />
                            <Text strong>Akile PushShop 二手集市在售挂单 ({akMarketSearch.trim() ? `已筛选 ${filteredAkMarket.length} / 共 ${akMarket.length}` : `${akMarket.length}`} 台)</Text>
                          </Space>
                        }
                        extra={
                          <Space size={12}>
                            <Input.Search
                              placeholder="搜索挂单标题 / 节点 / 备注..."
                              allowClear
                              size="small"
                              style={{ width: 260 }}
                              value={akMarketSearch}
                              onChange={e => setAkMarketSearch(e.target.value)}
                            />
                            <Text type="secondary" style={{ fontSize: 13 }}>
                              <ClockCircleOutlined style={{ marginRight: 4 }} />
                              更新时间: {formatTime(refreshTimes['market:akile'])}
                            </Text>
                            <Button size="small" icon={<ReloadOutlined spin={vendorMarketLoading['akile']} />} onClick={() => handleRefreshMarket('akile')}>
                              刷新集市
                            </Button>
                          </Space>
                        }
                        bordered={false}
                      >
                        <Table
                          size="small"
                          dataSource={filteredAkMarket}
                          rowKey="id"
                          scroll={{ x: 'max-content' }}
                          pagination={{
                            defaultPageSize: 50,
                            pageSizeOptions: ['20', '50', '100', '200'],
                            showSizeChanger: true,
                            showTotal: (total, range) => `第 ${range[0]}-${range[1]} 项 · 共 ${total} 台挂单`
                          }}
                          columns={[
                            {
                              title: '机器型号 / 节点',
                              dataIndex: 'name',
                              key: 'name',
                              sorter: (a, b) => (a.name || '').localeCompare(b.name || ''),
                              render: (name, r) => (
                                <div>
                                  <div style={{ fontWeight: 600, color: '#0f172a', fontSize: 13 }}>{name}</div>
                                  <div style={{ fontSize: 11, color: '#64748b' }}>
                                    {r.node_name && <Tag color="blue" style={{ fontSize: 10, padding: '0 4px', marginRight: 4 }}>{r.node_name}</Tag>}
                                    <span style={{ color: '#94a3b8' }}>ID: {r.id}</span>
                                  </div>
                                </div>
                              )
                            },
                            { 
                              title: '机房区域', 
                              dataIndex: 'location', 
                              key: 'location', 
                              width: 95, 
                              sorter: (a, b) => (a.location || '').localeCompare(b.location || ''),
                              render: (l) => <Tag color="geekblue" style={{ margin: 0 }}>{l || '全球'}</Tag> 
                            },
                            {
                              title: '硬件配置',
                              key: 'hardware',
                              width: 135,
                              sorter: (a, b) => ((parseInt(a.cpu || '1') || 1) * 10000 + (parseInt(a.memory || '0') || 0)) - ((parseInt(b.cpu || '1') || 1) * 10000 + (parseInt(b.memory || '0') || 0)),
                              render: (_, r) => (
                                <div>
                                  <Space size={4}>
                                    <Tag color="blue" style={{ margin: 0, fontWeight: 600, fontSize: 11, padding: '0 4px' }}>{r.cpu || '1C'}</Tag>
                                    <Tag color="cyan" style={{ margin: 0, fontWeight: 600, fontSize: 11, padding: '0 4px' }}>{r.memory || '1024M'}</Tag>
                                  </Space>
                                  <div style={{ fontSize: 11, color: '#64748b', marginTop: 1 }}>
                                    {r.disk ? `${r.disk} 磁盘` : (r.config_desc || '-')}
                                  </div>
                                </div>
                              )
                            },
                            {
                              title: '网络配额',
                              key: 'network',
                              width: 155,
                              sorter: (a, b) => (parseInt(a.bandwidth || '0') || 0) - (parseInt(b.bandwidth || '0') || 0),
                              render: (_, r) => (
                                <div>
                                  <div style={{ color: '#059669', fontWeight: 600, fontSize: 12 }}>
                                    {r.bandwidth || '1000 Mbps'}
                                  </div>
                                  <div style={{ fontSize: 11, color: '#64748b' }}>
                                    {r.flow_used ? (
                                      <span>已用: {r.flow_used}</span>
                                    ) : r.traffic_total ? (
                                      <span>月流量: {r.traffic_total}</span>
                                    ) : null}
                                  </div>
                                  {r.reset_price !== undefined && r.reset_price > 0 && (
                                    <div style={{ fontSize: 10, color: '#d97706' }}>
                                      重置: ¥{r.reset_price.toFixed(2)}/次
                                    </div>
                                  )}
                                </div>
                              )
                            },
                            {
                              title: '转让标价',
                              dataIndex: 'price',
                              key: 'price',
                              width: 105,
                              sorter: (a, b) => (a.price || 0) - (b.price || 0),
                              render: (p: number, r) => (
                                <div>
                                  <span style={{ color: '#ef4444', fontWeight: 700, fontSize: 14, fontFamily: 'monospace' }}>¥{p.toFixed(2)}</span>
                                  {r.bargain_tag && (
                                    <div style={{ marginTop: 1 }}>
                                      <Tag color="volcano" style={{ fontSize: 10, padding: '0 3px', margin: 0 }}>{r.bargain_tag}</Tag>
                                    </div>
                                  )}
                                </div>
                              )
                            },
                            {
                              title: '续费周期 / 价格',
                              dataIndex: 'renewal_price',
                              key: 'renewal_price',
                              width: 180,
                              sorter: (a, b) => (a.monthly_price || a.renewal_price || 0) - (b.monthly_price || b.renewal_price || 0),
                              filters: [
                                { text: '年付', value: 'yearly' },
                                { text: '月付', value: 'monthly' },
                                { text: '季付/半年付', value: 'other' }
                              ],
                              onFilter: (value, r) => {
                                const c = r.cycle || 1
                                if (value === 'yearly') return c >= 12
                                if (value === 'monthly') return c === 1
                                if (value === 'other') return c > 1 && c < 12
                                return true
                              },
                              render: (_, r) => renderRenewalDisplay(r)
                            },
                            {
                              title: '卖家信息',
                              key: 'seller',
                              width: 110,
                              render: () => (
                                <Tag bordered={false} style={{ background: '#f1f5f9', color: '#64748b', fontSize: 11, margin: 0, padding: '1px 6px' }}>
                                  官方匿名
                                </Tag>
                              )
                            },
                            {
                              title: 'IP 状态与三网测速',
                              key: 'ip_status',
                              width: 175,
                              render: (_, r) => {
                                const isNormal = r.detail?.includes('正常')
                                const isBlocked = r.detail?.includes('被墙')
                                return (
                                  <div>
                                    <div style={{ marginBottom: 2 }}>
                                      {isNormal ? (
                                        <Tag color="green" style={{ fontWeight: 600, fontSize: 10, margin: 0 }}>IP正常</Tag>
                                      ) : isBlocked ? (
                                        <Tag color="red" style={{ fontWeight: 600, fontSize: 10, margin: 0 }}>IP异常</Tag>
                                      ) : (
                                        <Tag color="default" style={{ fontSize: 10, margin: 0 }}>{r.detail || '未检测'}</Tag>
                                      )}
                                    </div>
                                    {r.ip_check_detail ? (
                                      <Tooltip title={<pre style={{ margin: 0, fontSize: 11 }}>{r.ip_check_detail}</pre>}>
                                        <span style={{ fontSize: 11, color: '#64748b', cursor: 'pointer', textDecoration: 'underline dotted' }}>
                                          三网: {r.ip_check_detail.split('\n')[1] || '测速详情'}
                                        </span>
                                      </Tooltip>
                                    ) : (
                                      <span style={{ fontSize: 11, color: '#94a3b8' }}>暂无延迟报告</span>
                                    )}
                                  </div>
                                )
                              }
                            },
                            {
                              title: '到期时间',
                              dataIndex: 'due_time',
                              key: 'due_time',
                              width: 120,
                              sorter: (a, b) => (a.due_time || '').localeCompare(b.due_time || ''),
                              render: (dt?: string) => <span style={{ fontSize: 11, color: '#64748b' }}>{dt || '-'}</span>
                            },
                            {
                              title: '操作',
                              key: 'action',
                              width: 135,
                              render: (_, r) => (
                                <Space size={4}>
                                  <Button
                                    size="small"
                                    type="primary"
                                    ghost
                                    icon={<AimOutlined />}
                                    onClick={() => handleCreatePlanFromMarket(r)}
                                  >
                                    盯盘
                                  </Button>
                                  <Popconfirm
                                    title={`确认以 ¥${r.price.toFixed(2)} 秒杀【${r.name}】？`}
                                    description="将调用 Akile API 从账户余额扣款购买！"
                                    onConfirm={() => handleDirectBuy('akile', r.id, r.name)}
                                    okText="秒杀购买"
                                    cancelText="取消"
                                    okButtonProps={{ danger: true }}
                                  >
                                    <Button
                                      size="small"
                                      type="primary"
                                      danger
                                      icon={<ThunderboltOutlined />}
                                    >
                                      秒杀
                                    </Button>
                                  </Popconfirm>
                                </Space>
                              )
                            }
                          ]}
                        />
                      </Card>
                    )
                  }
                ]}
              />
            </Card>
          </Space>
        )
      }

      case 'acck': {
        const acInstances = instances.filter(i => i.provider_id === 'acck')
        const acMarket = marketItems.filter(i => i.provider_id === 'acck')
        const filteredAcMarket = acMarket.filter(i => matchMarketSearch(i, acMarketSearch))
        const acStore = storeProducts.filter(i => i.provider_id === 'acck')

        return (
          <Space direction="vertical" size={20} style={{ width: '100%' }}>
            {renderProviderHeader('acck', 'ACCK 专区')}
            <Card bordered={false} className="vps-card">
              <Tabs
                defaultActiveKey="3"
                items={[
                  {
                    key: '1',
                    label: `我的主机资产 (${acInstances.length})`,
                    children: (
                      <div>
                        {/* 4列资产与打卡平衡卡片 */}
                        <Card size="small" bordered={false} style={{ background: '#f8fafc', marginBottom: 16, borderRadius: 8, border: '1px solid #e2e8f0' }}>
                          <Row gutter={[16, 16]} align="middle">
                            <Col xs={24} sm={6}>
                              <div style={{ color: '#64748b', fontSize: 13, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                                <span>ACCK 凭据状态</span>
                                <Button
                                  type="link"
                                  size="small"
                                  icon={<KeyOutlined />}
                                  style={{ padding: 0, height: 'auto', fontSize: 12 }}
                                  onClick={() => { setLoginTargetProvider('acck'); setLoginModalOpen(true); }}
                                >
                                  凭据 / 免密登录
                                </Button>
                              </div>
                              <div style={{ marginTop: 6, display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
                                {providerErrors['acck'] ? (
                                  <Tooltip title={providerErrors['acck']}>
                                    <Tag color="error">鉴权异常</Tag>
                                  </Tooltip>
                                ) : tokensConfig?.acck_is_expired ? (
                                  <Tag color="error">Token 已过期</Tag>
                                ) : tokensConfig?.acck_configured ? (
                                  <Tag color="green">Token 有效 ({tokensConfig.acck_token_masked})</Tag>
                                ) : (
                                  <Tag color="warning">未配置 Token</Tag>
                                )}
                              </div>
                              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>
                                {tokensConfig?.acck_expires_at ? (
                                  <span>
                                    {tokensConfig.acck_is_expired ? '已过期: ' : '到期: '}
                                    {formatTime(tokensConfig.acck_expires_at)}
                                    {tokensConfig.acck_seconds_left ? ` (${formatExpiry(tokensConfig.acck_expires_at, tokensConfig.acck_is_expired, tokensConfig.acck_seconds_left)})` : ''}
                                  </span>
                                ) : '私有机器鉴权校验'}
                              </div>
                            </Col>

                            <Col xs={12} sm={6}>
                              <Statistic
                                title={<span style={{ color: '#64748b', fontSize: 13 }}>名下主机实例</span>}
                                value={acInstances.length}
                                suffix={<span style={{ fontSize: 13, color: '#64748b' }}>台在线</span>}
                                valueStyle={{ color: '#0f172a', fontWeight: 700, fontSize: 22 }}
                              />
                              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>
                                同步于: {formatTime(refreshTimes['instances:acck'] || refreshTimes['instances:all'])}
                              </div>
                            </Col>

                            <Col xs={12} sm={6}>
                              <Statistic
                                title={<span style={{ color: '#64748b', fontSize: 13 }}>集市在售挂单</span>}
                                value={acMarket.length}
                                suffix={<span style={{ fontSize: 13, color: '#059669', fontWeight: 600 }}>台在售</span>}
                                valueStyle={{ color: '#059669', fontWeight: 700, fontSize: 22 }}
                              />
                              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>
                                更新于: {formatTime(refreshTimes['market:acck'])}
                              </div>
                            </Col>

                            <Col xs={12} sm={6}>
                              <div style={{ color: '#64748b', fontSize: 13 }}>每日打卡中心</div>
                              <div style={{ marginTop: 6 }}>
                                <Button
                                  type="primary"
                                  icon={<CheckCircleOutlined />}
                                  loading={vendorCheckinLoading['acck']}
                                  onClick={() => handleSingleVendorCheckin('acck')}
                                  style={{
                                    background: '#059669',
                                    borderColor: '#059669',
                                    height: 32,
                                    fontWeight: 600,
                                    borderRadius: 6,
                                  }}
                                >
                                  {vendorCheckinStatus['acck'] ? '今日已打卡' : '立即打卡领 AC币'}
                                </Button>
                              </div>
                              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>
                                {vendorCheckinMsg['acck'] ? `反馈: ${vendorCheckinMsg['acck']}` : '每日签到赠送 AC 币'}
                              </div>
                            </Col>
                          </Row>
                        </Card>

                        {acInstances.length === 0 ? (
                          <Alert
                            message="暂未拉取到 ACCK 主机"
                            description={
                              <div>
                                当前未拉取到名下 ACCK 主机。若尚未配置 Token，请先前往凭据设置填写；已配置则可点击下方按钮手动同步。
                                <div style={{ marginTop: 8 }}>
                                  <Button size="small" type="primary" loading={vendorInstancesLoading['acck']} onClick={() => handleRefreshInstances('acck')}>
                                    立即同步 ACCK 资产
                                  </Button>
                                </div>
                              </div>
                            }
                            type="info"
                            showIcon
                            style={{ borderRadius: 8, background: '#eff6ff', borderColor: '#bfdbfe' }}
                          />
                        ) : (
                          <Table
                            loading={vendorInstancesLoading['acck']}
                            dataSource={acInstances}
                            rowKey="id"
                            pagination={{
                              pageSize: 10,
                              showTotal: (total) => `共 ${total} 台 ACCK 主机 (动态同步)`,
                              showSizeChanger: true
                            }}
                            columns={[
                              {
                                title: '套餐名称 / ID',
                                dataIndex: 'name',
                                key: 'name',
                                render: (n, r) => (
                                  <div>
                                    <div style={{ fontWeight: 600, color: '#0f172a', display: 'flex', alignItems: 'center', gap: 6 }}>
                                      <span>{n}</span>
                                      {r.is_stale && (
                                        <Tooltip title={`离线缓存: ${r.stale_reason || 'Token 失效需重新录入'}`}>
                                          <Tag color="warning" style={{ fontSize: 11, cursor: 'pointer', margin: 0 }} onClick={(e) => {
                                            e.stopPropagation()
                                            setLoginTargetProvider('acck')
                                            setLoginModalOpen(true)
                                          }}>
                                            缓存
                                          </Tag>
                                        </Tooltip>
                                      )}
                                    </div>
                                    <div style={{ fontSize: 12, color: '#94a3b8' }}>ID: {r.id}</div>
                                  </div>
                                )
                              },
                              { title: '地区', dataIndex: 'region', key: 'region', render: (reg) => <Tag color="blue">{reg}</Tag> },
                              {
                                title: '网络 IP (v4/v6)',
                                key: 'ip',
                                render: (_, r) => (
                                  <div style={{ fontFamily: 'monospace', fontSize: 12 }}>
                                    <div><Tag color="blue" style={{ fontSize: 11, padding: '0 4px', marginRight: 4 }}>v4</Tag>{r.ipv4 || r.ip}</div>
                                    {r.ipv6 && (
                                      <div style={{ marginTop: 2, color: '#7c3aed', display: 'flex', alignItems: 'center', gap: 4, whiteSpace: 'nowrap' }}>
                                        <Tag color="purple" style={{ fontSize: 11, padding: '0 4px', margin: 0, flexShrink: 0 }}>v6</Tag>
                                        <span style={{ fontSize: 11 }}>{r.ipv6.length > 18 ? r.ipv6.substring(0, 16) + '...' : r.ipv6}</span>
                                      </div>
                                    )}
                                  </div>
                                )
                              },
                              { title: '规格', key: 'spec', render: (_, r) => <span>{r.cpu} / {r.memory}</span> },
                              {
                                title: '月度流量',
                                key: 'traffic',
                                width: 170,
                                render: (_, r) => {
                                  const pct = getTrafficPercent(r)
                                  const color = getTrafficColor(pct)
                                  return (
                                    <div>
                                      <div style={{ fontSize: 12, display: 'flex', justifyContent: 'space-between', gap: 6, marginBottom: 2, whiteSpace: 'nowrap' }}>
                                        <span style={{ fontWeight: 600 }}>{r.traffic_used || '0 GB'}</span>
                                        <span style={{ color: '#64748b' }}>{r.traffic_total || '1000 GB'} ({pct}%)</span>
                                      </div>
                                      <Progress percent={pct} size="small" showInfo={false} strokeColor={color} />
                                      {r.traffic_reset_date && (
                                        <div style={{ fontSize: 10, color: '#94a3b8', marginTop: 2 }}>
                                          重置日: {r.traffic_reset_date}
                                        </div>
                                      )}
                                    </div>
                                  )
                                }
                              },
                              { title: '续费周期 / 金额', key: 'renew', width: 145, render: (_, r) => renderRenewalDisplay(r) },
                              {
                                title: '操作',
                                key: 'action',
                                render: (_, r) => (
                                  <Button size="small" type="link" onClick={() => { setSelectedVps(r); setDetailDrawerOpen(true); }}>
                                    详情与凭据
                                  </Button>
                                )
                              }
                            ]}
                          />
                        )}
                      </div>
                    )
                  },
                  {
                    key: '2',
                    label: '官方新机商城',
                    children: (
                      <Card
                        size="small"
                        title={
                          <Space>
                            <ShoppingOutlined style={{ color: '#2563eb' }} />
                            <Text strong>ACCK 官方直营新机在售规格</Text>
                          </Space>
                        }
                        extra={
                          <Space size={12}>
                            <Text type="secondary" style={{ fontSize: 13 }}>
                              <ClockCircleOutlined style={{ marginRight: 4 }} />
                              库存更新于: {formatTime(refreshTimes['store:acck'] || refreshTimes['store:all'])}
                            </Text>
                            <Button size="small" icon={<ReloadOutlined spin={vendorStoreLoading['acck']} />} onClick={() => handleRefreshStore('acck')}>
                              刷新商城
                            </Button>
                          </Space>
                        }
                        bordered={false}
                      >
                        {acStore.length === 0 ? (
                          <Alert
                            type="info"
                            showIcon
                            message="ACCK 暂未开放官方新机商城数据"
                            description="ACCK 平台仅提供二手流转集市，未开放官方新机库存查询接口。请切换到「二手转让集市」查看实时挂单。"
                            style={{ borderRadius: 8 }}
                          />
                        ) : (
                        <Table
                          size="small"
                          dataSource={acStore}
                          rowKey="id"
                          scroll={{ x: 1080 }}
                          pagination={{
                            defaultPageSize: 50,
                            pageSizeOptions: ['20', '50', '100', '200'],
                            showSizeChanger: true,
                            showTotal: (total, range) => `第 ${range[0]}-${range[1]} 项 · 共 ${total} 款规格`
                          }}
                          columns={[
                            { 
                              title: '套餐型号', 
                              dataIndex: 'name', 
                              key: 'name', 
                              sorter: (a, b) => (a.name || '').localeCompare(b.name || ''),
                              render: (t) => <span style={{ fontWeight: 600 }}>{t}</span> 
                            },
                            { 
                              title: '机房区域', 
                              dataIndex: 'location', 
                              key: 'location', 
                              width: 100, 
                              sorter: (a, b) => (a.location || '').localeCompare(b.location || ''),
                              render: (l) => <Tag color="blue">{l}</Tag> 
                            },
                            {
                              title: '硬件配置',
                              key: 'hardware',
                              width: 140,
                              sorter: (a, b) => ((parseInt(a.cpu || '1') || 1) * 10000 + (parseInt(a.memory || '0') || 0)) - ((parseInt(b.cpu || '1') || 1) * 10000 + (parseInt(b.memory || '0') || 0)),
                              render: (_, r) => (
                                <div>
                                  <Space size={4}>
                                    <Tag color="blue" style={{ margin: 0, fontWeight: 600, fontSize: 11, padding: '0 4px' }}>{r.cpu || '1C'}</Tag>
                                    <Tag color="cyan" style={{ margin: 0, fontWeight: 600, fontSize: 11, padding: '0 4px' }}>{r.memory || '1024M'}</Tag>
                                  </Space>
                                  <div style={{ fontSize: 11, color: '#64748b', marginTop: 1 }}>
                                    {r.disk ? `${r.disk} 磁盘` : (r.config_desc || '-')}
                                  </div>
                                </div>
                              )
                            },
                            {
                              title: '网络配额',
                              key: 'network',
                              width: 150,
                              sorter: (a, b) => (parseInt(a.bandwidth || '0') || 0) - (parseInt(b.bandwidth || '0') || 0),
                              render: (_, r) => (
                                <div>
                                  <div style={{ color: '#059669', fontWeight: 600, fontSize: 12 }}>{r.bandwidth || '1000 Mbps'}</div>
                                  <div style={{ fontSize: 11, color: '#64748b' }}>
                                    {r.traffic_total ? `月流量: ${r.traffic_total}` : '按套餐配额'}
                                  </div>
                                </div>
                              )
                            },
                            { 
                              title: '资费价格', 
                              dataIndex: 'price', 
                              key: 'price', 
                              width: 110, 
                              sorter: (a, b) => (parseFloat(String(a.price).replace(/[^0-9.]/g, '')) || 0) - (parseFloat(String(b.price).replace(/[^0-9.]/g, '')) || 0),
                              render: (p) => <span style={{ color: '#059669', fontWeight: 700, fontFamily: 'monospace' }}>{p}</span> 
                            },
                            {
                              title: '库存状态',
                              dataIndex: 'in_stock',
                              key: 'in_stock',
                              width: 105,
                              sorter: (a, b) => (a.in_stock === b.in_stock ? 0 : a.in_stock ? -1 : 1),
                              render: (inStock: boolean) => inStock ? <Tag color="green" style={{ margin: 0 }}>有货</Tag> : <Tag color="default" style={{ margin: 0 }}>缺货</Tag>
                            },
                            {
                              title: '操作',
                              key: 'action',
                              width: 110,
                              render: (_, r) => (
                                <Button
                                  size="small"
                                  type="link"
                                  icon={<AimOutlined />}
                                  onClick={() => {
                                    planForm.setFieldsValue({
                                      name: `ACCK ${r.name} 补货盯盘`,
                                      provider: 'acck',
                                      market_type: 'new_stock',
                                      region_keyword: r.location,
                                      safe_mode: true,
                                      auto_buy: false,
                                      is_active: true
                                    })
                                    setEditingPlan(null)
                                    setPlanModalOpen(true)
                                  }}
                                >
                                  以此规格盯盘
                                </Button>
                              )
                            }
                          ]}
                        />
                        )}
                      </Card>
                    )
                  },
                  {
                    key: '3',
                    label: `二手转让集市 (${acMarketSearch.trim() ? `${filteredAcMarket.length}/${acMarket.length}` : `${acMarket.length}`})`,
                    children: (
                      <Card
                        size="small"
                        title={
                          <Space>
                            <SwapOutlined style={{ color: '#2563eb' }} />
                            <Text strong>ACCK 官方二手集市在售挂单 ({acMarketSearch.trim() ? `已筛选 ${filteredAcMarket.length} / 共 ${acMarket.length}` : `${acMarket.length}`} 台)</Text>
                          </Space>
                        }
                        extra={
                          <Space size={12}>
                            <Input.Search
                              placeholder="搜索挂单标题 / 节点 / 备注..."
                              allowClear
                              size="small"
                              style={{ width: 260 }}
                              value={acMarketSearch}
                              onChange={e => setAcMarketSearch(e.target.value)}
                            />
                            <Text type="secondary" style={{ fontSize: 13 }}>
                              <ClockCircleOutlined style={{ marginRight: 4 }} />
                              更新时间: {formatTime(refreshTimes['market:acck'])}
                            </Text>
                            <Button size="small" icon={<ReloadOutlined spin={vendorMarketLoading['acck']} />} onClick={() => handleRefreshMarket('acck')}>
                              刷新二手
                            </Button>
                          </Space>
                        }
                        bordered={false}
                      >
                        <Table
                          size="small"
                          dataSource={filteredAcMarket}
                          rowKey="id"
                          scroll={{ x: 'max-content' }}
                          pagination={{
                            defaultPageSize: 50,
                            pageSizeOptions: ['20', '50', '100', '200'],
                            showSizeChanger: true,
                            showTotal: (total, range) => `第 ${range[0]}-${range[1]} 项 · 共 ${total} 台挂单`
                          }}
                          columns={[
                            {
                              title: '套餐型号',
                              dataIndex: 'name',
                              key: 'name',
                              sorter: (a, b) => (a.name || '').localeCompare(b.name || ''),
                              render: (name, r) => (
                                <div>
                                  <div style={{ fontWeight: 600, color: '#0f172a', fontSize: 13 }}>{name}</div>
                                  <div style={{ fontSize: 11, color: '#94a3b8' }}>ID: {r.id}</div>
                                </div>
                              )
                            },
                            {
                              title: '优化线路 / 节点特性',
                              dataIndex: 'detail',
                              key: 'detail',
                              render: (detail) => (
                                <div style={{ fontSize: 12, color: '#334155' }}>
                                  {detail || '优质直连线路'}
                                </div>
                              )
                            },
                            { 
                              title: '机房区域', 
                              dataIndex: 'location', 
                              key: 'location', 
                              width: 95, 
                              sorter: (a, b) => (a.location || '').localeCompare(b.location || ''),
                              render: (l) => <Tag color="geekblue" style={{ margin: 0 }}>{l || '全球'}</Tag> 
                            },
                            {
                              title: '硬件配置',
                              key: 'hardware',
                              width: 135,
                              sorter: (a, b) => ((parseInt(a.cpu || '1') || 1) * 10000 + (parseInt(a.memory || '0') || 0)) - ((parseInt(b.cpu || '1') || 1) * 10000 + (parseInt(b.memory || '0') || 0)),
                              render: (_, r) => (
                                <div>
                                  <Space size={4}>
                                    <Tag color="blue" style={{ margin: 0, fontWeight: 600, fontSize: 11, padding: '0 4px' }}>{r.cpu || '1C'}</Tag>
                                    <Tag color="cyan" style={{ margin: 0, fontWeight: 600, fontSize: 11, padding: '0 4px' }}>{r.memory || '1024M'}</Tag>
                                  </Space>
                                  <div style={{ fontSize: 11, color: '#64748b', marginTop: 1 }}>
                                    {r.disk ? `${r.disk} 磁盘` : (r.config_desc || '-')}
                                  </div>
                                </div>
                              )
                            },
                            {
                              title: '网络配额',
                              key: 'network',
                              width: 155,
                              sorter: (a, b) => (parseInt(a.bandwidth || '0') || 0) - (parseInt(b.bandwidth || '0') || 0),
                              render: (_, r) => (
                                <div>
                                  <div style={{ color: '#059669', fontWeight: 600, fontSize: 12 }}>
                                    {r.bandwidth || '1000 Mbps'}
                                  </div>
                                  <div style={{ fontSize: 11, color: '#64748b' }}>
                                    {r.flow_used ? (
                                      <span>已用: {r.flow_used}</span>
                                    ) : r.traffic_total ? (
                                      <span>月流量: {r.traffic_total}</span>
                                    ) : null}
                                  </div>
                                  {r.reset_price !== undefined && r.reset_price > 0 && (
                                    <div style={{ fontSize: 10, color: '#d97706' }}>
                                      重置: ¥{r.reset_price.toFixed(2)}/次
                                    </div>
                                  )}
                                  {r.platform_fee ? (
                                    <div style={{ fontSize: 10, color: '#94a3b8' }}>
                                      过户费: ¥{r.platform_fee.toFixed(2)}
                                    </div>
                                  ) : null}
                                </div>
                              )
                            },
                            {
                              title: '转让标价',
                              dataIndex: 'price',
                              key: 'price',
                              width: 105,
                              sorter: (a, b) => (a.price || 0) - (b.price || 0),
                              render: (p: number, r) => (
                                <div>
                                  <span style={{ color: '#ef4444', fontWeight: 700, fontSize: 14, fontFamily: 'monospace' }}>¥{p.toFixed(2)}</span>
                                  {r.bargain_tag && (
                                    <div style={{ marginTop: 1 }}>
                                      <Tag color="volcano" style={{ fontSize: 10, padding: '0 3px', margin: 0 }}>{r.bargain_tag}</Tag>
                                    </div>
                                  )}
                                </div>
                              )
                            },
                            {
                              title: '续费周期 / 价格',
                              dataIndex: 'renewal_price',
                              key: 'renewal_price',
                              width: 180,
                              sorter: (a, b) => (a.monthly_price || a.renewal_price || 0) - (b.monthly_price || b.renewal_price || 0),
                              filters: [
                                { text: '年付', value: 'yearly' },
                                { text: '月付', value: 'monthly' },
                                { text: '季付/半年付', value: 'other' }
                              ],
                              onFilter: (value, r) => {
                                const c = r.cycle || 1
                                if (value === 'yearly') return c >= 12
                                if (value === 'monthly') return c === 1
                                if (value === 'other') return c > 1 && c < 12
                                return true
                              },
                              render: (_, r) => renderRenewalDisplay(r)
                            },
                            {
                              title: '卖家信息',
                              key: 'seller',
                              width: 120,
                              sorter: (a, b) => (a.seller_user_id || 0) - (b.seller_user_id || 0),
                              render: (_, r) => r.seller_user_id ? (
                                <Tag bordered={false} color="blue" style={{ fontSize: 11, margin: 0, padding: '1px 6px' }}>
                                  UID: {r.seller_user_id}
                                </Tag>
                              ) : (
                                <span style={{ fontSize: 11, color: '#94a3b8' }}>平台实名托管</span>
                              )
                            },
                            {
                              title: '到期时间',
                              dataIndex: 'due_time',
                              key: 'due_time',
                              width: 120,
                              sorter: (a, b) => (a.due_time || '').localeCompare(b.due_time || ''),
                              render: (dt?: string) => <span style={{ fontSize: 11, color: '#64748b' }}>{dt || '-'}</span>
                            },
                            {
                              title: '操作',
                              key: 'action',
                              width: 135,
                              render: (_, r) => (
                                <Space size={4}>
                                  <Button
                                    size="small"
                                    type="primary"
                                    ghost
                                    icon={<AimOutlined />}
                                    onClick={() => handleCreatePlanFromMarket(r)}
                                  >
                                    盯盘
                                  </Button>
                                  <Popconfirm
                                    title={`确认以 ¥${r.price.toFixed(2)} 秒杀【${r.name}】？`}
                                    description="将调用 ACCK API 从账户余额扣款购买！"
                                    onConfirm={() => handleDirectBuy('acck', r.id, r.name)}
                                    okText="秒杀购买"
                                    cancelText="取消"
                                    okButtonProps={{ danger: true }}
                                  >
                                    <Button
                                      size="small"
                                      type="primary"
                                      danger
                                      icon={<ThunderboltOutlined />}
                                    >
                                      秒杀
                                    </Button>
                                  </Popconfirm>
                                </Space>
                              )
                            }
                          ]}
                        />
                      </Card>
                    )
                  }
                ]}
              />
            </Card>
          </Space>
        )
      }

      case 'lazycat': {
        const lzInstances = instances.filter(i => i.provider_id === 'lazycat')
        const lzMarket = marketItems.filter(i => i.provider_id === 'lazycat')
        const filteredLzMarket = lzMarket.filter(i => matchMarketSearch(i, lzMarketSearch))
        const lzStore = storeProducts.filter(i => i.provider_id === 'lazycat')

        return (
          <Space direction="vertical" size={20} style={{ width: '100%' }}>
            {renderProviderHeader('lazycat', 'LazyCat 懒猫云')}
            <Card bordered={false} className="vps-card">
              <Tabs
                defaultActiveKey="1"
                items={[
                  {
                    key: '1',
                    label: `我的容器与概况 (${lzInstances.length})`,
                    children: (
                      <div>
                        {/* 4列资产与打卡平衡卡片 */}
                        <Card size="small" bordered={false} style={{ background: '#f8fafc', marginBottom: 16, borderRadius: 8, border: '1px solid #e2e8f0' }}>
                          <Row gutter={[16, 16]} align="middle">
                            <Col xs={24} sm={6}>
                              <div style={{ color: '#64748b', fontSize: 13, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                                <span>懒猫云凭据状态</span>
                                <Button
                                  type="link"
                                  size="small"
                                  icon={<LoginOutlined />}
                                  style={{ padding: 0, height: 'auto', fontSize: 12 }}
                                  onClick={() => { setLoginTargetProvider('lazycat'); setLoginModalOpen(true); }}
                                >
                                  一键登录/续期
                                </Button>
                              </div>
                              <div style={{ marginTop: 6, display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
                                {providerErrors['lazycat'] ? (
                                  <Tooltip title={providerErrors['lazycat']}>
                                    <Tag color="error">鉴权异常</Tag>
                                  </Tooltip>
                                ) : tokensConfig?.lazycat_is_expired ? (
                                  <Tag color="error">Token 已过期</Tag>
                                ) : tokensConfig?.lazycat_configured ? (
                                  <Tag color="green">Token 有效 ({tokensConfig.lazycat_token_masked})</Tag>
                                ) : (
                                  <Tag color="warning">未配置 Token</Tag>
                                )}
                                {tokensConfig?.lazycat_auto_refresh && (
                                  <Tag color="blue">自动重登</Tag>
                                )}
                              </div>
                              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>
                                {tokensConfig?.lazycat_expires_at ? (
                                  <span>
                                    {tokensConfig.lazycat_is_expired ? '已过期: ' : '到期: '}
                                    {formatTime(tokensConfig.lazycat_expires_at)}
                                    {tokensConfig.lazycat_seconds_left ? ` (${formatExpiry(tokensConfig.lazycat_expires_at, tokensConfig.lazycat_is_expired, tokensConfig.lazycat_seconds_left)})` : ''}
                                  </span>
                                ) : '私有家宽 NAT 凭据'}
                              </div>
                            </Col>

                            <Col xs={12} sm={6}>
                              <Statistic
                                title={<span style={{ color: '#64748b', fontSize: 13 }}>容器实例与端口</span>}
                                value={lzInstances.length}
                                suffix={<span style={{ fontSize: 13, color: '#64748b' }}>容器 / {lzInstances.reduce((sum, item) => sum + (item.port_count || 0), 0) || ports.length} 端口</span>}
                                valueStyle={{ color: '#0f172a', fontWeight: 700, fontSize: 22 }}
                              />
                              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>
                                同步于: {formatTime(refreshTimes['instances:lazycat'] || refreshTimes['instances:all'])}
                              </div>
                            </Col>

                            <Col xs={12} sm={6}>
                              <div style={{ color: '#64748b', fontSize: 13 }}>安全保障</div>
                              <div style={{ marginTop: 6 }}>
                                <Tag color="purple" style={{ border: '1px solid #ddd6fe', background: '#f5f3ff', color: '#6d28d9', padding: '3px 8px', borderRadius: 4, fontWeight: 600 }}>
                                  <SafetyCertificateOutlined style={{ marginRight: 4 }} />已锁定防销毁保护
                                </Tag>
                              </div>
                              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>硬编码物理防删</div>
                            </Col>

                            <Col xs={12} sm={6}>
                              <div style={{ color: '#64748b', fontSize: 13 }}>每日打卡中心</div>
                              <div style={{ marginTop: 6 }}>
                                <Button
                                  type="primary"
                                  icon={<CheckCircleOutlined />}
                                  loading={vendorCheckinLoading['lazycat']}
                                  onClick={() => handleSingleVendorCheckin('lazycat')}
                                  style={{
                                    background: '#7c3aed',
                                    borderColor: '#7c3aed',
                                    height: 32,
                                    fontWeight: 600,
                                    borderRadius: 6,
                                  }}
                                >
                                  {vendorCheckinStatus['lazycat'] ? '今日已打卡' : '立即打卡领积分'}
                                </Button>
                              </div>
                              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>
                                {vendorCheckinMsg['lazycat'] ? `反馈: ${vendorCheckinMsg['lazycat']}` : '每日签到赠送懒猫积分'}
                              </div>
                            </Col>
                          </Row>
                        </Card>

                        {/* 容器列表 */}
                        {lzInstances.length === 0 ? (
                          <Alert
                            message="暂未拉取到懒猫云容器"
                            description={
                              <div>
                                当前未拉取到名下懒猫云容器。请先前往凭据设置填写并保存 Token，随后点击下方同步。
                                <div style={{ marginTop: 8 }}>
                                  <Button size="small" type="primary" loading={vendorInstancesLoading['lazycat']} onClick={() => handleRefreshInstances('lazycat')}>
                                    立即同步懒猫云资产
                                  </Button>
                                </div>
                              </div>
                            }
                            type="info"
                            showIcon
                            style={{ borderRadius: 8, background: '#eff6ff', borderColor: '#bfdbfe' }}
                          />
                        ) : (
                          <Table
                            loading={vendorInstancesLoading['lazycat']}
                            dataSource={lzInstances}
                            rowKey="id"
                            onRow={(r) => ({
                              onClick: () => {
                                setLazycatTargetInstance(r)
                                setLazycatModalOpen(true)
                              },
                              style: { cursor: 'pointer' }
                            })}
                            pagination={{
                              pageSize: 10,
                              showTotal: (total) => `共 ${total} 台懒猫云容器 (动态同步)`,
                              showSizeChanger: true
                            }}
                            columns={[
                              {
                                title: '容器名称 / ID',
                                dataIndex: 'name',
                                key: 'name',
                                render: (t, r) => (
                                  <div>
                                    <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                                      <b style={{ color: '#2563eb' }}>{t}</b>
                                      {r.is_stale && (
                                        <Tooltip title={`离线缓存: ${r.stale_reason || 'Token 失效需重新登录'}`}>
                                          <Tag color="warning" style={{ fontSize: 11, cursor: 'pointer', margin: 0 }} onClick={(e) => {
                                            e.stopPropagation()
                                            setLoginTargetProvider('lazycat')
                                            setLoginModalOpen(true)
                                          }}>
                                            缓存
                                          </Tag>
                                        </Tooltip>
                                      )}
                                      <Tag color="purple" style={{ fontSize: 11, cursor: 'pointer' }}>管理/终端</Tag>
                                    </div>
                                    <div style={{ fontSize: 12, color: '#94a3b8' }}>ID: {r.id}</div>
                                  </div>
                                )
                              },
                              {
                                title: '区域机房',
                                dataIndex: 'region',
                                key: 'region',
                                render: (reg, r) => (
                                  <Space size={4} wrap>
                                    <Tag color="purple">{reg || 'NAT节点'}</Tag>
                                    {r.category && (
                                      <Tag color={r.category === '全球家宽' ? 'magenta' : 'blue'}>{r.category}</Tag>
                                    )}
                                  </Space>
                                )
                              },
                              { title: '内部映射出口', dataIndex: 'ip', key: 'ip', render: (ip) => <span style={{ fontFamily: 'monospace' }}>{ip || '动态内网IP'}</span> },
                              {
                                title: '硬件规格',
                                key: 'spec',
                                render: (_, r) => (
                                  <span>
                                    {r.cpu || r.memory ? `${r.cpu || '-'} / ${r.memory || '-'}` : (r.category || 'NAT 容器')}
                                  </span>
                                )
                              },
                              { title: '端口配额', key: 'ports', render: (_, r) => <span>{r.port_count || 0} / {r.port_limit || 10} 个</span> },
                              {
                                title: '月度流量',
                                key: 'traffic',
                                width: 170,
                                render: (_, r) => {
                                  const pct = getTrafficPercent(r)
                                  const color = getTrafficColor(pct)
                                  return (
                                    <div>
                                      <div style={{ fontSize: 12, display: 'flex', justifyContent: 'space-between', gap: 6, marginBottom: 2, whiteSpace: 'nowrap' }}>
                                        <span style={{ fontWeight: 600 }}>{r.traffic_used || '0 GB'}</span>
                                        <span style={{ color: '#64748b' }}>{r.traffic_total || '1000 GB'} ({pct}%)</span>
                                      </div>
                                      <Progress percent={pct} size="small" showInfo={false} strokeColor={color} />
                                      {r.traffic_reset_date && (
                                        <div style={{ fontSize: 10, color: '#94a3b8', marginTop: 2 }}>
                                          重置日: {r.traffic_reset_date}
                                        </div>
                                      )}
                                    </div>
                                  )
                                }
                              },
                              {
                                title: '操作',
                                key: 'action',
                                render: (_, r) => (
                                  <Space onClick={(e) => e.stopPropagation()}>
                                    <Button
                                      size="small"
                                      type="primary"
                                      icon={<CodeOutlined />}
                                      style={{ background: '#7c3aed', borderColor: '#7c3aed' }}
                                      onClick={() => {
                                        setLazycatTargetInstance(r)
                                        setLazycatModalOpen(true)
                                      }}
                                    >
                                      管理 / 终端
                                    </Button>
                                    <Button
                                      size="small"
                                      type="primary"
                                      ghost
                                      icon={<ApiOutlined />}
                                      onClick={() => {
                                        setSelectedInstId(r.id)
                                        loadLazyCatPorts(r.id)
                                        setContainerPortModalOpen(true)
                                      }}
                                    >
                                      NAT 端口 ({r.port_count || 0})
                                    </Button>
                                    <Button size="small" type="link" onClick={() => { setSelectedVps(r); setDetailDrawerOpen(true); }}>
                                      详情
                                    </Button>
                                  </Space>
                                )
                              }
                            ]}
                          />
                        )}
                      </div>
                    )
                  },
                  {
                    key: '2',
                    label: `官方新机商城 (${lzStore.length})`,
                    children: (
                      <Card
                        size="small"
                        title={
                          <Space>
                            <ShoppingOutlined style={{ color: '#7c3aed' }} />
                            <Text strong>LazyCat 官方直营在售规格 ({lzStore.length} 款真实套餐)</Text>
                          </Space>
                        }
                        extra={
                          <Space size={12}>
                            <Text type="secondary" style={{ fontSize: 13 }}>
                              <ClockCircleOutlined style={{ marginRight: 4 }} />
                              库存更新于: {formatTime(refreshTimes['store:lazycat'] || refreshTimes['store:all'])}
                            </Text>
                            <Button size="small" icon={<ReloadOutlined spin={vendorStoreLoading['lazycat']} />} onClick={() => handleRefreshStore('lazycat')}>
                              刷新商城
                            </Button>
                          </Space>
                        }
                        bordered={false}
                      >
                        <Table
                          size="small"
                          dataSource={lzStore}
                          rowKey="id"
                          pagination={{
                            defaultPageSize: 50,
                            pageSizeOptions: ['20', '50', '100', '200'],
                            showSizeChanger: true,
                            showTotal: (total, range) => `第 ${range[0]}-${range[1]} 项 · 共 ${total} 款规格`
                          }}
                          columns={[
                            { 
                              title: '套餐型号', 
                              dataIndex: 'name', 
                              key: 'name', 
                              sorter: (a, b) => (a.name || '').localeCompare(b.name || ''),
                              render: (t) => <span style={{ fontWeight: 600 }}>{t}</span> 
                            },
                            { 
                              title: '机房区域', 
                              dataIndex: 'location', 
                              key: 'location', 
                              width: 100, 
                              sorter: (a, b) => (a.location || '').localeCompare(b.location || ''),
                              render: (l) => <Tag color="purple">{l}</Tag> 
                            },
                            {
                              title: '硬件配置',
                              key: 'hardware',
                              width: 140,
                              sorter: (a, b) => ((parseInt(a.cpu || '1') || 1) * 10000 + (parseInt(a.memory || '0') || 0)) - ((parseInt(b.cpu || '1') || 1) * 10000 + (parseInt(b.memory || '0') || 0)),
                              render: (_, r) => (
                                <div>
                                  <Space size={4}>
                                    <Tag color="purple" style={{ margin: 0, fontWeight: 600, fontSize: 11, padding: '0 4px' }}>{r.cpu || '1C'}</Tag>
                                    <Tag color="cyan" style={{ margin: 0, fontWeight: 600, fontSize: 11, padding: '0 4px' }}>{r.memory || '512M'}</Tag>
                                  </Space>
                                  <div style={{ fontSize: 11, color: '#64748b', marginTop: 1 }}>
                                    {r.disk ? `${r.disk} 磁盘` : (r.config_desc || '-')}
                                  </div>
                                </div>
                              )
                            },
                            {
                              title: '网络配额',
                              key: 'network',
                              width: 150,
                              sorter: (a, b) => (parseInt(a.bandwidth || '0') || 0) - (parseInt(b.bandwidth || '0') || 0),
                              render: (_, r) => (
                                <div>
                                  <div style={{ color: '#059669', fontWeight: 600, fontSize: 12 }}>{r.bandwidth || '共享带宽'}</div>
                                  <div style={{ fontSize: 11, color: '#64748b' }}>
                                    {r.traffic_total ? `月流量: ${r.traffic_total}` : '按套餐配额'}
                                  </div>
                                </div>
                              )
                            },
                            { 
                              title: '资费价格', 
                              dataIndex: 'price', 
                              key: 'price', 
                              width: 110, 
                              sorter: (a, b) => (parseFloat(String(a.price).replace(/[^0-9.]/g, '')) || 0) - (parseFloat(String(b.price).replace(/[^0-9.]/g, '')) || 0),
                              render: (p) => <span style={{ color: '#059669', fontWeight: 700, fontFamily: 'monospace' }}>{p}</span> 
                            },
                            {
                              title: '库存状态',
                              dataIndex: 'in_stock',
                              key: 'in_stock',
                              width: 105,
                              sorter: (a, b) => (a.in_stock === b.in_stock ? 0 : a.in_stock ? -1 : 1),
                              render: (inStock: boolean) => inStock ? <Tag color="green" style={{ margin: 0 }}>有货</Tag> : <Tag color="default" style={{ margin: 0 }}>缺货</Tag>
                            },
                            {
                              title: '操作',
                              key: 'action',
                              width: 110,
                              render: (_, r) => (
                                <Button
                                  size="small"
                                  type="link"
                                  icon={<AimOutlined />}
                                  onClick={() => {
                                    planForm.setFieldsValue({
                                      name: `LazyCat ${r.name} 补货盯盘`,
                                      provider: 'all',
                                      market_type: 'new_stock',
                                      region_keyword: r.location,
                                      safe_mode: true,
                                      auto_buy: false,
                                      is_active: true
                                    })
                                    setEditingPlan(null)
                                    setPlanModalOpen(true)
                                  }}
                                >
                                  以此规格盯盘
                                </Button>
                              )
                            }
                          ]}
                        />
                      </Card>
                    )
                  },
                  {
                    key: '3',
                    label: `二手转让集市 (${lzMarketSearch.trim() ? `${filteredLzMarket.length}/${lzMarket.length}` : `${lzMarket.length}`})`,
                    children: (
                      <Card
                        size="small"
                        title={
                          <Space>
                            <SwapOutlined style={{ color: '#7c3aed' }} />
                            <Text strong>LazyCat 官方二手集市在售挂单 ({lzMarketSearch.trim() ? `已筛选 ${filteredLzMarket.length} / 共 ${lzMarket.length}` : `${lzMarket.length}`} 台)</Text>
                          </Space>
                        }
                        extra={
                          <Space size={12}>
                            <Input.Search
                              placeholder="搜索挂单标题 / 节点 / 备注..."
                              allowClear
                              size="small"
                              style={{ width: 260 }}
                              value={lzMarketSearch}
                              onChange={e => setLzMarketSearch(e.target.value)}
                            />
                            <Text type="secondary" style={{ fontSize: 13 }}>
                              <ClockCircleOutlined style={{ marginRight: 4 }} />
                              更新时间: {formatTime(refreshTimes['market:lazycat'])}
                            </Text>
                            <Button size="small" icon={<ReloadOutlined spin={vendorMarketLoading['lazycat']} />} onClick={() => handleRefreshMarket('lazycat')}>
                              刷新二手
                            </Button>
                          </Space>
                        }
                        bordered={false}
                      >
                        <Table
                          size="small"
                          dataSource={filteredLzMarket}
                          rowKey="id"
                          scroll={{ x: 'max-content' }}
                          pagination={{
                            defaultPageSize: 50,
                            pageSizeOptions: ['20', '50', '100', '200'],
                            showSizeChanger: true,
                            showTotal: (total, range) => `第 ${range[0]}-${range[1]} 项 · 共 ${total} 台挂单`
                          }}
                          columns={[
                            {
                              title: '挂单机器 / 节点',
                              dataIndex: 'name',
                              key: 'name',
                              sorter: (a, b) => (a.name || '').localeCompare(b.name || ''),
                              render: (name, r) => (
                                <div>
                                  <div style={{ fontWeight: 600, color: '#0f172a', fontSize: 13 }}>{name}</div>
                                  <div style={{ fontSize: 11, color: '#64748b' }}>
                                    {r.node_name && <Tag color="purple" style={{ fontSize: 10, padding: '0 4px', marginRight: 4 }}>{r.node_name}</Tag>}
                                    <span style={{ color: '#94a3b8' }}>ID: {r.id}</span>
                                  </div>
                                </div>
                              )
                            },
                            { 
                              title: '机房区域', 
                              dataIndex: 'location', 
                              key: 'location', 
                              width: 95, 
                              sorter: (a, b) => (a.location || '').localeCompare(b.location || ''),
                              render: (l) => <Tag color="purple" style={{ margin: 0 }}>{l || '全球'}</Tag> 
                            },
                            {
                              title: '硬件配置',
                              key: 'hardware',
                              width: 135,
                              sorter: (a, b) => ((parseInt(a.cpu || '1') || 1) * 10000 + (parseInt(a.memory || '0') || 0)) - ((parseInt(b.cpu || '1') || 1) * 10000 + (parseInt(b.memory || '0') || 0)),
                              render: (_, r) => (
                                <div>
                                  <Space size={4}>
                                    <Tag color="purple" style={{ margin: 0, fontWeight: 600, fontSize: 11, padding: '0 4px' }}>{r.cpu || '1C'}</Tag>
                                    <Tag color="cyan" style={{ margin: 0, fontWeight: 600, fontSize: 11, padding: '0 4px' }}>{r.memory || '512M'}</Tag>
                                  </Space>
                                  <div style={{ fontSize: 11, color: '#64748b', marginTop: 1 }}>
                                    {r.disk ? `${r.disk} 磁盘` : (r.config_desc || '-')}
                                  </div>
                                </div>
                              )
                            },
                            {
                              title: '网络配额',
                              key: 'network',
                              width: 155,
                              sorter: (a, b) => (parseInt(a.bandwidth || '0') || 0) - (parseInt(b.bandwidth || '0') || 0),
                              render: (_, r) => {
                                let flowDisplay = r.flow_used
                                if (flowDisplay && /^\d+$/.test(flowDisplay)) {
                                  flowDisplay = fmtBytes(flowDisplay)
                                }
                                return (
                                  <div>
                                    <div style={{ color: '#059669', fontWeight: 600, fontSize: 12 }}>
                                      {r.bandwidth || '1000 Mbps'}
                                    </div>
                                    <div style={{ fontSize: 11, color: '#64748b' }}>
                                      {flowDisplay ? (
                                        <span>已用: {flowDisplay}</span>
                                      ) : r.traffic_total ? (
                                        <span>月流量: {r.traffic_total}</span>
                                      ) : null}
                                    </div>
                                    {r.provider_id !== 'lazycat' && r.reset_price !== undefined && r.reset_price > 0 && (
                                      <div style={{ fontSize: 10, color: '#d97706' }}>
                                        重置: ¥{r.reset_price.toFixed(2)}/次
                                      </div>
                                    )}
                                  </div>
                                )
                              }
                            },
                            {
                              title: '转让标价',
                              dataIndex: 'price',
                              key: 'price',
                              width: 105,
                              sorter: (a, b) => (a.price || 0) - (b.price || 0),
                              render: (p: number, r) => (
                                <div>
                                  <span style={{ color: '#ef4444', fontWeight: 700, fontSize: 14, fontFamily: 'monospace' }}>¥{p.toFixed(2)}</span>
                                  {r.bargain_tag && (
                                    <div style={{ marginTop: 1 }}>
                                      <Tag color="volcano" style={{ fontSize: 10, padding: '0 3px', margin: 0 }}>{r.bargain_tag}</Tag>
                                    </div>
                                  )}
                                </div>
                              )
                            },
                            {
                              title: '续费周期 / 价格',
                              key: 'renewal',
                              width: 180,
                              sorter: (a, b) => (a.monthly_price || a.renewal_price || 0) - (b.monthly_price || b.renewal_price || 0),
                              filters: [
                                { text: '年付', value: 'yearly' },
                                { text: '月付', value: 'monthly' },
                                { text: '季付/半年付', value: 'other' }
                              ],
                              onFilter: (value, r) => {
                                const c = r.cycle || 1
                                if (value === 'yearly') return c >= 12
                                if (value === 'monthly') return c === 1
                                if (value === 'other') return c > 1 && c < 12
                                return true
                              },
                              render: (_, r) => renderRenewalDisplay(r)
                            },
                            {
                              title: 'SLA & 端口配额',
                              key: 'sla',
                              width: 130,
                              sorter: (a, b) => (a.port_limit || 0) - (b.port_limit || 0),
                              render: (_, r) => (
                                <Space direction="vertical" size={2}>
                                  <Tag color="green" style={{ fontWeight: 600, fontSize: 10, margin: 0 }}>SLA: {r.sla || '99.9%'}</Tag>
                                  <Tag color="purple" style={{ fontSize: 10, margin: 0 }}>
                                    NAT: {r.port_limit ? `${r.port_limit} 端口` : '10 端口'}
                                  </Tag>
                                </Space>
                              )
                            },
                            {
                              title: '卖家信息',
                              key: 'seller',
                              width: 130,
                              sorter: (a, b) => (a.seller_username || '').localeCompare(b.seller_username || ''),
                              render: (_, r) => (
                                <div style={{ fontSize: 11 }}>
                                  {r.seller_username ? (
                                    <div>
                                      <span style={{ color: '#0f172a', fontWeight: 600 }}>{r.seller_username}</span>
                                      {r.seller_user_id ? <div style={{ fontSize: 10, color: '#94a3b8' }}>UID: {r.seller_user_id}</div> : null}
                                    </div>
                                  ) : (
                                    <span style={{ color: '#94a3b8' }}>官方</span>
                                  )}
                                </div>
                              )
                            },
                            {
                              title: '操作',
                              key: 'action',
                              width: 135,
                              render: (_, r) => (
                                <Space size={4}>
                                  <Button
                                    size="small"
                                    type="primary"
                                    ghost
                                    icon={<AimOutlined />}
                                    onClick={() => handleCreatePlanFromMarket(r)}
                                  >
                                    盯盘
                                  </Button>
                                  <Popconfirm
                                    title={`确认以 ¥${r.price.toFixed(2)} 购买【${r.name}】？`}
                                    description="将调用懒猫云 API 从账户余额扣款购买！"
                                    onConfirm={() => handleDirectBuy('lazycat', r.id, r.name)}
                                    okText="立即购买"
                                    cancelText="取消"
                                    okButtonProps={{ danger: true }}
                                  >
                                    <Button
                                      size="small"
                                      type="primary"
                                      danger
                                      icon={<ThunderboltOutlined />}
                                    >
                                      购买
                                    </Button>
                                  </Popconfirm>
                                </Space>
                              )
                            }
                          ]}
                        />
                      </Card>
                    )
                  }
                ]}
              />
            </Card>
          </Space>
        )
      }

      case 'audit': {
        return (
          <Card
            title={
              <Space>
                <SafetyCertificateOutlined style={{ color: '#2563eb' }} />
                <Text strong>过去 30 天 AI 智能体与自动化安全审计日志</Text>
              </Space>
            }
            bordered={false}
            className="vps-card"
          >
            <Table
              dataSource={auditLogs}
              rowKey="id"
              pagination={{ pageSize: 15 }}
              columns={[
                {
                  title: '事件时间',
                  dataIndex: 'timestamp',
                  key: 'ts',
                  width: 190,
                  render: (t: string) => <span style={{ fontFamily: 'monospace', fontSize: 12, whiteSpace: 'nowrap' }}>{formatFullTime(t)}</span>
                },
                {
                  title: '触发来源',
                  dataIndex: 'source',
                  key: 'src',
                  width: 110,
                  render: (s: string, r: AuditEntry) => {
                    // 仅失败事件标红，成功的安全/运维动作按来源区分颜色
                    if (r.status !== 'SUCCESS') return <Tag color="red">{s}</Tag>
                    const color = s === 'security' ? 'geekblue' : s === 'sniper' ? 'purple' : s === 'rest_api' ? 'blue' : 'green'
                    return <Tag color={color}>{s}</Tag>
                  }
                },
                { title: '操作动作', dataIndex: 'action', key: 'act', width: 140 },
                { title: '执行详情说明', dataIndex: 'message', key: 'msg' },
                {
                  title: '状态',
                  dataIndex: 'status',
                  key: 'st',
                  width: 90,
                  render: (st) => st === 'SUCCESS' ? <Tag color="success">SUCCESS</Tag> : <Tag color="error">FAILED</Tag>
                }
              ]}
            />
          </Card>
        )
      }

      case 'settings': {
        return (
          <Space direction="vertical" size={20} style={{ width: '100%' }}>
            {unraidStatus && (
              <Card
                title={
                  <Space>
                    <CloudServerOutlined style={{ color: '#2563eb' }} />
                    <Text strong>Unraid 本地生态容器探测诊断</Text>
                  </Space>
                }
                bordered={false}
                className="vps-card"
              >
                {unraidStatus.credentials_found?.length > 0 && (
                  <div style={{ marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                    <Text type="secondary" style={{ fontSize: 12 }}>已发现持久化凭据文件:</Text>
                    {unraidStatus.credentials_found.map((c) => (
                      <Tag key={c} color="cyan" style={{ margin: 0, fontFamily: 'monospace', fontSize: 11 }}>{c}</Tag>
                    ))}
                  </div>
                )}
                <Table
                  dataSource={unraidStatus.containers}
                  rowKey="name"
                  pagination={false}
                  size="small"
                  columns={[
                    { title: '容器服务', dataIndex: 'name', key: 'name', render: (t) => <b>{t}</b> },
                    { title: '内网地址', dataIndex: 'target', key: 'target', render: (u) => <span style={{ fontFamily: 'monospace' }}>{u}</span> },
                    {
                      title: '连通性',
                      dataIndex: 'online',
                      key: 'online',
                      render: (on) => on ? <Badge status="success" text="在线" /> : <Badge status="error" text="离线" />
                    },
                    { title: '响应延迟', dataIndex: 'latency_ms', key: 'latency', render: (l) => `${l} ms` },
                    { title: '探测详情', dataIndex: 'detail', key: 'detail', render: (d) => <span style={{ color: '#64748b' }}>{d}</span> }
                  ]}
                />
              </Card>
            )}

            <Card
              title={
                <Space>
                  <SettingOutlined style={{ color: '#2563eb' }} />
                  <Text strong>云厂商 API 凭据与网络代理配置</Text>
                </Space>
              }
              extra={
                <Space wrap>
                  <Button
                    type="primary"
                    size="small"
                    icon={<LoginOutlined />}
                    onClick={() => { setLoginTargetProvider('akile'); setLoginModalOpen(true); }}
                  >
                    Akile 一键登录
                  </Button>
                  <Button
                    type="primary"
                    size="small"
                    icon={<LoginOutlined />}
                    style={{ background: '#059669', borderColor: '#059669' }}
                    onClick={() => { setLoginTargetProvider('lazycat'); setLoginModalOpen(true); }}
                  >
                    懒猫云一键登录
                  </Button>
                  <Button
                    size="small"
                    icon={<LoginOutlined />}
                    onClick={() => { setLoginTargetProvider('acck'); setLoginModalOpen(true); }}
                  >
                    ACCK 一键登录
                  </Button>
                </Space>
              }
              bordered={false}
              className="vps-card"
            >
              <Form
                form={tokensForm}
                layout="vertical"
                onFinish={handleSaveTokens}
                style={{ overflowX: 'hidden' }}
              >
                <Row gutter={24}>
                  <Col xs={24} md={12}>
                    <Form.Item
                      name="akile_token"
                      label={
                        <Space>
                          <span>Akile Cloud API Token</span>
                          {tokensConfig?.akile_configured && (
                            <Tag color={tokensConfig.akile_is_expired ? 'error' : 'green'}>
                              {tokensConfig.akile_is_expired ? '已过期' : '已配置'}
                            </Tag>
                          )}
                          {tokensConfig?.akile_auto_refresh && (
                            <Tag color="blue">自动续期中</Tag>
                          )}
                        </Space>
                      }
                      extra={
                        <div style={{ marginTop: 4 }}>
                          <span style={{ color: '#64748b' }}>用于拉取名下 Akile 节点、执行每日签到领 AK 币。</span>
                          {tokensConfig?.akile_expires_at && (
                            <div style={{ marginTop: 2, fontSize: 12, color: tokensConfig.akile_is_expired ? '#ef4444' : '#059669' }}>
                              到期时间: {tokensConfig.akile_expires_at} ({formatExpiry(tokensConfig.akile_expires_at, tokensConfig.akile_is_expired, tokensConfig.akile_seconds_left)})
                            </div>
                          )}
                          <div>
                            <Button
                              type="link"
                              size="small"
                              icon={<LoginOutlined />}
                              style={{ padding: 0, marginTop: 2 }}
                              onClick={() => { setLoginTargetProvider('akile'); setLoginModalOpen(true); }}
                            >
                              账号密码一键登录 / 2FA / 免密自动续期 &gt;
                            </Button>
                          </div>
                        </div>
                      }
                    >
                      <Input.Password placeholder={tokensConfig?.akile_token_masked ? `当前已保存: ${tokensConfig.akile_token_masked} (留空不修改)` : "输入 Akile Token (留空则不修改)"} />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={12}>
                    <Form.Item
                      name="acck_token"
                      label={
                        <Space>
                          <span>ACCK (acck.io) API Token</span>
                          {tokensConfig?.acck_configured && (
                            <Tag color={tokensConfig.acck_is_expired ? 'error' : 'green'}>
                              {tokensConfig.acck_is_expired ? '已过期' : '已配置'}
                            </Tag>
                          )}
                          {tokensConfig?.acck_auto_refresh && (
                            <Tag color="blue">自动续期中</Tag>
                          )}
                        </Space>
                      }
                      extra={
                        <div style={{ marginTop: 4 }}>
                          <span style={{ color: '#64748b' }}>用于拉取名下主机、查询余额与每日签到领 AC 币。</span>
                          {tokensConfig?.acck_expires_at && (
                            <div style={{ marginTop: 2, fontSize: 12, color: tokensConfig.acck_is_expired ? '#ef4444' : '#059669' }}>
                              到期时间: {tokensConfig.acck_expires_at} ({formatExpiry(tokensConfig.acck_expires_at, tokensConfig.acck_is_expired, tokensConfig.acck_seconds_left)})
                            </div>
                          )}
                          <div>
                            <Button
                              type="link"
                              size="small"
                              icon={<LoginOutlined />}
                              style={{ padding: 0, marginTop: 2 }}
                              onClick={() => { setLoginTargetProvider('acck'); setLoginModalOpen(true); }}
                            >
                              账号密码一键登录 / 2FA / 免密自动续期 &gt;
                            </Button>
                          </div>
                        </div>
                      }
                    >
                      <Input.Password placeholder={tokensConfig?.acck_token_masked ? `当前已保存: ${tokensConfig.acck_token_masked} (留空不修改)` : "输入 ACCK Token (留空则不修改)"} />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={12}>
                    <Form.Item
                      name="lazycat_token"
                      label={
                        <Space>
                          <span>懒猫云 (LazyCat) API Token</span>
                          {tokensConfig?.lazycat_configured && (
                            <Tag color={tokensConfig.lazycat_is_expired ? 'error' : 'green'}>
                              {tokensConfig.lazycat_is_expired ? '已过期' : '已配置'}
                            </Tag>
                          )}
                          {tokensConfig?.lazycat_auto_refresh && (
                            <Tag color="blue">自动重登中</Tag>
                          )}
                        </Space>
                      }
                      extra={
                        <div style={{ marginTop: 4 }}>
                          <span style={{ color: '#64748b' }}>用于同步家宽容器、管理端口映射与每日签到。</span>
                          {tokensConfig?.lazycat_expires_at && (
                            <div style={{ marginTop: 2, fontSize: 12, color: tokensConfig.lazycat_is_expired ? '#ef4444' : '#059669' }}>
                              到期时间: {tokensConfig.lazycat_expires_at} ({formatExpiry(tokensConfig.lazycat_expires_at, tokensConfig.lazycat_is_expired, tokensConfig.lazycat_seconds_left)})
                            </div>
                          )}
                          <div>
                            <Button
                              type="link"
                              size="small"
                              icon={<LoginOutlined />}
                              style={{ padding: 0, marginTop: 2 }}
                              onClick={() => { setLoginTargetProvider('lazycat'); setLoginModalOpen(true); }}
                            >
                              账号密码一键登录 / 2FA / 免密自动重登 &gt;
                            </Button>
                          </div>
                        </div>
                      }
                    >
                      <Input.Password placeholder={tokensConfig?.lazycat_token_masked ? `当前已保存: ${tokensConfig.lazycat_token_masked} (留空不修改)` : "输入懒猫云 Token (留空则不修改)"} />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={12}>
                    <Form.Item
                      name="proxy_url"
                      label="网络出口代理 (Proxy URL)"
                      extra="建议配置本地 Clash/Sing-box HTTP 代理 (如 http://192.168.8.11:7890)，防止国内直连 Cloudflare WAF 403 封禁"
                    >
                      <Input allowClear placeholder="http://192.168.8.11:7890" />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={12}>
                    <Form.Item
                      name="lazycat_proxy_url"
                      label="懒猫云专用代理 (LazyCat Proxy URL)"
                      extra="可选：为懒猫云 API 与 WebShell PTY 单独指定独立稳定代理 (如 http://192.168.8.11:33000)，留空则继承全局代理"
                    >
                      <Input allowClear placeholder={tokensConfig?.lazycat_effective_proxy_url ? `留空继承全局: ${tokensConfig.lazycat_effective_proxy_url}` : '留空继承全局代理，例如 http://192.168.8.11:33000'} />
                    </Form.Item>
                  </Col>
                </Row>
                <Button type="primary" htmlType="submit" loading={savingTokens} style={{ fontWeight: 600 }}>
                  保存并立即热加载凭据
                </Button>
              </Form>
            </Card>

            <Card
              title={
                <Space>
                  <LockOutlined style={{ color: '#2563eb' }} />
                  <Text strong>管理员登录密码变更</Text>
                </Space>
              }
              bordered={false}
              className="vps-card"
              style={{ maxWidth: 500 }}
            >
              <Form
                layout="vertical"
                onFinish={async (values) => {
                  try {
                    await api.changePassword(values.old_password, values.new_password)
                    message.success('密码修改成功，请重新登录')
                    clearToken()
                    setAuthed(false)
                  } catch (e: any) {
                    message.error('修改密码失败: ' + (e.response?.data?.error || e.message))
                  }
                }}
              >
                <Form.Item name="old_password" label="当前原密码" rules={[{ required: true }]}>
                  <Input.Password placeholder="原管理员密码" />
                </Form.Item>
                <Form.Item name="new_password" label="新密码" rules={[{ required: true, min: 5, message: '密码至少 5 位' }]}>
                  <Input.Password placeholder="新密码" />
                </Form.Item>
                <Button type="primary" danger htmlType="submit">
                  更新管理员密码
                </Button>
              </Form>
            </Card>
          </Space>
        )
      }

      default:
        return <div>页面开发中...</div>
    }
  }

  return (
    <ConfigProvider locale={zhCN} theme={{
      algorithm: theme.defaultAlgorithm,
      token: {
        colorPrimary: '#2563eb',
        colorBgBase: '#ffffff',
        colorTextBase: '#0f172a',
        borderRadius: 8,
        fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
      }
    }}>
      <Layout style={{ minHeight: '100vh', background: '#f8fafc' }}>
        <Sider
          collapsible
          collapsed={collapsed}
          onCollapse={setCollapsed}
          theme="light"
          style={{
            borderRight: '1px solid #e2e8f0',
            background: '#ffffff',
          }}
        >
          <div style={{
            height: 64,
            display: 'flex',
            alignItems: 'center',
            justifyContent: collapsed ? 'center' : 'flex-start',
            padding: collapsed ? '0' : '0 20px',
            borderBottom: '1px solid #e2e8f0',
          }}>
            <SafetyCertificateOutlined style={{ color: '#2563eb', fontSize: 20 }} />
            {!collapsed && (
              <span style={{ fontWeight: 700, fontSize: 16, color: '#0f172a', marginLeft: 10 }}>
                VPS Monitor
              </span>
            )}
          </div>

          <Menu
            mode="inline"
            selectedKeys={[page]}
            onClick={({ key }) => setPage(key)}
            style={{ borderRight: 0, marginTop: 8 }}
            items={[
              { key: 'overview', icon: <DashboardOutlined />, label: '全网资产看板' },
              { key: 'plans', icon: <AimOutlined />, label: '智能抢购中心' },
              { key: 'akile', icon: <ThunderboltOutlined />, label: 'Akile 专区' },
              { key: 'acck', icon: <RocketOutlined />, label: 'ACCK 专区' },
              { key: 'lazycat', icon: <CloudServerOutlined />, label: '懒猫云专区' },
              { key: 'audit', icon: <SafetyCertificateOutlined />, label: '安全审计中心' },
              { key: 'settings', icon: <SettingOutlined />, label: '系统与凭据' },
            ]}
          />
        </Sider>

        <Layout style={{ background: '#f8fafc' }}>
          <Header style={{
            background: '#ffffff',
            borderBottom: '1px solid #e2e8f0',
            padding: '0 24px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            height: 64,
          }}>
            {/* 顶栏左侧：清晰的面包屑与副标题 */}
            <Space size={10} align="baseline">
              <Text strong style={{ fontSize: 16, color: '#0f172a' }}>
                {pageTitleMap[page] || '控制台'}
              </Text>
              <Text type="secondary" style={{ fontSize: 12, color: '#94a3b8' }}>
                / {pageSubtitleMap[page] || ''}
              </Text>
            </Space>

            {/* 顶栏右侧：统一巡检状态指示器、全网一键刷新、用户下拉 */}
            <Space size={16} align="center">
              {/* 调度状态胶囊 */}
              {schedulerStatus?.mode === 'idle' && (
                <Badge
                  status="default"
                  text={<span style={{ fontSize: 12, color: '#64748b' }}>巡检引擎休眠 (无活跃计划)</span>}
                />
              )}
              {schedulerStatus?.mode === 'steady' && (
                <Badge
                  status="processing"
                  text={
                    <span style={{ fontSize: 12, color: '#2563eb' }}>
                      智能巡检中 ({schedulerStatus.active_plans_count}条计划 · {schedulerStatus.current_interval_sec}s)
                    </span>
                  }
                />
              )}
              {schedulerStatus?.mode === 'burst' && (
                <Badge
                  status="error"
                  text={
                    <span style={{ fontSize: 12, color: '#ef4444', fontWeight: 600 }}>
                      极速突发中 ({schedulerStatus.burst_remaining_seconds}s 倒计时)
                    </span>
                  }
                />
              )}

              {/* 全网一键刷新 */}
              <Tooltip title="全网资产与状态一键刷新">
                <Button
                  type="text"
                  shape="circle"
                  icon={<ReloadOutlined spin={loading} style={{ color: '#64748b', fontSize: 15 }} />}
                  onClick={loadData}
                />
              </Tooltip>

              {/* 管理员菜单 */}
              <Dropdown
                menu={{
                  items: [
                    {
                      key: 'settings',
                      icon: <SettingOutlined />,
                      label: '系统与凭据配置',
                      onClick: () => setPage('settings'),
                    },
                    {
                      type: 'divider',
                    },
                    {
                      key: 'logout',
                      icon: <LogoutOutlined />,
                      label: '退出登录',
                      danger: true,
                      onClick: () => {
                        clearToken()
                        setAuthed(false)
                      },
                    },
                  ],
                }}
                placement="bottomRight"
              >
                <div style={{
                  cursor: 'pointer',
                  padding: '0 10px',
                  height: 36,
                  lineHeight: 'normal',
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 8,
                  borderRadius: 6,
                  background: '#f8fafc',
                  border: '1px solid #e2e8f0',
                  boxSizing: 'border-box',
                }}>
                  <Avatar size={24} style={{ backgroundColor: '#2563eb', fontSize: 12, lineHeight: '24px' }}>A</Avatar>
                  <span style={{ fontSize: 13, fontWeight: 500, color: '#0f172a', lineHeight: '20px' }}>管理员</span>
                </div>
              </Dropdown>
            </Space>
          </Header>

          <Content style={{ padding: 24, minHeight: 'calc(100vh - 64px)' }}>
            {renderContent()}
          </Content>
        </Layout>
      </Layout>

      {/* VPS 详情抽屉 */}
      {renderVpsDetailDrawer()}

      {/* 懒猫云新增端口 Modal */}
      <Modal
        title="新增 NAT 动态端口映射"
        open={portModalOpen}
        onCancel={() => setPortModalOpen(false)}
        onOk={async () => {
          try {
            const vals = await portForm.validateFields()
            await api.addLazyCatPort(selectedInstId, vals.private_port, vals.protocol, vals.remark || '')
            message.success('端口映射添加成功')
            setPortModalOpen(false)
            loadLazyCatPorts(selectedInstId)
          } catch (e: any) {
            message.error('新增端口失败: ' + (e.message || '参数校验错误'))
          }
        }}
      >
        <Form form={portForm} layout="vertical">
          <Form.Item name="protocol" label="传输协议" rules={[{ required: true }]}>
            <Select>
              <Select.Option value="tcp">TCP</Select.Option>
              <Select.Option value="udp">UDP</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="private_port" label="容器内部私有端口" rules={[{ required: true, type: 'number', min: 1, max: 65535 }]}>
            <InputNumber style={{ width: '100%' }} placeholder="例如: 8080" />
          </Form.Item>
          <Form.Item name="remark" label="备注说明">
            <Input placeholder="例如: WebUI / SSH / Proxy" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 懒猫云特定容器 NAT 端口映射管理 Modal */}
      <Modal
        title={
          <Space>
            <ApiOutlined style={{ color: '#7c3aed' }} />
            <span>容器 NAT 动态端口映射管理 (容器 ID: {selectedInstId})</span>
          </Space>
        }
        open={containerPortModalOpen}
        onCancel={() => setContainerPortModalOpen(false)}
        footer={[
          <Button key="close" onClick={() => setContainerPortModalOpen(false)}>
            关闭
          </Button>,
          <Button
            key="add"
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => {
              portForm.resetFields()
              portForm.setFieldsValue({ protocol: 'tcp', private_port: 8080 })
              setPortModalOpen(true)
            }}
            style={{ background: '#7c3aed', borderColor: '#7c3aed' }}
          >
            新增动态端口映射
          </Button>
        ]}
        width={760}
      >
        <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Text type="secondary">
            公网动态随机端口与容器内私有端口双向绑定；配置即时生效。
          </Text>
          <Button size="small" icon={<ReloadOutlined spin={portsLoading} />} onClick={() => loadLazyCatPorts(selectedInstId)}>
            刷新端口
          </Button>
        </div>
        <Table
          dataSource={ports}
          rowKey="id"
          loading={portsLoading}
          pagination={{ pageSize: 5, showTotal: (t) => `共 ${t} 个映射规则` }}
          columns={[
            { title: '映射 ID', dataIndex: 'id', key: 'id', width: 90 },
            {
              title: '协议',
              dataIndex: 'protocol',
              key: 'protocol',
              width: 80,
              render: (p) => <Tag color={p === 'tcp' ? 'blue' : 'orange'}>{p.toUpperCase()}</Tag>
            },
            {
              title: '容器内私有端口',
              dataIndex: 'private_port',
              key: 'private_port',
              render: (p) => <span style={{ fontFamily: 'monospace', fontWeight: 600 }}>{p}</span>
            },
            {
              title: '公网随机分配端口',
              dataIndex: 'public_port',
              key: 'public_port',
              render: (p) => <span style={{ fontFamily: 'monospace', fontWeight: 700, color: '#7c3aed' }}>{p}</span>
            },
            { title: '备注说明', dataIndex: 'remark', key: 'remark' },
            {
              title: '操作',
              key: 'action',
              width: 90,
              render: (_, r) => (
                <Popconfirm
                  title="确认释放此端口映射？"
                  onConfirm={async () => {
                    try {
                      await api.deleteLazyCatPort(selectedInstId, r.id)
                      message.success('端口映射已释放')
                      loadLazyCatPorts(selectedInstId)
                    } catch (e: any) {
                      message.error('释放端口失败: ' + (e.message || '未知错误'))
                    }
                  }}
                  okText="释放"
                  cancelText="取消"
                  okButtonProps={{ danger: true }}
                >
                  <Button size="small" type="link" danger icon={<DeleteOutlined />}>
                    释放
                  </Button>
                </Popconfirm>
              )
            }
          ]}
        />
      </Modal>

      {/* 购买计划 新增/编辑 Modal */}
      <Modal
        title={editingPlan ? '编辑 VPS 智能抢购计划' : '新建 VPS 智能抢购计划'}
        open={planModalOpen}
        onCancel={() => {
          setPlanModalOpen(false)
          setEditingPlan(null)
          planForm.resetFields()
        }}
        onOk={handleSavePlan}
        okText="保存计划"
        cancelText="取消"
        width={620}
      >
        <Form form={planForm} layout="vertical">
          <Form.Item name="name" label="计划名称" rules={[{ required: true, message: '请输入计划名称' }]}>
            <Input placeholder="例如: 懒猫云3折超低价家宽捡漏 / 香港大带宽低价机" />
          </Form.Item>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="provider" label="监控平台" rules={[{ required: true }]}>
                <Select>
                  <Select.Option value="all">全网厂商聚合监控 (懒猫云/Akile/ACCK)</Select.Option>
                  <Select.Option value="lazycat">懒猫云 (全球家宽 / NAT 容器)</Select.Option>
                  <Select.Option value="akile">Akile Cloud</Select.Option>
                  <Select.Option value="acck">ACCK (acck.io)</Select.Option>
                </Select>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="market_type" label="市场类型" rules={[{ required: true }]}>
                <Select>
                  <Select.Option value="secondhand">二手流转交易集市</Select.Option>
                  <Select.Option value="new_stock">官方直营补货库存</Select.Option>
                  <Select.Option value="all">全部市场 (二手+新机)</Select.Option>
                </Select>
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="max_price" label="最高售价限价 (元)" extra="挂单售价高于此值自动忽略 (0不限)">
                <InputNumber style={{ width: '100%' }} min={0} placeholder="例如: 50.0" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="max_renewal_price" label="最高月续费限价 (元/月)" extra="按折合月供比对，防止高额续费刺客 (0不限)">
                <InputNumber style={{ width: '100%' }} min={0} placeholder="例如: 15.0" />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="cycle_filter" label="计费周期偏好" extra="筛选年付/月付/季付挂单 (默认不限)">
                <Select>
                  <Select.Option value="all">全部周期 (不限)</Select.Option>
                  <Select.Option value="yearly">仅限年付 (≥12个月)</Select.Option>
                  <Select.Option value="monthly">仅限月付 (1个月)</Select.Option>
                  <Select.Option value="other">仅限季付/半年付 (3~6个月)</Select.Option>
                </Select>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="min_discount_percent" label="最低续费折扣 (折算百分比)" extra="例如填 30 表示 ≤ 3折/30% (0不限)">
                <InputNumber style={{ width: '100%' }} min={0} max={100} placeholder="例如: 30" />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="min_port_count" label="最低端口配额限制" extra="例如懒猫云 NAT 容器要求端口数 (0不限)">
                <InputNumber style={{ width: '100%' }} min={0} placeholder="例如: 20" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="min_bandwidth" label="最低网络带宽要求 (Mbps)" extra="低于此带宽自动忽略 (0不限)">
                <InputNumber style={{ width: '100%' }} min={0} placeholder="例如: 500" />
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="region_keyword" label="机房/国家区域过滤" extra="如: 香港、HK、日本、JP、LAX (留空不限)">
                <Input placeholder="例如: 香港 或 HK" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="keyword_filter" label="标题/节点/卖家关键字过滤" extra="如: 移动 / 联通 / BGP / 超大杯 (留空不限)">
                <Input placeholder="例如: 移动 或 特惠" />
              </Form.Item>
            </Col>
          </Row>

          <div style={{ background: '#f8fafc', padding: '14px 18px', borderRadius: 8, border: '1px solid #e2e8f0', marginTop: 8 }}>
            <Row gutter={16} align="middle">
              <Col span={12}>
                <Form.Item name="auto_buy" valuePropName="checked" label="实盘秒杀模式 (自动下单扣款)" extra="默认关闭(模拟筛选模式)。开启后匹配将直接扣除余额买入！">
                  <Switch checkedChildren="实盘扣费" unCheckedChildren="模拟筛选" />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item name="max_buy_count" label="最大购买台数上限" extra="实盘秒杀成功买满此数量后自动转为模拟筛选">
                  <InputNumber style={{ width: '100%' }} min={1} max={10} defaultValue={1} />
                </Form.Item>
              </Col>
            </Row>

            <Row gutter={16} align="middle" style={{ marginTop: 8 }}>
              <Col span={12}>
                <Form.Item name="safe_mode" valuePropName="checked" label="安全告警模式" extra="开启后每次匹配均发送 Telegram 告警通知">
                  <Switch defaultChecked />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item name="is_active" valuePropName="checked" label="计划启用状态" extra="关闭后暂停盯盘，引擎不消耗 API">
                  <Switch defaultChecked />
                </Form.Item>
              </Col>
            </Row>
          </div>
        </Form>
      </Modal>

      {/* 懒猫云专属交互式控制中心 Modal (WebShell + NAT 端口映射 + Reality 对照矩阵 + 母鸡遥测) */}
      <LazyCatControlCenterModal
        open={lazycatModalOpen}
        onClose={() => setLazycatModalOpen(false)}
        instance={lazycatTargetInstance}
        onRefresh={() => handleRefreshInstances('lazycat')}
      />

      {/* 厂商一键登录 / 凭据录入 / 自动续期 Modal */}
      <ProviderLoginModal
        open={loginModalOpen}
        onClose={() => setLoginModalOpen(false)}
        provider={loginTargetProvider}
        onSuccess={() => {
          loadData()
        }}
      />
    </ConfigProvider>
  )
}
