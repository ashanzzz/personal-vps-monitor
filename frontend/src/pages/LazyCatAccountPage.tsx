import { useCallback, useEffect, useState } from 'react'
import {
  Alert, App as AntApp, Button, Card, Col, Descriptions, Empty, Progress, Row,
  Space, Spin, Statistic, Table, Tag, Typography,
} from 'antd'
import {
  CloudServerOutlined, ReloadOutlined, WalletOutlined, CrownOutlined,
  CheckCircleOutlined, LoginOutlined, GlobalOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '../api'
import type { LazyCatAccount, LazyCatInstance } from '../api'

const { Text } = Typography

function show(v: unknown): string {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'boolean') return v ? '是' : '否'
  return String(v)
}

function fmtBytes(bytes: number | null): string {
  if (bytes === null || !Number.isFinite(bytes)) return '—'
  const gb = bytes / 1024 ** 3
  if (gb >= 1024) return `${(gb / 1024).toFixed(2)} TB`
  if (gb >= 1) return `${gb.toFixed(gb >= 100 ? 0 : 1)} GB`
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`
}

function fmtDisk(mb: unknown): string {
  const n = Number(mb)
  if (!Number.isFinite(n) || n <= 0) return '—'
  return n >= 1024 ? `${(n / 1024).toFixed(n % 1024 === 0 ? 0 : 1)}G` : `${n}M`
}

function fmtDate(v: unknown): string {
  if (!v) return '—'
  const d = new Date(String(v))
  if (Number.isNaN(d.getTime())) return show(v)
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function daysLeft(v: unknown): number | null {
  if (!v) return null
  const t = new Date(String(v)).getTime()
  if (Number.isNaN(t)) return null
  return Math.ceil((t - Date.now()) / 86400000)
}

export function LazyCatAccountPage({ onGotoSettings }: { onGotoSettings?: () => void }) {
  const { message } = AntApp.useApp()
  const [account, setAccount] = useState<LazyCatAccount | null>(null)
  const [instances, setInstances] = useState<LazyCatInstance[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [checkingIn, setCheckingIn] = useState(false)

  const load = useCallback(async (announce = false) => {
    try {
      const [acc, inst] = await Promise.all([
        api.getLazyCatAccount().catch(() => null),
        api.getLazyCatInstances().catch(() => []),
      ])
      setAccount(acc)
      setInstances(Array.isArray(inst) ? inst : [])
      if (announce) message.success('已刷新懒猫云数据')
    } catch {
      message.error('加载懒猫云数据失败')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [message])

  useEffect(() => { load() }, [load])

  const refresh = useCallback(() => {
    setRefreshing(true)
    load(true)
  }, [load])

  const checkin = useCallback(async () => {
    setCheckingIn(true)
    try {
      const res = await api.lazycatCheckin()
      const text = String(res.message ?? res.msg ?? res.status_msg ?? JSON.stringify(res))
      message.info(`懒猫云签到：${text.slice(0, 120)}`)
      load()
    } catch {
      message.error('懒猫云签到失败')
    } finally {
      setCheckingIn(false)
    }
  }, [load, message])

  if (loading) {
    return <Spin size="large" style={{ display: 'block', margin: '120px auto' }} />
  }

  if (account?.error) {
    return (
      <Card>
        <Alert
          type="warning"
          showIcon
          message="懒猫云数据不可用"
          description={
            <Space direction="vertical" size={4}>
              <Text type="secondary">{account.error}</Text>
              {onGotoSettings && (
                <Button type="primary" icon={<LoginOutlined />} onClick={onGotoSettings}>
                  去系统配置重新登录
                </Button>
              )}
            </Space>
          }
        />
      </Card>
    )
  }

  const user = account?.user ?? null
  const balance = account?.balance ?? null
  const vipCard = account?.vip_card ?? null
  const checkinInfo = account?.checkin ?? null

  if (!user && instances.length === 0) {
    return (
      <Card>
        <Alert
          type="info"
          showIcon
          message="还没有配置懒猫云账号"
          description={
            <Space direction="vertical" size={4}>
              <Text type="secondary">
                请前往「系统配置」，展开「懒猫云一键登录（账号 + 密码 + 2FA）」，填写账号密码后点击登录，
                Token 会自动保存。
              </Text>
              {onGotoSettings && (
                <Button type="primary" icon={<LoginOutlined />} onClick={onGotoSettings}>
                  去系统配置登录
                </Button>
              )}
            </Space>
          }
        />
      </Card>
    )
  }

  const money = Number(balance?.balance ?? 0)
  const vipLevel = Number(user?.vipLevel ?? balance?.vipLevel ?? 0)
  const vipProgress = Number(balance?.vipProgress ?? 0)
  const vipNextThreshold = Number(balance?.vipNextThreshold ?? 0)
  const checkedIn = Boolean(checkinInfo?.checkedIn)
  const checkinPoints = Number(checkinInfo?.points ?? 0)
  const vipCardInfo = vipCard?.card as Record<string, unknown> | null | undefined

  const profileItems = [
    { key: 'username', label: '账号', children: show(user?.username) },
    { key: 'email', label: '邮箱', children: show(user?.email) },
    { key: 'uid', label: '用户 ID', children: show(user?.id) },
    { key: 'vip', label: '会员等级', children: `VIP ${vipLevel}` },
    {
      key: 'vip_progress',
      label: '升级进度',
      children: vipNextThreshold ? `${vipProgress} / ${vipNextThreshold}（距 VIP ${vipLevel + 1}）` : '—',
    },
    {
      key: 'checkin',
      label: '今日签到',
      children: (
        <Space size={4}>
          <Tag color={checkedIn ? 'success' : 'default'}>{checkedIn ? '已签到' : '未签到'}</Tag>
          {checkedIn && <Text type="secondary">+{checkinPoints} 积分</Text>}
        </Space>
      ),
    },
    { key: 'recharge', label: '累计充值', children: `¥${Number(balance?.totalRecharge ?? 0).toFixed(2)}` },
    { key: 'consume', label: '本月消费', children: `¥${Number(balance?.thisMonthConsume ?? 0).toFixed(2)}` },
    {
      key: 'vipcard',
      label: 'VIP 权益卡',
      children: vipCardInfo
        ? show(vipCardInfo.cardNumber)
        : vipCard?.enabled
          ? (vipCard.canClaim ? '可领取' : `需 VIP ${show(vipCard.minVipLevel)} 解锁`)
          : '未开放',
    },
  ]

  const columns: ColumnsType<LazyCatInstance> = [
    {
      title: '名称',
      key: 'name',
      render: (_: unknown, r) => <Text strong>{show(r.name)}</Text>,
    },
    {
      title: '区域',
      key: 'region',
      width: 170,
      render: (_: unknown, r) => {
        const host = r.host?.name
        const cc = (r.hostCountryCode ?? r.host?.country_code ?? '').toUpperCase()
        return host ? `${host}${cc ? ` · ${cc}` : ''}` : '—'
      },
    },
    {
      title: '配置',
      key: 'spec',
      width: 160,
      render: (_: unknown, r) => {
        const cpu = r.cpuCores ?? r.cpu
        const parts = [
          cpu ? `${cpu}C` : null,
          r.memory ? `${r.memory}M` : null,
          r.disk ? fmtDisk(r.disk) : null,
        ].filter(Boolean)
        return parts.length ? parts.join(' / ') : '—'
      },
    },
    {
      title: 'IP',
      key: 'ip',
      width: 180,
      render: (_: unknown, r) => (
        <Space direction="vertical" size={0}>
          <Text code>{show(r.ipv4)}</Text>
          {r.natPublicIp && r.natPublicIp !== r.ipv4 && (
            <Text type="secondary" style={{ fontSize: 11 }}>公网 {r.natPublicIp}</Text>
          )}
        </Space>
      ),
    },
    {
      title: '流量',
      key: 'traffic',
      width: 165,
      render: (_: unknown, r) => {
        const used = Number(r.monthlyTrafficUsed ?? NaN)
        const limit = Number(r.monthlyTrafficLimit ?? NaN)
        if (!Number.isFinite(used) || !Number.isFinite(limit)) return '—'
        const pct = limit > 0 ? Math.min(100, (used / limit) * 100) : 0
        return (
          <Space direction="vertical" size={0} style={{ width: '100%' }}>
            <Text style={{ fontSize: 12 }}>{fmtBytes(used)} / {fmtBytes(limit)}</Text>
            <Progress percent={Number(pct.toFixed(1))} size="small" showInfo={false} strokeColor="#2563eb" />
          </Space>
        )
      },
    },
    {
      title: '月费',
      key: 'price',
      width: 90,
      render: (_: unknown, r) => (r.billingPrice != null ? `¥${Number(r.billingPrice).toFixed(2)}` : '—'),
    },
    {
      title: '状态',
      key: 'status',
      width: 95,
      render: (_: unknown, r) => {
        const st = String(r.status ?? '')
        const map: Record<string, { color: string; text: string }> = {
          running: { color: 'success', text: '运行中' },
          stopped: { color: 'default', text: '已停止' },
          suspended: { color: 'warning', text: '已暂停' },
        }
        const hit = map[st]
        return <Tag color={hit?.color ?? 'default'}>{hit?.text ?? (st || '—')}</Tag>
      },
    },
    {
      title: '到期时间',
      key: 'expire',
      width: 140,
      render: (_: unknown, r) => {
        const left = daysLeft(r.expiresAt)
        const urgent = left !== null && left <= 7
        return (
          <Space direction="vertical" size={0}>
            <Text>{fmtDate(r.expiresAt)}</Text>
            {left !== null && (
              <Text type={urgent ? 'danger' : 'secondary'} style={{ fontSize: 11 }}>
                {left >= 0 ? `剩 ${left} 天` : `已过期 ${-left} 天`}
              </Text>
            )}
          </Space>
        )
      },
    },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card
        title={<Space><CloudServerOutlined /><span>懒猫云账户概览</span></Space>}
        extra={
          <Space>
            <Button icon={<ReloadOutlined spin={refreshing} />} onClick={refresh} loading={refreshing}>
              刷新
            </Button>
            <Button icon={<CheckCircleOutlined />} onClick={checkin} loading={checkingIn}>
              {checkedIn ? '今日已签到' : '每日签到'}
            </Button>
          </Space>
        }
      >
        <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
          <Col xs={24} sm={12} md={8}>
            <Card size="small" variant="borderless" style={{ background: '#f8fafc' }}>
              <Statistic
                title={<Space><WalletOutlined /><span>账户余额</span></Space>}
                value={money}
                precision={2}
                prefix="¥"
                valueStyle={{ color: '#0f172a', fontWeight: 600 }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={8}>
            <Card size="small" variant="borderless" style={{ background: '#f8fafc' }}>
              <Statistic
                title={<Space><CloudServerOutlined /><span>实例数量</span></Space>}
                value={instances.length}
                suffix="台"
                valueStyle={{ color: '#2563eb', fontWeight: 600 }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={8}>
            <Card size="small" variant="borderless" style={{ background: '#f8fafc' }}>
              <Statistic
                title={<Space><CrownOutlined /><span>会员等级</span></Space>}
                value={vipLevel}
                prefix="VIP"
                valueStyle={{ color: '#b45309', fontWeight: 600, fontSize: 22 }}
              />
            </Card>
          </Col>
        </Row>

        <Descriptions bordered size="small" column={{ xs: 1, sm: 2, md: 3 }} items={profileItems} />
      </Card>

      <Card
        title={<Space><GlobalOutlined /><span>我的实例</span></Space>}
        extra={<Text type="secondary" style={{ fontSize: 12 }}>共 {instances.length} 台</Text>}
      >
        {instances.length === 0 ? (
          <Empty description="名下暂无实例" />
        ) : (
          <Table<LazyCatInstance>
            rowKey={(r) => String(r.id ?? r.incusId ?? Math.random())}
            size="small"
            columns={columns}
            dataSource={instances}
            pagination={false}
            scroll={{ x: 1200 }}
          />
        )}
      </Card>
    </div>
  )
}
