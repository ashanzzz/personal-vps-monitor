import React, { useState, useEffect, useMemo } from 'react'
import {
  Card, Table, Tag, Button, Modal, Switch, Typography,
  App as AntApp, Tooltip, Radio, Space, Input
} from 'antd'
import {
  ShoppingCartOutlined,
  SafetyCertificateOutlined, ReloadOutlined,
  ThunderboltOutlined, FireOutlined, SearchOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '../api'
import type { StorePackage } from '../api'

const { Text } = Typography

export const UnifiedStorePage: React.FC = () => {
  const { message } = AntApp.useApp()
  const [loading, setLoading] = useState(false)
  const [packages, setPackages] = useState<StorePackage[]>([])
  const [filterType, setFilterType] = useState<string>('all') // 'all', 'top_tier', 'high_value', 'in_stock'
  const [searchKeyword, setSearchKeyword] = useState('')
  const [buyModalOpen, setBuyModalOpen] = useState(false)
  const [selectedPlan, setSelectedPlan] = useState<any>(null)
  const [safeMode, setSafeMode] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [buyResult, setBuyResult] = useState<any>(null)

  const loadStore = async () => {
    setLoading(true)
    try {
      const res = await api.getUnifiedStore()
      if (res.packages && res.packages.length > 0) {
        setPackages(res.packages)
      } else if (res.akile_store) {
        // Fallback manual parser
        const raw = res.akile_store
        const areas = Array.isArray(raw) ? raw : (raw.list || raw.areas || [])
        const pkgs: StorePackage[] = []
        for (const area of areas) {
          const nodes = area.nodes || area.Groups || []
          for (const node of nodes) {
            const plans = node.plans || node.Plans || []
            for (const plan of plans) {
              const p_cents = plan.price_datas?.[0]?.price || plan.price || 0
              const cycle = plan.price_datas?.[0]?.cycle || plan.cycle || 1
              const p_cny = p_cents / 100
              pkgs.push({
                uid: `akile:${plan.id}`,
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
                price_cny: p_cny,
                cycle,
                monthly_cny: cycle > 1 ? Number((p_cny / cycle).toFixed(2)) : p_cny,
                stock: plan.stock ?? 1,
                raw: plan,
              })
            }
          }
        }
        setPackages(pkgs)
      }
    } catch (e: any) {
      message.error(`加载商店数据失败: ${e.message}`)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadStore()
  }, [])

  // Filter packages based on active filter and keyword
  const filteredPackages = useMemo(() => {
    return packages.filter((pkg) => {
      // Keyword filter
      if (searchKeyword.trim()) {
        const kw = searchKeyword.toLowerCase()
        const match =
          pkg.name.toLowerCase().includes(kw) ||
          pkg.node.toLowerCase().includes(kw) ||
          pkg.area.toLowerCase().includes(kw) ||
          (pkg.grade_label && pkg.grade_label.toLowerCase().includes(kw))
        if (!match) return false
      }

      // Filter tabs
      if (filterType === 'top_tier') {
        return (pkg.score || 0) >= 88
      }
      if (filterType === 'high_value') {
        return (pkg.score || 0) >= 75
      }
      if (filterType === 'in_stock') {
        return pkg.stock > 0
      }
      return true
    })
  }, [packages, filterType, searchKeyword])

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

  const columns: ColumnsType<StorePackage> = [
    {
      title: '平台',
      dataIndex: 'platform',
      key: 'platform',
      width: 85,
      render: (p: string) => (
        <span className={p === 'akile' ? 'badge-proxy' : 'badge-online'} style={{ textTransform: 'uppercase' }}>
          {p}
        </span>
      ),
    },
    {
      title: '套餐名称 / 节点地区',
      dataIndex: 'name',
      key: 'name',
      width: 250,
      render: (name: string, r) => (
        <div>
          <div style={{ fontWeight: 600, color: '#0f172a' }}>{name}</div>
          <div style={{ fontSize: 11, color: '#64748b' }}>{r.node} · {r.area}</div>
        </div>
      ),
    },
    {
      title: '性价比评分 (0-100)',
      key: 'score',
      width: 175,
      sorter: (a, b) => (a.score || 0) - (b.score || 0),
      defaultSortOrder: 'descend',
      render: (_, r) => {
        const ce = r.cost_effectiveness
        const score = r.score ?? 50
        const gradeLabel = r.grade_label || (score >= 88 ? '【神机/必抢】' : score >= 75 ? '【超值高性价】' : '【平价可选】')
        const color = r.badge_color || (score >= 88 ? 'purple' : score >= 75 ? 'green' : score >= 60 ? 'blue' : 'orange')

        return (
          <Tooltip
            title={
              <div style={{ fontSize: 12 }}>
                <div style={{ fontWeight: 700, marginBottom: 4 }}>
                  综合性价比: {score} 分 {gradeLabel}
                </div>
                {ce?.breakdown && (
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2px 8px', margin: '4px 0' }}>
                    <div>• 线路网络: <b>{ce.breakdown.network_score}/35</b></div>
                    <div>• 硬件产出: <b>{ce.breakdown.hardware_score}/25</b></div>
                    <div>• 流量资产: <b>{ce.breakdown.traffic_score}/20</b></div>
                    <div>• 价格指数: <b>{ce.breakdown.price_score}/20</b></div>
                  </div>
                )}
                {ce?.reasons && ce.reasons.length > 0 && (
                  <div style={{ marginTop: 4, borderTop: '1px solid rgba(255,255,255,0.2)', paddingTop: 4, color: '#e2e8f0' }}>
                    {ce.reasons.join('；')}
                  </div>
                )}
              </div>
            }
          >
            <div style={{ cursor: 'help' }}>
              <Tag
                color={color}
                style={{ fontWeight: 700, fontSize: 11, padding: '1px 8px', margin: 0, borderRadius: 4 }}
              >
                {score}分 {gradeLabel}
              </Tag>
            </div>
          </Tooltip>
        )
      },
    },
    {
      title: '核心配置',
      key: 'specs',
      width: 170,
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
      width: 110,
      render: (f: number) => <span className="mono-num">{f > 0 ? `${f} GB` : '无限'}</span>,
    },
    {
      title: '价格 / 折合月付',
      dataIndex: 'price_cny',
      key: 'price_cny',
      width: 140,
      sorter: (a, b) => a.monthly_cny - b.monthly_cny,
      render: (p: number, r) => (
        <div>
          <span className="mono-num" style={{ fontWeight: 700, color: '#0f172a', fontSize: 14 }}>
            ¥{p.toFixed(2)}
          </span>
          <span style={{ fontSize: 11, color: '#64748b', marginLeft: 4 }}>
            /{r.cycle === 12 ? '年' : (r.cycle > 1 ? `${r.cycle}月` : '月')}
          </span>
          {r.cycle > 1 ? (
            <div style={{ fontSize: 11, color: '#059669' }}>
              折合 ¥{r.monthly_cny.toFixed(2)}/月
            </div>
          ) : null}
        </div>
      ),
    },
    {
      title: '库存状态',
      dataIndex: 'stock',
      key: 'stock',
      width: 100,
      render: (s: number) => (
        s > 0 ? (
          <span className="badge-online">有货 ({s})</span>
        ) : (
          <span className="badge-offline">缺货</span>
        )
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_, r) => (
        <Button
          type="primary"
          size="small"
          style={{ fontSize: 12, height: 26, padding: '0 10px' }}
          onClick={() => openBuyModal(r)}
        >
          选购
        </Button>
      ),
    },
  ]

  const topTierCount = packages.filter(p => (p.score || 0) >= 88).length
  const highValueCount = packages.filter(p => (p.score || 0) >= 75).length
  const inStockCount = packages.filter(p => p.stock > 0).length

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card
        className="ashan-card"
        title={
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 10 }}>
            <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <ShoppingCartOutlined style={{ color: '#2563eb' }} />
              官方一手云服务器商城（自动性价比量化与智能评分）
            </span>
            <Space size={8}>
              <Input
                placeholder="搜索节点/线路/地区..."
                prefix={<SearchOutlined style={{ color: '#94a3b8' }} />}
                value={searchKeyword}
                onChange={(e) => setSearchKeyword(e.target.value)}
                size="small"
                style={{ width: 170 }}
                allowClear
              />
              <Button
                type="text"
                size="small"
                icon={<ReloadOutlined spin={loading} />}
                onClick={loadStore}
              >
                刷新货架
              </Button>
            </Space>
          </div>
        }
      >
        {/* Quick Filter Bar */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12, flexWrap: 'wrap', gap: 8 }}>
          <Radio.Group
            value={filterType}
            onChange={(e) => setFilterType(e.target.value)}
            size="small"
            buttonStyle="solid"
          >
            <Radio.Button value="all">全部商品 ({packages.length})</Radio.Button>
            <Radio.Button value="top_tier">
              <FireOutlined style={{ color: '#d97706', marginRight: 4 }} />
              神机必抢 ≥88分 ({topTierCount})
            </Radio.Button>
            <Radio.Button value="high_value">
              <ThunderboltOutlined style={{ color: '#16a34a', marginRight: 4 }} />
              超值推荐 ≥75分 ({highValueCount})
            </Radio.Button>
            <Radio.Button value="in_stock">有现货 ({inStockCount})</Radio.Button>
          </Radio.Group>

          <span style={{ fontSize: 12, color: '#64748b' }}>
            已按性价比评分从高到低自动排序，悬停标签可查看多维度细分
          </span>
        </div>

        <Table
          columns={columns}
          dataSource={filteredPackages}
          rowKey={(r) => `${r.platform}-${r.id}-${r.name}`}
          loading={loading}
          pagination={{ pageSize: 15, size: 'small', showTotal: (t) => `共 ${t} 项在售套餐` }}
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
            <Text type="secondary">性价比评估：</Text>
            <Tag color={selectedPlan?.badge_color || 'blue'}>
              {selectedPlan?.score}分 {selectedPlan?.grade_label}
            </Tag>
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
