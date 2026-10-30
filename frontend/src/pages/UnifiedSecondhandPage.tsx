import React, { useState, useEffect } from 'react'
import {
  Card, Table, Button, Modal, Switch, Typography, Input,
  App as AntApp, Tag, Tooltip, Checkbox, Alert
} from 'antd'
import {
  SwapOutlined, SafetyCertificateOutlined, ReloadOutlined,
  ThunderboltOutlined, WarningOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { api } from '../api'
import type { SecondhandItem } from '../api'

const { Text } = Typography

export const UnifiedSecondhandPage: React.FC = () => {
  const { message } = AntApp.useApp()
  const [items, setItems] = useState<SecondhandItem[]>([])
  const [counts, setCounts] = useState<Record<string, number>>({ akile: 0, acck: 0 })
  const [loading, setLoading] = useState(false)
  const [buyModalOpen, setBuyModalOpen] = useState(false)
  const [selectedItem, setSelectedItem] = useState<SecondhandItem | null>(null)
  const [safeMode, setSafeMode] = useState(true)
  const [forceBuyDepleted, setForceBuyDepleted] = useState(false)
  const [captcha, setCaptcha] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [buyResult, setBuyResult] = useState<any>(null)

  const loadData = async () => {
    setLoading(true)
    try {
      const res = await api.getUnifiedSecondhand(1, 50)
      setItems(res.items || [])
      setCounts(res.counts || { akile: 0, acck: 0 })
    } catch (err: any) {
      message.error(`加载二手市场失败: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [])

  const openBuyModal = (item: SecondhandItem) => {
    setSelectedItem(item)
    setSafeMode(true)
    setForceBuyDepleted(false)
    setCaptcha('')
    setBuyResult(null)
    setBuyModalOpen(true)
  }

  const handleExecuteBuy = async () => {
    if (!selectedItem) return
    
    // Safety check for depleted traffic
    if (!safeMode && selectedItem.traffic?.is_depleted && !forceBuyDepleted) {
      message.warning('该主机可用流量已耗尽！若确认购买，请勾选强制购买选项。')
      return
    }

    setSubmitting(true)
    try {
      const res = await api.buySecondhandItem(
        selectedItem.platform,
        selectedItem.id,
        selectedItem.price_cents,
        safeMode,
        captcha,
        forceBuyDepleted
      )
      setBuyResult(res)
      if (res.protected) {
        message.info(res.msg || '防误购保护锁生效，试算完成。')
      } else {
        message.success('二手抢购下单成功！')
        loadData()
      }
    } catch (err: any) {
      message.error(`抢购失败: ${err?.response?.data?.detail || err.message}`)
    } finally {
      setSubmitting(false)
    }
  }

  const renderTierTag = (item: SecondhandItem) => {
    const route = item.route
    if (!route) return null
    let color = 'default'
    if (route.tier === 'S') color = 'purple'
    else if (route.tier === 'A') color = 'blue'
    else if (route.tier === 'B') color = 'cyan'

    return (
      <Tag color={color} style={{ fontSize: 11, fontWeight: 600, padding: '0 6px', margin: 0 }}>
        {route.tier_label}
      </Tag>
    )
  }

  const columns: ColumnsType<SecondhandItem> = [
    {
      title: '市场源',
      dataIndex: 'platform',
      key: 'platform',
      width: 90,
      render: (p: string) => (
        <span className={p === 'akile' ? 'badge-proxy' : 'badge-online'} style={{ textTransform: 'uppercase' }}>
          {p === 'akile' ? 'PushShop' : 'Exchange'}
        </span>
      ),
    },
    {
      title: '挂售商品 / 线路与IP评估',
      key: 'title_route',
      width: 290,
      render: (_, r) => {
        const route = r.route
        return (
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <span style={{ fontWeight: 600, color: '#0f172a' }}>{r.title}</span>
              {renderTierTag(r)}
            </div>
            <div style={{ fontSize: 11, color: '#64748b', display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap', marginTop: 2 }}>
              <span>{r.node ? `${r.node} · ` : ''}{r.area}</span>
              {route?.native_ip && (
                <Tag color="green" style={{ fontSize: 10, padding: '0 4px', margin: 0 }}>原生IP</Tag>
              )}
              {!route?.ip_clean && (
                <Tag color="error" style={{ fontSize: 10, padding: '0 4px', margin: 0 }}>IP被墙⚠️</Tag>
              )}
              {route?.routes && route.routes.length > 0 && (
                <span style={{ color: '#2563eb', fontWeight: 500 }}>
                  {route.primary_route}
                </span>
              )}
            </div>
            {r.intro ? (
              <div style={{ fontSize: 10, color: '#94a3b8', marginTop: 1, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 280 }}>
                {r.intro}
              </div>
            ) : null}
          </div>
        )
      },
    },
    {
      title: '转让售价 / 续费成本',
      key: 'price',
      width: 140,
      render: (_, r) => (
        <div>
          <span className="mono-num" style={{ fontWeight: 700, color: '#0f172a', fontSize: 14 }}>
            ¥{typeof r.price_cny === 'number' ? r.price_cny.toFixed(2) : '0.00'}
          </span>
          {r.renew_price_cny ? (
            <div style={{ fontSize: 11, color: '#64748b' }}>
              续费: <span className="mono-num">¥{r.renew_price_cny.toFixed(2)}</span>
            </div>
          ) : null}
        </div>
      ),
    },
    {
      title: '可用流量 / 重置单价 (价值核算)',
      key: 'traffic',
      width: 190,
      render: (_, r) => {
        const tr = r.traffic
        if (!tr) {
          return <span className="mono-num">{r.flow ? `${r.flow}GB` : '无限'}</span>
        }
        return (
          <div>
            <div className="mono-num" style={{ fontSize: 12, fontWeight: 600 }}>
              {tr.remaining_gb} / {tr.total_gb} GB
            </div>
            <div style={{ marginTop: 2 }}>
              {tr.is_depleted ? (
                <Tag color="error" style={{ fontSize: 10, padding: '0 4px', margin: 0 }}>
                  已耗尽 🛑 (重置需¥{tr.reset_price_cny})
                </Tag>
              ) : tr.is_low ? (
                <Tag color="warning" style={{ fontSize: 10, padding: '0 4px', margin: 0 }}>
                  余量偏低 ({tr.remaining_ratio_percent}%)
                </Tag>
              ) : (
                <Tag color="success" style={{ fontSize: 10, padding: '0 4px', margin: 0 }}>
                  充沛 ({tr.remaining_ratio_percent}%)
                </Tag>
              )}
            </div>
          </div>
        )
      },
    },
    {
      title: '带宽 / 规格',
      key: 'specs',
      width: 120,
      render: (_, r) => (
        <span className="mono-num" style={{ fontSize: 12, color: '#334155' }}>
          {r.bandwidth ? `${r.bandwidth}M` : '—'}
        </span>
      ),
    },
    {
      title: '综合捡漏评估',
      key: 'sniping',
      width: 150,
      render: (_, r) => {
        const sn = r.sniping
        const pr = r.pricing_analysis
        if (!sn) return '—'

        let tagColor = 'default'
        if (sn.verdict_text === '【优质捡漏】') tagColor = 'green'
        else if (sn.verdict_text === '【可入手】') tagColor = 'blue'
        else tagColor = 'error'

        return (
          <div>
            <Tooltip title={sn.reasons.length > 0 ? sn.reasons.join('；') : `内在评估价值: ¥${pr?.intrinsic_value_cny || 0}`}>
              <Tag color={tagColor} style={{ fontWeight: 600, fontSize: 11, cursor: 'help' }}>
                {sn.verdict_text} ({sn.score}分)
              </Tag>
            </Tooltip>
            {pr?.real_total_cost_cny && pr.real_total_cost_cny !== r.price_cny ? (
              <div style={{ fontSize: 10, color: '#dc2626', marginTop: 2 }}>
                含重置总成本: ¥{pr.real_total_cost_cny}
              </div>
            ) : null}
          </div>
        )
      },
    },
    {
      title: '到期剩余',
      dataIndex: 'due_time',
      key: 'due_time',
      width: 140,
      render: (ts: number) => {
        if (!ts) return '—'
        const due = dayjs.unix(ts)
        const daysLeft = due.diff(dayjs(), 'day')
        return (
          <span className="mono-num" style={{ fontSize: 12 }}>
            {due.format('YYYY-MM-DD')}
            <span style={{ fontSize: 10, marginLeft: 6, color: daysLeft <= 5 ? '#b45309' : '#15803d' }}>
              ({daysLeft > 0 ? `剩${daysLeft}天` : '临期'})
            </span>
          </span>
        )
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_, r) => (
        <Button
          type="primary"
          size="small"
          style={{ fontSize: 12, height: 26, padding: '0 8px' }}
          icon={<ThunderboltOutlined />}
          onClick={() => openBuyModal(r)}
        >
          抢购/捡漏
        </Button>
      ),
    },
  ]

  const isDepleted = selectedItem?.traffic?.is_depleted

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card
        className="ashan-card"
        title={
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <SwapOutlined style={{ color: '#2563eb' }} />
              双平台二手集市与捡漏大宽表（Akile PushShop: {counts.akile} / ACCK Exchange: {counts.acck}）
            </span>
            <Button
              type="text"
              size="small"
              icon={<ReloadOutlined spin={loading} />}
              onClick={loadData}
            >
              刷新集市
            </Button>
          </div>
        }
      >
        <Table
          columns={columns}
          dataSource={items}
          rowKey="uid"
          loading={loading}
          pagination={{ pageSize: 15, size: 'small' }}
          size="small"
        />
      </Card>

      {/* Buy Modal */}
      <Modal
        title={`二手转让抢单 - ${selectedItem?.title}`}
        open={buyModalOpen}
        onCancel={() => setBuyModalOpen(false)}
        onOk={handleExecuteBuy}
        confirmLoading={submitting}
        okText={safeMode ? '执行试算 (防误购)' : (isDepleted && !forceBuyDepleted ? '流量耗尽(需勾选强制)' : '确认立即付款抢单')}
        okButtonProps={{ disabled: !safeMode && isDepleted && !forceBuyDepleted }}
        cancelText="取消"
        width={560}
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14, padding: '10px 0' }}>
          <div>
            <Text type="secondary">交易市场：</Text>
            <Tag color={selectedItem?.platform === 'akile' ? 'blue' : 'green'} style={{ textTransform: 'uppercase' }}>
              {selectedItem?.platform === 'akile' ? 'Akile PushShop' : 'ACCK Exchange'}
            </Tag>
          </div>

          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <Text type="secondary">转让标价：</Text>
              <span className="mono-num" style={{ fontSize: 18, fontWeight: 700, color: '#0f172a' }}>
                ¥{selectedItem?.price_cny?.toFixed(2)}
              </span>
            </div>
            <div>
              <Text type="secondary">线路评估：</Text>
              <Tag color={selectedItem?.route?.tier === 'S' ? 'purple' : 'blue'}>
                {selectedItem?.route?.tier_label} ({selectedItem?.route?.primary_route})
              </Tag>
            </div>
          </div>

          {/* Traffic Warning Alert */}
          {isDepleted && (
            <Alert
              type="error"
              showIcon
              icon={<WarningOutlined />}
              message="【重大风控预警：可用流量已全部耗尽！】"
              description={
                <div style={{ fontSize: 12 }}>
                  <div>当前主机可用流量为 <b>0 GB</b>（总计 {selectedItem?.traffic?.total_gb}GB 已全被原号主用光）。</div>
                  <div>官方重置流量单价为 <b>¥{selectedItem?.traffic?.reset_price_cny}</b>。</div>
                  <div style={{ marginTop: 4, fontWeight: 600, color: '#b91c1c' }}>
                    真实买入总成本 = 挂售价 ¥{selectedItem?.price_cny} + 流量重置费 ¥{selectedItem?.traffic?.reset_price_cny} = <b>¥{selectedItem?.pricing_analysis?.real_total_cost_cny}</b>！
                  </div>
                </div>
              }
            />
          )}

          {/* Detailed Traffic Info */}
          <div style={{ padding: 10, background: '#f8fafc', borderRadius: 6, border: '1px solid #e2e8f0', fontSize: 12 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>已用流量：<b>{selectedItem?.traffic?.used_gb ?? 0} GB</b></span>
              <span>总额度：<b>{selectedItem?.traffic?.total_gb ?? 0} GB</b></span>
              <span>剩余可用：<b style={{ color: isDepleted ? '#dc2626' : '#15803d' }}>{selectedItem?.traffic?.remaining_gb ?? 0} GB</b></span>
            </div>
          </div>

          {selectedItem?.platform === 'akile' && !safeMode && (
            <div>
              <Text type="secondary">图形验证码（Akile 下单）：</Text>
              <Input
                placeholder="请输入 Akile 验证码（如需要）"
                value={captcha}
                onChange={(e) => setCaptcha(e.target.value)}
                style={{ marginTop: 4 }}
              />
            </div>
          )}

          {/* Safe Mode Switch */}
          <div style={{ padding: 12, background: safeMode ? '#eff6ff' : '#fffbeb', borderRadius: 6, border: `1px solid ${safeMode ? '#bfdbfe' : '#fde68a'}` }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
              <span style={{ fontWeight: 600, color: safeMode ? '#1d4ed8' : '#b45309', display: 'flex', alignItems: 'center', gap: 6 }}>
                <SafetyCertificateOutlined />
                防误购安全模式 (Safe Dry-Run)
              </span>
              <Switch checked={safeMode} onChange={setSafeMode} />
            </div>
            <p style={{ margin: 0, fontSize: 12, color: safeMode ? '#1e40af' : '#92400e' }}>
              {safeMode
                ? '已开启安全保护锁：点击确认仅对订单参数和接口合法性做试算校验，不发起真实资金扣除。'
                : '⚠️ 警告：保护锁已关闭！点击确认将直接从您的账户余额划扣并抢购该二手主机！'}
            </p>
          </div>

          {/* Force Buy Depleted Checkbox */}
          {!safeMode && isDepleted && (
            <div style={{ padding: '8px 12px', background: '#fef2f2', borderRadius: 6, border: '1px solid #fecaca' }}>
              <Checkbox checked={forceBuyDepleted} onChange={(e) => setForceBuyDepleted(e.target.checked)}>
                <span style={{ fontSize: 12, color: '#991b1b', fontWeight: 600 }}>
                  我已明确知晓该主机 0G 流量已用光，确认强制买入并愿意承担后续 ¥{selectedItem?.traffic?.reset_price_cny} 重置流量费
                </span>
              </Checkbox>
            </div>
          )}

          {buyResult && (
            <div style={{ padding: 10, background: '#f8fafc', borderRadius: 6, border: '1px solid #e2e8f0' }}>
              <div style={{ fontWeight: 600, marginBottom: 4, color: '#0f172a' }}>回执：</div>
              <pre style={{ margin: 0, fontSize: 11, maxHeight: 150, overflow: 'auto', fontFamily: 'monospace' }}>
                {JSON.stringify(buyResult, null, 2)}
              </pre>
            </div>
          )}
        </div>
      </Modal>
    </div>
  )
}
