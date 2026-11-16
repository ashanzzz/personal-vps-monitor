import React, { useState, useEffect, useCallback } from 'react'
import {
  Card, Row, Col, Statistic, Table, Button, Switch, Tag, Space,
  Tooltip, Modal, InputNumber, App as AntApp, Typography
} from 'antd'
import {
  CloudServerOutlined, WalletOutlined,
  CopyOutlined, ReloadOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { api } from '../api'
import type { UnifiedServer, RefreshState } from '../api'

const { Text } = Typography

export const UnifiedDashboard: React.FC = () => {
  const { message } = AntApp.useApp()
  const [servers, setServers] = useState<UnifiedServer[]>([])
  const [counts, setCounts] = useState<Record<string, number>>({ akile: 0, acck: 0 })
  const [refreshState, setRefreshState] = useState<RefreshState | null>(null)
  const [loading, setLoading] = useState(false)
  const [renewModalOpen, setRenewModalOpen] = useState(false)
  const [selectedServer, setSelectedServer] = useState<UnifiedServer | null>(null)
  const [renewCycle, setRenewCycle] = useState(1)
  const [autoPay, setAutoPay] = useState(true)
  const [submittingRenew, setSubmittingRenew] = useState(false)

  // Account balances
  const [akileBalance, setAkileBalance] = useState<number | null>(null)
  const [acckBalance, setAcckBalance] = useState<number | null>(null)
  const [akCoin, setAkCoin] = useState<number | null>(null)
  const [acCoin, setAcCoin] = useState<number | null>(null)

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [srvRes, refRes, akAcc, acAcc] = await Promise.all([
        api.getUnifiedServers().catch(() => ({ total: 0, servers: [], counts: { akile: 0, acck: 0 } })),
        api.getRefreshStatus().catch(() => null),
        api.getAccount().catch(() => null),
        api.getAcckAccount().catch(() => null),
      ])

      setServers(srvRes.servers || [])
      setCounts(srvRes.counts || { akile: 0, acck: 0 })
      if (refRes) setRefreshState(refRes)

      // Extract balances
      if (akAcc?.profile) {
        setAkileBalance(akAcc.profile.money != null ? akAcc.profile.money / 100 : null)
        setAkCoin(akAcc.profile.ak_coin ?? null)
      }
      if (acAcc?.profile) {
        // ACCK money is in cents (分), coins is AC币
        const acckMoney = acAcc.profile.money != null ? acAcc.profile.money / 100 : (acAcc.profile.points != null ? acAcc.profile.points / 100 : null)
        setAcckBalance(acckMoney)
        setAcCoin(acAcc.profile.coins ?? null)
      }
    } catch (e) {
      console.error('Unified dashboard load error:', e)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadData()
  }, [loadData])

  // Polling loop based on current interval
  useEffect(() => {
    // 硬下限 60s，避免高频刷平台接口
    const intervalSec = Math.max(60, refreshState?.current_interval || 60)
    const timer = setInterval(() => {
      loadData()
    }, intervalSec * 1000)
    return () => clearInterval(timer)
  }, [refreshState?.current_interval, loadData])

  // Copy helper
  const copyText = (txt: string) => {
    navigator.clipboard.writeText(txt)
    message.success('已复制到剪贴板')
  }

  // Handle auto renew toggle
  const handleToggleAutoRenew = async (record: UnifiedServer, checked: boolean) => {
    try {
      await api.updateServerAutoRenew(record.platform, record.id, checked)
      message.success(`${record.name} 自动续费已${checked ? '开启' : '关闭'}`)
      setServers(prev => prev.map(s => s.uid === record.uid ? { ...s, auto_renew: checked } : s))
    } catch (err: any) {
      message.error(`设置自动续费失败: ${err?.response?.data?.detail || err.message}`)
    }
  }

  // Handle open renew modal
  const openRenewModal = (record: UnifiedServer) => {
    setSelectedServer(record)
    setRenewCycle(1)
    setAutoPay(true)
    setRenewModalOpen(true)
  }

  // Execute renew order
  const handleExecuteRenew = async () => {
    if (!selectedServer) return
    setSubmittingRenew(true)
    try {
      const res = await api.renewServer(selectedServer.platform, selectedServer.id, renewCycle, autoPay)
      if (res.paid) {
        message.success(`${selectedServer.name} 续费成功，余额已划扣！`)
      } else {
        message.info(`${selectedServer.name} 续费单据已创建，请前往账单中心完成支付。`)
      }
      setRenewModalOpen(false)
      loadData()
    } catch (err: any) {
      message.error(`续费失败: ${err?.response?.data?.detail || err.message}`)
    } finally {
      setSubmittingRenew(false)
    }
  }

  // Dual-rate burst toggles
  const handleStartBurst = async (durationSec: number) => {
    try {
      const res = await api.startBurstRefresh(durationSec, 2)
      setRefreshState(res)
      message.success(`已开启 ${durationSec / 60} 分钟高速狙击刷新（2s/次）`)
      loadData()
    } catch (err: any) {
      message.error('切换高速刷新失败')
    }
  }

  const handleStopBurst = async () => {
    try {
      const res = await api.stopBurstRefresh()
      setRefreshState(res)
      message.info('已恢复为标准慢刷新（30s/次）')
      loadData()
    } catch (err: any) {
      message.error('恢复慢刷新失败')
    }
  }

  // Columns for 38px table
  const columns: ColumnsType<UnifiedServer> = [
    {
      title: '平台',
      dataIndex: 'platform',
      key: 'platform',
      width: 90,
      render: (p: string) => (
        <span className={p === 'akile' ? 'badge-proxy' : 'badge-online'} style={{ textTransform: 'uppercase' }}>
          {p}
        </span>
      ),
    },
    {
      title: '主机名称 / 别名',
      dataIndex: 'name',
      key: 'name',
      width: 220,
      render: (name: string, r) => (
        <div>
          <div style={{ fontWeight: 600, color: '#0f172a' }}>{name}</div>
          <div style={{ fontSize: 11, color: '#64748b' }}>{r.node_name} · {r.area_name}</div>
        </div>
      ),
    },
    {
      title: '公网 IPv4',
      dataIndex: 'ipv4',
      key: 'ipv4',
      width: 170,
      render: (ip: string) => (
        <span className="mono-num" style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
          {ip || '—'}
          {ip && (
            <Tooltip title="复制 IP">
              <Button type="text" size="small" icon={<CopyOutlined style={{ fontSize: 12, color: '#64748b' }} />} onClick={() => copyText(ip)} />
            </Tooltip>
          )}
        </span>
      ),
    },
    {
      title: '配置规格',
      key: 'specs',
      width: 170,
      render: (_, r) => (
        <span className="mono-num" style={{ fontSize: 12, color: '#334155' }}>
          {r.cpu}C / {r.memory >= 1024 ? `${r.memory / 1024}G` : `${r.memory}M`} / {r.disk}G / {r.bandwidth}M
        </span>
      ),
    },
    {
      title: '续费价格 / 周期',
      key: 'price',
      width: 140,
      render: (_, r) => (
        <div className="mono-num" style={{ fontSize: 13 }}>
          <span style={{ fontWeight: 700, color: '#0f172a' }}>
            {r.price_cny > 0 ? `¥${r.price_cny.toFixed(2)}` : (r.price ? `¥${(r.price / 100).toFixed(2)}` : '—')}
          </span>
          <span style={{ fontSize: 11, color: '#64748b', marginLeft: 4 }}>
            /{r.cycle_text || (r.cycle === 12 ? '年' : `${r.cycle || 1}月`)}
          </span>
        </div>
      ),
    },
    {
      title: '已用 / 总流量',
      key: 'flow',
      width: 140,
      render: (_, r) => {
        const flowTotal = r.flow > 0 ? `${r.flow}GB` : '无限'
        const usedGb = r.used_flow > 0 ? (r.used_flow / (1024 ** 3)).toFixed(1) : '0'
        return <span className="mono-num" style={{ fontSize: 12 }}>{usedGb} / {flowTotal}</span>
      },
    },
    {
      title: '到期时间',
      dataIndex: 'due_time',
      key: 'due_time',
      width: 170,
      render: (ts: number) => {
        if (!ts) return '—'
        const due = dayjs.unix(ts)
        const daysLeft = due.diff(dayjs(), 'day')
        const isUrgent = daysLeft <= 7
        return (
          <div className="mono-num" style={{ fontSize: 12 }}>
            <div>{due.format('YYYY-MM-DD')}</div>
            <span className={isUrgent ? 'badge-warning' : 'badge-online'} style={{ fontSize: 10, padding: '0 4px' }}>
              {daysLeft > 0 ? `剩 ${daysLeft} 天` : '已到期'}
            </span>
          </div>
        )
      },
    },
    {
      title: '自动续费',
      dataIndex: 'auto_renew',
      key: 'auto_renew',
      width: 100,
      render: (checked: boolean, r) => (
        <Switch
          size="small"
          checked={checked}
          onChange={(val) => handleToggleAutoRenew(r, val)}
        />
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 110,
      render: (_, r) => (
        <Space size={4}>
          <Button
            type="primary"
            size="small"
            style={{ fontSize: 12, height: 24, padding: '0 8px' }}
            onClick={() => openRenewModal(r)}
          >
            续费
          </Button>
        </Space>
      ),
    },
  ]

  const isBurst = refreshState?.mode === 'burst'
  const burstRemaining = refreshState?.remaining_seconds ?? 0

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* 4-Metric Overview Cards */}
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} md={6}>
          <Card className="ashan-card" bordered={false}>
            <Statistic
              title={<span style={{ color: '#64748b', fontSize: 13, fontWeight: 500 }}>在线服务器总数</span>}
              value={servers.length}
              suffix={<span style={{ fontSize: 12, color: '#64748b' }}>台 (Akile: {counts.akile} / ACCK: {counts.acck})</span>}
              prefix={<CloudServerOutlined style={{ color: '#2563eb', marginRight: 6 }} />}
              valueStyle={{ color: '#0f172a', fontWeight: 700, fontSize: 24 }}
            />
          </Card>
        </Col>

        <Col xs={24} sm={12} md={6}>
          <Card className="ashan-card" bordered={false}>
            <Statistic
              title={<span style={{ color: '#64748b', fontSize: 13, fontWeight: 500 }}>Akile 余额 & 资产</span>}
              value={akileBalance != null ? `¥${akileBalance.toFixed(2)}` : '未连接'}
              suffix={akCoin != null ? <span style={{ fontSize: 12, color: '#64748b' }}>({akCoin} AK币)</span> : ''}
              prefix={<WalletOutlined style={{ color: '#059669', marginRight: 6 }} />}
              valueStyle={{ color: '#0f172a', fontWeight: 700, fontSize: 22 }}
            />
          </Card>
        </Col>

        <Col xs={24} sm={12} md={6}>
          <Card className="ashan-card" bordered={false}>
            <Statistic
              title={<span style={{ color: '#64748b', fontSize: 13, fontWeight: 500 }}>ACCK 余额 & 积分</span>}
              value={acckBalance != null ? `¥${acckBalance.toFixed(2)}` : '未连接'}
              suffix={acCoin != null ? <span style={{ fontSize: 12, color: '#64748b' }}>({acCoin} AC币)</span> : ''}
              prefix={<WalletOutlined style={{ color: '#0284c7', marginRight: 6 }} />}
              valueStyle={{ color: '#0f172a', fontWeight: 700, fontSize: 22 }}
            />
          </Card>
        </Col>

        <Col xs={24} sm={12} md={6}>
          <Card className="ashan-card" bordered={false}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <div style={{ color: '#64748b', fontSize: 13, fontWeight: 500, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span>双频刷新状态</span>
                <span className={isBurst ? 'refresh-pill refresh-pill-burst' : 'refresh-pill refresh-pill-slow'}>
                  {isBurst ? `⚡ 高速狙击 (${refreshState?.burst_interval}s)` : `🌱 平时慢刷新 (${refreshState?.slow_interval}s)`}
                </span>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: 4 }}>
                <span style={{ fontSize: 13, color: '#334155' }}>
                  {isBurst ? `剩余 ${Math.floor(burstRemaining / 60)}分${burstRemaining % 60}秒` : '背景平稳巡检'}
                </span>
                <Space size={6}>
                  {isBurst ? (
                    <Button size="small" danger onClick={handleStopBurst}>停止高速</Button>
                  ) : (
                    <>
                      <Button size="small" type="primary" ghost onClick={() => handleStartBurst(300)}>5分高速</Button>
                      <Button size="small" type="primary" ghost onClick={() => handleStartBurst(900)}>15分高速</Button>
                    </>
                  )}
                </Space>
              </div>
            </div>
          </Card>
        </Col>
      </Row>

      {/* Main Servers High-Density Wide Table (38px Row Height) */}
      <Card
        className="ashan-card"
        title={
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <CloudServerOutlined style={{ color: '#2563eb' }} />
              核心服务器实例资产大宽表（38px 高密人体工学）
            </span>
            <Button
              type="text"
              size="small"
              icon={<ReloadOutlined spin={loading} />}
              onClick={loadData}
            >
              刷新资产
            </Button>
          </div>
        }
      >
        <Table
          columns={columns}
          dataSource={servers}
          rowKey="uid"
          loading={loading}
          pagination={false}
          size="small"
        />
      </Card>

      {/* Renew Modal */}
      <Modal
        title={`服务器续费 - ${selectedServer?.name}`}
        open={renewModalOpen}
        onCancel={() => setRenewModalOpen(false)}
        onOk={handleExecuteRenew}
        confirmLoading={submittingRenew}
        okText="立即发起续费"
        cancelText="取消"
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14, padding: '10px 0' }}>
          <div>
            <Text type="secondary">所属平台：</Text>
            <Tag color={selectedServer?.platform === 'akile' ? 'blue' : 'green'} style={{ textTransform: 'uppercase' }}>
              {selectedServer?.platform}
            </Tag>
          </div>
          <div>
            <Text type="secondary">当前到期：</Text>
            <span className="mono-num">
              {selectedServer?.due_time ? dayjs.unix(selectedServer.due_time).format('YYYY-MM-DD HH:mm') : '—'}
            </span>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <Text type="secondary">续费周期：</Text>
            <InputNumber min={1} max={12} value={renewCycle} onChange={(v) => setRenewCycle(v || 1)} addonAfter="个月" />
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <Text type="secondary">自动余额扣款：</Text>
            <Switch checked={autoPay} onChange={setAutoPay} />
            <span style={{ fontSize: 12, color: '#64748b' }}>生成续费订单后立即从账户余额结清</span>
          </div>
        </div>
      </Modal>
    </div>
  )
}
