import React, { useEffect, useState, useCallback } from 'react'
import {
  Table,
  Button,
  Space,
  Tag,
  Typography,
  message,
  Card,
  Tooltip,
  Alert,
  Modal,
} from 'antd'
import {
  ReloadOutlined,
  CopyOutlined,
  CheckOutlined,
  ThunderboltOutlined,
  LinkOutlined,
  CheckCircleOutlined,
  WarningOutlined,
  PlusOutlined,
  QrcodeOutlined,
  ClockCircleOutlined,
} from '@ant-design/icons'
import { api } from '../api'
import type { EgressNode, PortMapping } from '../api'

const { Text } = Typography

interface Props {
  instanceId: string
  instanceName?: string
}

export const LazyCatRealityMatrixView: React.FC<Props> = ({ instanceId }) => {
  const [loading, setLoading] = useState(false)
  const [egressNodes, setEgressNodes] = useState<EgressNode[]>([])
  const [hostPublicIp, setHostPublicIp] = useState<string>('')
  const [copiedKey, setCopiedKey] = useState<string | null>(null)
  const [bindingLoading, setBindingLoading] = useState<Record<number, boolean>>({})
  const [syncing, setSyncing] = useState(false)
  const [qrModalNode, setQrModalNode] = useState<EgressNode | null>(null)
  const [lastRefreshedAt, setLastRefreshedAt] = useState<string>('')

  const fetchData = useCallback(async (refresh?: boolean) => {
    if (!instanceId) return
    setLoading(true)
    try {
      const [nodesRes, portsRes] = await Promise.all([
        api.lazycatGetEgressNodes(instanceId, refresh),
        api.getLazyCatPorts(instanceId),
      ])

      const hostIp = portsRes.host_public_ip || ''
      setHostPublicIp(hostIp)
      setLastRefreshedAt(nodesRes.refreshed_at || new Date().toISOString())

      const activePorts = portsRes.ports || []

      // 联动匹配：将容器 nodes.json 与 NAT 端口映射动态联结
      const mappedNodes: EgressNode[] = (nodesRes.nodes || []).map((node: EgressNode) => {
        const privateP = node.private_port || 10000 + node.id
        const matched = activePorts.find((p: PortMapping) => p.private_port === privateP)
        const publicP = matched ? matched.public_port : 0

        let vlessUrl = ''
        if (publicP > 0 && hostIp) {
          const nodeTitle = `${node.flag_emoji || ''} ${node.name}`
          vlessUrl = `vless://lazycat@${hostIp}:${publicP}?encryption=none&flow=${node.flow || 'xtls-rprx-vision'}&security=reality&sni=${node.sni || 'www.apple.com'}&fp=chrome&type=tcp#${encodeURIComponent(nodeTitle)}`
        }

        return {
          ...node,
          private_port: privateP,
          public_port: publicP,
          vless_url: vlessUrl,
        }
      })

      setEgressNodes(mappedNodes)
    } catch (err: any) {
      message.error('加载家宽出口矩阵失败: ' + (err.response?.data?.error || err.message))
    } finally {
      setLoading(false)
    }
  }, [instanceId])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const copyToClipboard = (text: string, key: string) => {
    navigator.clipboard.writeText(text)
    setCopiedKey(key)
    message.success(`已复制到剪贴板`)
    setTimeout(() => setCopiedKey(null), 2000)
  }

  // 一键绑定 NAT 端口
  const handleAutoBindPort = async (node: EgressNode) => {
    const targetPrivatePort = node.private_port || 10000 + node.id
    setBindingLoading((prev) => ({ ...prev, [node.id]: true }))
    try {
      const res = await api.addLazyCatPort(
        instanceId,
        targetPrivatePort,
        'tcp',
        `Reality-${node.country_code || 'GW'}-${node.name}`
      )
      message.success(`已为 [${node.name}] 成功绑定公网 NAT 端口 :${res.public_port}！`)
      await fetchData()
    } catch (err: any) {
      message.error('一键绑定 NAT 端口失败: ' + (err.response?.data?.error || err.message))
    } finally {
      setBindingLoading((prev) => ({ ...prev, [node.id]: false }))
    }
  }

  // 快捷触发出口凭据自愈与重载
  const handleSyncEgress = async () => {
    setSyncing(true)
    try {
      const res = await api.syncLazyCatEgress()
      message.success(res.message || '已触发家宽出口凭据自愈与 sing-box 热重载')
      await fetchData()
    } catch (err: any) {
      message.error('触发自愈失败: ' + (err.response?.data?.error || err.message))
    } finally {
      setSyncing(false)
    }
  }

  // 批量复制所有可用 VLESS 节点链接
  const handleBatchCopyVless = () => {
    const validLinks = egressNodes
      .filter((n) => n.vless_url)
      .map((n) => n.vless_url as string)

    if (validLinks.length === 0) {
      message.warning('当前暂无可用的已映射 VLESS 链接，请先绑定公网端口')
      return
    }

    navigator.clipboard.writeText(validLinks.join('\n'))
    message.success(`已成功批量复制 ${validLinks.length} 条 VLESS Reality 节点订阅链接！`)
  }

  // 将 NAT 端口映射状态与出口节点对应
  const mappedNodeCount = egressNodes.filter((n) => (n.public_port ?? 0) > 0).length

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* 提示与状态横幅 */}
      <Alert
        message={
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span>
              <b>动态住宅出口与 VLESS Reality 对照矩阵</b> (根据容器 <Text code>/root/nodes.json</Text> 动态自适应探测，当前共 {egressNodes.length} 大国家宽出口)
            </span>
            <Space>
              <Button
                size="small"
                icon={<ReloadOutlined spin={loading} />}
                onClick={() => fetchData(true)}
              >
                 重新探测 Reality 映射
              </Button>
              <Button
                size="small"
                type="primary"
                icon={<CopyOutlined />}
                onClick={handleBatchCopyVless}
                style={{ background: '#059669', borderColor: '#059669' }}
              >
                批量复制已映射 VLESS
              </Button>
              <Button
                size="small"
                type="primary"
                danger
                ghost
                icon={<ThunderboltOutlined />}
                loading={syncing}
                onClick={handleSyncEgress}
              >
                 凭据自愈 / 重载 sing-box
              </Button>
            </Space>
          </div>
        }
        description={
          <div style={{ marginTop: 6, fontSize: 13, color: '#475569', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              母鸡宿主机公网 IPv4: <b style={{ fontFamily: 'monospace' }}>{hostPublicIp || '未检测到'}</b> · 
              已联动 NAT 端口: <b>{mappedNodeCount} / {egressNodes.length}</b> 个出口 · 
              私有端口规则: 容器内部对应 <Text code>10000 + ID</Text> 入站监听。
            </div>
            <div style={{ fontSize: 12, color: '#64748b' }}>
              <ClockCircleOutlined style={{ marginRight: 4 }} />
              更新时间: <b>{lastRefreshedAt ? new Date(lastRefreshedAt).toLocaleTimeString('zh-CN', { hour12: false }) : '刚刚'}</b>
            </div>
          </div>
        }
        type="info"
        showIcon
        style={{ borderRadius: 8, background: '#f8fafc', borderColor: '#cbd5e1' }}
      />

      {/* 矩阵表格 */}
      <Card
        size="small"
        title={
          <Space>
            <Text strong style={{ fontSize: 15, color: '#1e293b' }}>
              出口节点与 Reality 入站联动详情 ({egressNodes.length} 个动态出口)
            </Text>
          </Space>
        }
        extra={
          <Button size="small" icon={<ReloadOutlined spin={loading} />} onClick={() => fetchData()}>
            刷新节点矩阵
          </Button>
        }
        bordered={false}
      >
        <Table
          loading={loading}
          dataSource={egressNodes}
          rowKey="id"
          pagination={false}
          size="middle"
          columns={[
            {
              title: '出口序号',
              key: 'tag',
              width: 110,
              render: (_, record) => (
                <Space>
                  <span style={{ fontSize: 16 }}>{record.flag_emoji || ''}</span>
                  <Tag color="purple" style={{ fontFamily: 'monospace', fontWeight: 600 }}>
                    {record.outbound_tag || `out-${record.id}`}
                  </Tag>
                </Space>
              ),
            },
            {
              title: '住宅出口名称',
              dataIndex: 'name',
              key: 'name',
              render: (name, record) => (
                <div>
                  <div style={{ fontWeight: 600, color: '#0f172a' }}>{name}</div>
                  <div style={{ fontSize: 12, color: '#64748b' }}>
                    地区: {record.country_code || '全球住宅'}
                  </div>
                </div>
              ),
            },
            {
              title: '容器内私有端口',
              dataIndex: 'private_port',
              key: 'private_port',
              width: 140,
              render: (port, record) => (
                <span style={{ fontFamily: 'monospace', fontWeight: 600, color: '#334155' }}>
                  :{port || 10000 + record.id}
                </span>
              ),
            },
            {
              title: '公网 NAT 映射状态',
              key: 'nat_status',
              width: 150,
              render: (_, record) => {
                if ((record.public_port ?? 0) > 0) {
                  return (
                    <Tag icon={<CheckCircleOutlined />} color="success" style={{ fontWeight: 600 }}>
                      已映射 :{record.public_port}
                    </Tag>
                  )
                }
                return (
                  <Tag icon={<WarningOutlined />} color="warning">
                    未分配公网映射
                  </Tag>
                )
              },
            },
            {
              title: '公网直连入口',
              key: 'public_endpoint',
              render: (_, record) => {
                if ((record.public_port ?? 0) > 0 && hostPublicIp) {
                  const endpoint = `${hostPublicIp}:${record.public_port}`
                  const isCopied = copiedKey === `ep-${record.id}`
                  return (
                    <Space>
                      <span
                        style={{
                          fontFamily: 'monospace',
                          fontSize: 12,
                          background: '#f1f5f9',
                          padding: '2px 6px',
                          borderRadius: 4,
                          color: '#0f172a',
                        }}
                      >
                        {endpoint}
                      </span>
                      <Tooltip title="复制公网直连地址">
                        <Button
                          size="small"
                          type="text"
                          icon={isCopied ? <CheckOutlined style={{ color: '#10b981' }} /> : <CopyOutlined />}
                          onClick={() => copyToClipboard(endpoint, `ep-${record.id}`)}
                        />
                      </Tooltip>
                    </Space>
                  )
                }
                return <span style={{ color: '#94a3b8', fontSize: 12 }}>需先绑定公网端口</span>
              },
            },
            {
              title: 'Reality VLESS 链接',
              key: 'vless',
              render: (_, record) => {
                if (!record.vless_url) {
                  return <span style={{ color: '#94a3b8', fontSize: 12 }}>端口未映射无外网链接</span>
                }
                const isCopied = copiedKey === `vless-${record.id}`
                return (
                  <Space>
                    <Tooltip title="复制此节点的完整 VLESS Reality 导入链接">
                      <Button
                        size="small"
                        type="primary"
                        ghost
                        icon={isCopied ? <CheckOutlined /> : <LinkOutlined />}
                        onClick={() => copyToClipboard(record.vless_url || '', `vless-${record.id}`)}
                      >
                        {isCopied ? '已复制' : '复制 VLESS'}
                      </Button>
                    </Tooltip>
                    <Tooltip title="查看导入信息">
                      <Button
                        size="small"
                        type="text"
                        icon={<QrcodeOutlined />}
                        onClick={() => setQrModalNode(record)}
                      />
                    </Tooltip>
                  </Space>
                )
              },
            },
            {
              title: '联动操作',
              key: 'action',
              width: 140,
              render: (_, record) => {
                if ((record.public_port ?? 0) > 0) {
                  return (
                    <Tag color="default" style={{ color: '#64748b' }}>
                      已完成联动
                    </Tag>
                  )
                }

                return (
                  <Button
                    size="small"
                    type="primary"
                    icon={<PlusOutlined />}
                    loading={bindingLoading[record.id]}
                    onClick={() => handleAutoBindPort(record)}
                    style={{ background: '#7c3aed', borderColor: '#7c3aed' }}
                  >
                    一键绑定 NAT
                  </Button>
                )
              },
            },
          ]}
        />
      </Card>

      {/* 节点详情 Modal */}
      <Modal
        title={
          qrModalNode && (
            <Space>
              <span style={{ fontSize: 18 }}>{qrModalNode.flag_emoji}</span>
              <span>{qrModalNode.name} - VLESS Reality 节点凭据</span>
            </Space>
          )
        }
        open={!!qrModalNode}
        onCancel={() => setQrModalNode(null)}
        footer={[
          <Button key="close" onClick={() => setQrModalNode(null)}>
            关闭
          </Button>,
          <Button
            key="copy"
            type="primary"
            icon={<CopyOutlined />}
            onClick={() => {
              if (qrModalNode?.vless_url) {
                copyToClipboard(qrModalNode.vless_url, 'modal-vless')
              }
            }}
            style={{ background: '#7c3aed', borderColor: '#7c3aed' }}
          >
            复制完整节点链接
          </Button>,
        ]}
      >
        {qrModalNode && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12, padding: '8px 0' }}>
            <div>
              <Text type="secondary" style={{ fontSize: 12 }}>
                完整 VLESS Reality 客户端导入链接:
              </Text>
              <div
                style={{
                  background: '#0f172a',
                  color: '#38bdf8',
                  padding: 12,
                  borderRadius: 6,
                  fontFamily: 'monospace',
                  fontSize: 12,
                  wordBreak: 'break-all',
                  marginTop: 6,
                  maxHeight: 180,
                  overflowY: 'auto',
                }}
              >
                {qrModalNode.vless_url || '暂无'}
              </div>
            </div>

            <div style={{ fontSize: 13, color: '#64748b' }}>
              <div>公网 IP: <b style={{ color: '#0f172a' }}>{hostPublicIp}</b></div>
              <div>公网映射端口: <b style={{ color: '#7c3aed' }}>{qrModalNode.public_port}</b></div>
              <div>容器内部监听: <b style={{ color: '#0f172a' }}>{qrModalNode.private_port}</b></div>
              <div>传输协议: <b>TCP + Reality (sing-box 原生兼容)</b></div>
            </div>
          </div>
        )}
      </Modal>
    </div>
  )
}
