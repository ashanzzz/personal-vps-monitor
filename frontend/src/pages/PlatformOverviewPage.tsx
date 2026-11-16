import { useCallback, useEffect, useState } from 'react'
import {
  Alert, App as AntApp, Badge, Button, Card, Col, Descriptions, Empty, Row,
  Space, Spin, Statistic, Table, Tag, Tooltip, Typography,
} from 'antd'
import {
  ApiOutlined, CheckCircleFilled, CloseCircleFilled, CloudServerOutlined,
  ReloadOutlined, SyncOutlined, ThunderboltOutlined, WarningOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '../api'
import type { PlatformHealth, PlatformsHealth, UnifiedServer } from '../api'

const { Text } = Typography

function fmtTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

function ago(iso: string | null | undefined): string {
  if (!iso) return '从未同步'
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return '未知'
  const sec = Math.max(0, Math.round((Date.now() - t) / 1000))
  if (sec < 60) return `${sec} 秒前`
  const min = Math.round(sec / 60)
  if (min < 60) return `${min} 分钟前`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr} 小时前`
  return `${Math.round(hr / 24)} 天前`
}

function fmtDate(ts: number | null | undefined): string {
  if (!ts) return '—'
  const ms = ts > 1e11 ? ts : ts * 1000
  const d = new Date(ms)
  if (Number.isNaN(d.getTime())) return '—'
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

function daysLeft(ts: number | null | undefined): number | null {
  if (!ts) return null
  const ms = ts > 1e11 ? ts : ts * 1000
  return Math.ceil((ms - Date.now()) / 86400000)
}

const PLATFORM_COLOR: Record<string, string> = {
  lazycat: '#0ea5e9',
  akile: '#2563eb',
  acck: '#7c3aed',
}

export function PlatformOverviewPage({ onGotoSettings }: { onGotoSettings?: () => void }) {
  const { message } = AntApp.useApp()
  const [health, setHealth] = useState<PlatformsHealth | null>(null)
  const [servers, setServers] = useState<UnifiedServer[]>([])
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)

  const load = useCallback(async (force = false, announce = false) => {
    try {
      const [h, srv] = await Promise.all([
        api.getPlatformsHealth(force).catch(() => null),
        api.getUnifiedServers().catch(() => ({ total: 0, servers: [], counts: {} })),
      ])
      setHealth(h)
      setServers(srv?.servers ?? [])
      setCounts(srv?.counts ?? {})
      if (announce) message.success('已重新探测三平台状态')
    } catch {
      message.error('加载平台总览失败')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [message])

  useEffect(() => { load() }, [load])

  const refresh = useCallback(() => {
    setRefreshing(true)
    load(true, true)
  }, [load])

  if (loading && !health) {
    return <Spin size="large" style={{ display: 'block', margin: '140px auto' }} />
  }

  const platforms = health?.platforms ?? []
  const broken = platforms.filter(p => p.configured && !p.token_valid)
  const unconfigured = platforms.filter(p => !p.configured)

  const columns: ColumnsType<UnifiedServer> = [
    {
      title: '平台',
      key: 'platform',
      width: 100,
      filters: [
        { text: '懒猫云', value: 'lazycat' },
        { text: 'Akile', value: 'akile' },
        { text: 'ACCK', value: 'acck' },
      ],
      onFilter: (v, r) => r.platform === v,
      render: (_: unknown, r) => {
        const label = r.platform === 'lazycat' ? '懒猫云' : r.platform === 'akile' ? 'Akile' : 'ACCK'
        return <Tag color={PLATFORM_COLOR[r.platform] ?? 'default'}>{label}</Tag>
      },
    },
    {
      title: '名称',
      key: 'name',
      render: (_: unknown, r) => <Text strong>{r.name}</Text>,
    },
    {
      title: '区域',
      key: 'area',
      width: 160,
      render: (_: unknown, r) => (
        <Text type="secondary">{r.node_name || r.area_name || '—'}</Text>
      ),
    },
    {
      title: '配置',
      key: 'spec',
      width: 150,
      render: (_: unknown, r) => `${r.cpu ?? '—'}C / ${r.memory ?? '—'}M / ${r.disk ?? '—'}G`,
    },
    {
      title: 'IP',
      key: 'ip',
      width: 150,
      render: (_: unknown, r) => (r.ipv4 ? <Text code>{r.ipv4}</Text> : '—'),
    },
    {
      title: '流量',
      key: 'flow',
      width: 130,
      render: (_: unknown, r) => {
        const used = Number(r.used_flow ?? 0)
        const total = Number(r.flow ?? 0)
        if (!total) return '—'
        return `${used}G / ${total}G`
      },
    },
    {
      title: '到期',
      key: 'due',
      width: 150,
      render: (_: unknown, r) => {
        const left = daysLeft(r.due_time)
        if (!r.due_time) return '—'
        return (
          <Space direction="vertical" size={0}>
            <Text>{fmtDate(r.due_time)}</Text>
            {left !== null && (
              <Text type={left <= 7 ? 'danger' : 'secondary'} style={{ fontSize: 11 }}>
                {left >= 0 ? `剩 ${left} 天` : `已过期 ${-left} 天`}
              </Text>
            )}
          </Space>
        )
      },
    },
    {
      title: '月费',
      key: 'price',
      width: 100,
      render: (_: unknown, r) => (r.price_monthly != null ? `¥${Number(r.price_monthly).toFixed(2)}` : '—'),
    },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {broken.length > 0 && (
        <Alert
          type="error"
          showIcon
          icon={<WarningOutlined />}
          message={`${broken.length} 个平台凭证已失效：${broken.map(b => b.name).join('、')}`}
          description={
            <Space direction="vertical" size={2}>
              <Text>
                这些平台当前展示的是<strong>本地旧缓存</strong>，并非远端实时数据，实例与余额可能已过期。
              </Text>
              {onGotoSettings && (
                <Button size="small" type="primary" onClick={onGotoSettings}>去系统配置更新凭证</Button>
              )}
            </Space>
          }
        />
      )}

      {unconfigured.length > 0 && (
        <Alert
          type="info"
          showIcon
          message={`${unconfigured.map(u => u.name).join('、')} 尚未配置凭证`}
          description="未配置的平台不会显示任何实例与资产数据。"
          action={onGotoSettings ? <Button size="small" onClick={onGotoSettings}>去配置</Button> : null}
        />
      )}

      <Card
        title={<Space><ApiOutlined /><span>平台状态</span></Space>}
        extra={
          <Space size={12}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              探测于 {fmtTime(health?.checked_at)}（{health?.cache_ttl_seconds ?? 60}s 缓存）
            </Text>
            <Button size="small" icon={<ReloadOutlined spin={refreshing} />} onClick={refresh} loading={refreshing}>
              重新探测
            </Button>
          </Space>
        }
      >
        <Row gutter={[16, 16]}>
          {platforms.map(p => (
            <Col xs={24} sm={12} lg={8} key={p.key}>
              <PlatformCard platform={p} serverCount={counts[p.key] ?? 0} onGotoSettings={onGotoSettings} />
            </Col>
          ))}
        </Row>
      </Card>

      <Card
        title={<Space><CloudServerOutlined /><span>全部实例</span></Space>}
        extra={
          <Space size={12}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              共 {servers.length} 台 · 懒猫云 {counts.lazycat ?? 0} / Akile {counts.akile ?? 0} / ACCK {counts.acck ?? 0}
            </Text>
          </Space>
        }
      >
        {servers.length === 0 ? (
          <Empty description="暂无实例（或各平台凭证已失效）" />
        ) : (
          <Table<UnifiedServer>
            rowKey={r => r.uid}
            size="small"
            columns={columns}
            dataSource={servers}
            pagination={{ pageSize: 20, hideOnSinglePage: true }}
            scroll={{ x: 1100 }}
          />
        )}
      </Card>
    </div>
  )
}

function PlatformCard({
  platform,
  serverCount,
  onGotoSettings,
}: {
  platform: PlatformHealth
  serverCount: number
  onGotoSettings?: () => void
}) {
  const ok = platform.token_valid
  const color = PLATFORM_COLOR[platform.key] ?? '#64748b'

  return (
    <Card
      size="small"
      style={{ borderLeft: `3px solid ${ok ? color : '#ef4444'}`, background: '#ffffff' }}
    >
      <Space direction="vertical" size={10} style={{ width: '100%' }}>
        <Space style={{ width: '100%', justifyContent: 'space-between' }}>
          <Space size={8}>
            <span style={{ fontWeight: 700, fontSize: 14, color: '#0f172a' }}>{platform.name}</span>
            <Text type="secondary" style={{ fontSize: 11 }}>{platform.host}</Text>
          </Space>
          <Tooltip title={ok ? '凭证有效' : platform.error ?? '凭证失效'}>
            <Badge
              status={ok ? 'success' : platform.configured ? 'error' : 'default'}
              text={
                <span style={{ fontSize: 12 }}>
                  {ok ? '凭证有效' : platform.configured ? '凭证失效' : '未配置'}
                </span>
              }
            />
          </Tooltip>
        </Space>

        <Row gutter={8}>
          <Col span={12}>
            <Statistic
              title={<span style={{ fontSize: 12 }}>实例</span>}
              value={serverCount}
              suffix="台"
              valueStyle={{ fontSize: 20, fontWeight: 600, color: '#0f172a' }}
            />
          </Col>
          <Col span={12}>
            <Statistic
              title={<span style={{ fontSize: 12 }}>数据来源</span>}
              value={platform.data_source === 'live' ? '实时' : '缓存'}
              prefix={
                platform.data_source === 'live'
                  ? <ThunderboltOutlined style={{ color: '#059669', fontSize: 14 }} />
                  : <SyncOutlined style={{ color: '#b45309', fontSize: 14 }} />
              }
              valueStyle={{ fontSize: 18, fontWeight: 600, color: platform.data_source === 'live' ? '#059669' : '#b45309' }}
            />
          </Col>
        </Row>

        <Descriptions
          size="small"
          column={1}
          colon={false}
          items={[
            {
              key: 'src',
              label: <Text type="secondary" style={{ fontSize: 12 }}>读取方式</Text>,
              children: <Text style={{ fontSize: 12 }}>{platform.data_source_label}</Text>,
            },
            {
              key: 'sync',
              label: <Text type="secondary" style={{ fontSize: 12 }}>最后同步</Text>,
              children: (
                <Text style={{ fontSize: 12 }}>
                  {platform.data_source === 'live' ? (
                    <Space size={4}><CheckCircleFilled style={{ color: '#059669', fontSize: 11 }} />每次请求实时拉取</Space>
                  ) : (
                    <Space size={4}>
                      {platform.token_valid ? (
                        <CheckCircleFilled style={{ color: '#059669', fontSize: 11 }} />
                      ) : (
                        <CloseCircleFilled style={{ color: '#ef4444', fontSize: 11 }} />
                      )}
                      {ago(platform.cache_updated_at)}
                    </Space>
                  )}
                </Text>
              ),
            },
            ...(platform.display_name
              ? [{
                  key: 'who',
                  label: <Text type="secondary" style={{ fontSize: 12 }}>账号</Text>,
                  children: (
                    <Text style={{ fontSize: 12 }}>
                      {platform.display_name}
                      {platform.vip_level ? ` · VIP ${platform.vip_level}` : ''}
                    </Text>
                  ),
                }]
              : []),
            ...(!ok && platform.error
              ? [{
                  key: 'err',
                  label: <Text type="secondary" style={{ fontSize: 12 }}>错误</Text>,
                  children: <Text type="danger" style={{ fontSize: 12 }}>{platform.error}</Text>,
                }]
              : []),
          ]}
        />

        {!ok && platform.configured && onGotoSettings && (
          <Button size="small" danger type="primary" ghost block onClick={onGotoSettings}>
            去更新凭证
          </Button>
        )}
      </Space>
    </Card>
  )
}
