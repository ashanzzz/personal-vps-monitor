import { useState } from 'react'
import { Table, Tag, Select, Typography, Button } from 'antd'
import type { AlertEvent } from '../api'
import { EVENT_LABELS, CAT_LABELS, fmtPrice } from '../api'
import dayjs from 'dayjs'

interface Props {
  events: AlertEvent[]
}

export function AlertPanel({ events }: Props) {
  const [catFilter, setCatFilter] = useState<string>('all')
  const [typeFilter, setTypeFilter] = useState<string>('all')

  const filtered = events.filter(e =>
    (catFilter === 'all' || e.category === catFilter) &&
    (typeFilter === 'all' || e.event_type === typeFilter)
  )

  const columns = [
    {
      title: '时间',
      dataIndex: 'created_at',
      width: 155,
      render: (v: string) => dayjs(v).format('MM-DD HH:mm:ss'),
    },
    {
      title: '类型',
      dataIndex: 'category',
      width: 80,
      render: (v: string) => CAT_LABELS[v] ?? v,
    },
    {
      title: '事件',
      dataIndex: 'event_type',
      width: 90,
      render: (v: string) => {
        const info = EVENT_LABELS[v]
        return info ? <Tag color={info.color}>{info.label}</Tag> : <Tag>{v}</Tag>
      },
    },
    {
      title: '机器',
      dataIndex: 'name',
      ellipsis: true,
    },
    {
      title: '区域',
      dataIndex: 'area',
      width: 90,
      ellipsis: true,
    },
    {
      title: '变化',
      key: 'change',
      width: 180,
      render: (_: unknown, r: AlertEvent) => {
        if (r.event_type === 'price_change') {
          return (
            <span style={{ color: '#aaa', fontSize: 12 }}>
              {fmtPrice(Number(r.old_value))} → {fmtPrice(Number(r.new_value))}
            </span>
          )
        }
        return null
      },
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', gap: 8, marginBottom: 12, flexWrap: 'wrap' }}>
        <Select
          value={catFilter}
          onChange={setCatFilter}
          options={[
            { value: 'all', label: '全部类型' },
            { value: 'vps', label: '新机VPS' },
            { value: 'ls', label: '轻量机' },
            { value: 'secondhand', label: '二手机' },
          ]}
          style={{ width: 120 }}
        />
        <Select
          value={typeFilter}
          onChange={setTypeFilter}
          options={[
            { value: 'all', label: '全部事件' },
            ...Object.entries(EVENT_LABELS).map(([k, v]) => ({ value: k, label: v.label }))
          ]}
          style={{ width: 120 }}
        />
        <Typography.Text type="secondary" style={{ lineHeight: '32px' }}>
          共 {filtered.length} 条
        </Typography.Text>
      </div>
      <Table
        size="small"
        dataSource={filtered}
        columns={columns}
        rowKey="id"
        pagination={{ pageSize: 20, showSizeChanger: false }}
        style={{ fontSize: 13 }}
      />
    </div>
  )
}
