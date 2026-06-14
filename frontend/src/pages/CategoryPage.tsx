import { useMemo, useState } from 'react'
import { Input, Select, Switch, Space, Button, Tooltip, Table, Tag, Typography, App as AntApp } from 'antd'
import { SyncOutlined, ShoppingCartOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Machine, AppSettings, PageBuyConfig } from '../api'
import { fmtPrice, fmtMemory } from '../api'
import { BuyRulesEditor } from '../components/BuyRulesEditor'
import { pageBuyDefaults } from '../buyDefaults'

const { Text } = Typography

// Page key -> machine category + buy-config setting key.
const PAGE_MAP: Record<string, { category: Machine['category']; buyKey: keyof AppSettings; label: string }> = {
  cloud: { category: 'vps', buyKey: 'cloud_buy', label: '云服务器' },
  lightweight: { category: 'ls', buyKey: 'ls_buy', label: '轻量服务器' },
  secondhand: { category: 'secondhand', buyKey: 'secondhand_buy', label: '二手机' },
}

interface Props {
  pageKey: 'cloud' | 'lightweight' | 'secondhand'
  machines: Machine[]
  settings: AppSettings | null
  onBuy: (planId: string) => void
  onRefresh: () => void
  onSaveBuyConfig: (buyKey: keyof AppSettings, cfg: PageBuyConfig) => void
}

export function CategoryPage({ pageKey, machines, settings, onBuy, onRefresh, onSaveBuyConfig }: Props) {
  const { modal } = AntApp.useApp()
  const meta = PAGE_MAP[pageKey]
  const [search, setSearch] = useState('')
  const [areaFilter, setAreaFilter] = useState<string>('all')
  const [availOnly, setAvailOnly] = useState(false)

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
    return true
  }), [rows, areaFilter, availOnly, search])

  const buyCfg: PageBuyConfig = (settings?.[meta.buyKey] as PageBuyConfig) ?? pageBuyDefaults()

  const columns: ColumnsType<Machine> = [
    {
      title: '名称 / 标签', dataIndex: 'name', key: 'name',
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
    { title: '地区', dataIndex: 'area', key: 'area', width: 90, render: a => a || '—' },
    {
      title: '配置', key: 'spec', width: 200,
      render: (_, m) => (
        <Text style={{ color: '#bbb', fontSize: 12 }}>
          {m.cpu}核 / {fmtMemory(m.memory_mb)} / {m.disk_gb}GB
          {m.bandwidth_mbps ? ` / ${m.bandwidth_mbps}M` : ''}
          {m.traffic_gb ? ` / ${m.traffic_gb}GB流量` : ''}
        </Text>
      ),
    },
    {
      title: '月付 / 年付', key: 'price', width: 140,
      render: (_, m) => (
        <Space direction="vertical" size={0}>
          <Text style={{ color: '#52c41a' }}>{fmtPrice(m.price_monthly)}/月</Text>
          {m.price_yearly ? <Text type="secondary" style={{ fontSize: 11 }}>{fmtPrice(m.price_yearly)}/年</Text> : null}
        </Space>
      ),
    },
    {
      title: '库存/状态', key: 'stock', width: 110,
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
      title: '操作', key: 'action', width: 90,
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

  return (
    <div>
      <BuyRulesEditor
        pageLabel={meta.label}
        value={buyCfg}
        areas={areas}
        onChange={cfg => onSaveBuyConfig(meta.buyKey, cfg)}
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
        <Tooltip title="立即刷新"><Button icon={<SyncOutlined />} size="small" onClick={onRefresh} /></Tooltip>
        <span style={{ color: '#666', fontSize: 12 }}>共 {filtered.length} 台</span>
      </div>

      <Table<Machine>
        rowKey="id"
        size="small"
        columns={columns}
        dataSource={filtered}
        pagination={{ pageSize: 20, showSizeChanger: true }}
        locale={{ emptyText: meta.category === 'secondhand' ? '二手市场当前无在售机器(空仓)' : '无匹配机器' }}
      />
    </div>
  )
}
