import React, { useEffect, useState, useCallback } from 'react'
import {
  Table,
  Button,
  Modal,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Tag,
  Progress,
  Typography,
  Popconfirm,
  message,
  Card,
  Row,
  Col,
  Tooltip,
} from 'antd'
import {
  PlusOutlined,
  ReloadOutlined,
  DeleteOutlined,
  CopyOutlined,
  ApiOutlined,
  CheckOutlined,
  InfoCircleOutlined,
} from '@ant-design/icons'
import { api } from '../api'
import type { PortMapping } from '../api'

const { Text } = Typography

interface Props {
  instanceId: string
  instanceName?: string
}

export const LazyCatPortManagementView: React.FC<Props> = ({ instanceId }) => {
  const [loading, setLoading] = useState(false)
  const [ports, setPorts] = useState<PortMapping[]>([])
  const [hostPublicIp, setHostPublicIp] = useState<string>('')
  const [portLimit, setPortLimit] = useState<number>(10)
  const [copiedKey, setCopiedKey] = useState<string | null>(null)

  // 弹窗状态
  const [addModalVisible, setAddModalVisible] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()

  const fetchPorts = useCallback(async () => {
    if (!instanceId) return
    setLoading(true)
    try {
      const res = await api.getLazyCatPorts(instanceId)
      setPorts(res.ports || [])
      if (res.host_public_ip) {
        setHostPublicIp(res.host_public_ip)
      }
      if (res.port_limit) {
        setPortLimit(res.port_limit)
      }
    } catch (err: any) {
      message.error('拉取端口映射失败: ' + (err.response?.data?.error || err.message))
    } finally {
      setLoading(false)
    }
  }, [instanceId])

  useEffect(() => {
    fetchPorts()
  }, [fetchPorts])

  const copyToClipboard = (text: string, key: string) => {
    navigator.clipboard.writeText(text)
    setCopiedKey(key)
    message.success(`已复制: ${text}`)
    setTimeout(() => setCopiedKey(null), 2000)
  }

  const handleAddPort = async () => {
    try {
      const values = await form.validateFields()
      setSubmitting(true)
      const res = await api.addLazyCatPort(
        instanceId,
        values.private_port,
        values.protocol,
        values.remark || ''
      )
      message.success(`端口映射添加成功！公网端口: ${res.public_port}`)
      setAddModalVisible(false)
      form.resetFields()
      fetchPorts()
    } catch (err: any) {
      message.error('添加端口映射失败: ' + (err.response?.data?.error || err.message))
    } finally {
      setSubmitting(false)
    }
  }

  const handleDeletePort = async (mappingId: number) => {
    try {
      await api.deleteLazyCatPort(instanceId, mappingId)
      message.success(`已删除端口映射 #${mappingId}`)
      fetchPorts()
    } catch (err: any) {
      message.error('删除端口映射失败: ' + (err.response?.data?.error || err.message))
    }
  }

  const usedCount = ports.length
  const quotaPercent = Math.min(100, Math.round((usedCount / (portLimit || 10)) * 100))

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* 顶部配额与外网入口指示 */}
      <Card
        size="small"
        style={{
          background: '#f8fafc',
          borderRadius: 8,
          border: '1px solid #e2e8f0',
        }}
      >
        <Row gutter={[16, 16]} align="middle">
          <Col xs={24} sm={10}>
            <div style={{ fontSize: 13, color: '#64748b' }}>公网 NAT 出口 IPv4:</div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 4 }}>
              <span
                style={{
                  fontFamily: 'monospace',
                  fontSize: 16,
                  fontWeight: 700,
                  color: '#0f172a',
                }}
              >
                {hostPublicIp || '未检测到公网IP (请刷新)'}
              </span>
              {hostPublicIp && (
                <Button
                  size="small"
                  type="text"
                  icon={copiedKey === 'host-ip' ? <CheckOutlined style={{ color: '#10b981' }} /> : <CopyOutlined />}
                  onClick={() => copyToClipboard(hostPublicIp, 'host-ip')}
                >
                  {copiedKey === 'host-ip' ? '已复制' : '复制IP'}
                </Button>
              )}
            </div>
            <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>
              所有端口映射均基于此母鸡公网 IP 进行 NAT 转发
            </div>
          </Col>

          <Col xs={24} sm={14}>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13 }}>
              <span style={{ color: '#64748b' }}>
                <ApiOutlined style={{ marginRight: 6 }} />
                端口映射配额占用:
              </span>
              <span style={{ fontWeight: 600, color: usedCount >= portLimit ? '#ef4444' : '#0f172a' }}>
                {usedCount} / {portLimit} 个 ({quotaPercent}%)
              </span>
            </div>
            <Progress
              percent={quotaPercent}
              status={usedCount >= portLimit ? 'exception' : 'active'}
              strokeColor={usedCount >= portLimit ? '#ef4444' : '#7c3aed'}
              style={{ marginTop: 6 }}
            />
          </Col>
        </Row>
      </Card>

      {/* 操作工具栏 */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Space>
          <Text strong style={{ fontSize: 15, color: '#1e293b' }}>
            活动 NAT 端口映射列表 ({ports.length})
          </Text>
          <Button
            size="small"
            icon={<ReloadOutlined spin={loading} />}
            onClick={fetchPorts}
          >
            刷新规则
          </Button>
        </Space>

        <Button
          type="primary"
          icon={<PlusOutlined />}
          disabled={usedCount >= portLimit}
          onClick={() => {
            form.resetFields()
            form.setFieldsValue({ protocol: 'tcp', private_port: 8080 })
            setAddModalVisible(true)
          }}
          style={{ background: '#7c3aed', borderColor: '#7c3aed' }}
        >
          新增端口映射
        </Button>
      </div>

      {/* 端口映射表格 */}
      <Table
        loading={loading}
        dataSource={ports}
        rowKey="id"
        pagination={false}
        size="middle"
        columns={[
          {
            title: '规则 ID',
            dataIndex: 'id',
            key: 'id',
            width: 90,
            render: (id) => <span style={{ fontFamily: 'monospace', color: '#64748b' }}>#{id}</span>,
          },
          {
            title: '协议',
            dataIndex: 'protocol',
            key: 'protocol',
            width: 90,
            render: (proto) => (
              <Tag color={proto === 'udp' ? 'orange' : 'blue'} style={{ fontWeight: 600 }}>
                {String(proto || 'TCP').toUpperCase()}
              </Tag>
            ),
          },
          {
            title: '容器内部私有端口',
            dataIndex: 'private_port',
            key: 'private_port',
            width: 150,
            render: (port) => (
              <span style={{ fontFamily: 'monospace', fontWeight: 600, color: '#334155' }}>
                :{port}
              </span>
            ),
          },
          {
            title: '公网 NAT 映射端口',
            dataIndex: 'public_port',
            key: 'public_port',
            width: 150,
            render: (pub) => (
              <span style={{ fontFamily: 'monospace', fontWeight: 700, color: '#7c3aed' }}>
                :{pub}
              </span>
            ),
          },
          {
            title: '公网访问入口 (IP:Port)',
            key: 'endpoint',
            render: (_, record) => {
              const endpoint = hostPublicIp ? `${hostPublicIp}:${record.public_port}` : `:${record.public_port}`
              const isCopied = copiedKey === `port-${record.id}`
              return (
                <Space>
                  <span
                    style={{
                      fontFamily: 'monospace',
                      fontWeight: 600,
                      color: '#0f172a',
                      background: '#f1f5f9',
                      padding: '2px 8px',
                      borderRadius: 4,
                    }}
                  >
                    {endpoint}
                  </span>
                  <Tooltip title="一键复制外网访问地址">
                    <Button
                      size="small"
                      type="text"
                      icon={isCopied ? <CheckOutlined style={{ color: '#10b981' }} /> : <CopyOutlined />}
                      onClick={() => copyToClipboard(endpoint, `port-${record.id}`)}
                    />
                  </Tooltip>
                </Space>
              )
            },
          },
          {
            title: '备注说明',
            dataIndex: 'remark',
            key: 'remark',
            render: (rem) => (
              <span style={{ color: rem ? '#475569' : '#94a3b8' }}>{rem || '无备注'}</span>
            ),
          },
          {
            title: '操作',
            key: 'action',
            width: 100,
            render: (_, record) => (
              <Popconfirm
                title="删除端口映射"
                description={`确定删除端口映射 #${record.id} (内部 :${record.private_port} -> 外网 :${record.public_port}) 吗？`}
                onConfirm={() => handleDeletePort(record.id)}
                okText="确认删除"
                cancelText="取消"
                okButtonProps={{ danger: true }}
              >
                <Button size="small" type="link" danger icon={<DeleteOutlined />}>
                  删除
                </Button>
              </Popconfirm>
            ),
          },
        ]}
      />

      {/* 新增端口映射 Modal */}
      <Modal
        title={
          <Space>
            <PlusOutlined style={{ color: '#7c3aed' }} />
            <span>新增公网 NAT 端口映射</span>
          </Space>
        }
        open={addModalVisible}
        onOk={handleAddPort}
        confirmLoading={submitting}
        onCancel={() => setAddModalVisible(false)}
        okText="立即映射"
        cancelText="取消"
        width={480}
      >
        <div style={{ margin: '12px 0 16px', color: '#64748b', fontSize: 13 }}>
          <InfoCircleOutlined style={{ marginRight: 6, color: '#3b82f6' }} />
          平台将自动为您分配 10000 以上的母鸡随机高位公网端口，外部流量将自动转发至容器内部私有端口。
        </div>

        <Form form={form} layout="vertical">
          <Form.Item
            label="容器内部私有端口 (Private Port)"
            name="private_port"
            rules={[
              { required: true, message: '请输入私有端口号' },
              {
                type: 'number',
                min: 1,
                max: 65535,
                message: '端口号必须在 1 ~ 65535 范围内',
              },
            ]}
            extra={
              <Space style={{ marginTop: 6 }}>
                <span style={{ fontSize: 12, color: '#94a3b8' }}>快捷填入:</span>
                {[
                  { label: '22 (SSH)', port: 22 },
                  { label: '80 (HTTP)', port: 80 },
                  { label: '443 (HTTPS)', port: 443 },
                  { label: '10001 (家宽1)', port: 10001 },
                  { label: '10002 (家宽2)', port: 10002 },
                ].map((item) => (
                  <Button
                    key={item.port}
                    size="small"
                    type="dashed"
                    style={{ fontSize: 11 }}
                    onClick={() => form.setFieldsValue({ private_port: item.port })}
                  >
                    {item.label}
                  </Button>
                ))}
              </Space>
            }
          >
            <InputNumber style={{ width: '100%' }} placeholder="例如: 8080 或 10001" min={1} max={65535} />
          </Form.Item>

          <Form.Item
            label="传输层协议 (Protocol)"
            name="protocol"
            initialValue="tcp"
            rules={[{ required: true }]}
          >
            <Select
              options={[
                { label: 'TCP 协议 (推荐绝大多数场景)', value: 'tcp' },
                { label: 'UDP 协议 (音视频/游戏/QUIC)', value: 'udp' },
              ]}
            />
          </Form.Item>

          <Form.Item label="规则备注 (可选)" name="remark">
            <Input placeholder="例如: Web 控制台、VLESS Reality 节点 1 等" maxLength={40} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
