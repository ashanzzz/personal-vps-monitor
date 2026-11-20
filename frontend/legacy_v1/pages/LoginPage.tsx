import { useEffect, useState } from 'react'
import { Card, Form, Input, Button, Typography, message, Alert } from 'antd'
import { LockOutlined, UserOutlined } from '@ant-design/icons'
import { api, setToken } from '../api'

const { Title, Text } = Typography

interface Props {
  onAuthed: (username: string) => void
}

/**
 * Gatekeeper screen. Shows a first-time "set admin password" form when the
 * backend reports no admin exists, otherwise a normal login form.
 */
export function LoginPage({ onAuthed }: Props) {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [initialized, setInitialized] = useState<boolean | null>(null)

  useEffect(() => {
    api.authStatus()
      .then(s => setInitialized(s.initialized))
      .catch(() => setInitialized(true))  // assume login on error
  }, [])

  const handleSubmit = async () => {
    try {
      const vals = await form.validateFields()
      setLoading(true)
      const res = initialized
        ? await api.login(vals.username, vals.password)
        : await api.setupAdmin(vals.username, vals.password)
      setToken(res.token)
      message.success(initialized ? '登录成功' : '管理员已创建，已自动登录')
      onAuthed(res.username)
    } catch (e: any) {
      if (e?.errorFields) return
      message.error(e?.response?.data?.detail ?? (initialized ? '登录失败' : '创建失败'))
    } finally {
      setLoading(false)
    }
  }

  const isSetup = initialized === false

  return (
    <div style={{
      minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: '#0d0d0d',
    }}>
      <Card style={{ width: 380, background: '#161616', borderColor: '#1f1f1f' }}>
        <div style={{ textAlign: 'center', marginBottom: 20 }}>
          <Title level={4} style={{ color: '#e0e0e0', marginBottom: 4 }}>🖥 个人 vps 监控</Title>
          <Text type="secondary">{isSetup ? '首次使用，请设置管理员账号' : '请登录'}</Text>
        </div>

        {isSetup && (
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message="设置后请牢记账号密码"
            description="也可在部署时用环境变量 ADMIN_USER / ADMIN_PASSWORD 预设。"
          />
        )}

        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="用户名" autoComplete="username" size="large" />
          </Form.Item>
          <Form.Item
            name="password"
            rules={[
              { required: true, message: '请输入密码' },
              ...(isSetup ? [{ min: 6, message: '密码至少 6 位' }] : []),
            ]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder={isSetup ? '设置密码（至少 6 位）' : '密码'}
              autoComplete={isSetup ? 'new-password' : 'current-password'}
              size="large"
              onPressEnter={handleSubmit}
            />
          </Form.Item>
          <Button type="primary" block size="large" loading={loading} onClick={handleSubmit} disabled={initialized === null}>
            {isSetup ? '创建并进入' : '登录'}
          </Button>
        </Form>
      </Card>
    </div>
  )
}
