import { useMemo } from 'react'
import { Card, Col, Row, Statistic, Tag, Timeline, Empty } from 'antd'
import {
  CloudServerOutlined,
  ShopOutlined,
  ThunderboltOutlined,
  BellOutlined,
} from '@ant-design/icons'
import dayjs from 'dayjs'
import type { Machine, AlertEvent } from '../api'
import { CAT_LABELS, EVENT_LABELS, fmtPrice } from '../api'

interface Props {
  machines: Machine[]
  events: AlertEvent[]
}

export function Overview({ machines, events }: Props) {
  const stats = useMemo(() => {
    const vps = machines.filter(m => m.category === 'vps')
    const ls  = machines.filter(m => m.category === 'ls')
    const sh  = machines.filter(m => m.category === 'secondhand')
    return {
      vpsTotal:   vps.length,
      vpsAvail:   vps.filter(m => m.available).length,
      lsTotal:    ls.length,
      lsAvail:    ls.filter(m => m.available).length,
      shTotal:    sh.length,
      shAvail:    sh.filter(m => m.available).length,
      recentEvents: events.slice(0, 20),
    }
  }, [machines, events])

  return (
    <div style={{ padding: '0 4px' }}>
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={8}>
          <Card size="small">
            <Statistic
              title={<span><CloudServerOutlined /> 新机VPS</span>}
              value={stats.vpsAvail}
              suffix={`/ ${stats.vpsTotal} 有货`}
              valueStyle={{ color: stats.vpsAvail > 0 ? '#52c41a' : '#888' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card size="small">
            <Statistic
              title={<span><ThunderboltOutlined /> 轻量机</span>}
              value={stats.lsAvail}
              suffix={`/ ${stats.lsTotal} 有货`}
              valueStyle={{ color: stats.lsAvail > 0 ? '#52c41a' : '#888' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card size="small">
            <Statistic
              title={<span><ShopOutlined /> 二手机</span>}
              value={stats.shAvail}
              suffix={`/ ${stats.shTotal} 有货`}
              valueStyle={{ color: stats.shAvail > 0 ? '#fa8c16' : '#888' }}
            />
          </Card>
        </Col>
      </Row>

      <Card
        size="small"
        title={<span><BellOutlined /> 最近预警</span>}
        bodyStyle={{ maxHeight: 420, overflowY: 'auto' }}
      >
        {stats.recentEvents.length === 0 ? (
          <Empty description="暂无预警记录" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        ) : (
          <Timeline
            items={stats.recentEvents.map(e => {
              const info = EVENT_LABELS[e.event_type]
              return {
                color: info?.color ?? 'blue',
                children: (
                  <span style={{ fontSize: 13 }}>
                    <Tag color={info?.color} style={{ marginRight: 6 }}>{info?.label ?? e.event_type}</Tag>
                    <Tag color="default" style={{ marginRight: 6 }}>{CAT_LABELS[e.category] ?? e.category}</Tag>
                    <strong>{e.name}</strong>
                    {e.area && <span style={{ color: '#888', marginLeft: 4 }}>{e.area}</span>}
                    {e.event_type === 'price_change' && (
                      <span style={{ color: '#aaa', marginLeft: 8, fontSize: 12 }}>
                        {fmtPrice(Number(e.old_value))} → {fmtPrice(Number(e.new_value))}
                      </span>
                    )}
                    <span style={{ color: '#555', marginLeft: 8, fontSize: 11 }}>
                      {dayjs(e.created_at).format('MM-DD HH:mm:ss')}
                    </span>
                  </span>
                ),
              }
            })}
          />
        )}
      </Card>
    </div>
  )
}
