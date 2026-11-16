import { useCallback, useEffect, useState } from 'react'
import {
  Alert, App as AntApp, Button, Card, Col, Descriptions, Divider, Drawer, Empty,
  Form, Input, InputNumber, Popconfirm, Progress, Row, Select, Space, Spin,
  Statistic, Table, Tag, Tooltip, Typography,
} from 'antd'
import {
  CloudServerOutlined, ReloadOutlined, WalletOutlined, CrownOutlined,
  CheckCircleOutlined, LoginOutlined, GlobalOutlined, ApiOutlined,
  CodeOutlined, DeleteOutlined, PlusOutlined, ExportOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '../api'
import type { LazyCatAccount, LazyCatInstance, LazyCatPortMapping } from '../api'

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

  // ---- 实例详情抽屉：端口映射 + 终端 ----
  const [detail, setDetail] = useState<LazyCatInstance | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [addingPort, setAddingPort] = useState(false)
  const [portForm] = Form.useForm<{ protocol: string; privatePort: number; remark?: string }>()

  const openDetail = useCallback(async (inst: LazyCatInstance) => {
    if (inst.id == null) return
    setDetail(inst)
    setDetailLoading(true)
    try {
      const full = await api.getLazyCatInstance(inst.id)
      if (!full.error) setDetail(full)
    } catch {
      message.error('加载实例详情失败')
    } finally {
      setDetailLoading(false)
    }
  }, [message])

  const refreshDetail = useCallback(async () => {
    if (detail?.id == null) return
    const full = await api.getLazyCatInstance(detail.id).catch(() => null)
    if (full && !full.error) setDetail(full)
  }, [detail?.id])

  const submitPort = useCallback(async (values: { protocol: string; privatePort: number; remark?: string }) => {
    if (detail?.id == null) return
    setAddingPort(true)
    try {
      const res = await api.addLazyCatPort(detail.id, values)
      if (res.error) {
        message.error(`${res.error}${res.detail ? `：${res.detail}` : ''}`)
      } else {
        const m = res.mapping
        message.success(m?.publicPort ? `已添加 公网 ${m.publicPort} → 内网 ${m.privatePort}` : '端口映射已添加')
        portForm.resetFields()
        await refreshDetail()
        load()
      }
    } catch {
      message.error('添加端口映射失败')
    } finally {
      setAddingPort(false)
    }
  }, [detail?.id, portForm, refreshDetail, load, message])

  const removePort = useCallback(async (portId: number) => {
    if (detail?.id == null) return
    try {
      const res = await api.deleteLazyCatPort(detail.id, portId)
      if (res.error) {
        message.error(res.error)
      } else {
        message.success('端口映射已删除')
        await refreshDetail()
        load()
      }
    } catch {
      message.error('删除端口映射失败')
    }
  }, [detail?.id, refreshDetail, load, message])

  const openTerminal = useCallback(async () => {
    if (detail?.id == null) return
    try {
      const res = await api.getLazyCatTerminal(detail.id)
      if (res.error || !res.official_page) {
        message.error(res.error ?? '无法获取终端地址')
        return
      }
      window.open(res.official_page, '_blank', 'noopener,noreferrer')
    } catch {
      message.error('打开终端失败')
    }
  }, [detail?.id, message])

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
      title: '操作',
      key: 'action',
      width: 90,
      fixed: 'right' as const,
      render: (_: unknown, r) => (
        <Button size="small" type="link" onClick={() => openDetail(r)}>详情</Button>
      ),
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

      <Drawer
        width={560}
        open={detail !== null}
        onClose={() => setDetail(null)}
        title={
          <Space>
            <CloudServerOutlined />
            <span>{detail?.name ?? '实例详情'}</span>
          </Space>
        }
        extra={
          <Space>
            <Button size="small" icon={<ReloadOutlined />} onClick={refreshDetail} loading={detailLoading}>
              刷新
            </Button>
            <Tooltip title="在官方 xterm.js 终端打开（同一 WebSocket 后端）">
              <Button size="small" type="primary" icon={<CodeOutlined />} onClick={openTerminal}>
                打开终端
              </Button>
            </Tooltip>
          </Space>
        }
      >
        {detail && (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Descriptions
              size="small"
              column={2}
              bordered
              items={[
                { key: 'id', label: '实例 ID', children: show(detail.id) },
                { key: 'incus', label: 'Incus', children: show(detail.incusId) },
                { key: 'img', label: '系统', children: show(detail.imageName ?? detail.image) },
                { key: 'ip', label: '内网 IP', children: <Text code>{show(detail.ipv4)}</Text> },
                { key: 'ssh', label: 'SSH 内网端口', children: show(detail.sshPort ?? detail.ssh_port ?? 22) },
                { key: 'limit', label: '端口映射额度', children: show(detail.portLimit ?? detail.port_limit) },
                {
                  key: 'pub',
                  label: '公网 IP',
                  children: detail.hidePublicIpOnlyCopy ?? detail.hide_public_ip_only_copy
                    ? <Text type="secondary">平台隐藏，需在官网复制</Text>
                    : show(detail.natPublicIp ?? detail.nat_public_ip ?? detail.hostPublicIp),
                },
                { key: 'bw', label: '带宽限制', children: show(detail.limitsIngress ?? detail.limitsEgress) },
              ]}
            />

            <Divider titlePlacement="left" style={{ margin: 0 }}>
              <Space size={6}><ApiOutlined /><span>端口映射</span></Space>
            </Divider>

            <Form
              form={portForm}
              layout="inline"
              initialValues={{ protocol: 'tcp', privatePort: 22 }}
              onFinish={submitPort}
              style={{ rowGap: 8 }}
            >
              <Form.Item name="protocol" rules={[{ required: true }]}>
                <Select
                  style={{ width: 90 }}
                  options={[
                    { value: 'tcp', label: 'TCP' },
                    { value: 'udp', label: 'UDP' },
                  ]}
                />
              </Form.Item>
              <Form.Item name="privatePort" rules={[{ required: true, message: '内网端口' }]}>
                <InputNumber min={1} max={65535} placeholder="内网端口" style={{ width: 120 }} />
              </Form.Item>
              <Form.Item name="remark">
                <Input placeholder="备注（可选）" style={{ width: 130 }} allowClear />
              </Form.Item>
              <Form.Item>
                <Button type="primary" htmlType="submit" icon={<PlusOutlined />} loading={addingPort}>
                  添加映射
                </Button>
              </Form.Item>
            </Form>

            <Text type="secondary" style={{ fontSize: 12 }}>
              公网端口由平台自动分配，不需要自己填。添加后用「公网端口」访问实例内部端口。
            </Text>

            <Table<LazyCatPortMapping>
              rowKey={(r) => String(r.id)}
              size="small"
              pagination={false}
              dataSource={(detail.portMappings ?? detail.port_mappings ?? []) as LazyCatPortMapping[]}
              locale={{ emptyText: <Empty description="暂无端口映射" image={Empty.PRESENTED_IMAGE_SIMPLE} /> }}
              columns={[
                {
                  title: '公网端口',
                  key: 'pub',
                  width: 110,
                  render: (_: unknown, r) => <Text code>{show(r.publicPort ?? r.public_port)}</Text>,
                },
                {
                  title: '内网端口',
                  key: 'priv',
                  width: 100,
                  render: (_: unknown, r) => show(r.privatePort ?? r.private_port),
                },
                {
                  title: '协议',
                  key: 'proto',
                  width: 80,
                  render: (_: unknown, r) => <Tag>{String(r.protocol ?? '').toUpperCase()}</Tag>,
                },
                {
                  title: '备注',
                  key: 'remark',
                  render: (_: unknown, r) => show(r.remark),
                },
                {
                  title: '',
                  key: 'op',
                  width: 60,
                  render: (_: unknown, r) => (
                    <Popconfirm
                      title="删除这条端口映射？"
                      description="只删除映射，不影响实例本身。"
                      okText="删除"
                      cancelText="取消"
                      okButtonProps={{ danger: true }}
                      onConfirm={() => removePort(r.id)}
                    >
                      <Button size="small" type="link" danger icon={<DeleteOutlined />} />
                    </Popconfirm>
                  ),
                },
              ]}
            />

            <Button
              icon={<ExportOutlined />}
              href={`https://dash.lazycat.wiki/instances`}
              target="_blank"
              rel="noreferrer"
              block
            >
              在官网打开实例控制台
            </Button>
          </Space>
        )}
      </Drawer>
    </div>
  )
}
