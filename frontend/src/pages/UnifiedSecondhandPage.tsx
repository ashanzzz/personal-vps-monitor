import React, { useState, useEffect } from 'react'
import {
  Card, Table, Button, Modal, Switch, Typography, Input,
  App as AntApp, Tag, Tooltip, Checkbox, Alert, Space, Radio, InputNumber, Select
} from 'antd'
import {
  SwapOutlined, SafetyCertificateOutlined, ReloadOutlined,
  ThunderboltOutlined, WarningOutlined, DollarOutlined,
  FieldTimeOutlined, RiseOutlined, GlobalOutlined
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

  // 专属续费价格区间与评分/性价比筛选状态
  const [platformFilter, setPlatformFilter] = useState<string>('all') // 'all', 'akile', 'acck'
  const [renewRange, setRenewRange] = useState<string>('all') // 'heirloom_2' (<=2元), 'budget_10' (<=10元), 'fair_20' (<=20元), 'custom', 'all'
  const [minRenew, setMinRenew] = useState<number | undefined>(undefined)
  const [maxRenew, setMaxRenew] = useState<number | undefined>(undefined)
  const [minScore, setMinScore] = useState<number | undefined>(undefined)
  const [paybackPreset, setPaybackPreset] = useState<string>('all') // 'fast_6' (<=6月), 'year_12' (<=12月), 'mid_24' (<=24月), 'all'
  const [maxPayback, setMaxPayback] = useState<number | undefined>(undefined)
  const [sortBy, setSortBy] = useState<string>('price') // 'price', 'renew', 'score', 'ppy', 'payback', 'roi'
  const [sortOrder, setSortOrder] = useState<string>('asc')

  const handleRangePreset = (preset: string) => {
    setRenewRange(preset)
    if (preset === 'heirloom_2') {
      setMinRenew(0)
      setMaxRenew(2.0)
    } else if (preset === 'budget_10') {
      setMinRenew(0)
      setMaxRenew(10.0)
    } else if (preset === 'fair_20') {
      setMinRenew(0)
      setMaxRenew(20.0)
    } else if (preset === 'all') {
      setMinRenew(undefined)
      setMaxRenew(undefined)
    }
  }

  const handlePaybackPreset = (preset: string) => {
    setPaybackPreset(preset)
    if (preset === 'fast_6') {
      setMaxPayback(6.0)
    } else if (preset === 'year_12') {
      setMaxPayback(12.0)
    } else if (preset === 'mid_24') {
      setMaxPayback(24.0)
    } else {
      setMaxPayback(undefined)
    }
  }

  const loadData = async () => {
    setLoading(true)
    try {
      const res = await api.getUnifiedSecondhand({
        page: 1,
        page_size: 100,
        platform: platformFilter === 'all' ? undefined : platformFilter,
        min_renew: minRenew,
        max_renew: maxRenew,
        min_score: minScore,
        max_payback: maxPayback,
        sort_by: sortBy,
        sort_order: sortOrder,
      })
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
  }, [platformFilter, minRenew, maxRenew, minScore, maxPayback, sortBy, sortOrder])

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
      width: 155,
      sorter: (a, b) => (a.price_cny || 0) - (b.price_cny || 0),
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
          {r.payback_tier ? (
            <div style={{ marginTop: 2 }}>
              <Tag color={r.payback_badge_color || 'green'} style={{ fontSize: 10, padding: '0 4px', margin: 0 }}>
                {r.payback_tier}
              </Tag>
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
      title: '综合评分 / 性价比比率',
      key: 'sniping',
      width: 195,
      sorter: (a, b) => (a.score || 0) - (b.score || 0),
      render: (_, r) => {
        const ce = r.cost_effectiveness
        const pr = r.pricing_analysis
        const score = r.score ?? r.sniping?.score ?? 50
        const ppy = r.points_per_yuan ?? ce?.points_per_yuan ?? Number((score / Math.max(0.1, r.price_cny)).toFixed(2))
        const ppd = r.points_per_usd ?? ce?.points_per_usd ?? Number((ppy * 7.25).toFixed(2))
        const gradeLabel = r.grade_label ?? r.sniping?.verdict_text ?? (score >= 88 ? '【神机/必抢】' : score >= 75 ? '【超值高性价】' : '【平价可选】')
        const color = r.badge_color || (score >= 88 ? 'purple' : score >= 75 ? 'green' : score >= 60 ? 'blue' : 'error')

        const vr = r.value_ratio ?? ce?.value_ratio
        const ml = r.monthly_leverage ?? ce?.monthly_leverage
        const mm = r.m_market ?? ce?.m_market

        return (
          <div>
            <Tooltip
              title={
                <div style={{ fontSize: 12 }}>
                  <div style={{ fontWeight: 700, marginBottom: 4 }}>
                    综合品质评分: {score} 分 {gradeLabel}
                  </div>
                  <div style={{ color: '#fbbf24', marginBottom: 4, fontWeight: 600 }}>
                    ⚡ 真实每元产出比: {ppy} 分/元 (约 ${ppd} 分/USD)
                  </div>
                  {r.payback_months != null && r.payback_months < 990 ? (
                    <div style={{ color: r.payback_months <= 12 ? '#4ade80' : '#38bdf8', marginBottom: 4, fontWeight: 700 }}>
                      ⏳ 现金流投资回本期: {r.payback_months <= 0.01 ? '即时全额回本 (纯白送)' : `${r.payback_months} 个月`} (首年年化ROI: {r.annual_roi_percent || 0}%)
                    </div>
                  ) : r.payback_tier ? (
                    <div style={{ color: '#f87171', marginBottom: 4, fontWeight: 600 }}>
                      ⏳ 回本状态: {r.payback_tier}
                    </div>
                  ) : null}
                  {r.monthly_cash_savings ? (
                    <div style={{ color: '#38bdf8', marginBottom: 4 }}>
                      • 相比公开市场月租(¥{mm}/月) 净省: <b>¥{r.monthly_cash_savings}/月</b>
                    </div>
                  ) : null}
                  {vr ? (
                    <div style={{ color: vr >= 1.5 ? '#4ade80' : vr >= 1.0 ? '#38bdf8' : '#f87171', marginBottom: 4, fontWeight: 600 }}>
                      📈 资产套利倍数: {vr}x ({vr >= 1.5 ? '超值套利捡漏' : vr >= 1.0 ? '公允价值成交' : '溢价割韭菜'})
                    </div>
                  ) : null}
                  {ml ? (
                    <div style={{ color: '#38bdf8', marginBottom: 4 }}>
                      • 月供产能杠杆: <b>{ml}x</b> (等效市场月租: ¥{mm}/月)
                    </div>
                  ) : null}
                  {ce?.monthly_points_per_yuan ? (
                    <div style={{ color: '#94a3b8', marginBottom: 4 }}>
                      • 长线月供比: {ce.monthly_points_per_yuan} 分/元·月 (${ce.monthly_points_per_usd}/$)
                    </div>
                  ) : null}
                  {ce?.fair_market_value_cny ? (
                    <div style={{ color: '#4ade80', marginBottom: 4, fontWeight: 600 }}>
                      💰 公允转让估值: ¥{ce.fair_market_value_cny}
                      {ce.valuation_rationale ? <div style={{ fontSize: 11, color: '#cbd5e1', fontWeight: 400 }}>{ce.valuation_rationale}</div> : null}
                    </div>
                  ) : null}
                  {ce?.cost_per_point_cny ? (
                    <div style={{ color: '#94a3b8', marginBottom: 4 }}>
                      • 单位品质成本: ¥{ce.cost_per_point_cny} / 分
                    </div>
                  ) : null}
                  {ce?.breakdown && (
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2px 8px', margin: '4px 0' }}>
                      <div>• 线路网络: <b>{ce.breakdown.network_score}/30</b></div>
                      <div>• 长期持有: <b>{ce.breakdown.ltv_score}/25</b></div>
                      <div>• 周期配额: <b>{ce.breakdown.traffic_score}/25</b></div>
                      <div>• 入手门槛: <b>{ce.breakdown.price_score}/20</b></div>
                    </div>
                  )}
                  {pr?.intrinsic_value_cny ? (
                    <div style={{ marginTop: 2 }}>内在评估基准价值: <b>¥{pr.intrinsic_value_cny}</b></div>
                  ) : null}
                  {pr?.ip_repair_fee_cny ? (
                    <div style={{ color: '#fbbf24', marginTop: 2 }}>
                      • 换新IP成本: <b>¥{pr.ip_repair_fee_cny}</b> (或 1500 AK币)
                    </div>
                  ) : null}
                  {r.reasons && r.reasons.length > 0 && (
                    <div style={{ marginTop: 4, borderTop: '1px solid rgba(255,255,255,0.2)', paddingTop: 4, color: '#e2e8f0' }}>
                      {r.reasons.join('；')}
                    </div>
                  )}
                </div>
              }
            >
              <Tag color={color} style={{ fontWeight: 700, fontSize: 11, cursor: 'help', padding: '1px 8px', margin: 0, borderRadius: 4 }}>
                {score}分 {gradeLabel}
              </Tag>
              <div style={{ fontSize: 11, color: '#2563eb', fontWeight: 600, marginTop: 2 }}>
                {ppy} 分/元 <span style={{ color: '#64748b', fontWeight: 400 }}>(${ppd}/$)</span>
                {vr ? (
                  <span style={{ marginLeft: 4, color: vr >= 1.5 ? '#16a34a' : vr >= 1.0 ? '#2563eb' : '#dc2626', fontWeight: 700 }}>
                    [{vr}x]
                  </span>
                ) : null}
              </div>
            </Tooltip>
            {pr?.real_total_cost_cny && pr.real_total_cost_cny !== r.price_cny ? (
              <div style={{ fontSize: 10, color: '#dc2626', marginTop: 2, fontWeight: 500 }}>
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
        {/* 山哥专属：续费价格区间、评分与性价比联合筛选条 */}
        <div style={{
          background: '#f8fafc',
          border: '1px solid #e2e8f0',
          borderRadius: 8,
          padding: '10px 14px',
          marginBottom: 16,
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 12
        }}>
          <Space wrap size={12} align="center">
            <span style={{ fontSize: 13, fontWeight: 600, color: '#334155', display: 'flex', alignItems: 'center', gap: 4 }}>
              <GlobalOutlined style={{ color: '#2563eb' }} /> 网站平台:
            </span>
            <Radio.Group
              value={platformFilter}
              onChange={e => setPlatformFilter(e.target.value)}
              buttonStyle="solid"
              size="small"
            >
              <Radio.Button value="all">全部平台</Radio.Button>
              <Radio.Button value="akile">🔵 Akile ({counts.akile})</Radio.Button>
              <Radio.Button value="acck">🟢 ACCK ({counts.acck})</Radio.Button>
            </Radio.Group>

            <span style={{ fontSize: 13, fontWeight: 600, color: '#334155', display: 'flex', alignItems: 'center', gap: 4, marginLeft: 6 }}>
              <DollarOutlined style={{ color: '#059669' }} /> 续费区间:
            </span>
            <Radio.Group
              value={renewRange}
              onChange={e => handleRangePreset(e.target.value)}
              buttonStyle="solid"
              size="small"
            >
              <Radio.Button value="heirloom_2">传家宝 (≤ ¥2/月)</Radio.Button>
              <Radio.Button value="budget_10">超值车 (≤ ¥10/月)</Radio.Button>
              <Radio.Button value="fair_20">平价车 (≤ ¥20/月)</Radio.Button>
              <Radio.Button value="all">不限</Radio.Button>
            </Radio.Group>

            <span style={{ color: '#94a3b8', fontSize: 12 }}>自定义区间:</span>
            <InputNumber
              size="small"
              placeholder="最低月供"
              prefix="¥"
              min={0}
              step={1}
              style={{ width: 100 }}
              value={minRenew}
              onChange={v => { setMinRenew(v ?? undefined); setRenewRange('custom') }}
            />
            <span style={{ color: '#94a3b8' }}>~</span>
            <InputNumber
              size="small"
              placeholder="最高月供"
              prefix="¥"
              min={0}
              step={1}
              style={{ width: 100 }}
              value={maxRenew}
              onChange={v => { setMaxRenew(v ?? undefined); setRenewRange('custom') }}
            />

            <span style={{ marginLeft: 6, fontSize: 13, fontWeight: 600, color: '#334155' }}>最低评分:</span>
            <Select
              size="small"
              value={minScore ?? 0}
              style={{ width: 110 }}
              onChange={v => setMinScore(v === 0 ? undefined : v)}
              options={[
                { value: 0, label: '全部评分' },
                { value: 60, label: '≥ 60 分 (及格)' },
                { value: 75, label: '≥ 75 分 (良好)' },
                { value: 85, label: '≥ 85 分 (极佳)' },
                { value: 90, label: '≥ 90 分 (神机)' },
              ]}
            />
          </Space>

          <Space size={10} align="center">
            <span style={{ fontSize: 12, color: '#64748b' }}>排序维度:</span>
            <Select
              size="small"
              value={sortBy}
              style={{ width: 140 }}
              onChange={v => setSortBy(v)}
              options={[
                { value: 'price', label: '挂售价格' },
                { value: 'renew', label: '月续费金额' },
                { value: 'ppy', label: '每元性价比' },
                { value: 'score', label: '综合质量分' },
                { value: 'payback', label: '回本周期最快' },
                { value: 'roi', label: '年化套利ROI最高' },
              ]}
            />
            <Select
              size="small"
              value={sortOrder}
              style={{ width: 80 }}
              onChange={v => setSortOrder(v)}
              options={[
                { value: 'asc', label: '升序' },
                { value: 'desc', label: '降序' },
              ]}
            />
          </Space>

          {/* 回本周期套利筛选行 */}
          <div style={{
            width: '100%',
            display: 'flex',
            flexWrap: 'wrap',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 12,
            paddingTop: 8,
            borderTop: '1px dashed #e2e8f0',
          }}>
            <Space wrap size={12} align="center">
              <span style={{ fontSize: 13, fontWeight: 600, color: '#334155', display: 'flex', alignItems: 'center', gap: 4 }}>
                <FieldTimeOutlined style={{ color: '#2563eb' }} /> 资产套利回本:
              </span>
              <Radio.Group
                value={paybackPreset}
                onChange={e => handlePaybackPreset(e.target.value)}
                buttonStyle="solid"
                size="small"
              >
                <Radio.Button value="year_12">⚡ 一年内回本 (超值爆棚)</Radio.Button>
                <Radio.Button value="fast_6">⚡ 半年内回本 (极速暴利)</Radio.Button>
                <Radio.Button value="mid_24">2年内回本</Radio.Button>
                <Radio.Button value="all">不限</Radio.Button>
              </Radio.Group>
            </Space>

            <span style={{ fontSize: 12, color: '#64748b', display: 'flex', alignItems: 'center', gap: 4 }}>
              <RiseOutlined style={{ color: '#16a34a' }} /> 一年回本年化ROI超100%，资产套利顶级神车
            </span>
          </div>
        </div>

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
