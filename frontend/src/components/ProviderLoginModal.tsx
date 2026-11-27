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
  Select,
  Tag,
  Divider,
  message,
} from 'antd'
import {
  KeyOutlined,
  SafetyCertificateOutlined,
  CloudServerOutlined,
  MailOutlined,
  LockOutlined,
  UserOutlined,
  CheckCircleOutlined,
  LinkOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { api, type VaultCipherSummary, type TokensConfig } from '../api'

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
  const [activeTab, setActiveTab] = useState<'vault' | 'manual' | 'token'>('vault')

  // Vaultwarden 状态
  const [vaultForm] = Form.useForm()
  const [connectingVault, setConnectingVault] = useState(false)
  const [bindingVault, setBindingVault] = useState(false)
  const [ciphers, setCiphers] = useState<VaultCipherSummary[]>([])
  const [selectedCipherId, setSelectedCipherId] = useState<string>('')

  // 手动测试登录状态
  const [manualForm] = Form.useForm()
  const [testingManual, setTestingManual] = useState(false)

  // Token 状态
  const [tokenForm] = Form.useForm()
  const [submittingToken, setSubmittingToken] = useState(false)
  const [tokenInfo, setTokenInfo] = useState<TokensConfig | null>(null)

  // 快捷免密状态
  const [quickLogging, setQuickLogging] = useState(false)
  const [quickNeed2FA, setQuickNeed2FA] = useState(false)
  const [quick2FAMessage, setQuick2FAMessage] = useState('')
  const [quickGaCode, setQuickGaCode] = useState('')

  useEffect(() => {
    if (open) {
      setCiphers([])
      setSelectedCipherId('')
      setQuickNeed2FA(false)
      setQuick2FAMessage('')
      setQuickGaCode('')
      manualForm.resetFields()
      tokenForm.resetFields()

      // 设置默认 Vaultwarden 地址
      vaultForm.setFieldsValue({
        server: 'http://192.168.8.11:4743',
        username: '',
        password: '',
      })

      setActiveTab('vault')
      api.getTokens().then(setTokenInfo).catch(() => setTokenInfo(null))
    }
  }, [open, provider, vaultForm, manualForm, tokenForm])

  const getProviderTitle = () => {
    switch (provider) {
      case 'akile':
        return 'Akile Cloud'
      case 'lazycat':
        return 'LazyCat 懒猫云'
      case 'acck':
        return 'ACCK'
    }
  }

  // 各平台 JWT 取用位置与当前存储状态（用于 Token 兜底页引导）
  const getTokenGuide = () => {
    if (provider === 'akile') {
      return {
        site: 'https://akile.io',
        steps: '登录 Akile 控制台 → F12 打开开发者工具 → Application / 存储 → Local Storage → 复制 token 字段值',
        masked: tokenInfo?.akile_token_masked,
        expiresAt: tokenInfo?.akile_expires_at,
        isExpired: tokenInfo?.akile_is_expired,
        secondsLeft: tokenInfo?.akile_seconds_left,
      }
    }
    if (provider === 'lazycat') {
      return {
        site: 'https://www.lazycat.cloud',
        steps: '登录懒猫云控制台 → F12 打开 Network → 任选一条 API 请求 → Request Headers 的 Authorization: Bearer 之后的整段字符串',
        masked: tokenInfo?.lazycat_token_masked,
        expiresAt: tokenInfo?.lazycat_expires_at,
        isExpired: tokenInfo?.lazycat_is_expired,
        secondsLeft: tokenInfo?.lazycat_seconds_left,
      }
    }
    return {
      site: 'https://acck.io',
      steps: '登录 acck.io → F12 打开开发者工具 → Application / 存储 → Local Storage → 复制 token 字段值（也可直接用上方的账号密码免密登录自动签发）',
      masked: tokenInfo?.acck_token_masked,
      expiresAt: tokenInfo?.acck_expires_at,
      isExpired: tokenInfo?.acck_is_expired,
      secondsLeft: tokenInfo?.acck_seconds_left,
    }
  }

  const tokenGuide = getTokenGuide()

  const formatLeft = (seconds?: number, expired?: boolean) => {
    if (expired) return '已过期'
    if (!seconds || seconds <= 0) return ''
    if (seconds < 3600) return `剩余 ${Math.max(1, Math.floor(seconds / 60))} 分钟`
    const hours = Math.floor(seconds / 3600)
    const days = Math.floor(hours / 24)
    if (days > 0) return `剩余 ${days} 天 ${hours % 24} 小时`
    return `剩余 ${hours} 小时`
  }

  // 1. Vaultwarden: 连接并拉取密码库列表
  const handleConnectVault = async () => {
    try {
      const values = await vaultForm.validateFields(['server', 'username', 'password'])
      setConnectingVault(true)
      const res = await api.vendorVaultListCiphers({
        server: values.server,
        username: values.username,
        password: values.password,
      })
      setCiphers(res.ciphers || [])
      message.success(`成功读取密码库，共获取 ${res.count} 个条目`)

      // 智能预选最匹配的一项
      if (res.ciphers && res.ciphers.length > 0) {
        const provLower = provider.toLowerCase()
        const matched = res.ciphers.find(c => {
          const nameLower = c.name.toLowerCase()
          if (provLower === 'akile') return nameLower.includes('akile')
          if (provLower === 'lazycat') return nameLower.includes('lazycat') || nameLower.includes('懒猫')
          if (provLower === 'acck') return nameLower.includes('acck')
          return false
        })
        if (matched) {
          setSelectedCipherId(matched.id)
        } else {
          setSelectedCipherId(res.ciphers[0].id)
        }
      }
    } catch (err: any) {
      message.error(err.response?.data?.error || err.message || '连接 Vaultwarden 失败，请检查地址或密码')
    } finally {
      setConnectingVault(false)
    }
  }

  // 1.2 Vaultwarden: 确认关联并测试登录
  const handleBindVaultCipher = async () => {
    if (!selectedCipherId) {
      message.warning('请先选择一个密码条目')
      return
    }
    try {
      setBindingVault(true)
      const values = vaultForm.getFieldsValue()
      const res = await api.vendorVaultBind({
        provider,
        cipher_id: selectedCipherId,
        server: values.server,
        username: values.username,
        password: values.password,
      })
      message.success(res.message || '测试登录成功，已关联并生效')
      onSuccess?.()
      onClose()
    } catch (err: any) {
      message.error(err.response?.data?.error || err.message || '绑定与测试登录失败')
    } finally {
      setBindingVault(false)
    }
  }

  // 2. 手动账号密码 + 2FA 原始代码测试与保存
  const handleManualTestAndSave = async () => {
    try {
      const values = await manualForm.validateFields()
      setTestingManual(true)
      const res = await api.vendorTestAndSave({
        provider,
        username: values.username,
        password: values.password,
        totp_secret: values.totp_secret || '',
        auto_refresh: values.auto_refresh ?? true,
      })
      message.success(res.message || '测试登录成功，凭据已保存生效')
      onSuccess?.()
      onClose()
    } catch (err: any) {
      message.error(err.response?.data?.error || err.message || '测试登录失败，请检查账号密码或2FA密钥')
    } finally {
      setTestingManual(false)
    }
  }

  // 3. 直接保存 Token (兜底)
  const handleTokenSubmit = async () => {
    try {
      const values = await tokenForm.validateFields()
      setSubmittingToken(true)

      if (provider === 'acck') {
        const res = await api.acckLogin(values.token)
        message.success(res.message || 'ACCK Token 校验通过并已保存')
      } else if (provider === 'akile') {
        await api.saveTokens({ akile_token: values.token })
        message.success('Akile Token 已成功保存')
      } else if (provider === 'lazycat') {
        await api.saveTokens({ lazycat_token: values.token })
        message.success('LazyCat Token 已成功保存')
      }

      onSuccess?.()
      onClose()
    } catch (err: any) {
      message.error(err.response?.data?.error || err.message || '保存 Token 失败')
    } finally {
      setSubmittingToken(false)
    }
  }

  // 一键快捷免密自愈重登
  const handleQuickLogin = async (useInlineCode = false) => {
    const gaCode = useInlineCode ? quickGaCode.trim() : ''
    if (useInlineCode && !gaCode) {
      message.warning('请输入 6 位 2FA 动态验证码')
      return
    }
    try {
      setQuickLogging(true)
      const res = await api.vendorQuickLogin(provider, gaCode || undefined)
      if (res.need_2fa) {
        setQuickNeed2FA(true)
        setQuick2FAMessage(res.message || '该账号已开启 2FA 二次验证，请输入 6 位动态验证码')
        message.info(res.message || '请输入 6 位 2FA 动态验证码')
        return
      }
      message.success(res.message || '免密登录成功，Token 已自动刷新生效')
      setQuickNeed2FA(false)
      setQuickGaCode('')
      onSuccess?.()
      onClose()
    } catch (err: any) {
      message.warning(err.response?.data?.error || err.message || '凭据库尚未录入凭据，请选择下方方式配置')
    } finally {
      setQuickLogging(false)
    }
  }

  return (
    <Modal
      title={
        <Space>
          <SafetyCertificateOutlined style={{ color: '#2563eb' }} />
          <span>{getProviderTitle()} 凭据授权与登录管理</span>
        </Space>
      }
      open={open}
      onCancel={onClose}
      footer={null}
      width={640}
      destroyOnClose
    >
      <div style={{ marginTop: 12 }}>
        {/* 顶部一键免密自愈快捷栏 */}
        <div style={{ marginBottom: 16 }}>
          <Button
            type="primary"
            icon={<ThunderboltOutlined />}
            loading={quickLogging}
            onClick={() => handleQuickLogin(false)}
            block
            style={{
              background: '#059669',
              borderColor: '#059669',
              height: 38,
              fontWeight: 600,
            }}
          >
            使用已保存凭据一键免密登录 (自动计算 2FA)
          </Button>
          {quickNeed2FA && (
            <div style={{ marginTop: 10, padding: 12, borderRadius: 8, background: '#f8fafc', border: '1px solid #cbd5e1' }}>
              <div style={{ fontSize: 12, color: '#334155', marginBottom: 8 }}>
                {quick2FAMessage}
              </div>
              <Space.Compact style={{ width: '100%' }}>
                <Input
                  value={quickGaCode}
                  onChange={(e) => setQuickGaCode(e.target.value)}
                  placeholder="请输入 6 位 2FA 动态码"
                  maxLength={8}
                  style={{ fontFamily: 'monospace' }}
                  onPressEnter={() => handleQuickLogin(true)}
                />
                <Button
                  type="primary"
                  loading={quickLogging}
                  onClick={() => handleQuickLogin(true)}
                >
                  提交动态码
                </Button>
              </Space.Compact>
            </div>
          )}
        </div>

        <Tabs
          activeKey={activeTab}
          onChange={(k) => setActiveTab(k as any)}
          items={[
            {
              key: 'vault',
              label: (
                <span>
                  <LinkOutlined /> 密码库关联
                </span>
              ),
              children: (
                <div>
                  <Alert
                    type="info"
                    showIcon
                    message="密码库直接关联"
                    description="输入您的 Vaultwarden 服务器信息，系统将安全读取密码条目列表供您选择，解密后自动完成测试登录与 2FA 绑定。"
                    style={{ marginBottom: 16 }}
                  />

                  <Form form={vaultForm} layout="vertical">
                    <Form.Item
                      label="Vaultwarden 服务器地址"
                      name="server"
                      rules={[{ required: true, message: '请输入 Vaultwarden 服务器地址' }]}
                    >
                      <Input
                        prefix={<CloudServerOutlined style={{ color: '#94a3b8' }} />}
                        placeholder="http://192.168.8.11:4743"
                      />
                    </Form.Item>

                    <Form.Item
                      label="账号 (邮箱)"
                      name="username"
                      rules={[{ required: true, message: '请输入 Vaultwarden 注册邮箱' }]}
                    >
                      <Input
                        prefix={<MailOutlined style={{ color: '#94a3b8' }} />}
                        placeholder="user@example.com"
                      />
                    </Form.Item>

                    <Form.Item
                      label="主密码 (Master Password)"
                      name="password"
                      rules={[{ required: true, message: '请输入 Vaultwarden 主密码' }]}
                    >
                      <Input.Password
                        prefix={<LockOutlined style={{ color: '#94a3b8' }} />}
                        placeholder="输入主密码用于本地临时解密"
                      />
                    </Form.Item>

                    <Button
                      type="default"
                      icon={<LinkOutlined />}
                      loading={connectingVault}
                      onClick={handleConnectVault}
                      block
                      style={{ height: 38, fontWeight: 600, marginBottom: 16, borderColor: '#2563eb', color: '#1d4ed8' }}
                    >
                      连接并读取密码库列表
                    </Button>

                    {ciphers.length > 0 && (
                      <div style={{ background: '#f8fafc', padding: 14, borderRadius: 8, border: '1px solid #e2e8f0', marginBottom: 16 }}>
                        <div style={{ fontWeight: 600, marginBottom: 8, color: '#334155' }}>
                          选择要关联到 {getProviderTitle()} 的条目:
                        </div>
                        <Select
                          style={{ width: '100%', marginBottom: 12 }}
                          value={selectedCipherId}
                          onChange={(val) => setSelectedCipherId(val)}
                          showSearch
                          filterOption={(input, option) =>
                            (option?.label as string || '').toLowerCase().includes(input.toLowerCase())
                          }
                          options={ciphers.map((c) => ({
                            value: c.id,
                            label: `${c.name} (${c.username_masked || '无账号'})`,
                          }))}
                          dropdownRender={(menu) => (
                            <div>
                              {menu}
                              <Divider style={{ margin: '4px 0' }} />
                              <div style={{ padding: '4px 8px', fontSize: 12, color: '#64748b' }}>
                                支持按名称关键字直接搜索条目
                              </div>
                            </div>
                          )}
                        />

                        {selectedCipherId && (
                          <div style={{ fontSize: 12, color: '#475569', marginBottom: 14 }}>
                            {(() => {
                              const item = ciphers.find((c) => c.id === selectedCipherId)
                              if (!item) return null
                              return (
                                <Space wrap>
                                  <span>条目名称: <strong>{item.name}</strong></span>
                                  <span>账号: <strong>{item.username_masked}</strong></span>
                                  {item.has_totp ? (
                                    <Tag color="green">包含 2FA 动态码</Tag>
                                  ) : (
                                    <Tag color="default">无 2FA</Tag>
                                  )}
                                </Space>
                              )
                            })()}
                          </div>
                        )}

                        <Button
                          type="primary"
                          icon={<CheckCircleOutlined />}
                          loading={bindingVault}
                          onClick={handleBindVaultCipher}
                          block
                          style={{ background: '#2563eb', height: 38, fontWeight: 600 }}
                        >
                          确认关联并测试登录
                        </Button>
                      </div>
                    )}
                  </Form>
                </div>
              ),
            },
            {
              key: 'manual',
              label: (
                <span>
                  <UserOutlined /> 账号密码 + 2FA
                </span>
              ),
              children: (
                <div>
                  <Alert
                    type="info"
                    showIcon
                    message="手动测试与保存"
                    description="输入服务商账号密码以及 2FA 原始代码密钥，点击测试登录，系统验证通过后将自动保存并开启掉线自愈。"
                    style={{ marginBottom: 16 }}
                  />

                  <Form
                    form={manualForm}
                    layout="vertical"
                    initialValues={{ auto_refresh: true }}
                    onFinish={handleManualTestAndSave}
                  >
                    <Form.Item
                      label={provider === 'akile' ? 'Akile 账号 (注册邮箱)' : `${getProviderTitle()} 用户名`}
                      name="username"
                      rules={[{ required: true, message: '请输入账号' }]}
                    >
                      <Input
                        prefix={<UserOutlined style={{ color: '#94a3b8' }} />}
                        placeholder={provider === 'akile' ? 'your-email@example.com' : '用户名'}
                      />
                    </Form.Item>

                    <Form.Item
                      label="登录密码"
                      name="password"
                      rules={[{ required: true, message: '请输入密码' }]}
                    >
                      <Input.Password
                        prefix={<LockOutlined style={{ color: '#94a3b8' }} />}
                        placeholder="账户密码"
                      />
                    </Form.Item>

                    <Form.Item
                      label="2FA 原始代码密钥 (TOTP Secret)"
                      name="totp_secret"
                      extra="请输入开启二次验证时获取的 Base32 密钥代码 (例如 JO2BRAKUPATIELNQ...)。若账号未开启 2FA，请保持留空。"
                    >
                      <Input
                        prefix={<KeyOutlined style={{ color: '#94a3b8' }} />}
                        placeholder="Base32 原始密钥 (可选)"
                        style={{ fontFamily: 'monospace' }}
                      />
                    </Form.Item>

                    <Form.Item name="auto_refresh" valuePropName="checked">
                      <Checkbox>
                        开启免密自动续期与掉线自愈 (遇 Token 失效后台自动重新登录)
                      </Checkbox>
                    </Form.Item>

                    <Button
                      type="primary"
                      htmlType="submit"
                      loading={testingManual}
                      block
                      style={{
                        background: '#2563eb',
                        height: 38,
                        fontWeight: 600,
                        marginTop: 4,
                      }}
                    >
                      测试登录并保存凭据
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
                  <Alert
                    type="info"
                    showIcon
                    message="直接填入 Token（兜底方式）"
                    description={`适用于 ${getProviderTitle()} 已登录但自动登录暂不可用的场景。Token 仅保存到服务端数据卷，不会回显明文。`}
                    style={{ marginBottom: 12 }}
                  />

                  <div style={{ background: '#f8fafc', border: '1px solid #e2e8f0', borderRadius: 8, padding: 12, marginBottom: 16 }}>
                    <div style={{ fontSize: 12, color: '#334155', marginBottom: 8, fontWeight: 600 }}>当前已存 Token</div>
                    <Space size={8} wrap>
                      <Tag color={tokenGuide.masked ? (tokenGuide.isExpired ? 'error' : 'green') : 'default'} style={{ margin: 0, fontFamily: 'monospace' }}>
                        {tokenGuide.masked || '尚未保存'}
                      </Tag>
                      {tokenGuide.expiresAt ? (
                        <span style={{ fontSize: 12, color: tokenGuide.isExpired ? '#ef4444' : '#059669' }}>
                          {tokenGuide.isExpired ? '已过期' : '到期'} {tokenGuide.expiresAt}
                          {formatLeft(tokenGuide.secondsLeft, tokenGuide.isExpired) ? ` · ${formatLeft(tokenGuide.secondsLeft, tokenGuide.isExpired)}` : ''}
                        </span>
                      ) : (
                        <span style={{ fontSize: 12, color: '#94a3b8' }}>无到期信息（非标准 JWT 或未配置）</span>
                      )}
                    </Space>
                  </div>

                  <div style={{ fontSize: 12, color: '#475569', lineHeight: '20px', marginBottom: 12 }}>
                    <div style={{ fontWeight: 600, color: '#334155', marginBottom: 4 }}>获取步骤</div>
                    <div>1. 打开并登录 {tokenGuide.site}</div>
                    <div>2. {tokenGuide.steps}</div>
                    <div>3. 将复制到的整段字符串粘贴到下方输入框并校验保存</div>
                  </div>

                  <Form form={tokenForm} layout="vertical" onFinish={handleTokenSubmit}>
                    <Form.Item
                      label="API Token (JWT)"
                      name="token"
                      rules={[{ required: true, message: '请输入 Token' }]}
                    >
                      <Input.TextArea
                        rows={4}
                        placeholder="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
                        style={{ fontFamily: 'monospace', fontSize: 12, resize: 'none' }}
                      />
                    </Form.Item>
                    <Button
                      type="primary"
                      htmlType="submit"
                      loading={submittingToken}
                      block
                      style={{ background: '#2563eb', height: 38, fontWeight: 600 }}
                    >
                      校验并保存 Token
                    </Button>
                  </Form>
                </div>
              ),
            },
          ]}
        />
      </div>
    </Modal>
  )
}
