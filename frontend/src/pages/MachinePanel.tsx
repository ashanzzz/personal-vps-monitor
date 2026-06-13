import { useState, useMemo } from 'react'
import { Input, Select, Switch, Space, Button, message, Tooltip } from 'antd'
import { SyncOutlined, FilterOutlined } from '@ant-design/icons'
import type { Machine, PriceHistory, AppSettings } from '../api'
import { CAT_LABELS, fmtMemory } from '../api'
import { MachineCard } from '../components/MachineCard'

interface Props {
  machines: Machine[]
  priceHistory: PriceHistory[]
  settings: AppSettings | null
  onWatch: (planId: string, watched: boolean) => void
  onBuy: (planId: string) => void
  onRefresh: () => void
}

const REGIONS = ['HK', 'JP', 'SG', 'TW', 'US', 'DE', 'MO', 'KR']

export function MachinePanel({ machines, priceHistory, settings, onWatch, onBuy, onRefresh }: Props) {
  const [catFilter, setCatFilter] = useState<string>('all')
  const [regionFilter, setRegionFilter] = useState<string>('all')
  const [availOnly, setAvailOnly] = useState(false)
  const [watchedOnly, setWatchedOnly] = useState(false)
  const [search, setSearch] = useState('')

  const watchedPlans = new Set(settings?.watched_plans ?? [])

  // group price history by plan_id
  const histByPlan = useMemo(() => {
    const m: Record<string, { price: number; cycle: number }[]> = {}
    for (const h of priceHistory) {
      if (!m[h.plan_id]) m[h.plan_id] = []
      m[h.plan_id].push({ price: h.price, cycle: h.cycle })
    }
    return m
  }, [priceHistory])

  const filtered = useMemo(() => machines.filter(m => {
    if (catFilter !== 'all' && m.category !== catFilter) return false
    if (regionFilter !== 'all' && m.area !== regionFilter) return false
    if (availOnly && !m.available) return false
    if (watchedOnly && !watchedPlans.has(m.plan_id)) return false
    if (search) {
      const q = search.toLowerCase()
      if (!m.name.toLowerCase().includes(q) && !m.area.toLowerCase().includes(q)) return false
    }
    return true
  }), [machines, catFilter, regionFilter, availOnly, watchedOnly, search])

  return (
    <div>
      {/* Filters */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap', alignItems: 'center' }}>
        <Input.Search
          placeholder="搜索名称/区域"
          value={search}
          onChange={e => setSearch(e.target.value)}
          allowClear
          style={{ width: 180 }}
        />
        <Select
          value={catFilter}
          onChange={setCatFilter}
          options={[
            { value: 'all', label: '全部分类' },
            ...Object.entries(CAT_LABELS).map(([k, v]) => ({ value: k, label: v }))
          ]}
          style={{ width: 120 }}
        />
        <Select
          value={regionFilter}
          onChange={setRegionFilter}
          options={[
            { value: 'all', label: '全部区域' },
            ...REGIONS.map(r => ({ value: r, label: r }))
          ]}
          style={{ width: 100 }}
        />
        <Space>
          <span style={{ color: '#aaa', fontSize: 13 }}>仅有货</span>
          <Switch size="small" checked={availOnly} onChange={setAvailOnly} />
        </Space>
        <Space>
          <span style={{ color: '#aaa', fontSize: 13 }}>仅关注</span>
          <Switch size="small" checked={watchedOnly} onChange={setWatchedOnly} />
        </Space>
        <Tooltip title="立即刷新">
          <Button
            icon={<SyncOutlined />}
            size="small"
            onClick={onRefresh}
          />
        </Tooltip>
        <span style={{ color: '#666', fontSize: 12 }}>共 {filtered.length} 台</span>
      </div>

      {/* Cards grid */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))',
        gap: 12,
      }}>
        {filtered.map(m => (
          <MachineCard
            key={m.id}
            machine={m}
            watched={watchedPlans.has(m.plan_id)}
            onWatch={onWatch}
            onBuy={onBuy}
            priceHistory={histByPlan[m.plan_id]}
            autoBuyEnabled={settings?.auto_buy_enabled}
          />
        ))}
      </div>

      {filtered.length === 0 && (
        <div style={{ textAlign: 'center', color: '#555', padding: '60px 0' }}>
          无匹配机器
        </div>
      )}
    </div>
  )
}
