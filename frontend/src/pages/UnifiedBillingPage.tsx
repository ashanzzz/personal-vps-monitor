import React, { useState, useEffect } from 'react'
import {
  Card, Table, Tag, Button, Tabs, App as AntApp, Space, Popconfirm
} from 'antd'
import {
  ReloadOutlined, CreditCardOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { api } from '../api'
import type { UnifiedBillingData } from '../api'

export const UnifiedBillingPage: React.FC = () => {
  const { message } = AntApp.useApp()
  const [loading, setLoading] = useState(false)
  const [data, setData] = useState<UnifiedBillingData | null>(null)
  const [payingKey, setPayingKey] = useState<string | null>(null)

  const loadBilling = async () => {
    setLoading(true)
    try {
      const res = await api.getUnifiedBilling(1, 30)
      setData(res)
    } catch (err: any) {
      message.error(`加载账单数据失败: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadBilling()
  }, [])

  // Execute payment
  const handlePayOrder = async (platform: string, order_id?: any, order_no?: string) => {
    const key = `${platform}-${order_id || order_no}`
    setPayingKey(key)
    try {
      await api.payUnifiedOrder(platform, order_id, order_no, 'balance')
      message.success(`订单已通过余额支付结清！`)
      loadBilling()
    } catch (err: any) {
      message.error(`支付失败: ${err?.response?.data?.detail || err.message}`)
    } finally {
      setPayingKey(null)
    }
  }

  // Akile Orders Columns
  const akileOrderColumns: ColumnsType<any> = [
    {
      title: '订单号',
      dataIndex: 'order_no',
      key: 'order_no',
      width: 220,
      render: (no: string) => <span className="mono-num" style={{ fontWeight: 600 }}>{no}</span>,
    },
    {
      title: '订单内容',
      dataIndex: 'order_name',
      key: 'order_name',
      width: 260,
      render: (t: string) => <span style={{ color: '#0f172a' }}>{t}</span>,
    },
    {
      title: '金额',
      dataIndex: 'order_price',
      key: 'order_price',
      width: 120,
      render: (p: number) => (
        <span className="mono-num" style={{ fontWeight: 700, color: '#0f172a', fontSize: 14 }}>
          ¥{(p / 100).toFixed(2)}
        </span>
      ),
    },
    {
      title: '状态',
      dataIndex: 'order_status',
      key: 'order_status',
      width: 110,
      render: (st: string) => {
        const isPaid = st === '3' || st === 'paid'
        return isPaid ? (
          <span className="badge-online">已支付</span>
        ) : (
          <span className="badge-warning">待支付</span>
        )
      },
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 170,
      render: (t: string) => <span className="mono-num" style={{ fontSize: 12 }}>{t ? dayjs(t).format('YYYY-MM-DD HH:mm') : '—'}</span>,
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_, r) => {
        const isPending = r.order_status !== '3' && r.order_status !== 'paid'
        if (!isPending) return <span style={{ color: '#94a3b8', fontSize: 12 }}>已结清</span>
        return (
          <Popconfirm title="确认使用账户余额支付此订单？" onConfirm={() => handlePayOrder('akile', undefined, r.order_no)}>
            <Button
              type="primary"
              size="small"
              loading={payingKey === `akile-${r.order_no}`}
              style={{ fontSize: 12, height: 24 }}
            >
              余额结清
            </Button>
          </Popconfirm>
        )
      },
    },
  ]

  // ACCK Orders Columns
  const acckOrderColumns: ColumnsType<any> = [
    {
      title: '订单号',
      dataIndex: 'order_no',
      key: 'order_no',
      width: 220,
      render: (no: string) => <span className="mono-num" style={{ fontWeight: 600 }}>{no}</span>,
    },
    {
      title: '订单名称',
      dataIndex: 'order_name',
      key: 'order_name',
      width: 260,
      render: (t: string) => <span style={{ color: '#0f172a' }}>{t}</span>,
    },
    {
      title: '金额',
      dataIndex: 'order_price',
      key: 'order_price',
      width: 120,
      render: (p: number) => (
        <span className="mono-num" style={{ fontWeight: 700, color: '#0f172a', fontSize: 14 }}>
          ¥{(p / 100).toFixed(2)}
        </span>
      ),
    },
    {
      title: '支付方式 / 状态',
      key: 'status',
      width: 140,
      render: (_, r) => {
        const isPaid = r.order_status === 'paid'
        return (
          <Space size={6}>
            <Tag color={isPaid ? 'green' : 'orange'}>{r.order_status || '待付'}</Tag>
            <span style={{ fontSize: 11, color: '#64748b' }}>{r.order_pay_type || ''}</span>
          </Space>
        )
      },
    },
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 170,
      render: (t: string) => <span className="mono-num" style={{ fontSize: 12 }}>{t ? dayjs(t).format('YYYY-MM-DD HH:mm') : '—'}</span>,
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_, r) => {
        const isPending = r.order_status !== 'paid'
        if (!isPending) return <span style={{ color: '#94a3b8', fontSize: 12 }}>已结清</span>
        return (
          <Popconfirm title="确认使用 ACCK 账户余额支付此订单？" onConfirm={() => handlePayOrder('acck', r.id)}>
            <Button
              type="primary"
              size="small"
              loading={payingKey === `acck-${r.id}`}
              style={{ fontSize: 12, height: 24 }}
            >
              余额结清
            </Button>
          </Popconfirm>
        )
      },
    },
  ]

  // Money Log Columns
  const moneyLogColumns: ColumnsType<any> = [
    {
      title: '流水号 / 事务',
      dataIndex: 'id',
      key: 'id',
      width: 120,
      render: (id: number) => <span className="mono-num">#{id}</span>,
    },
    {
      title: '变动说明',
      dataIndex: 'remark',
      key: 'remark',
      render: (rmk: string, r) => (
        <div>
          <div style={{ color: '#0f172a', fontWeight: 500 }}>{rmk || r.name || '余额变动'}</div>
          {r.order_no && <span className="mono-num" style={{ fontSize: 11, color: '#64748b' }}>关联订单: {r.order_no}</span>}
        </div>
      ),
    },
    {
      title: '收支金额',
      dataIndex: 'money',
      key: 'money',
      width: 140,
      render: (m: number) => {
        const isPositive = m >= 0
        return (
          <span className="mono-num" style={{ fontWeight: 700, color: isPositive ? '#15803d' : '#b91c1c' }}>
            {isPositive ? `+¥${(m / 100).toFixed(2)}` : `-¥${(Math.abs(m) / 100).toFixed(2)}`}
          </span>
        )
      },
    },
    {
      title: '记录时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 170,
      render: (t: string) => <span className="mono-num" style={{ fontSize: 12 }}>{t ? dayjs(t).format('YYYY-MM-DD HH:mm') : '—'}</span>,
    },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card
        className="ashan-card"
        title={
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <CreditCardOutlined style={{ color: '#2563eb' }} />
              统一账单与支付中心（订单历史、待支付结清与资金变动流水）
            </span>
            <Button
              type="text"
              size="small"
              icon={<ReloadOutlined spin={loading} />}
              onClick={loadBilling}
            >
              刷新账单
            </Button>
          </div>
        }
      >
        <Tabs
          defaultActiveKey="akile_orders"
          items={[
            {
              key: 'akile_orders',
              label: `Akile 订单记录 (${data?.akile.orders.length || 0})`,
              children: (
                <Table
                  columns={akileOrderColumns}
                  dataSource={data?.akile.orders || []}
                  rowKey="order_no"
                  loading={loading}
                  pagination={{ pageSize: 10, size: 'small' }}
                  size="small"
                />
              ),
            },
            {
              key: 'acck_orders',
              label: `ACCK 订单记录 (${data?.acck.orders.length || 0})`,
              children: (
                <Table
                  columns={acckOrderColumns}
                  dataSource={data?.acck.orders || []}
                  rowKey="order_no"
                  loading={loading}
                  pagination={{ pageSize: 10, size: 'small' }}
                  size="small"
                />
              ),
            },
            {
              key: 'akile_money',
              label: `Akile 资金变动流水 (${data?.akile.money_logs.length || 0})`,
              children: (
                <Table
                  columns={moneyLogColumns}
                  dataSource={data?.akile.money_logs || []}
                  rowKey="id"
                  loading={loading}
                  pagination={{ pageSize: 10, size: 'small' }}
                  size="small"
                />
              ),
            },
          ]}
        />
      </Card>
    </div>
  )
}
