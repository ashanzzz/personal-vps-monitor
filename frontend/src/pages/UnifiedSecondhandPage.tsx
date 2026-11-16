import React, { useState, useEffect } from 'react'
import {
  Card, Table, Button,
  App as AntApp, Tag, Tooltip, Space, Radio, InputNumber, Select
} from 'antd'
import {
  SwapOutlined, ReloadOutlined, ThunderboltOutlined, DollarOutlined,
  FieldTimeOutlined, RiseOutlined, GlobalOutlined, SettingOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { api } from '../api'
import type { SecondhandItem, PageBuyConfig } from '../api'
import { pageBuyDefaults } from '../buyDefaults'
import { SecondhandBuyModal } from '../components/SecondhandBuyModal'
import { SecondhandRulesModal } from '../components/SecondhandRulesModal'

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
  const [sniperStatus, setSniperStatus] = useState<any>(null)
  const [rulesModalOpen, setRulesModalOpen] = useState(false)
  const [buyConfig, setBuyConfig] = useState<PageBuyConfig | null>(null)
  const [savingRules, setSavingRules] = useState(false)

  const openRulesModal = async () => {
    try {
      const s = await api.getSettings()
      setBuyConfig(s.secondhand_buy ?? pageBuyDefaults())
      setRulesModalOpen(true)
    } catch {
      message.error('获取抢购配置失败')
    }
  }

  const saveRules = async () => {
    if (!buyConfig) return
    setSavingRules(true)
    try {
      await api.saveSettings({ secondhand_buy: buyConfig })
      message.success('秒杀规则已保存并实时生效')
      setRulesModalOpen(false)
      loadSniper()
    } catch {
      message.error('保存秒杀配置失败')
    } finally {
      setSavingRules(false)
    }
  }

  const loadSniper = async () => {
    try {
      const res = await api.getSniperStatus()
      setSniperStatus(res)
    } catch {
      /* ignore */
    }
  }

  useEffect(() => {
    loadSniper()
    // 60s 一轮：不要高频刷对方站点
    const timer = setInterval(loadSniper, 60000)
    return () => clearInterval(timer)
  }, [])

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

  const loadData = async (force = false) => {
    setLoading(true)
    try {
      const res = await api.getUnifiedSecondhand({
        platform: platformFilter === 'all' ? undefined : platformFilter,
        force_refresh: force,
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
                <Tooltip title={`资产提示：不影响长期使用权，下月续费自动重置为 ${tr.total_gb}GB 满血；如当期急用可花¥${tr.reset_price_cny}随时重置`}>
                  <Tag color="orange" style={{ fontSize: 10, padding: '0 4px', margin: 0, cursor: 'help' }}>
                    当期用尽 ⏳ (下月续费重置)
                  </Tag>
                </Tooltip>
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
                  {ce?.intent_label ? (
                    <div style={{ color: score >= 80 ? '#4ade80' : score >= 65 ? '#38bdf8' : '#fbbf24', marginBottom: 4, fontWeight: 700 }}>
                      🎯 购买决策建议: {ce.intent_label}
                    </div>
                  ) : null}
                  {ce?.renew_intent_tip ? (
                    <div style={{ color: '#c084fc', marginBottom: 4, fontWeight: 600 }}>
                      💳 月供心理阶梯: {ce.renew_intent_tip}
                    </div>
                  ) : null}
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
                  {/* 物理四大维度细分面板 (网络40 / 配置20 / 带宽10 / 流量20) */}
                  <div style={{ background: 'rgba(255, 255, 255, 0.08)', borderRadius: 6, padding: '6px 8px', margin: '6px 0', border: '1px solid rgba(255, 255, 255, 0.15)' }}>
                    <div style={{ fontWeight: 700, color: '#38bdf8', marginBottom: 4, fontSize: 11, display: 'flex', justifyContent: 'space-between' }}>
                      <span>🌐 物理全景: {r.network_tier_name || '6级网络·普通国际无优化BGP'}</span>
                      <span style={{ color: '#fbbf24' }}>综合底座: {r.hardware_perf_score ?? r.performance_score ?? '—'}/100</span>
                    </div>
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '3px 8px', fontSize: 11 }}>
                      <div>• 网络线路: <b style={{ color: (r.route_score || 0) >= 30 ? '#4ade80' : (r.route_score || 0) >= 20 ? '#38bdf8' : '#fbbf24' }}>{r.route_score ?? '—'}/40</b></div>
                      <div>• 性能配置: <b style={{ color: (r.specs_score || 0) >= 15 ? '#4ade80' : '#38bdf8' }}>{r.specs_score ?? '—'}/20</b></div>
                      <div>• 带宽口径: <b style={{ color: (r.bandwidth_score || 0) >= 8 ? '#4ade80' : (r.bandwidth_score || 0) <= 3 ? '#f87171' : '#fbbf24' }}>{r.bandwidth_score ?? '—'}/10</b>{(r.bandwidth_score || 0) <= 3 ? ' (慢口⚠️)' : ''}</div>
                      <div>• 流量配额: <b style={{ color: '#4ade80' }}>{r.traffic_score ?? '—'}/20</b></div>
                    </div>
                  </div>
                  {/* 经济套利模型面板 (利润 / 售价 / 回本周期) */}
                  <div style={{ background: 'rgba(255, 255, 255, 0.05)', borderRadius: 6, padding: '6px 8px', margin: '4px 0', border: '1px solid rgba(255, 255, 255, 0.1)' }}>
                    <div style={{ fontWeight: 700, color: '#a78bfa', marginBottom: 4, fontSize: 11 }}>
                      💰 现金流与套利模型:
                    </div>
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '3px 8px', fontSize: 11 }}>
                      <div>• 销售价格: <b style={{ color: (r.sale_price_score || 0) >= 150 ? '#c084fc' : (r.sale_price_score || 0) >= 80 ? '#4ade80' : '#f87171' }}>{r.sale_price_score ?? '—'}/200</b></div>
                      <div>• 回本套利分: <b style={{ color: (r.arbitrage_score || 0) >= 200 ? '#c084fc' : (r.arbitrage_score || 0) >= 80 ? '#4ade80' : '#fbbf24' }}>{r.arbitrage_score ?? '—'} 分</b></div>
                      <div style={{ gridColumn: 'span 2' }}>• 综合性价比: <b style={{ color: (r.score || 0) >= 1000 ? '#ec4899' : (r.score || 0) >= 100 ? '#c084fc' : (r.score || 0) >= 80 ? '#4ade80' : '#38bdf8' }}>{r.score ?? '—'} 分</b> (标尺: 80~100超级, 100-N无敌)</div>
                    </div>
                  </div>
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
              {ce?.intent_label ? (
                <div style={{ fontSize: 10, fontWeight: 700, marginTop: 2, color: score >= 80 ? '#16a34a' : score >= 65 ? '#2563eb' : '#d97706' }}>
                  🎯 {ce.intent_label.split(' ')[0]}
                </div>
              ) : null}
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
              onClick={() => loadData(true)}
            >
              刷新集市
            </Button>
          </div>
        }
      >
        {/* 🎯 定向毫秒级精准秒杀看门狗状态面板 */}
        {sniperStatus && (
          <div style={{
            background: 'linear-gradient(135deg, #0f172a 0%, #1e293b 100%)',
            border: '1px solid #334155',
            borderRadius: 8,
            padding: '12px 16px',
            marginBottom: 16,
            color: '#f8fafc',
            boxShadow: '0 4px 6px -1px rgba(0, 0, 0, 0.1)',
          }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <span style={{ fontSize: 14, fontWeight: 700, color: '#38bdf8', display: 'flex', alignItems: 'center', gap: 6 }}>
                  ⚡ 定向极速秒杀看门狗 (每 {sniperStatus.interval_seconds || 5}s 极速轮询)
                </span>
                <Tag color={sniperStatus.status === 'running' ? 'success' : 'error'} style={{ margin: 0, fontWeight: 600 }}>
                  {sniperStatus.status === 'running' ? '● RUNNING 正在监听' : 'STOPPED'}
                </Tag>
                <Tag color={sniperStatus.real_purchase_enabled ? 'error' : 'warning'} style={{ margin: 0 }}>
                  {sniperStatus.real_purchase_enabled ? '🔥 真实自动扣款模式' : '🛡️ 模拟匹配 (Dry-Run)'}
                </Tag>
                <span style={{ fontSize: 12, color: '#94a3b8' }}>
                  今日已抢购: <b style={{ color: '#fbbf24' }}>{sniperStatus.daily_bought ?? 0}</b> 台
                </span>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12, fontSize: 12, color: '#cbd5e1' }}>
                <span>物理隔离机制: <Tag color="blue" style={{ margin: 0 }}>Akile/ACCK 独立分流，无任务零刷新</Tag></span>
                <Button
                  type="primary"
                  size="small"
                  icon={<SettingOutlined />}
                  onClick={openRulesModal}
                  style={{
                    background: '#0284c7',
                    borderColor: '#0284c7',
                    fontWeight: 600,
                    boxShadow: '0 2px 4px rgba(2, 132, 199, 0.3)',
                  }}
                >
                  配置秒杀规则
                </Button>
              </div>
            </div>

            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, alignItems: 'center' }}>
              <span style={{ fontSize: 12, color: '#94a3b8' }}>当前生效中的定向抢购规则:</span>
              {sniperStatus.rules && sniperStatus.rules.length > 0 ? (
                sniperStatus.rules.map((r: any) => (
                  <Tag
                    key={r.id}
                    color="purple"
                    style={{
                      background: 'rgba(168, 85, 247, 0.15)',
                      border: '1px solid rgba(168, 85, 247, 0.4)',
                      padding: '2px 8px',
                      color: '#e9d5ff',
                      fontSize: 12,
                      display: 'inline-flex',
                      alignItems: 'center',
                      gap: 6,
                    }}
                  >
                    <b style={{ color: '#a855f7' }}>[{r.platform ? r.platform.toUpperCase() : 'AKILE'}]</b>
                    <span>{r.label || r.id}</span>
                    <span style={{ color: '#4ade80', fontWeight: 700 }}>
                      ≤ ¥{r.sale_price_max ? (r.sale_price_max / 100).toFixed(0) : 0}
                    </span>
                    <span style={{ color: '#94a3b8', fontSize: 11 }}>
                      (前缀: {r.search_prefix || '自动匹配'})
                    </span>
                  </Tag>
                ))
              ) : (
                <span style={{ color: '#64748b', fontSize: 12 }}>暂无启用中的定向秒杀规则</span>
              )}
            </div>
          </div>
        )}

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
          pagination={{
            pageSize: 20,
            showSizeChanger: true,
            pageSizeOptions: ['15', '20', '50', '100'],
            showTotal: total => `共 ${total} 台在售`,
            size: 'small',
          }}
          size="small"
        />
      </Card>

      {/* Buy Modal */}
      <SecondhandBuyModal
        buyModalOpen={buyModalOpen}
        setBuyModalOpen={setBuyModalOpen}
        selectedItem={selectedItem}
        safeMode={safeMode}
        setSafeMode={setSafeMode}
        forceBuyDepleted={forceBuyDepleted}
        setForceBuyDepleted={setForceBuyDepleted}
        captcha={captcha}
        setCaptcha={setCaptcha}
        submitting={submitting}
        buyResult={buyResult}
        handleExecuteBuy={handleExecuteBuy}
        isDepleted={isDepleted}
      />

      <SecondhandRulesModal
        rulesModalOpen={rulesModalOpen}
        setRulesModalOpen={setRulesModalOpen}
        buyConfig={buyConfig}
        setBuyConfig={setBuyConfig}
        savingRules={savingRules}
        saveRules={saveRules}
      />
    </div>
  )
}
