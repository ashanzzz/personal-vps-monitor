import React, { useState, useEffect } from 'react'
import {
  Modal,
  Tabs,
  Form,
  Input,
  Button,
  Checkbox,
  Space,
  Alert,
  message,
} from 'antd'
import {
  LoginOutlined,
  KeyOutlined,
  SafetyCertificateOutlined,
  InfoCircleOutlined,
} from '@ant-design/icons'
import { api } from '../api'

interface Props {
  open: boolean
  onClose: () => void
  provider: 'akile' | 'lazycat' | 'acck'
  onSuccess?: () => void
}

export const ProviderLoginModal: React.FC<Props> = ({
  open,
  onClose,
  provider,
  onSuccess,
}) => {
  const [activeTab, setActiveTab] = useState<'password' | 'token'>('password')
  const [submitting, setSubmitting] = useState(false)
  const [need2FA, setNeed2FA] = useState(false)
  const [twoFaPrompt, setTwoFaPrompt] = useState('')

  const [pwdForm] = Form.useForm()
  const [tokenForm] = Form.useForm()

  useEffect(() => {
    if (open) {
      setNeed2FA(false)
      setTwoFaPrompt('')
      pwdForm.resetFields()
      tokenForm.resetFields()
      if (provider === 'acck') {
        setActiveTab('token')
      } else {
        setActiveTab('password')
      }
    }
  }, [open, provider, pwdForm, tokenForm])

  const getProviderTitle = () => {
    switch (provider) {
      case 'akile':
        return 'Akile Cloud 凭据与免密登录'
      case 'lazycat':
        return '懒猫云 (LazyCat) 凭据与免密登录'
      case 'acck':
        return 'ACCK (acck.io) 凭据管理'
    }
  }

  // 账号密码登录
  const handlePasswordLogin = async () => {
    try {
      const values = await pwdForm.validateFields()
      setSubmitting(true)

      if (provider === 'akile') {
        const res = await api.akileLogin({
          username: values.username,
          password: values.password,
          ga_code: values.ga_code || '',
          save_credentials: values.save_credentials ?? true,
        })

        if (res.need_2fa) {
          setNeed2FA(true)
          setTwoFaPrompt(res.message || '检测到该账号开启了二次验证，请输入 6 位动态验证码')
          message.info('请输入 6 位二次验证动态码后点击重新登录')
          return
        }

        message.success(res.message || 'Akile 登录成功！Token 已自动保存')
        onSuccess?.()
        onClose()
      } else if (provider === 'lazycat') {
        const res = await api.lazycatLogin({
          username: values.username,
          password: values.password,
          ga_code: values.ga_code || '',
          save_credentials: values.save_credentials ?? true,
        })

        if (res.need_2fa) {
          setNeed2FA(true)
          setTwoFaPrompt(res.message || '检测到该账号开启了二次验证，请输入 6 位动态验证码')
          message.info('请输入 6 位二次验证动态码后点击重新登录')
          return
        }

        message.success(res.message || '懒猫云登录成功！Token 已自动保存')
        onSuccess?.()
        onClose()
      }
    } catch (err: any) {
      message.error(err.response?.data?.error || err.message || '登录失败，请检查账号密码')
    } finally {
      setSubmitting(false)
    }
  }

  // 直接填入 Token 保存
  const handleTokenSubmit = async () => {
    try {
      const values = await tokenForm.validateFields()
      setSubmitting(true)

      if (provider === 'acck') {
        const res = await api.acckLogin(values.token)
        message.success(res.message || 'ACCK Token 校验通过并已保存')
      } else if (provider === 'akile') {
        await api.saveTokens({ akile_token: values.token })
        message.success('Akile Token 已成功保存')
      } else if (provider === 'lazycat') {
        await api.saveTokens({ lazycat_token: values.token })
        message.success('懒猫云 Token 已成功保存')
      }

      onSuccess?.()
      onClose()
    } catch (err: any) {
      message.error(err.response?.data?.error || err.message || '保存 Token 失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={
        <Space>
          <SafetyCertificateOutlined style={{ color: '#2563eb' }} />
          <span>{getProviderTitle()}</span>
        </Space>
      }
      open={open}
      onCancel={onClose}
      footer={null}
      width={500}
      destroyOnClose
    >
      <div style={{ marginTop: 12 }}>
        {provider === 'acck' ? (
          <div>
            <Alert
              message="ACCK 凭据配置"
              description="ACCK 开启了 Cloudflare 人机验证防护，请从浏览器登录控制台后复制 Token 粘贴至下方，系统将自动校验连通性并持久化。"
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
            />
            <Form form={tokenForm} layout="vertical" onFinish={handleTokenSubmit}>
              <Form.Item
                label="ACCK API Token (JWT)"
                name="token"
                rules={[{ required: true, message: '请输入 ACCK Token' }]}
              >
                <Input.TextArea
                  rows={4}
                  placeholder="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
                  style={{ fontFamily: 'monospace', fontSize: 12 }}
                />
              </Form.Item>
              <Button
                type="primary"
                htmlType="submit"
                loading={submitting}
                block
                style={{ background: '#2563eb', height: 38, fontWeight: 600 }}
              >
                校验并保存 ACCK 凭据
              </Button>
            </Form>
          </div>
        ) : (
          <Tabs
            activeKey={activeTab}
            onChange={(k) => setActiveTab(k as 'password' | 'token')}
            items={[
              {
                key: 'password',
                label: (
                  <span>
                    <LoginOutlined /> 账号密码登录 (推荐)
                  </span>
                ),
                children: (
                  <div>
                    {need2FA && (
                      <Alert
                        message="二次验证 (2FA)"
                        description={twoFaPrompt}
                        type="warning"
                        showIcon
                        style={{ marginBottom: 16 }}
                      />
                    )}

                    <Form
                      form={pwdForm}
                      layout="vertical"
                      initialValues={{ save_credentials: true }}
                      onFinish={handlePasswordLogin}
                    >
                      <Form.Item
                        label={provider === 'akile' ? 'Akile 账号 (注册邮箱)' : '懒猫云账号 (用户名)'}
                        name="username"
                        rules={[{ required: true, message: '请输入账号' }]}
                      >
                        <Input placeholder={provider === 'akile' ? 'your-email@example.com' : '懒猫云用户名'} />
                      </Form.Item>

                      <Form.Item
                        label="登录密码"
                        name="password"
                        rules={[{ required: true, message: '请输入密码' }]}
                      >
                        <Input.Password placeholder="账户密码" />
                      </Form.Item>

                      {need2FA && (
                        <Form.Item
                          label="2FA 动态验证码 (TOTP)"
                          name="ga_code"
                          rules={[{ required: true, message: '请输入 6 位动态验证码' }]}
                        >
                          <Input
                            placeholder="例如: 123456"
                            maxLength={6}
                            style={{ fontFamily: 'monospace', fontSize: 16, letterSpacing: 4 }}
                          />
                        </Form.Item>
                      )}

                      <Form.Item name="save_credentials" valuePropName="checked">
                        <Checkbox>
                          记住账号密码并开启后台免密自动续期 (推荐)
                          <div style={{ fontSize: 12, color: '#94a3b8', marginLeft: 22 }}>
                            系统后台将在 Token 临近过期前自动完成免密重新登录与续期，永不掉线。
                          </div>
                        </Checkbox>
                      </Form.Item>

                      <Button
                        type="primary"
                        htmlType="submit"
                        loading={submitting}
                        block
                        style={{
                          background: provider === 'akile' ? '#2563eb' : '#7c3aed',
                          borderColor: provider === 'akile' ? '#2563eb' : '#7c3aed',
                          height: 38,
                          fontWeight: 600,
                          marginTop: 8,
                        }}
                      >
                        {need2FA ? '提交 2FA 验证码并完成登录' : '立即登录并自动保存 Token'}
                      </Button>
                    </Form>
                  </div>
                ),
              },
              {
                key: 'token',
                label: (
                  <span>
                    <KeyOutlined /> 直接填入 Token
                  </span>
                ),
                children: (
                  <div>
                    <div style={{ fontSize: 13, color: '#64748b', marginBottom: 12 }}>
                      <InfoCircleOutlined style={{ marginRight: 6 }} />
                      若您已有从浏览器控制台获取的 Token，可直接粘贴保存：
                    </div>
                    <Form form={tokenForm} layout="vertical" onFinish={handleTokenSubmit}>
                      <Form.Item
                        label="API Token (JWT)"
                        name="token"
                        rules={[{ required: true, message: '请输入 Token' }]}
                      >
                        <Input.TextArea
                          rows={4}
                          placeholder="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
                          style={{ fontFamily: 'monospace', fontSize: 12 }}
                        />
                      </Form.Item>
                      <Button
                        type="primary"
                        htmlType="submit"
                        loading={submitting}
                        block
                        style={{ background: '#2563eb', height: 38, fontWeight: 600 }}
                      >
                        保存并立即生效
                      </Button>
                    </Form>
                  </div>
                ),
              },
            ]}
          />
        )}
      </div>
    </Modal>
  )
}
