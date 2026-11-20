import React, { useState, useEffect, useCallback } from 'react'
import {
  Card, Row, Col, Typography, Tag, Button, Space, Table, Select,
  Modal, Input, message as antMessage, Alert, Spin
} from 'antd'
import {
  ThunderboltOutlined, ApiOutlined, ReloadOutlined, CheckCircleOutlined,
  CloseCircleOutlined, SyncOutlined, SendOutlined, LinkOutlined, PlayCircleOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '../api'
import type { UnifiedServer } from '../api'

const { Text, Paragraph } = Typography

export const GatewayPage: React.FC = () => {
  const [gatewayStatus, setGatewayStatus] = useState<any>(null)
  const [substoreStatus, setSubstoreStatus] = useState<any>(null)
  const [loading, setLoading] = useState(false)
  const [proxies, setProxies] = useState<any>(null)
  const [selectedGroup, setSelectedGroup] = useState<string>('自有节点')
  const [reloadingGateway, setReloadingGateway] = useState(false)
  const [testingDelay, setTestingDelay] = useState<Record<string, number>>({})
  const [servers, setServers] = useState<UnifiedServer[]>([])

  // Exporter Modal state
  const [exportModalOpen, setExportModalOpen] = useState(false)
  const [selectedServerForExport, setSelectedServerForExport] = useState<UnifiedServer | null>(null)
  const [generatedNodes, setGeneratedNodes] = useState<any[]>([])
  const [generating, setGenerating] = useState(false)

  const loadStatus = useCallback(async () => {
    setLoading(true)
    try {
      const [gw, sub, srvRes] = await Promise.all([
        api.getSingboxStatus().catch(e => ({ running: false, error: String(e) })),
        api.getSubStoreStatus().catch(e => ({ online: false, error: String(e) })),
        api.getUnifiedServers().catch(() => ({ servers: [] })),
      ])
      setGatewayStatus(gw)
      setSubstoreStatus(sub)
      setServers(srvRes.servers || [])

      if (gw.running) {
        const pr = await api.getSingboxProxies().catch(() => null)
        if (pr) setProxies(pr)
      }
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadStatus()
  }, [loadStatus])

  const handleReloadGateway = async () => {
    setReloadingGateway(true)
    try {
      const res = await api.reloadSingboxConfig()
      if (res.status === 'success') {
        antMessage.success('Unraid Sing-box 网关配置已热重载！')
        loadStatus()
      } else {
        antMessage.error(`重载失败: ${res.error || res.message}`)
      }
    } catch (e: any) {
      antMessage.error(`重载失败: ${e.message}`)
    } finally {
      setReloadingGateway(false)
    }
  }

  const handleTestDelay = async (nodeName: string) => {
    try {
      const res = await api.testSingboxDelay(nodeName)
      if (!res.error && res.delay >= 0) {
        antMessage.success(`${nodeName}: 延迟 ${res.delay}ms`)
        setTestingDelay(prev => ({ ...prev, [nodeName]: res.delay }))
      } else {
        antMessage.warning(`${nodeName}: 节点不可达 (${res.error || '超时'})`)
        setTestingDelay(prev => ({ ...prev, [nodeName]: -1 }))
      }
    } catch (e: any) {
      antMessage.error(`测试失败: ${e.message}`)
    }
  }

  const handleSelectNode = async (groupName: string, nodeName: string) => {
    try {
      const res = await api.selectSingboxNode(groupName, nodeName)
      if (res.status === 'success') {
        antMessage.success(`已切换 [${groupName}] 出口至: ${nodeName}`)
        loadStatus()
      } else {
        antMessage.error(`切换失败: ${res.error}`)
      }
    } catch (e: any) {
      antMessage.error(`切换失败: ${e.message}`)
    }
  }

  const handleExportToSubStore = async (srv: UnifiedServer) => {
    setSelectedServerForExport(srv)
    setGenerating(true)
    setExportModalOpen(true)
    try {
      const res = await api.generateSubStoreNodesForVPS({ server_info: srv })
      setGeneratedNodes(res.nodes || [])
    } catch (e: any) {
      antMessage.error(`生成节点规范失败: ${e.message}`)
    } finally {
      setGenerating(false)
    }
  }

  // Active policy group nodes
  const groupData = proxies?.groups?.[selectedGroup]
  const groupNodes: string[] = groupData?.all || []
  const activeNow: string = groupData?.now || ''

  const nodeColumns: ColumnsType<{ name: string }> = [
    {
      title: '节点名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => {
        const isActive = name === activeNow
        return (
          <Space>
            <span style={{ fontWeight: isActive ? 700 : 400, color: isActive ? '#2563eb' : '#0f172a' }}>
              {name}
            </span>
            {isActive && <Tag color="blue">当前生效出口</Tag>}
          </Space>
        )
      },
    },
    {
      title: '测速延迟',
      key: 'delay',
      width: 140,
      render: (_, r) => {
        const d = testingDelay[r.name]
        if (d === undefined) return <span style={{ color: '#94a3b8' }}>未测试</span>
        if (d < 0) return <Tag color="error">超时/失败</Tag>
        return <Tag color={d < 200 ? 'success' : d < 500 ? 'warning' : 'error'}>{d} ms</Tag>
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 180,
      render: (_, r) => (
        <Space size={6}>
          <Button
            size="small"
            icon={<PlayCircleOutlined />}
            onClick={() => handleTestDelay(r.name)}
          >
            测延迟
          </Button>
          {r.name !== activeNow && (
            <Button
              type="primary"
              size="small"
              onClick={() => handleSelectNode(selectedGroup, r.name)}
            >
              切为此节点
            </Button>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Alert
        message="外部生态外挂连接器（完全解耦模式）"
        description="本项目遵循领域驱动模块化设计：服务器资产与集市逻辑不掺杂任何代理私钥。Sub-Store 与 Unraid 网关仅作为可选生态桥梁，即使离线也不影响任何 VPS 监控与集市抢购业务。"
        type="info"
        showIcon
      />

      {/* 2 Top Service Cards */}
      <Row gutter={[16, 16]}>
        {/* Unraid Sing-box Gateway Card */}
        <Col xs={24} md={12}>
          <Card
            title={
              <Space>
                <ThunderboltOutlined style={{ color: '#2563eb' }} />
                <span>Unraid 全局 Sing-box / Clash.Meta 网关</span>
              </Space>
            }
            extra={
              <Button
                size="small"
                icon={<ReloadOutlined spin={reloadingGateway} />}
                onClick={handleReloadGateway}
                disabled={!gatewayStatus?.running}
              >
                热重载配置
              </Button>
            }
          >
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text type="secondary">网关 REST 地址：</Text>
                <Text code>http://192.168.8.11:9090</Text>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text type="secondary">运行状态与版本：</Text>
                {gatewayStatus?.running ? (
                  <Tag icon={<CheckCircleOutlined />} color="success">
                    在线 · {gatewayStatus.version || 'sing-box/clash'}
                  </Tag>
                ) : (
                  <Tag icon={<CloseCircleOutlined />} color="error">
                    离线 / 未连接
                  </Tag>
                )}
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text type="secondary">当前托管拓扑：</Text>
                <Text strong>
                  {proxies ? `${proxies.groups_count} 个策略组 · ${proxies.nodes_count} 个节点` : '—'}
                </Text>
              </div>
            </div>
          </Card>
        </Col>

        {/* Sub-Store Service Card */}
        <Col xs={24} md={12}>
          <Card
            title={
              <Space>
                <ApiOutlined style={{ color: '#059669' }} />
                <span>Sub-Store 订阅聚合管线管理器</span>
              </Space>
            }
            extra={
              <Button
                size="small"
                icon={<LinkOutlined />}
                onClick={() => window.open('http://192.168.8.11:3000', '_blank')}
              >
                打开 Sub-Store
              </Button>
            }
          >
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text type="secondary">Sub-Store 服务地址：</Text>
                <Text code>http://192.168.8.11:3000</Text>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text type="secondary">运行状态与版本：</Text>
                {substoreStatus?.online ? (
                  <Tag icon={<CheckCircleOutlined />} color="success">
                    在线 · v{substoreStatus.version}
                  </Tag>
                ) : (
                  <Tag icon={<CloseCircleOutlined />} color="error">
                    离线 / 未连接
                  </Tag>
                )}
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text type="secondary">核心定位：</Text>
                <Text type="secondary">
                  负责自由节点管理、外部订阅清洗、规则脚本排序并喂给 Unraid 网关。
                </Text>
              </div>
            </div>
          </Card>
        </Col>
      </Row>

      {/* Gateway Policy Group Inspection */}
      {proxies?.groups && (
        <Card
          title={
            <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
              <span>策略组观测与选路切换：</span>
              <Select
                value={selectedGroup}
                onChange={setSelectedGroup}
                style={{ width: 200 }}
                options={Object.keys(proxies.groups).map(k => ({ label: k, value: k }))}
              />
            </div>
          }
          extra={
            <Button size="small" icon={<SyncOutlined spin={loading} />} onClick={loadStatus}>
              刷新节点
            </Button>
          }
        >
          <Table
            size="small"
            pagination={{ pageSize: 8 }}
            columns={nodeColumns}
            dataSource={groupNodes.map(n => ({ key: n, name: n }))}
          />
        </Card>
      )}

      {/* Quick VPS to Sub-Store Exporter */}
      <Card
        title="⚡ 机器资产快速导出为 Sub-Store 节点规范 (Node Exporter)"
      >
        <Paragraph type="secondary" style={{ marginBottom: 16 }}>
          选择名下任意一台机器，自动识别公网 IP 或 NAT 动态映射端口，一键生成规范格式，省去手动复制端口的麻烦。
        </Paragraph>
        <Table
          size="small"
          pagination={false}
          columns={[
            {
              title: '平台',
              dataIndex: 'platform',
              key: 'platform',
              width: 90,
              render: (p: string) => <Tag color={p === 'lazycat' ? 'purple' : 'blue'}>{p.toUpperCase()}</Tag>,
            },
            {
              title: '机器名称',
              dataIndex: 'name',
              key: 'name',
            },
            {
              title: '公网 IP / 入口',
              dataIndex: 'ipv4',
              key: 'ipv4',
              render: (ip: string) => <Text code>{ip || '动态分配'}</Text>,
            },
            {
              title: '网络模式',
              key: 'mode',
              render: (_, r) => (
                <Tag color={r.platform === 'lazycat' ? 'orange' : 'green'}>
                  {r.platform === 'lazycat' ? 'NAT 端口映射' : '独立公网 IPv4'}
                </Tag>
              ),
            },
            {
              title: '操作',
              key: 'action',
              width: 140,
              render: (_, r) => (
                <Button
                  size="small"
                  type="primary"
                  icon={<SendOutlined />}
                  onClick={() => handleExportToSubStore(r)}
                >
                  导出节点 JSON
                </Button>
              ),
            },
          ]}
          dataSource={servers.map(s => ({ ...s, key: s.uid }))}
        />
      </Card>

      {/* Exporter Modal */}
      <Modal
        title={`导出 Sub-Store 节点规范 · ${selectedServerForExport?.name || ''}`}
        open={exportModalOpen}
        onCancel={() => setExportModalOpen(false)}
        footer={[
          <Button key="close" onClick={() => setExportModalOpen(false)}>
            关闭
          </Button>,
          <Button
            key="copy"
            type="primary"
            onClick={() => {
              navigator.clipboard.writeText(JSON.stringify(generatedNodes, null, 2))
              antMessage.success('已复制节点 JSON 到剪贴板！可以直接粘贴到 Sub-Store 中。')
            }}
          >
            复制节点 JSON
          </Button>,
        ]}
        width={700}
      >
        {generating ? (
          <Spin tip="正在解析网络端口与生成规范..." />
        ) : (
          <div>
            <Paragraph type="secondary">
              系统已根据该机器的网络出口（{selectedServerForExport?.ipv4}）和端口映射自动生成规范配置，可以直接导入 Sub-Store：
            </Paragraph>
            <Input.TextArea
              rows={12}
              value={JSON.stringify(generatedNodes, null, 2)}
              readOnly
              style={{ fontFamily: 'monospace', fontSize: 12 }}
            />
          </div>
        )}
      </Modal>
    </div>
  )
}
