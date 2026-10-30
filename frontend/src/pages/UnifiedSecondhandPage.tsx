import React, { useState, useEffect } from 'react'
import {
  Card, Table, Button, Modal, Switch, Typography, Input,
  App as AntApp, Tag
} from 'antd'
import {
  SwapOutlined, SafetyCertificateOutlined, ReloadOutlined,
  ThunderboltOutlined
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
    setCaptcha('')
    setBuyResult(null)
    setBuyModalOpen(true)
  }

  const handleExecuteBuy = async () => {
    if (!selectedItem) return
    setSubmitting(true)
    try {
      const res = await api.buySecondhandItem(
        selectedItem.platform,
        selectedItem.id,
        selectedItem.price_cents,
        safeMode,
        captcha
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
      title: '挂售商品 / 套餐',
      dataIndex: 'title',
      key: 'title',
      width: 260,
      render: (t: string, r) => (
        <div>
          <div style={{ fontWeight: 600, color: '#0f172a' }}>{t}</div>
          <div style={{ fontSize: 11, color: '#64748b', display: 'flex', alignItems: 'center', gap: 4, flexWrap: 'wrap' }}>
            <span>{r.node ? `${r.node} · ` : ''}{r.area}</span>
            {r.intro ? (
              <span className="badge-warning" style={{ fontSize: 10, padding: '0 4px', borderRadius: 3 }}>
                {r.intro}
              </span>
            ) : null}
          </div>
        </div>
      ),
    },
    {
      title: '转让售价 / 续费成本',
      key: 'price',
      width: 150,
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
      title: '带宽 / 流量',
      key: 'specs',
      width: 160,
      render: (_, r) => (
        <span className="mono-num" style={{ fontSize: 12, color: '#334155' }}>
          {r.bandwidth ? `${r.bandwidth}Mbps` : '—'} / {r.flow ? `${r.flow}GB` : '无限'}
        </span>
      ),
    },
    {
      title: '到期剩余',
      dataIndex: 'due_time',
      key: 'due_time',
      width: 150,
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
      title: '卖家',
      dataIndex: 'seller',
      key: 'seller',
      width: 110,
      render: (s: string) => <span style={{ fontSize: 12, color: '#64748b' }}>{s}</span>,
    },
    {
      title: '操作',
      key: 'actions',
      width: 110,
      render: (_, r) => (
        <Button
          type="primary"
          size="small"
          style={{ fontSize: 12, height: 26, padding: '0 10px' }}
          icon={<ThunderboltOutlined />}
          onClick={() => openBuyModal(r)}
        >
          抢购/捡漏
        </Button>
      ),
    },
  ]

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
        okText={safeMode ? '执行试算 (防误购)' : '确认立即付款抢单'}
        cancelText="取消"
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14, padding: '10px 0' }}>
          <div>
            <Text type="secondary">交易市场：</Text>
            <Tag color={selectedItem?.platform === 'akile' ? 'blue' : 'green'} style={{ textTransform: 'uppercase' }}>
              {selectedItem?.platform === 'akile' ? 'Akile PushShop' : 'ACCK Exchange'}
            </Tag>
          </div>
          <div>
            <Text type="secondary">转让标价：</Text>
            <span className="mono-num" style={{ fontSize: 18, fontWeight: 700, color: '#0f172a' }}>
              ¥{selectedItem?.price_cny?.toFixed(2)}
            </span>
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
