import { useCallback, useEffect, useState } from 'react'
import { Card, Button, Space, Select, Typography, App as AntApp } from 'antd'
import { ReloadOutlined, DownloadOutlined } from '@ant-design/icons'
import { api } from '../api'

const { Text } = Typography

export function LogsPage() {
  const { message } = AntApp.useApp()
  const [content, setContent] = useState('')
  const [lines, setLines] = useState(500)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const r = await api.getAppLog(lines)
      setContent(r.content || '(日志为空)')
    } catch {
      message.error('加载日志失败')
    } finally {
      setLoading(false)
    }
  }, [lines, message])

  useEffect(() => { load() }, [load])

  const download = () => {
    const blob = new Blob([content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `akile-monitor-${new Date().toISOString().slice(0, 19)}.log`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <Card
      size="small"
      title="调试日志"
      extra={
        <Space>
          <Select value={lines} style={{ width: 120 }} onChange={setLines}
            options={[
              { value: 200, label: '最近 200 行' },
              { value: 500, label: '最近 500 行' },
              { value: 1000, label: '最近 1000 行' },
              { value: 5000, label: '最近 5000 行' },
            ]} />
          <Button icon={<ReloadOutlined />} loading={loading} onClick={load}>刷新</Button>
          <Button icon={<DownloadOutlined />} onClick={download}>下载</Button>
        </Space>
      }
    >
      <Text type="secondary" style={{ fontSize: 12 }}>
        出问题时点「下载」把日志发给开发者即可定位。日志含 poll.fetch / autobuy.match / autobuy.result / token.refresh 等结构化事件。
      </Text>
      <pre style={{
        marginTop: 12, background: '#0a0a0a', color: '#9fdf9f', padding: 12,
        borderRadius: 6, maxHeight: '65vh', overflow: 'auto', fontSize: 12,
        lineHeight: 1.5, whiteSpace: 'pre-wrap', wordBreak: 'break-all',
      }}>
        {content}
      </pre>
    </Card>
  )
}
