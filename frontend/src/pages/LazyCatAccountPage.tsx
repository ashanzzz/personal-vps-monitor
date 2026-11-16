import { useCallback, useEffect, useState } from 'react'
import {
  Alert, App as AntApp, Button, Card, Col, Descriptions, Empty, Row,
  Space, Spin, Statistic, Table, Tag, Typography,
} from 'antd'
import {
  CloudServerOutlined, ReloadOutlined, WalletOutlined, CrownOutlined,
  CheckCircleOutlined, LoginOutlined,
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

function pick(obj: Record<string, unknown> | null | undefined, keys: string[]): unknown {
  if (!obj) return undefined
  for (const k of keys) {
    const v = obj[k]
    if (v !== undefined && v !== null && v !== '') return v
  }
  return undefined
}

function num(obj: Record<string, unknown> | null | undefined, keys: string[]): number | null {
  const v = pick(obj, keys)
  if (v === undefined) return null
  const n = Number(v)
  return Number.isFinite(n) ? n : null
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

  const notConfigured = Boolean(account?.error) || (!account?.user && instances.length === 0)
  if (notConfigured) {
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

  const user = account?.user ?? null
  const balance = account?.balance ?? null
  const vip = account?.vip_card ?? null

  const username = pick(user, ['username', 'name', 'nickname', 'email']) as string | undefined
  const email = pick(user, ['email']) as string | undefined
  const uid = pick(user, ['id', 'uid', 'userId'])
  const money = num(balance, ['balance', 'amount', 'money', 'available', 'total'])

  const profileItems = [
    { key: 'username', label: '账号', children: show(username) },
    { key: 'email', label: '邮箱', children: show(email) },
    { key: 'uid', label: '用户 ID', children: show(uid) },
    {
      key: 'vip',
      label: '会员等级',
      children: show(pick(vip, ['level', 'name', 'vipLevel', 'title'])) ?? '—',
    },
    {
      key: 'vip_expire',
      label: '会员到期',
      children: show(pick(vip, ['expireAt', 'expired_at', 'expire_time', 'due_date'])),
    },
  ]

  const columns: ColumnsType<LazyCatInstance> = [
    {
      title: '名称',
      key: 'name',
      render: (_: unknown, r) => (
        <Text strong>{show(pick(r as Record<string, unknown>, ['name', 'hostname', 'title']))}</Text>
      ),
    },
    {
      title: '区域',
      key: 'region',
      width: 130,
      render: (_: unknown, r) => show(pick(r as Record<string, unknown>, ['region', 'area', 'node', 'location'])),
    },
    {
      title: '配置',
      key: 'spec',
      width: 200,
      render: (_: unknown, r) => {
        const o = r as Record<string, unknown>
        const parts = [
          pick(o, ['cpu', 'vcpu']) ? `${pick(o, ['cpu', 'vcpu'])}C` : null,
          pick(o, ['memory', 'ram']) ? `${pick(o, ['memory', 'ram'])}M` : null,
          pick(o, ['disk', 'storage']) ? `${pick(o, ['disk', 'storage'])}G` : null,
        ].filter(Boolean)
        return parts.length ? parts.join(' / ') : '—'
      },
    },
    {
      title: 'IP',
      key: 'ip',
      width: 150,
      render: (_: unknown, r) => show(pick(r as Record<string, unknown>, ['ip', 'ipv4', 'main_ip'])),
    },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render: (_: unknown, r) => {
        const st = String(pick(r as Record<string, unknown>, ['status', 'state']) ?? '')
        const running = /run|active|正常|on/i.test(st)
        return <Tag color={running ? 'success' : 'default'}>{st || '—'}</Tag>
      },
    },
    {
      title: '到期时间',
      key: 'expire',
      width: 180,
      render: (_: unknown, r) =>
        show(pick(r as Record<string, unknown>, ['expired_at', 'expire_time', 'due_date', 'expireAt'])),
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
              每日签到
            </Button>
          </Space>
        }
      >
        <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
          <Col xs={24} sm={12} md={8}>
            <Card size="small" variant="borderless" style={{ background: '#f8fafc' }}>
              <Statistic
                title={<Space><WalletOutlined /><span>账户余额</span></Space>}
                value={money ?? 0}
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
                value={show(pick(vip, ['level', 'name', 'vipLevel', 'title']))}
                valueStyle={{ color: '#b45309', fontWeight: 600, fontSize: 20 }}
              />
            </Card>
          </Col>
        </Row>

        <Descriptions
          bordered
          size="small"
          column={{ xs: 1, sm: 2, md: 3 }}
          items={profileItems}
        />
      </Card>

      <Card
        title={<Space><CloudServerOutlined /><span>我的实例</span></Space>}
        extra={<Text type="secondary" style={{ fontSize: 12 }}>共 {instances.length} 台</Text>}
      >
        {instances.length === 0 ? (
          <Empty description="名下暂无实例" />
        ) : (
          <Table<LazyCatInstance>
            rowKey={(r) => String(pick(r as Record<string, unknown>, ['id', 'uuid', 'instanceId']) ?? Math.random())}
            size="small"
            columns={columns}
            dataSource={instances}
            pagination={false}
            scroll={{ x: 900 }}
          />
        )}
      </Card>
    </div>
  )
}
