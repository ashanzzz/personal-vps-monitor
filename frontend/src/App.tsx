import { useCallback, useEffect, useState } from 'react'
import {
  App as AntApp, Badge, Button, ConfigProvider, Layout, Menu, message as antMessage,
  Spin, Typography, Tooltip
} from 'antd'
import {
  DashboardOutlined, SwapOutlined, ShoppingCartOutlined,
  CreditCardOutlined, BellOutlined, SettingOutlined, LogoutOutlined,
  FileTextOutlined, WalletOutlined, SafetyCertificateOutlined,
  ReloadOutlined, ThunderboltOutlined, SyncOutlined, CloudServerOutlined
} from '@ant-design/icons'

import { api, clearToken, getToken, setUnauthorizedHandler } from './api'
import type { AlertEvent, AppSettings, VersionInfo, RefreshState } from './api'
import { useWebSocket } from './useWebSocket'
import { UnifiedDashboard } from './pages/UnifiedDashboard'
import { UnifiedStorePage } from './pages/UnifiedStorePage'
import { UnifiedSecondhandPage } from './pages/UnifiedSecondhandPage'
import { UnifiedBillingPage } from './pages/UnifiedBillingPage'
import { AccountPage } from './pages/AccountPage'
import { AcckAccountPage } from './pages/AcckAccountPage'
import { LazyCatAccountPage } from './pages/LazyCatAccountPage'
import { AlertHistory } from './pages/AlertHistory'
import { SettingsPage } from './pages/SettingsPage'
import { LoginPage } from './pages/LoginPage'
import { LogsPage } from './pages/LogsPage'

import './App.css'

const { Header, Sider, Content } = Layout
const { Text } = Typography

type PageKey =
  | 'overview'
  | 'store'
  | 'secondhand'
  | 'billing'
  | 'account'
  | 'acck'
  | 'lazycat'
  | 'alerts'
  | 'logs'
  | 'settings'

export default function App() {
  const [authed, setAuthed] = useState<boolean>(!!getToken())
  const [page, setPage] = useState<PageKey>('overview')
  const [events, setEvents] = useState<AlertEvent[]>([])
  const [settings, setSettings] = useState<AppSettings | null>(null)
  const [loading, setLoading] = useState(true)
  const [newAlerts, setNewAlerts] = useState(0)
  const [collapsed, setCollapsed] = useState(false)
  const [version, setVersion] = useState<VersionInfo | null>(null)
  const [refreshState, setRefreshState] = useState<RefreshState | null>(null)

  // Bounce back to login on any 401.
  useEffect(() => {
    setUnauthorizedHandler(() => setAuthed(false))
  }, [])

  const loadAll = useCallback(async () => {
    try {
      const [e, s, refState] = await Promise.all([
        api.getEvents({ limit: 200 }).catch(() => []),
        api.getSettings().catch(() => null),
        api.getRefreshStatus().catch(() => null),
      ])
      setEvents(e)
      setSettings(s)
      if (refState) setRefreshState(refState)
      api.getVersion().then(setVersion).catch(() => {})
    } catch {
      antMessage.error('加载系统数据失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (authed) loadAll()
  }, [authed, loadAll])

  // Dual-rate refresh polling countdown
  useEffect(() => {
    const timer = setInterval(() => {
      api.getRefreshStatus().then(st => {
        setRefreshState(st)
      }).catch(() => {})
    }, 2000)
    return () => clearInterval(timer)
  }, [])

  const handleLogout = useCallback(() => {
    clearToken()
    setAuthed(false)
    antMessage.success('已安全退出')
  }, [])

  // WebSocket real-time events
  useWebSocket(msg => {
    if (msg.type === 'refresh') {
      loadAll()
      setNewAlerts(n => n + 1)
    } else if (msg.type === 'refresh_mode_change') {
      api.getRefreshStatus().then(setRefreshState).catch(() => {})
    } else if (msg.type === 'events') {
      loadAll()
      setNewAlerts(n => n + 1)
    } else if (msg.type === 'purchased') {
      antMessage.success(`购买成功：${msg.plan_id}`)
      loadAll()
    }
  })

  // Quick burst mode triggers
  const handleStartBurst = async (durationSec: number) => {
    try {
      const res = await api.startBurstRefresh(durationSec, 2)
      setRefreshState(res)
      antMessage.success(`已开启 ${durationSec / 60} 分钟高速刷新 (2s/次)`)
    } catch {
      antMessage.error('开启高速刷新失败')
    }
  }

  const handleStopBurst = async () => {
    try {
      const res = await api.stopBurstRefresh()
      setRefreshState(res)
      antMessage.info('已恢复为平时慢刷新 (30s/次)')
    } catch {
      antMessage.error('恢复慢刷新失败')
    }
  }

  // Not logged in → Login / Setup
  if (!authed) {
    return (
      <ConfigProvider
        theme={{
          token: {
            colorPrimary: '#2563eb',
            colorBgBase: '#ffffff',
            colorBgLayout: '#f8fafc',
            borderRadius: 6,
          },
        }}
      >
        <AntApp>
          <LoginPage onAuthed={() => { setAuthed(true); setLoading(true) }} />
        </AntApp>
      </ConfigProvider>
    )
  }

  const isBurst = refreshState?.mode === 'burst'
  const burstRemaining = refreshState?.remaining_seconds ?? 0

  return (
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: '#2563eb',
          colorBgBase: '#ffffff',
          colorBgLayout: '#f8fafc',
          colorText: '#334155',
          colorTextHeading: '#0f172a',
          colorTextSecondary: '#64748b',
          colorBorder: '#e2e8f0',
          colorBorderSecondary: '#f1f5f9',
          borderRadius: 6,
          fontSize: 13,
        },
      }}
    >
      <AntApp>
        <Layout className="ashan-layout">
          {/* Ashan Slate 220px Sider */}
          <Sider
            collapsible
            collapsed={collapsed}
            onCollapse={setCollapsed}
            width={220}
            className="ashan-sider"
          >
            <div className="ashan-logo">
              <div className="ashan-logo-icon">A</div>
              {!collapsed && <div className="ashan-logo-text">个人 vps 监控</div>}
            </div>
            <Menu
              mode="inline"
              selectedKeys={[page]}
              onClick={({ key }) => {
                setPage(key as PageKey)
                if (key === 'alerts') setNewAlerts(0)
              }}
              style={{ borderRight: 0, padding: '8px 0' }}
              items={[
                { key: 'overview', icon: <DashboardOutlined />, label: '系统大盘' },
                { key: 'store', icon: <ShoppingCartOutlined />, label: '一手新购' },
                { key: 'secondhand', icon: <SwapOutlined />, label: '二手集市' },
                { key: 'billing', icon: <CreditCardOutlined />, label: '账单与支付' },
                { type: 'divider' },
                { key: 'account', icon: <WalletOutlined />, label: 'Akile 资产' },
                { key: 'acck', icon: <SafetyCertificateOutlined />, label: 'ACCK 资产' },
                { key: 'lazycat', icon: <CloudServerOutlined />, label: '懒猫云资产' },
                {
                  key: 'alerts',
                  icon: <BellOutlined />,
                  label: (
                    <Badge count={newAlerts} size="small" offset={[8, 0]}>
                      预警动态
                    </Badge>
                  ),
                },
                { key: 'logs', icon: <FileTextOutlined />, label: '运行日志' },
                { key: 'settings', icon: <SettingOutlined />, label: '系统配置' },
              ]}
            />
          </Sider>

          <Layout>
            {/* Ashan Slate Top Context Bar */}
            <Header className="ashan-header">
              <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
                <span style={{ fontSize: 16, fontWeight: 700, color: '#0f172a' }}>
                  {{
                    overview: '双平台核心大盘',
                    store: '官方一手云服务器商城',
                    secondhand: '二手交易与捡漏集市',
                    billing: '统一订单与账单支付中心',
                    account: 'Akile 平台资产',
                    acck: 'ACCK 平台资产',
                    lazycat: '懒猫云平台资产',
                    alerts: '系统预警历史',
                    logs: '服务运行日志',
                    settings: '系统与刷新设置',
                  }[page]}
                </span>
                <span className="badge-online">Akile 永续保活</span>
                <span className="badge-proxy">ACCK 7天 Token 复用</span>
                <span className="badge-proxy">懒猫云 每日签到</span>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
                {/* Dual-rate refresh control pill */}
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span className={isBurst ? 'refresh-pill refresh-pill-burst' : 'refresh-pill refresh-pill-slow'}>
                    {isBurst ? (
                      <>
                        <ThunderboltOutlined style={{ color: '#2563eb' }} />
                        高速刷新中 (2s/次) · 剩 {Math.floor(burstRemaining / 60)}m{burstRemaining % 60}s
                      </>
                    ) : (
                      <>
                        <SyncOutlined spin style={{ color: '#059669', fontSize: 11 }} />
                        平时慢刷新 (30s/次)
                      </>
                    )}
                  </span>
                  {isBurst ? (
                    <Button size="small" danger onClick={handleStopBurst}>切回慢刷</Button>
                  ) : (
                    <Button size="small" type="primary" ghost onClick={() => handleStartBurst(300)}>5分高速</Button>
                  )}
                </div>

                <Tooltip title="一键全量刷新">
                  <Button
                    type="text"
                    size="small"
                    icon={<ReloadOutlined spin={loading} />}
                    onClick={loadAll}
                    style={{ color: '#64748b' }}
                  />
                </Tooltip>

                <Text style={{ color: '#94a3b8', fontSize: 12 }}>
                  {version ? `v${version.version}` : ''}
                </Text>

                <Button
                  type="text"
                  size="small"
                  icon={<LogoutOutlined />}
                  onClick={handleLogout}
                  style={{ color: '#64748b' }}
                >
                  退出
                </Button>
              </div>
            </Header>

            {/* Ashan Slate Main Content */}
            <Content className="ashan-content">
              {loading && !refreshState ? (
                <Spin size="large" style={{ display: 'block', margin: '140px auto' }} />
              ) : (
                <>
                  {page === 'overview' && <UnifiedDashboard />}
                  {page === 'store' && <UnifiedStorePage />}
                  {page === 'secondhand' && <UnifiedSecondhandPage />}
                  {page === 'billing' && <UnifiedBillingPage />}
                  {page === 'account' && <AccountPage />}
                  {page === 'acck' && <AcckAccountPage />}
                  {page === 'lazycat' && <LazyCatAccountPage onGotoSettings={() => setPage('settings')} />}
                  {page === 'alerts' && <AlertHistory events={events} />}
                  {page === 'logs' && <LogsPage />}
                  {page === 'settings' && <SettingsPage settings={settings} onSaved={setSettings} />}
                </>
              )}
            </Content>
          </Layout>
        </Layout>
      </AntApp>
    </ConfigProvider>
  )
}
