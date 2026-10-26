import { useCallback, useEffect, useState } from 'react'
import {
  Card, Statistic, Row, Col, Button, Space, Empty, Table, Spin,
  Descriptions, Alert, Typography, App as AntApp,
} from 'antd'
import {
  ReloadOutlined, WalletOutlined, CloudServerOutlined, CheckCircleTwoTone,
  CloseCircleTwoTone, GoldOutlined, SafetyCertificateOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '../api'
import type { AcckAccountInfo, AcckServer } from '../api'

const { Text } = Typography

function show(v: unknown): string {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'boolean') return v ? '是' : '否'
  return String(v)
}

function timeAgo(iso: string | null | undefined): string {
  if (!iso) return '从未'
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return '未知'
  const sec = Math.max(0, Math.round((Date.now() - then) / 1000))
  if (sec < 60) return `${sec} 秒前`
  const min = Math.round(sec / 60)
  if (min < 60) return `${min} 分钟前`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr} 小时前`
  return `${Math.round(hr / 24)} 天前`
}

function pick(s: AcckServer, keys: string[]): unknown {
  for (const k of keys) if (s[k] !== undefined && s[k] !== null && s[k] !== '') return s[k]
  return undefined
}

export function AcckAccountPage() {
  const { message } = AntApp.useApp()
  const [data, setData] = useState<AcckAccountInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [checkingIn, setCheckingIn] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setData(await api.getAcckAccount())
    } catch {
      message.error('加载 ACCK 账户信息失败')
    } finally {
      setLoading(false)
    }
  }, [message])

  const refresh = useCallback(async () => {
    setRefreshing(true)
    try {
      setData(await api.refreshAcckAccount())
      message.success('已刷新 ACCK 账户数据')
    } catch {
      message.error('刷新 ACCK 账户数据失败')
    } finally {
      setRefreshing(false)
    }
  }, [message])

  const checkin = useCallback(async () => {
    setCheckingIn(true)
    try {
      const res = await api.checkinAcck()
      const msg = String(res.message || res.msg || res.status_msg || JSON.stringify(res))
      message.info(`ACCK 签到响应: ${msg}`)
      try { setData(await api.refreshAcckAccount()) } catch { /* keep current */ }
    } catch {
      message.error('ACCK 签到请求失败')
    } finally {
      setCheckingIn(false)
    }
  }, [message])

  useEffect(() => { load() }, [load])

  if (loading && !data) {
    return <Spin size="large" style={{ display: 'block', margin: '120px auto' }} />
  }

  const profile = data?.profile ?? null
  const servers = data?.servers ?? []

  const PREFERRED: { title: string; keys: string[] }[] = [
    { title: '名称', keys: ['name', 'hostname', 'title'] },
    { title: '节点/区域', keys: ['node', 'area', 'region', 'location'] },
    { title: 'IP', keys: ['ip', 'ipv4', 'main_ip'] },
    { title: '状态', keys: ['status', 'state'] },
    { title: '到期', keys: ['expired_at', 'expire_time', 'due_time'] },
  ]

  const columns: ColumnsType<AcckServer> = servers.length
    ? PREFERRED.map(p => ({
        title: p.title,
        key: p.title,
        render: (_: unknown, s: AcckServer) => show(pick(s, p.keys)),
      }))
    : []

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Space size="middle" align="center">
          <Text strong style={{ color: '#ccc', fontSize: 16 }}>ACCK 账户与资产</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>
            数据更新于 {timeAgo(data?.updated_at)}
          </Text>
        </Space>
        <Space wrap>
          <Button
            type="primary"
            icon={<GoldOutlined />}
            loading={checkingIn}
            onClick={checkin}
          >
            每日签到打卡
          </Button>
          <Button type="primary" ghost icon={<ReloadOutlined />} loading={refreshing} onClick={refresh}>
            强制刷新
          </Button>
        </Space>
      </div>

      <Alert
        type="info"
        showIcon
        icon={<SafetyCertificateOutlined />}
        style={{ marginBottom: 16 }}
        message="防误购安全模式已锁定 (Dry-Run Protected)"
        description="系统当前开启了防误购保护锁，禁止向 ACCK 或 Akile 发起自动扣款与真实购买。所有购买逻辑均处于模拟与安全观测模式。"
      />

      {data && !data.configured && (
        <Alert type="warning" showIcon style={{ marginBottom: 16 }}
          message="尚未配置 ACCK Token" description="请到「设置 → API 与连接」填入 auth_token 凭据。" />
      )}

      {data?.errors?.length ? (
        <Alert type="error" showIcon style={{ marginBottom: 16 }}
          message="部分 ACCK 数据获取失败" description={data.errors.join('; ')} />
      ) : null}

      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col xs={24} sm={8}>
          <Card size="small">
            <Statistic
              title={<Space><WalletOutlined />AC 积分 / 余额</Space>}
              value={profile?.points != null ? profile.points : '—'}
              suffix="积分"
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card size="small">
            <Statistic
              title={<Space><GoldOutlined />金币/权益</Space>}
              value={profile?.coins != null ? profile.coins : '0'}
              valueStyle={{ color: '#faad14' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card size="small" title="ACCK 账户详情">
            {profile ? (
              <Descriptions size="small" column={{ xs: 1, sm: 2 }}>
                <Descriptions.Item label="用户名">{show(profile.username)}</Descriptions.Item>
                <Descriptions.Item label="邮箱">{show(profile.email)}</Descriptions.Item>
                <Descriptions.Item label="角色">{show(profile.role)}</Descriptions.Item>
                <Descriptions.Item label="状态">
                  {profile.status === 'active'
                    ? <span><CheckCircleTwoTone twoToneColor="#52c41a" /> 正常</span>
                    : <span><CloseCircleTwoTone twoToneColor="#faad14" /> {show(profile.status)}</span>}
                </Descriptions.Item>
              </Descriptions>
            ) : <Empty description="无账户信息" />}
          </Card>
        </Col>
      </Row>

      <Card size="small" title={<Space><CloudServerOutlined />我的 ACCK 服务器 ({servers.length})</Space>}>
        {servers.length === 0 ? (
          <Empty description="当前没有检测到服务器实例" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        ) : (
          <Table<AcckServer>
            rowKey={(s, i) => String(pick(s, ['id', 'server_id', 'uuid']) ?? i)}
            size="small"
            columns={columns}
            dataSource={servers}
            pagination={false}
            expandable={{
              expandedRowRender: s => (
                <pre style={{ margin: 0, fontSize: 12, color: '#aaa', whiteSpace: 'pre-wrap' }}>
                  {JSON.stringify(s, null, 2)}
                </pre>
              ),
            }}
          />
        )}
      </Card>
    </div>
  )
}
