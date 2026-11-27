import React, { useState } from 'react'
import {
  Modal,
  Tabs,
  Tag,
  Space,
  Button,
  message,
  Popconfirm,
  Badge,
} from 'antd'
import {
  CodeOutlined,
  ApiOutlined,
  NodeIndexOutlined,
  DashboardOutlined,
  ThunderboltOutlined,
  SyncOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons'
import { api } from '../api'
import type { Instance } from '../api'
import { LazyCatWebShellView } from './LazyCatWebShellView'
import { LazyCatPortManagementView } from './LazyCatPortManagementView'
import { LazyCatRealityMatrixView } from './LazyCatRealityMatrixView'
import { LazyCatFleetProbeView } from './LazyCatFleetProbeView'

interface Props {
  open: boolean
  onClose: () => void
  instance: Instance | null
  onRefresh?: () => void
}

export const LazyCatControlCenterModal: React.FC<Props> = ({
  open,
  onClose,
  instance,
  onRefresh,
}) => {
  const [activeTab, setActiveTab] = useState<string>('terminal')
  const [networkRestarting, setNetworkRestarting] = useState(false)
  const [singboxSyncing, setSingboxSyncing] = useState(false)

  if (!instance) return null

  // 判定是否属于「全球家宽」专属系列
  const isResidential =
    instance.name?.includes('家宽') ||
    instance.name?.toLowerCase().includes('global') ||
    instance.category?.includes('家宽') ||
    instance.region?.includes('家宽') ||
    instance.name?.includes('NJR3An') ||
    instance.id === '11433'

  // 重启容器网络
  const handleRestartNetworking = async () => {
    setNetworkRestarting(true)
    try {
      const res = await api.lazycatExecCommand(instance.id, '/etc/init.d/networking restart', 10)
      message.success(`容器网络重启完成: ${res.output.trim() || 'OK'}`)
    } catch (err: any) {
      message.error('重启网络失败: ' + (err.response?.data?.error || err.message))
    } finally {
      setNetworkRestarting(false)
    }
  }

  // 触发 sing-box 自愈与凭据热重载
  const handleSyncEgress = async () => {
    setSingboxSyncing(true)
    try {
      const res = await api.syncLazyCatEgress(instance.id)
      message.success(res.message || '已成功触发出口凭据自愈与 sing-box 热重载')
      onRefresh?.()
    } catch (err: any) {
      message.error('触发自愈失败: ' + (err.response?.data?.error || err.message))
    } finally {
      setSingboxSyncing(false)
    }
  }

  const tabItems = [
    {
      key: 'terminal',
      label: (
        <span>
          <CodeOutlined style={{ marginRight: 6 }} />
          实时 WebShell 终端 (PTY)
        </span>
      ),
      children: (
        <div style={{ padding: '8px 0' }}>
          <LazyCatWebShellView instanceId={instance.id} instanceName={instance.name} />
        </div>
      ),
    },
    {
      key: 'ports',
      label: (
        <span>
          <ApiOutlined style={{ marginRight: 6 }} />
          公网 NAT 端口映射 (公共能力)
        </span>
      ),
      children: (
        <div style={{ padding: '8px 0' }}>
          <LazyCatPortManagementView instanceId={instance.id} instanceName={instance.name} />
        </div>
      ),
    },
    ...(isResidential
      ? [
          {
            key: 'reality',
            label: (
              <span>
                <NodeIndexOutlined style={{ marginRight: 6, color: '#7c3aed' }} />
                Reality 出口对照矩阵 (专属能力)
              </span>
            ),
            children: (
              <div style={{ padding: '8px 0' }}>
                <LazyCatRealityMatrixView instanceId={instance.id} instanceName={instance.name} />
              </div>
            ),
          },
          {
            key: 'fleet',
            label: (
              <span>
                <DashboardOutlined style={{ marginRight: 6, color: '#7c3aed' }} />
                住宅母鸡遥测大屏 (专属能力)
              </span>
            ),
            children: (
              <div style={{ padding: '8px 0' }}>
                <LazyCatFleetProbeView instanceId={instance.id} />
              </div>
            ),
          },
        ]
      : []),
  ]

  return (
    <Modal
      title={
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            paddingRight: 28,
            borderBottom: '1px solid #f1f5f9',
            paddingBottom: 12,
          }}
        >
          <Space size={10} wrap>
            <span style={{ fontSize: 17, fontWeight: 700, color: '#0f172a' }}>
              {instance.name}
            </span>
            <Tag color="purple" style={{ fontFamily: 'monospace', fontWeight: 600 }}>
              ID: {instance.id}
            </Tag>
            {isResidential ? (
              <Tag color="magenta" style={{ fontWeight: 600 }}>
                全球家宽专属系列
              </Tag>
            ) : (
              <Tag color="blue">标准 NAT 容器</Tag>
            )}
            <Badge
              status={instance.status === 'running' ? 'success' : 'default'}
              text={instance.status === 'running' ? '运行中 (Online)' : (instance.status || '未知状态')}
              style={{ fontSize: 13 }}
            />
            <Tag
              color="purple"
              style={{
                border: '1px solid #ddd6fe',
                background: '#f5f3ff',
                color: '#6d28d9',
                borderRadius: 4,
                fontWeight: 600,
              }}
            >
              <SafetyCertificateOutlined style={{ marginRight: 4 }} />
              硬编码防销毁保护
            </Tag>
          </Space>

          <Space size={8}>
            {isResidential && (
              <Button
                size="small"
                type="primary"
                ghost
                danger
                icon={<ThunderboltOutlined />}
                loading={singboxSyncing}
                onClick={handleSyncEgress}
              >
                 快捷自愈 / 重载
              </Button>
            )}

            <Popconfirm
              title="重启网络服务"
              description="确定在容器内部执行 /etc/init.d/networking restart 吗？"
              onConfirm={handleRestartNetworking}
              okText="确认重启"
              cancelText="取消"
            >
              <Button
                size="small"
                type="default"
                icon={<SyncOutlined spin={networkRestarting} />}
                loading={networkRestarting}
              >
                重启网络
              </Button>
            </Popconfirm>
          </Space>
        </div>
      }
      open={open}
      onCancel={onClose}
      footer={null}
      width={1120}
      destroyOnClose
      style={{ top: 20 }}
      styles={{
        body: { padding: '12px 24px 20px', minHeight: 560 },
      }}
    >
      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={tabItems}
        size="middle"
      />
    </Modal>
  )
}
