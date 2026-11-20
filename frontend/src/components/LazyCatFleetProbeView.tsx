import React, { useEffect, useState, useCallback } from 'react'
import {
  Table,
  Button,
  Space,
  Tag,
  Typography,
  message,
  Card,
  Row,
  Col,
  Statistic,
  Progress,
  Badge,
} from 'antd'
import {
  ReloadOutlined,
  CloudServerOutlined,
  DashboardOutlined,
  ArrowUpOutlined,
  ArrowDownOutlined,
  ClockCircleOutlined,
} from '@ant-design/icons'
import { api } from '../api'
import type { FleetNode } from '../api'

const { Text } = Typography

interface Props {
  instanceId?: string
}

function formatSpeed(kb: number): string {
  if (kb <= 0) return '0 KB/s'
  if (kb >= 1024) {
    return (kb / 1024).toFixed(2) + ' MB/s'
  }
  return kb.toFixed(1) + ' KB/s'
}

function formatUptimeDays(days: number): string {
  if (days <= 0) return '刚刚启动'
  if (days >= 1) {
    const d = Math.floor(days)
    const hours = Math.round((days - d) * 24)
    return `${d}天 ${hours}小时`
  }
  const hours = Math.round(days * 24)
  return `${hours}小时`
}

export const LazyCatFleetProbeView: React.FC<Props> = () => {
  const [loading, setLoading] = useState(false)
  const [nodes, setNodes] = useState<FleetNode[]>([])
  const [refreshedAt, setRefreshedAt] = useState<string>('')

  const fetchProbe = useCallback(async (forceRefresh = false) => {
    setLoading(true)
    try {
      const res = await api.lazycatGetFleetProbe(forceRefresh)
      setNodes(res.nodes || [])
      setRefreshedAt(res.refreshed_at || new Date().toLocaleTimeString())
    } catch (err: any) {
      message.error('拉取家宽母鸡遥测失败: ' + (err.response?.data?.error || err.message))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchProbe(false)
  }, [fetchProbe])

  const onlineCount = nodes.filter((n) => n.status === 'UP').length
  const totalDownKb = nodes.reduce((acc, cur) => acc + (cur.speed_down_kb || 0), 0)
  const totalUpKb = nodes.reduce((acc, cur) => acc + (cur.speed_up_kb || 0), 0)
  const avgCpu = nodes.length > 0
    ? (nodes.reduce((acc, cur) => acc + (cur.cpu_percent || 0), 0) / nodes.length).toFixed(1)
    : '0'

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* 遥测概况指标卡片 */}
      <Card
        size="small"
        style={{
          background: '#f8fafc',
          borderRadius: 8,
          border: '1px solid #e2e8f0',
        }}
      >
        <Row gutter={[16, 16]} align="middle">
          <Col xs={12} sm={6}>
            <Statistic
              title={<span style={{ color: '#64748b', fontSize: 13 }}>母鸡与出口状态</span>}
              value={onlineCount}
              suffix={<span style={{ fontSize: 13, color: '#64748b' }}>/ {nodes.length} 台在线</span>}
              valueStyle={{
                color: onlineCount === nodes.length && nodes.length > 0 ? '#10b981' : '#f59e0b',
                fontWeight: 700,
              }}
              prefix={
                <Badge
                  status={onlineCount === nodes.length && nodes.length > 0 ? 'success' : 'warning'}
                  style={{ marginRight: 6 }}
                />
              }
            />
            <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>tz.lazycat.my Komari RPC</div>
          </Col>

          <Col xs={12} sm={6}>
            <Statistic
              title={<span style={{ color: '#64748b', fontSize: 13 }}>实时下行总带宽</span>}
              value={formatSpeed(totalDownKb)}
              valueStyle={{ color: '#0ea5e9', fontWeight: 700 }}
              prefix={<ArrowDownOutlined style={{ marginRight: 4 }} />}
            />
            <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>全网集群实时接收</div>
          </Col>

          <Col xs={12} sm={6}>
            <Statistic
              title={<span style={{ color: '#64748b', fontSize: 13 }}>实时上行总带宽</span>}
              value={formatSpeed(totalUpKb)}
              valueStyle={{ color: '#7c3aed', fontWeight: 700 }}
              prefix={<ArrowUpOutlined style={{ marginRight: 4 }} />}
            />
            <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>全网集群实时发送</div>
          </Col>

          <Col xs={12} sm={6}>
            <Statistic
              title={<span style={{ color: '#64748b', fontSize: 13 }}>集群平均 CPU 负荷</span>}
              value={`${avgCpu}%`}
              valueStyle={{ color: '#334155', fontWeight: 700 }}
              prefix={<DashboardOutlined style={{ marginRight: 4 }} />}
            />
            <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>
              上次采样: {refreshedAt ? new Date(refreshedAt).toLocaleTimeString() : '--:--:--'}
            </div>
          </Col>
        </Row>
      </Card>

      {/* 遥测列表工具栏 */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Space>
          <Text strong style={{ fontSize: 15, color: '#1e293b' }}>
            <CloudServerOutlined style={{ marginRight: 6, color: '#7c3aed' }} />
            住宅母鸡与出口节点遥测明细 ({nodes.length} 个监测节点)
          </Text>
          <Tag color="geekblue">按需拉取模式 (零后台轮询消耗)</Tag>
        </Space>

        <Button
          type="primary"
          icon={<ReloadOutlined spin={loading} />}
          loading={loading}
          onClick={() => fetchProbe(true)}
          style={{ background: '#7c3aed', borderColor: '#7c3aed' }}
        >
           立即刷新遥测
        </Button>
      </div>

      {/* 遥测节点列表 */}
      <Table
        loading={loading}
        dataSource={nodes}
        rowKey="uuid"
        pagination={{
          pageSize: 15,
          showTotal: (total) => `共 ${total} 台母鸡与出口节点`,
          showSizeChanger: true,
        }}
        size="middle"
        columns={[
          {
            title: '运行状态',
            dataIndex: 'status',
            key: 'status',
            width: 100,
            render: (st) => (
              <Tag color={st === 'UP' ? 'success' : 'error'} style={{ fontWeight: 600 }}>
                {st === 'UP' ? '● UP' : '○ DOWN'}
              </Tag>
            ),
          },
          {
            title: '母鸡 / 出口节点名称',
            dataIndex: 'name',
            key: 'name',
            render: (name, record) => (
              <Space>
                <span style={{ fontWeight: 600, color: '#0f172a' }}>{name}</span>
                {record.is_mother && (
                  <Tag color="volcano" style={{ fontSize: 11 }}>
                    母鸡
                  </Tag>
                )}
                {record.is_residential && (
                  <Tag color="purple" style={{ fontSize: 11 }}>
                    住宅出口
                  </Tag>
                )}
              </Space>
            ),
          },
          {
            title: '地区 / 机房',
            dataIndex: 'region',
            key: 'region',
            width: 120,
            render: (reg) => (
              <span style={{ fontSize: 14 }}>{reg || ''}</span>
            ),
          },
          {
            title: 'CPU 负载',
            dataIndex: 'cpu_percent',
            key: 'cpu_percent',
            width: 150,
            render: (cpu) => {
              const val = Math.round(cpu || 0)
              let color = '#10b981'
              if (val > 80) color = '#ef4444'
              else if (val > 50) color = '#f59e0b'
              return (
                <div style={{ width: 130 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
                    <span>{val}%</span>
                  </div>
                  <Progress percent={val} size="small" strokeColor={color} showInfo={false} />
                </div>
              )
            },
          },
          {
            title: '内存占用 (RAM)',
            key: 'mem',
            width: 160,
            render: (_, r) => {
              const percent = Math.round(r.ram_percent || 0)
              return (
                <div>
                  <div style={{ fontSize: 12, color: '#475569' }}>
                    {r.ram_used_gb?.toFixed(1) || '0.0'}G / {r.ram_total_gb?.toFixed(1) || '0.0'}G ({percent}%)
                  </div>
                  <Progress percent={percent} size="small" strokeColor="#6366f1" showInfo={false} />
                </div>
              )
            },
          },
          {
            title: '实时上下行网速',
            key: 'speed',
            width: 200,
            render: (_, r) => (
              <Space size={10}>
                <span style={{ color: '#0ea5e9', fontSize: 12, fontFamily: 'monospace' }}>
                  <ArrowDownOutlined style={{ marginRight: 2 }} />
                  {formatSpeed(r.speed_down_kb || 0)}
                </span>
                <span style={{ color: '#7c3aed', fontSize: 12, fontFamily: 'monospace' }}>
                  <ArrowUpOutlined style={{ marginRight: 2 }} />
                  {formatSpeed(r.speed_up_kb || 0)}
                </span>
              </Space>
            ),
          },
          {
            title: '运行时间',
            dataIndex: 'uptime_days',
            key: 'uptime_days',
            width: 140,
            render: (days) => (
              <span style={{ color: '#64748b', fontSize: 12 }}>
                <ClockCircleOutlined style={{ marginRight: 4 }} />
                {formatUptimeDays(days || 0)}
              </span>
            ),
          },
        ]}
      />
    </div>
  )
}
