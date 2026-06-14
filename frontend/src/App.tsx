import { useCallback, useEffect, useState } from 'react'
import {
  App as AntApp, Badge, Button, ConfigProvider, Layout, Menu, message as antMessage,
  Spin, theme, Typography,
} from 'antd'
import {
  DashboardOutlined, CloudServerOutlined,
  BellOutlined, SettingOutlined, LogoutOutlined,
} from '@ant-design/icons'

import { api, clearToken, getToken, setUnauthorizedHandler } from './api'
import type { AlertEvent, AppSettings, Machine, PriceHistory } from './api'
import { useWebSocket } from './useWebSocket'
import { Overview }      from './pages/Overview'
import { MachinePanel } from './pages/MachinePanel'
import { AlertHistory }  from './pages/AlertHistory'
import { SettingsPage }  from './pages/SettingsPage'
import { LoginPage }     from './pages/LoginPage'

import './App.css'

const { Header, Sider, Content } = Layout
const { Title } = Typography

type PageKey = 'overview' | 'machines' | 'alerts' | 'settings'

export default function App() {
  const [authed, setAuthed]       = useState<boolean>(!!getToken())
  const [page, setPage]           = useState<PageKey>('overview')
  const [machines, setMachines]   = useState<Machine[]>([])
  const [events, setEvents]       = useState<AlertEvent[]>([])
  const [history, setHistory]     = useState<PriceHistory[]>([])
  const [settings, setSettings]   = useState<AppSettings | null>(null)
  const [loading, setLoading]     = useState(true)
  const [newAlerts, setNewAlerts] = useState(0)
  const [collapsed, setCollapsed] = useState(false)

  // Bounce back to login on any 401.
  useEffect(() => {
    setUnauthorizedHandler(() => setAuthed(false))
  }, [])

  const loadAll = useCallback(async () => {
    try {
      const [m, e, h, s] = await Promise.all([
        api.getMachines(),
        api.getEvents({ limit: 200 }),
        api.getPriceHistory(),
        api.getSettings(),
      ])
      setMachines(m)
      setEvents(e)
      setHistory(h)
      setSettings(s)
    } catch {
      antMessage.error('加载数据失败，请检查后端连接')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { if (authed) loadAll() }, [authed, loadAll])

  const handleLogout = useCallback(() => {
    clearToken()
    setAuthed(false)
    antMessage.success('已退出登录')
  }, [])

  // WebSocket real-time updates
  useWebSocket(msg => {
    if (msg.type === 'refresh') {
      loadAll()
      setNewAlerts(n => n + 1)
    } else if (msg.type === 'events') {
      loadAll()
      setNewAlerts(n => n + 1)
    } else if (msg.type === 'purchased') {
      antMessage.success(`购买成功：${msg.plan_id}`)
      loadAll()
    }
  })

  const handleWatch = useCallback(async (planId: string, watched: boolean) => {
    if (!settings) return
    const current = new Set(settings.watched_plans ?? [])
    if (watched) current.add(planId)
    else current.delete(planId)
    const updated = await api.saveSettings({ watched_plans: [...current] })
    setSettings(updated)
  }, [settings])

  const handleBuy = useCallback(async (planId: string) => {
    try {
      const result = await api.buyMachine(planId)
      if (result?.error) antMessage.error(result.error)
      else antMessage.success('购买请求已提交')
    } catch {
      antMessage.error('购买失败')
    }
  }, [])

  const handleRefresh = useCallback(() => {
    api.triggerPoll().catch(() => {})
    setTimeout(loadAll, 1500)
  }, [loadAll])

  // Not logged in → show the gatekeeper (login or first-time setup).
  if (!authed) {
    return (
      <ConfigProvider theme={{ algorithm: theme.darkAlgorithm, token: { colorPrimary: '#177ddc', borderRadius: 6 } }}>
        <AntApp>
          <LoginPage onAuthed={() => { setAuthed(true); setLoading(true) }} />
        </AntApp>
      </ConfigProvider>
    )
  }

  return (
    <ConfigProvider
      theme={{
        algorithm: theme.darkAlgorithm,
        token: {
          colorPrimary: '#177ddc',
          borderRadius: 6,
        },
      }}
    >
      <AntApp>
        <Layout style={{ minHeight: '100vh' }}>
          <Sider
            collapsible
            collapsed={collapsed}
            onCollapse={setCollapsed}
            theme="dark"
            width={200}
            style={{ background: '#111' }}
          >
            <div style={{ padding: '16px 12px', borderBottom: '1px solid #1f1f1f' }}>
              <Title level={5} style={{ color: '#e0e0e0', margin: 0, fontSize: collapsed ? 0 : 14 }}>
                {!collapsed && '🖥 Akile Monitor'}
              </Title>
            </div>
            <Menu
              theme="dark"
              mode="inline"
              selectedKeys={[page]}
              onClick={({ key }) => {
                setPage(key as PageKey)
                if (key === 'alerts') setNewAlerts(0)
              }}
              style={{ background: '#111', borderRight: 0 }}
              items={[
                { key: 'overview',  icon: <DashboardOutlined />,   label: '总览' },
                { key: 'machines',  icon: <CloudServerOutlined />,  label: '机器列表' },
                {
                  key: 'alerts',
                  icon: <BellOutlined />,
                  label: (
                    <Badge count={newAlerts} size="small" offset={[8, 0]}>
                      预警历史
                    </Badge>
                  ),
                },
                { key: 'settings', icon: <SettingOutlined />, label: '设置' },
              ]}
            />
          </Sider>

          <Layout>
            <Header style={{
              background: '#161616',
              padding: '0 20px',
              borderBottom: '1px solid #1f1f1f',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
            }}>
              <Typography.Text style={{ color: '#ccc', margin: 0 }}>
                {{ overview: '总览', machines: '机器列表', alerts: '预警历史', settings: '系统设置' }[page]}
              </Typography.Text>
              <span style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
                <Typography.Text style={{ color: '#555', fontSize: 12 }}>
                  {settings ? `刷新间隔 ${settings.refresh_interval}s` : ''}
                </Typography.Text>
                <Button
                  type="text"
                  size="small"
                  icon={<LogoutOutlined />}
                  onClick={handleLogout}
                  style={{ color: '#888' }}
                >
                  退出
                </Button>
              </span>
            </Header>

            <Content style={{ padding: 20, background: '#0d0d0d', minHeight: 0 }}>
              {loading ? (
                <Spin size="large" style={{ display: 'block', margin: '120px auto' }} />
              ) : (
                <>
                  {page === 'overview'  && <Overview machines={machines} events={events} />}
                  {page === 'machines'  && (
                    <MachinePanel
                      machines={machines}
                      priceHistory={history}
                      settings={settings}
                      onWatch={handleWatch}
                      onBuy={handleBuy}
                      onRefresh={handleRefresh}
                    />
                  )}
                  {page === 'alerts'   && <AlertHistory events={events} />}
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
