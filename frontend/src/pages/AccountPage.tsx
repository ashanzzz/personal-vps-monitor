import { useCallback, useEffect, useState } from 'react'
import {
  Card, Statistic, Row, Col, Button, Space, Tag, Empty, Table, Spin,
  Descriptions, Alert, Typography, App as AntApp,
} from 'antd'
import {
  ReloadOutlined, WalletOutlined, CloudServerOutlined, CheckCircleTwoTone,
  CloseCircleTwoTone,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api, fmtPrice } from '../api'
import type { AccountInfo, MyServer } from '../api'

const { Text } = Typography

// Render a value that may be string/number/bool into something printable.
function show(v: unknown): string {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'boolean') return v ? '是' : '否'
  return String(v)
}

// Pick the first present key from a server object (Akile field names vary).
function pick(s: MyServer, keys: string[]): unknown {
  for (const k of keys) if (s[k] !== undefined && s[k] !== null && s[k] !== '') return s[k]
  return undefined
}

export function AccountPage() {
  const { message } = AntApp.useApp()
  const [data, setData] = useState<AccountInfo | null>(null)
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setData(await api.getAccount())
    } catch {
      message.error('加载账户信息失败')
    } finally {
      setLoading(false)
    }
  }, [message])

  useEffect(() => { load() }, [load])

  if (loading && !data) {
    return <Spin size="large" style={{ display: 'block', margin: '120px auto' }} />
  }

  const profile = data?.profile ?? null
  const servers = data?.servers ?? []

  // Build columns dynamically from the union of server keys, but prefer a
  // curated set of common Akile fields when present.
  const PREFERRED: { title: string; keys: string[] }[] = [
    { title: '名称', keys: ['name', 'hostname', 'title'] },
    { title: '区域', keys: ['area', 'region', 'node', 'location'] },
    { title: 'IP', keys: ['ip', 'ipv4', 'public_ip', 'main_ip'] },
    { title: '状态', keys: ['status', 'state'] },
    { title: '到期', keys: ['expired_at', 'expire_time', 'due_time', 'end_time'] },
  ]
  const columns: ColumnsType<MyServer> = servers.length
    ? PREFERRED.map(p => ({
        title: p.title,
        key: p.title,
        render: (_: unknown, s: MyServer) => show(pick(s, p.keys)),
      }))
    : []

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Text strong style={{ color: '#ccc', fontSize: 16 }}>我的账户</Text>
        <Button icon={<ReloadOutlined />} loading={loading} onClick={load}>刷新</Button>
      </div>

      {data && !data.configured && (
        <Alert type="warning" showIcon style={{ marginBottom: 16 }}
          message="尚未配置 Akile Token" description="请到「设置 → API 与连接」填入 token 后再查看账户信息。" />
      )}

      {data?.errors?.length ? (
        <Alert type="error" showIcon style={{ marginBottom: 16 }}
          message="部分账户数据获取失败" description={data.errors.join('; ')} />
      ) : null}

      {/* Balance + profile */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col xs={24} sm={8}>
          <Card size="small">
            <Statistic
              title={<Space><WalletOutlined />账户余额</Space>}
              value={profile?.money != null ? fmtPrice(profile.money) : '—'}
              valueStyle={{ color: '#52c41a' }}
            />
            {profile?.money != null && (
              <Text type="secondary" style={{ fontSize: 12 }}>{profile.money} 分</Text>
            )}
          </Card>
        </Col>
        <Col xs={24} sm={16}>
          <Card size="small" title="账户信息">
            {profile ? (
              <Descriptions size="small" column={{ xs: 1, sm: 2 }}>
                <Descriptions.Item label="用户名">{show(profile.username)}</Descriptions.Item>
                <Descriptions.Item label="邮箱">{show(profile.email)}</Descriptions.Item>
                <Descriptions.Item label="角色">{show(profile.role)}</Descriptions.Item>
                <Descriptions.Item label="Telegram">{show(profile.telegram_id)}</Descriptions.Item>
                <Descriptions.Item label="实名">
                  {profile.is_real_name
                    ? <span><CheckCircleTwoTone twoToneColor="#52c41a" /> 已实名</span>
                    : <span><CloseCircleTwoTone twoToneColor="#faad14" /> 未实名</span>}
                </Descriptions.Item>
                <Descriptions.Item label="二次验证">
                  {profile.is_ga ? <Tag color="green">已开启</Tag> : <Tag>未开启</Tag>}
                </Descriptions.Item>
              </Descriptions>
            ) : <Empty description="无账户信息" />}
          </Card>
        </Col>
      </Row>

      {/* My servers */}
      <Card size="small" title={<Space><CloudServerOutlined />我的服务器 ({servers.length})</Space>}>
        {servers.length === 0 ? (
          <Empty description="当前没有服务器" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        ) : (
          <Table<MyServer>
            rowKey={(s, i) => String(pick(s, ['id', 'server_id', 'uuid']) ?? i)}
            size="small"
            columns={columns}
            dataSource={servers}
            pagination={false}
            expandable={{
              expandedRowRender: s => (
                <pre style={{ margin: 0, fontSize: 12, color: '#aaa', whiteSpace: 'pre-wrap' }}>
                  {JSON.stringify(s, null, 2)}
                </pre>
              ),
            }}
          />
        )}
      </Card>
    </div>
  )
}
