import React, { useState, useEffect } from 'react'
import {
  Card, Table, Tag, Button, Modal, Switch, Typography,
  App as AntApp
} from 'antd'
import {
  ShoppingCartOutlined,
  SafetyCertificateOutlined, ReloadOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '../api'

const { Text } = Typography

export const UnifiedStorePage: React.FC = () => {
  const { message } = AntApp.useApp()
  const [loading, setLoading] = useState(false)
  const [storeData, setStoreData] = useState<any>(null)
  const [buyModalOpen, setBuyModalOpen] = useState(false)
  const [selectedPlan, setSelectedPlan] = useState<any>(null)
  const [safeMode, setSafeMode] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [buyResult, setBuyResult] = useState<any>(null)

  const loadStore = async () => {
    setLoading(true)
    try {
      const res = await api.getUnifiedStore()
      setStoreData(res)
    } catch (e: any) {
      message.error(`加载商店数据失败: ${e.message}`)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadStore()
  }, [])

  // Extract Akile VPS Packages
  const akilePackages: any[] = []
  if (storeData?.akile_store) {
    const raw = storeData.akile_store
    const areas = Array.isArray(raw) ? raw : (raw.list || raw.areas || [])
    for (const area of areas) {
      const nodes = area.nodes || area.Groups || []
      for (const node of nodes) {
        const plans = node.plans || node.Plans || []
        for (const plan of plans) {
          akilePackages.push({
            platform: 'akile',
            id: plan.id,
            name: plan.plan_name || plan.name,
            area: area.area_name || area.name || '全球',
            node: node.group_name || node.node_name || '主节点',
            cpu: plan.cpu || 1,
            memory: plan.memory || 1024,
            disk: plan.disk || 10,
            bandwidth: plan.bandwidth || 1000,
            flow: plan.flow || 1000,
            price_cny: (plan.price || 0) / 100,
            cycle: plan.cycle || 1,
            stock: plan.stock ?? 1,
            raw: plan,
          })
        }
      }
    }
  }

  // Handle open buy modal
  const openBuyModal = (record: any) => {
    setSelectedPlan(record)
    setSafeMode(true)
    setBuyResult(null)
    setBuyModalOpen(true)
  }

  // Handle submit purchase
  const handleExecuteBuy = async () => {
    if (!selectedPlan) return
    setSubmitting(true)
    try {
      const res = await api.buyStorePackage(selectedPlan.platform, selectedPlan.id, [], safeMode)
      setBuyResult(res)
      if (res.protected) {
        message.info(res.msg || '防误购安全模式已生效，试算完成。')
      } else {
        message.success('新购下单已提交！')
      }
    } catch (err: any) {
      message.error(`购买失败: ${err?.response?.data?.detail || err.message}`)
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<any> = [
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
      title: '套餐名称 / 节点',
      dataIndex: 'name',
      key: 'name',
      width: 240,
      render: (name: string, r) => (
        <div>
          <div style={{ fontWeight: 600, color: '#0f172a' }}>{name}</div>
          <div style={{ fontSize: 11, color: '#64748b' }}>{r.node} · {r.area}</div>
        </div>
      ),
    },
    {
      title: '核心配置',
      key: 'specs',
      width: 180,
      render: (_, r) => (
        <span className="mono-num" style={{ fontSize: 12, color: '#334155' }}>
          {r.cpu}C / {r.memory >= 1024 ? `${r.memory / 1024}G` : `${r.memory}M`} / {r.disk}G / {r.bandwidth}M
        </span>
      ),
    },
    {
      title: '月流量',
      dataIndex: 'flow',
      key: 'flow',
      width: 100,
      render: (f: number) => <span className="mono-num">{f > 0 ? `${f} GB` : '无限'}</span>,
    },
    {
      title: '价格',
      dataIndex: 'price_cny',
      key: 'price_cny',
      width: 120,
      render: (p: number, r) => (
        <span className="mono-num" style={{ fontWeight: 700, color: '#0f172a', fontSize: 14 }}>
          ¥{p.toFixed(2)}
          <span style={{ fontSize: 11, color: '#64748b', fontWeight: 400 }}>/{r.cycle === 12 ? '年' : '月'}</span>
        </span>
      ),
    },
    {
      title: '库存状态',
      dataIndex: 'stock',
      key: 'stock',
      width: 110,
      render: (s: number) => (
        s > 0 ? (
          <span className="badge-online">有货 (余{s})</span>
        ) : (
          <span className="badge-offline">缺货售罄</span>
        )
      ),
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
          onClick={() => openBuyModal(r)}
        >
          立即选购
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
              <ShoppingCartOutlined style={{ color: '#2563eb' }} />
              官方一手云服务器商城（38px 高密人体工学）
            </span>
            <Button
              type="text"
              size="small"
              icon={<ReloadOutlined spin={loading} />}
              onClick={loadStore}
            >
              刷新货架
            </Button>
          </div>
        }
      >
        <Table
          columns={columns}
          dataSource={akilePackages}
          rowKey={(r) => `${r.platform}-${r.id}-${r.name}`}
          loading={loading}
          pagination={{ pageSize: 15, size: 'small' }}
          size="small"
        />
      </Card>

      {/* Buy & Dry Run Modal */}
      <Modal
        title={`新购选单 - ${selectedPlan?.name}`}
        open={buyModalOpen}
        onCancel={() => setBuyModalOpen(false)}
        onOk={handleExecuteBuy}
        confirmLoading={submitting}
        okText={safeMode ? '执行试算 (防误购)' : '确认立即付款下单'}
        cancelText="取消"
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14, padding: '10px 0' }}>
          <div>
            <Text type="secondary">所属平台：</Text>
            <Tag color={selectedPlan?.platform === 'akile' ? 'blue' : 'green'} style={{ textTransform: 'uppercase' }}>
              {selectedPlan?.platform}
            </Tag>
          </div>
          <div>
            <Text type="secondary">机型规格：</Text>
            <span className="mono-num">
              {selectedPlan?.cpu}核 CPU · {selectedPlan?.memory}MB 内存 · {selectedPlan?.disk}GB 硬盘 · {selectedPlan?.bandwidth}Mbps 带宽
            </span>
          </div>
          <div>
            <Text type="secondary">结算价格：</Text>
            <span className="mono-num" style={{ fontSize: 16, fontWeight: 700, color: '#2563eb' }}>
              ¥{selectedPlan?.price_cny?.toFixed(2)}
            </span>
          </div>

          <div style={{ padding: 12, background: safeMode ? '#eff6ff' : '#fffbeb', borderRadius: 6, border: `1px solid ${safeMode ? '#bfdbfe' : '#fde68a'}` }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
              <span style={{ fontWeight: 600, color: safeMode ? '#1d4ed8' : '#b45309', display: 'flex', alignItems: 'center', gap: 6 }}>
                <SafetyCertificateOutlined />
                防误购保护锁 (Safe Dry-Run)
              </span>
              <Switch checked={safeMode} onChange={setSafeMode} />
            </div>
            <p style={{ margin: 0, fontSize: 12, color: safeMode ? '#1e40af' : '#92400e' }}>
              {safeMode
                ? '已开启安全保护锁：点击确认将仅进行参数合法性校验和价格试算，绝不真正扣费。'
                : '⚠️ 警告：保护锁已关闭！点击确认将直接向官方 API 发起真实划扣和订购，请确保意图明确。'}
            </p>
          </div>

          {buyResult && (
            <div style={{ padding: 10, background: '#f8fafc', borderRadius: 6, border: '1px solid #e2e8f0' }}>
              <div style={{ fontWeight: 600, marginBottom: 4, color: '#0f172a' }}>执行回执：</div>
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
