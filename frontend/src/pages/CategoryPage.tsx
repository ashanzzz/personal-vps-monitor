import { useEffect, useMemo, useRef, useState } from 'react'
import { Input, Select, Switch, Space, Button, Tooltip, Table, Tag, Typography, Card, InputNumber, App as AntApp } from 'antd'
import { SyncOutlined, ShoppingCartOutlined, ColumnWidthOutlined, FilterOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { ResizeCallbackData } from 'react-resizable'
import type { Machine, AppSettings, PageBuyConfig, ScoreRules } from '../api'
import { fmtPrice, fmtMemory, api } from '../api'
import { BuyRulesEditor } from '../components/BuyRulesEditor'
import { ScoreRulesEditor } from '../components/ScoreRulesEditor'
import { ResizableTitle } from '../components/ResizableTitle'
import { pageBuyDefaults } from '../buyDefaults'

const { Text } = Typography

// Page key -> machine category + buy-config setting key + column-width setting key.
const PAGE_MAP: Record<string, {
  category: Machine['category']
  buyKey: keyof AppSettings
  widthKey: 'col_widths_cloud' | 'col_widths_ls' | 'col_widths_secondhand'
  scoreKey: 'score_rules_cloud' | 'score_rules_ls' | 'score_rules_secondhand'
  label: string
}> = {
  cloud:       { category: 'vps',        buyKey: 'cloud_buy',      widthKey: 'col_widths_cloud',      scoreKey: 'score_rules_cloud',      label: '云服务器' },
  lightweight: { category: 'ls',         buyKey: 'ls_buy',         widthKey: 'col_widths_ls',         scoreKey: 'score_rules_ls',         label: '轻量服务器' },
  secondhand:  { category: 'secondhand', buyKey: 'secondhand_buy', widthKey: 'col_widths_secondhand', scoreKey: 'score_rules_secondhand', label: '二手机' },
}

// Default column widths (px). The "恢复默认宽度" button restores exactly these.
const DEFAULT_COL_WIDTHS: Record<string, number> = {
  name: 200,
  area: 90,
  cpu: 80,
  memory: 100,
  disk: 100,
  bandwidth: 100,
  traffic: 110,
  price: 140,
  score: 90,
  stock: 120,
  action: 80,
}

const SAVE_DEBOUNCE_MS = 500

interface Props {
  pageKey: 'cloud' | 'lightweight' | 'secondhand'
  machines: Machine[]
  settings: AppSettings | null
  onBuy: (planId: string) => void
  onSaveBuyConfig: (buyKey: keyof AppSettings, cfg: PageBuyConfig) => void
  onSaveColWidths: (
    widthKey: 'col_widths_cloud' | 'col_widths_ls' | 'col_widths_secondhand',
    widths: Record<string, number>,
  ) => void
  onSaveScoreRules: (
    scoreKey: 'score_rules_cloud' | 'score_rules_ls' | 'score_rules_secondhand',
    rules: ScoreRules,
  ) => void
}

export function CategoryPage({ pageKey, machines, settings, onBuy, onSaveBuyConfig, onSaveColWidths, onSaveScoreRules }: Props) {
  const { modal } = AntApp.useApp()
  const meta = PAGE_MAP[pageKey]
  const [search, setSearch] = useState('')
  const [areaFilter, setAreaFilter] = useState<string>('all')
  const [availOnly, setAvailOnly] = useState(false)

  // Advanced, client-side filters over the already-loaded snapshot. All are
  // optional; empty/0 means "no constraint". They never refetch — purely
  // narrow the rows already in memory, so they apply instantly.
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [tagFilter, setTagFilter] = useState<string[]>([])      // match name/node
  const [ipFilter, setIpFilter] = useState<'all' | 'normal' | 'gfw'>('all')
  const [cpuRange, setCpuRange] = useState<[number, number]>([0, 0])
  const [memRange, setMemRange] = useState<[number, number]>([0, 0])   // MB
  const [diskRange, setDiskRange] = useState<[number, number]>([0, 0]) // GB
  const [priceRange, setPriceRange] = useState<[number, number]>([0, 0]) // 元/月
  const [scoreRange, setScoreRange] = useState<[number, number]>([0, 0]) // 评分
  const [salePriceMax, setSalePriceMax] = useState(0)                  // 元

  // Column widths: seed once from defaults overlaid with whatever was persisted
  // for THIS page. CategoryPage remounts on every page switch (App renders it
  // conditionally) and only after settings have loaded, so a lazy initializer
  // is enough — no effect-driven prop sync needed.
  const savedWidths = settings?.[meta.widthKey] as Record<string, number> | undefined
  const [colWidths, setColWidths] = useState<Record<string, number>>(
    () => ({ ...DEFAULT_COL_WIDTHS, ...(savedWidths ?? {}) }),
  )
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Debounced persist of the current widths to settings.
  const scheduleSave = (widths: Record<string, number>) => {
    if (saveTimer.current) clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(() => {
      onSaveColWidths(meta.widthKey, widths)
    }, SAVE_DEBOUNCE_MS)
  }

  useEffect(() => () => { if (saveTimer.current) clearTimeout(saveTimer.current) }, [])

  const handleResize = (key: string) =>
    (_e: React.SyntheticEvent, { size }: ResizeCallbackData) => {
      setColWidths(prev => {
        const next = { ...prev, [key]: Math.max(60, Math.round(size.width)) }
        scheduleSave(next)
        return next
      })
    }

  const restoreDefaultWidths = () => {
    if (saveTimer.current) clearTimeout(saveTimer.current)
    setColWidths(DEFAULT_COL_WIDTHS)
    // Clear the per-page override entirely -> backend falls back to {} = defaults.
    onSaveColWidths(meta.widthKey, {})
  }

  // Inline label style + reset for the advanced filter panel.
  const advLbl = { color: '#999', fontSize: 12, display: 'block', marginBottom: 2 }
  const resetAdvanced = () => {
    setTagFilter([])
    setIpFilter('all')
    setCpuRange([0, 0])
    setMemRange([0, 0])
    setDiskRange([0, 0])
    setPriceRange([0, 0])
    setScoreRange([0, 0])
    setSalePriceMax(0)
  }

  const labelMap = settings?.plan_label_map ?? {}
  const rows = useMemo(
    () => machines.filter(m => m.category === meta.category),
    [machines, meta.category],
  )

  const areas = useMemo(() => {
    const s = new Set<string>()
    rows.forEach(m => m.area && s.add(m.area))
    return [...s].sort()
  }, [rows])

  const filtered = useMemo(() => rows.filter(m => {
    if (areaFilter !== 'all' && m.area !== areaFilter) return false
    if (availOnly && !m.available) return false
    if (search) {
      const q = search.toLowerCase()
      if (!m.name.toLowerCase().includes(q) && !m.area.toLowerCase().includes(q)) return false
    }
    // Advanced filters (client-side over loaded snapshot; 0/empty = no constraint).
    if (tagFilter.length) {
      const hay = `${m.name}\n${m.node ?? ''}`.toLowerCase()
      if (!tagFilter.some(t => hay.includes(t.toLowerCase()))) return false
    }
    if (ipFilter !== 'all' && m.ip_status !== ipFilter) return false
    const inRange = (v: number, lo: number, hi: number) =>
      (!lo || v >= lo) && (!hi || v <= hi)
    if (!inRange(m.cpu, cpuRange[0], cpuRange[1])) return false
    if (!inRange(m.memory_mb, memRange[0], memRange[1])) return false
    if (!inRange(m.disk_gb, diskRange[0], diskRange[1])) return false
    if (!inRange(m.price_monthly, priceRange[0] * 100, priceRange[1] * 100)) return false
    if (!inRange(m.score ?? 0, scoreRange[0], scoreRange[1])) return false
    if (salePriceMax) {
      // sale_price isn't always present; only exclude rows that carry one.
      const sale = (m as { sale_price?: number }).sale_price
      if (sale != null && sale > salePriceMax * 100) return false
    }
    return true
  }), [rows, areaFilter, availOnly, search, tagFilter, ipFilter, cpuRange, memRange, diskRange, priceRange, scoreRange, salePriceMax])

  const buyCfg: PageBuyConfig = (settings?.[meta.buyKey] as PageBuyConfig) ?? pageBuyDefaults()

  const baseColumns: ColumnsType<Machine> = [
    {
      title: '名称 / 标签', dataIndex: 'name', key: 'name', width: colWidths.name,
      sorter: (a, b) => a.name.localeCompare(b.name),
      render: (_, m) => (
        <Space direction="vertical" size={0}>
          <Text style={{ color: '#eee' }}>{m.name}</Text>
          <Tooltip title={`plan_id: ${m.plan_id}`}>
            <Tag color={labelMap[m.plan_id] ? 'geekblue' : 'default'} style={{ fontSize: 11 }}>
              {labelMap[m.plan_id] || `ID:${m.plan_id}`}
            </Tag>
          </Tooltip>
        </Space>
      ),
    },
    {
      title: '地区', dataIndex: 'area', key: 'area', width: colWidths.area,
      sorter: (a, b) => (a.area || '').localeCompare(b.area || ''),
      render: a => a || '—',
    },
    {
      title: 'CPU', key: 'cpu', width: colWidths.cpu,
      sorter: (a, b) => a.cpu - b.cpu,
      render: (_, m) => <Text style={{ color: '#bbb', fontSize: 12 }}>{m.cpu}核</Text>,
    },
    {
      title: '内存', key: 'memory', width: colWidths.memory,
      sorter: (a, b) => a.memory_mb - b.memory_mb,
      render: (_, m) => <Text style={{ color: '#bbb', fontSize: 12 }}>{fmtMemory(m.memory_mb)}</Text>,
    },
    {
      title: '硬盘', key: 'disk', width: colWidths.disk,
      sorter: (a, b) => a.disk_gb - b.disk_gb,
      render: (_, m) => <Text style={{ color: '#bbb', fontSize: 12 }}>{m.disk_gb}GB</Text>,
    },
    {
      title: '带宽', key: 'bandwidth', width: colWidths.bandwidth,
      sorter: (a, b) => a.bandwidth_mbps - b.bandwidth_mbps,
      render: (_, m) => <Text style={{ color: '#bbb', fontSize: 12 }}>{m.bandwidth_mbps ? `${m.bandwidth_mbps}M` : '—'}</Text>,
    },
    {
      title: '流量', key: 'traffic', width: colWidths.traffic,
      sorter: (a, b) => a.traffic_gb - b.traffic_gb,
      render: (_, m) => <Text style={{ color: '#bbb', fontSize: 12 }}>{m.traffic_gb ? `${m.traffic_gb}GB` : '—'}</Text>,
    },
    {
      title: '月付 / 年付', key: 'price', width: colWidths.price,
      sorter: (a, b) => a.price_monthly - b.price_monthly,
      render: (_, m) => (
        <Space direction="vertical" size={0}>
          <Text style={{ color: '#52c41a' }}>{fmtPrice(m.price_monthly)}/月</Text>
          {m.price_yearly ? <Text type="secondary" style={{ fontSize: 11 }}>{fmtPrice(m.price_yearly)}/年</Text> : null}
        </Space>
      ),
    },
    {
      title: '评分', key: 'score', width: colWidths.score,
      defaultSortOrder: 'descend',
      sorter: (a, b) => (a.score ?? 0) - (b.score ?? 0),
      render: (_, m) => (
        <Tooltip title="按本页评分规则加权计算，可在顶部「评分规则」调整权重">
          <Text strong style={{ color: '#faad14' }}>{(m.score ?? 0).toFixed(1)}</Text>
        </Tooltip>
      ),
    },
    {
      title: '库存/状态', key: 'stock', width: colWidths.stock,
      sorter: (a, b) => a.stock - b.stock,
      render: (_, m) => (
        <Space direction="vertical" size={2}>
          {m.stock === -1
            ? <Tag>不适用</Tag>
            : <Tag color={m.available ? 'green' : 'red'}>{m.available ? `有货 ${m.stock}` : '无货'}</Tag>}
          {meta.category === 'secondhand' && m.ip_status === 'gfw' && <Tag color="volcano">IP被墙</Tag>}
          {meta.category === 'secondhand' && m.ip_status === 'normal' && <Tag color="cyan">IP正常</Tag>}
        </Space>
      ),
    },
    {
      title: '操作', key: 'action', width: colWidths.action,
      render: (_, m) => (
        <Button size="small" icon={<ShoppingCartOutlined />} disabled={!m.available}
          onClick={() => modal.confirm({
            title: '确认手动抢购?',
            content: `${m.name} — ${fmtPrice(m.price_monthly)}/月`,
            onOk: () => onBuy(m.plan_id),
          })}>
          抢
        </Button>
      ),
    },
  ]

  // Attach the resizable header handle to every column that declares a width.
  const columns: ColumnsType<Machine> = baseColumns.map(col => {
    const key = String(col.key)
    return {
      ...col,
      onHeaderCell: (column: ColumnsType<Machine>[number]) => ({
        width: (column as { width?: number }).width,
        onResize: handleResize(key),
      }),
    }
  })

  return (
    <div>
      <BuyRulesEditor
        pageLabel={meta.label}
        value={buyCfg}
        onChange={cfg => onSaveBuyConfig(meta.buyKey, cfg)}
      />

      <ScoreRulesEditor
        pageLabel={meta.label}
        value={(settings?.[meta.scoreKey] as ScoreRules)}
        onChange={rules => onSaveScoreRules(meta.scoreKey, rules)}
      />

      <div style={{ display: 'flex', gap: 8, marginBottom: 12, flexWrap: 'wrap', alignItems: 'center' }}>
        <Input.Search placeholder="搜索名称/区域" value={search} allowClear style={{ width: 180 }}
          onChange={e => setSearch(e.target.value)} />
        <Select value={areaFilter} onChange={setAreaFilter} style={{ width: 120 }}
          options={[{ value: 'all', label: '全部区域' }, ...areas.map(a => ({ value: a, label: a }))]} />
        <Space>
          <span style={{ color: '#aaa', fontSize: 13 }}>仅有货</span>
          <Switch size="small" checked={availOnly} onChange={setAvailOnly} />
        </Space>
        <Tooltip title="立即强制刷新本页面数据">
          <Button type="primary" icon={<SyncOutlined />} size="small"
            onClick={() => api.triggerPagePoll(meta.category === 'vps' ? 'cloud' : meta.category === 'ls' ? 'ls' : meta.category)}>
            强制刷新
          </Button>
        </Tooltip>
        <Button size="small" type={showAdvanced ? 'primary' : 'default'} icon={<FilterOutlined />}
          onClick={() => setShowAdvanced(v => !v)}>高级筛选</Button>
        <Tooltip title="恢复默认列宽">
          <Button icon={<ColumnWidthOutlined />} size="small" onClick={restoreDefaultWidths}>恢复默认宽度</Button>
        </Tooltip>
        <span style={{ color: '#666', fontSize: 12 }}>共 {filtered.length} 台</span>
      </div>

      {showAdvanced && (
        <Card size="small" style={{ marginBottom: 12 }}
          title={<Text style={{ color: '#ccc' }}>高级筛选（对当前已加载数据实时生效，留空/0=不限）</Text>}
          extra={<Button size="small" onClick={resetAdvanced}>重置</Button>}>
          <Space direction="vertical" size={10} style={{ width: '100%' }}>
            <Space wrap size="middle" align="start">
              <span>
                <span style={advLbl}>标签包含(名称/节点,任一命中)</span>
                <Select mode="tags" value={tagFilter} style={{ width: 240 }}
                  placeholder="如 jppro、jpiij" tokenSeparators={[',', ' ']} open={false}
                  onChange={setTagFilter} />
              </span>
              <span>
                <span style={advLbl}>IP 状态</span>
                <Select value={ipFilter} style={{ width: 120 }} onChange={setIpFilter}
                  options={[
                    { value: 'all', label: '全部' },
                    { value: 'normal', label: 'IP正常' },
                    { value: 'gfw', label: 'IP被墙' },
                  ]} />
              </span>
            </Space>
            <Space wrap size="middle" align="start">
              <span>
                <span style={advLbl}>CPU 核 min-max</span>
                <Space.Compact>
                  <InputNumber min={0} value={cpuRange[0]} style={{ width: 70 }} placeholder="min"
                    onChange={v => setCpuRange([Number(v) || 0, cpuRange[1]])} />
                  <InputNumber min={0} value={cpuRange[1]} style={{ width: 70 }} placeholder="max"
                    onChange={v => setCpuRange([cpuRange[0], Number(v) || 0])} />
                </Space.Compact>
              </span>
              <span>
                <span style={advLbl}>内存 MB min-max</span>
                <Space.Compact>
                  <InputNumber min={0} value={memRange[0]} style={{ width: 80 }} placeholder="min"
                    onChange={v => setMemRange([Number(v) || 0, memRange[1]])} />
                  <InputNumber min={0} value={memRange[1]} style={{ width: 80 }} placeholder="max"
                    onChange={v => setMemRange([memRange[0], Number(v) || 0])} />
                </Space.Compact>
              </span>
              <span>
                <span style={advLbl}>硬盘 GB min-max</span>
                <Space.Compact>
                  <InputNumber min={0} value={diskRange[0]} style={{ width: 70 }} placeholder="min"
                    onChange={v => setDiskRange([Number(v) || 0, diskRange[1]])} />
                  <InputNumber min={0} value={diskRange[1]} style={{ width: 70 }} placeholder="max"
                    onChange={v => setDiskRange([diskRange[0], Number(v) || 0])} />
                </Space.Compact>
              </span>
            </Space>
            <Space wrap size="middle" align="start">
              <span>
                <span style={advLbl}>月付价 元 min-max</span>
                <Space.Compact>
                  <InputNumber min={0} value={priceRange[0]} style={{ width: 80 }} placeholder="min"
                    onChange={v => setPriceRange([Number(v) || 0, priceRange[1]])} />
                  <InputNumber min={0} value={priceRange[1]} style={{ width: 80 }} placeholder="max"
                    onChange={v => setPriceRange([priceRange[0], Number(v) || 0])} />
                </Space.Compact>
              </span>
              <span>
                <span style={advLbl}>评分 min-max</span>
                <Space.Compact>
                  <InputNumber step={0.1} value={scoreRange[0]} style={{ width: 80 }} placeholder="min"
                    onChange={v => setScoreRange([Number(v) || 0, scoreRange[1]])} />
                  <InputNumber step={0.1} value={scoreRange[1]} style={{ width: 80 }} placeholder="max"
                    onChange={v => setScoreRange([scoreRange[0], Number(v) || 0])} />
                </Space.Compact>
              </span>
              <span>
                <span style={advLbl}>售价 ≤ 元</span>
                <InputNumber min={0} value={salePriceMax} style={{ width: 110 }}
                  onChange={v => setSalePriceMax(Number(v) || 0)} />
              </span>
            </Space>
          </Space>
        </Card>
      )}

      <Table<Machine>
        rowKey="id"
        size="small"
        columns={columns}
        components={{ header: { cell: ResizableTitle } }}
        dataSource={filtered}
        pagination={{ pageSize: 20, showSizeChanger: true }}
        scroll={{ x: 'max-content' }}
        locale={{ emptyText: meta.category === 'secondhand' ? '二手市场当前无在售机器(空仓)' : '无匹配机器' }}
      />
    </div>
  )
}
