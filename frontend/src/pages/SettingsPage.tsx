import { useEffect, useState, useCallback } from 'react'
import {
  Card, Form, Input, InputNumber, Switch, Select, Button, Collapse,
  Divider, message, Typography, Radio, TimePicker, Space, Affix, Modal,
} from 'antd'
import {
  SaveOutlined, SendOutlined, ReloadOutlined, CheckCircleTwoTone,
  WarningTwoTone, CloseCircleTwoTone, LoginOutlined,
} from '@ant-design/icons'
import dayjs from 'dayjs'
import type { AppSettings, TokenStatus, VersionInfo } from '../api'
import { api } from '../api'

const { Text } = Typography

const LS_REGION_OPTIONS = ['HK', 'JP', 'SG', 'TW', 'US', 'DE', 'MO', 'KR'].map(r => ({ value: r, label: r }))

interface Props {
  settings: AppSettings | null
  onSaved: (s: AppSettings) => void
}

// Format remaining seconds as "23.5 小时" / "45 分钟"
function fmtLeft(sec: number): string {
  if (sec <= 0) return '已过期'
  const h = sec / 3600
  if (h >= 1) return `${h.toFixed(1)} 小时`
  return `${Math.round(sec / 60)} 分钟`
}

function TokenStatusBadge({ status }: { status: TokenStatus | null }) {
  if (!status) return <Text type="secondary">加载中…</Text>
  if (!status.configured) {
    return <span><CloseCircleTwoTone twoToneColor="#ff4d4f" /> <Text type="secondary">未配置 Token</Text></span>
  }
  const left = status.seconds_left
  if (left <= 0) {
    return <span><CloseCircleTwoTone twoToneColor="#ff4d4f" /> <Text type="danger">已过期，请重新登录获取</Text></span>
  }
  const warn = left < 6 * 3600
  const icon = warn
    ? <WarningTwoTone twoToneColor="#faad14" />
    : <CheckCircleTwoTone twoToneColor="#52c41a" />
  return (
    <span>
      {icon}{' '}
      <Text type={warn ? 'warning' : 'success'}>有效，剩余 {fmtLeft(left)}</Text>
      {status.expires_at && <Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>到期 {status.expires_at}</Text>}
    </span>
  )
}

export function SettingsPage({ settings, onSaved }: Props) {
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const [testingTg, setTestingTg] = useState(false)
  const [tokenStatus, setTokenStatus] = useState<TokenStatus | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [version, setVersion] = useState<VersionInfo | null>(null)
  const [akileUsername, setAkileUsername] = useState('')
  const [akilePassword, setAkilePassword] = useState('')
  const [loggingIn, setLoggingIn] = useState(false)
  const [_2faPending, set2faPending] = useState<{ username: string; password: string } | null>(null)
  const [_2faCode, set2faCode] = useState('')
  const [_2faSubmitting, set2faSubmitting] = useState(false)

  const loadTokenStatus = useCallback(async () => {
    try {
      setTokenStatus(await api.getTokenStatus())
    } catch {
      /* ignore */
    }
  }, [])

  useEffect(() => {
    if (settings) {
      form.setFieldsValue({
        ...settings,
        watched_plans: (settings.watched_plans ?? []).join('\n'),
        dnd_start: settings.dnd_start ? dayjs(settings.dnd_start, 'HH:mm') : null,
        dnd_end: settings.dnd_end ? dayjs(settings.dnd_end, 'HH:mm') : null,
        refresh_margin_min: settings.refresh_margin_min_seconds ?? 60,
        refresh_margin_max: settings.refresh_margin_max_seconds ?? 21600,
      })
    }
  }, [settings, form])

  useEffect(() => { loadTokenStatus() }, [loadTokenStatus])

  useEffect(() => {
    api.getVersion().then(setVersion).catch(() => { /* ignore */ })
  }, [])

  const handleSave = async () => {
    try {
      const vals = await form.validateFields()
      setSaving(true)
      const payload = {
        ...vals,
        watched_plans: splitLines(vals.watched_plans),
        dnd_start: vals.dnd_start ? dayjs(vals.dnd_start).format('HH:mm') : '23:00',
        dnd_end: vals.dnd_end ? dayjs(vals.dnd_end).format('HH:mm') : '08:00',
        refresh_margin_min_seconds: vals.refresh_margin_min ?? 60,
        refresh_margin_max_seconds: vals.refresh_margin_max ?? 21600,
      }
      const saved = await api.saveSettings(payload)
      onSaved(saved)
      message.success('设置已保存')
      loadTokenStatus()
    } catch (e: any) {
      if (e?.errorFields) message.error('请检查表单填写')
      else message.error(e?.response?.data?.detail ?? '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const manualRefresh = async () => {
    setRefreshing(true)
    try {
      const res = await api.refreshToken()
      if (res.status === 'ok') {
        message.success('Token 已续期')
        loadTokenStatus()
      } else {
        message.error('续期失败，请查看后端日志或重新登录获取 Token')
      }
    } catch {
      message.error('续期请求失败')
    } finally {
      setRefreshing(false)
    }
  }

  const akileLogin = async () => {
    if (!akileUsername || !akilePassword) {
      message.warning('请输入 Akile 用户名和密码')
      return
    }
    setLoggingIn(true)
    try {
      const status = await api.akileLogin(akileUsername, akilePassword)
      if ((status as any).need_2fa) {
        // 2FA required — show modal
        set2faPending({ username: akileUsername, password: akilePassword })
        set2faCode('')
        return
      }
      setTokenStatus(status)
      message.success('登录成功！Token 已自动保存')
      setAkilePassword('')
      loadTokenStatus()
    } catch (e: any) {
      message.error(e?.response?.data?.detail ?? '登录失败')
    } finally {
      setLoggingIn(false)
    }
  }

  const submit2fa = async () => {
    if (!_2faPending || !_2faCode.trim()) {
      message.warning('请输入二次验证码')
      return
    }
    set2faSubmitting(true)
    try {
      const status = await api.akileLogin(_2faPending.username, _2faPending.password, _2faCode.trim())
      setTokenStatus(status)
      message.success('登录成功！Token 已自动保存')
      setAkilePassword('')
      set2faPending(null)
      set2faCode('')
      loadTokenStatus()
    } catch (e: any) {
      message.error(e?.response?.data?.detail ?? '验证失败')
    } finally {
      set2faSubmitting(false)
    }
  }

  const testTelegram = async () => {
    const vals = form.getFieldsValue()
    if (!vals.tg_bot_token || !vals.tg_chat_id) {
      message.warning('请先填写 Bot Token 和 Chat ID')
      return
    }
    setTestingTg(true)
    try {
      const res = await fetch(
        `https://api.telegram.org/bot${vals.tg_bot_token}/sendMessage`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ chat_id: vals.tg_chat_id, text: '🔔 Akile Monitor 测试消息，连接正常！' }),
        }
      )
      const data = await res.json()
      if (data.ok) message.success('Telegram 发送成功！')
      else message.error(`Telegram 错误：${data.description}`)
    } catch {
      message.error('网络请求失败，请检查 Token')
    } finally {
      setTestingTg(false)
    }
  }

  return (
    <div style={{ maxWidth: 760, margin: '0 auto', paddingBottom: 72 }}>
      <Form form={form} layout="vertical" onFinish={handleSave}>
        {/* ── Card 1: API & connection ── */}
        <Card size="small" title="API 与连接" style={{ marginBottom: 16 }}>
          <Form.Item label="Akile Token" name="akile_token" rules={[{ required: true, message: '必填' }]}>
            <Input.Password placeholder="your-token" autoComplete="off" />
          </Form.Item>

          <Form.Item label="Token 状态">
            <Space wrap>
              <TokenStatusBadge status={tokenStatus} />
              <Button size="small" icon={<ReloadOutlined />} loading={refreshing} onClick={manualRefresh}>
                立即续期
              </Button>
            </Space>
          </Form.Item>

          <Collapse
            ghost
            style={{ marginBottom: 12 }}
            items={[{
              key: 'auto-login',
              label: '自动登录获取 Token（用户名 + 密码）',
              children: (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  <Space wrap>
                    <Input
                      placeholder="Akile 用户名/邮箱"
                      value={akileUsername}
                      onChange={e => setAkileUsername(e.target.value)}
                      style={{ width: 200 }}
                      autoComplete="username"
                    />
                    <Input.Password
                      placeholder="密码"
                      value={akilePassword}
                      onChange={e => setAkilePassword(e.target.value)}
                      style={{ width: 200 }}
                      autoComplete="current-password"
                    />
                  </Space>
                  <Button
                    type="primary"
                    icon={<LoginOutlined />}
                    loading={loggingIn}
                    onClick={akileLogin}
                    style={{ alignSelf: 'flex-start' }}
                  >
                    登录获取 Token
                  </Button>
                </div>
              ),
            }]}
          />

          <Form.Item label=" ">
            <Text type="secondary" style={{ fontSize: 12 }}>
              💡 如何手动获取 Token？打开{' '}
              <a href="https://akile.ai/console/pushshop" target="_blank" rel="noopener noreferrer">
                akile.ai/console/pushshop
              </a>{' '}
              登录后，按 F12 打开开发者工具 → Console 标签页，输入：
              <Text code copyable style={{ fontSize: 12, marginLeft: 4 }}>
                localStorage.getItem('akile-token')
              </Text>
              ，复制输出的值粘贴到上方 Token 输入框即可。
            </Text>
          </Form.Item>

          <Space size="large" wrap>
            <Form.Item label="自动续期" name="auto_refresh_token" valuePropName="checked" tooltip="后端拿到首个 Token 后自动续命，无需重新登录">
              <Switch />
            </Form.Item>
          </Space>

          <Form.Item
            label={<span>提前续期时间窗口 <Text type="secondary" style={{ fontSize: 12 }}>Token 到期前多少秒到多少秒之间随机续期（避免固定时间被识别）</Text></span>}
            style={{ marginBottom: 4 }}
          >
            <Space direction="vertical" style={{ width: '100%' }}>
              <div>
                <Text type="secondary" style={{ fontSize: 12 }}>快捷选择</Text>
                <Space wrap style={{ marginTop: 4, display: 'flex' }}>
                  {[
                    { label: '1 分钟', min: 60, max: 60 },
                    { label: '5 分钟', min: 300, max: 300 },
                    { label: '30 分钟', min: 1800, max: 1800 },
                    { label: '1 小时', min: 60, max: 3600 },
                    { label: '3 小时', min: 60, max: 10800 },
                    { label: '6 小时', min: 60, max: 21600 },
                  ].map(p => (
                    <Button
                      key={p.label}
                      size="small"
                      onClick={() => form.setFieldsValue({ refresh_margin_min: p.min, refresh_margin_max: p.max })}
                    >{p.label}</Button>
                  ))}
                </Space>
              </div>
              <Space wrap>
                <Form.Item name="refresh_margin_min" noStyle>
                  <InputNumber
                    min={60}
                    max={21600}
                    step={60}
                    addonBefore="最小"
                    addonAfter="秒"
                    style={{ width: 160 }}
                    placeholder="60"
                  />
                </Form.Item>
                <Form.Item name="refresh_margin_max" noStyle>
                  <InputNumber
                    min={60}
                    max={21600}
                    step={60}
                    addonBefore="最大"
                    addonAfter="秒"
                    style={{ width: 160 }}
                    placeholder="21600"
                  />
                </Form.Item>
              </Space>
            </Space>
          </Form.Item>

          <Form.Item
            label={<span>页面轮询间隔 (秒)<Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>每页（新机VPS / 轻量 / 二手）各自独立轮询 Akile 的间隔；建议 30–300 秒，太短触发限频</Text></span>}
            name="refresh_interval"
          >
            <Space wrap>
              {[15, 30, 60, 120, 300, 600].map(v => (
                <Button
                  key={v}
                  size="small"
                  onClick={() => form.setFieldValue('refresh_interval', v)}
                  type={form.getFieldValue('refresh_interval') === v ? 'primary' : 'default'}
                >{v >= 60 ? `${v / 60} 分钟` : `${v}s`}</Button>
              ))}
            </Space>
          </Form.Item>

          <Form.Item
            label={<span>账户信息刷新间隔 (秒)<Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>「我的账户」页面（余额 / 服务器列表）从缓存读取，按此间隔后台刷新；默认 300 秒</Text></span>}
            name="account_refresh_interval"
          >
            <Space wrap>
              {[60, 120, 300, 600, 1800, 3600].map(v => (
                <Button
                  key={v}
                  size="small"
                  onClick={() => form.setFieldValue('account_refresh_interval', v)}
                  type={form.getFieldValue('account_refresh_interval') === v ? 'primary' : 'default'}
                >{v >= 60 ? `${v / 60} 分钟` : `${v}s`}</Button>
              ))}
            </Space>
          </Form.Item>

          <Space size="large" wrap>
            <Form.Item label="🎲 随机化轮询间隔" name="random_jitter_enabled" valuePropName="checked"
              tooltip="开启后轮询间隔在设定值附近随机波动（±20%），避免固定间隔被识别为机器人">
              <Switch />
            </Form.Item>
          </Space>
          <Collapse ghost items={[{
            key: 'adv',
            label: '高级（请求超时 / 重试）',
            children: (
              <Space size="large" wrap>
                <Form.Item label="请求超时 (秒)" name="request_timeout" style={{ marginBottom: 0 }}>
                  <InputNumber min={5} max={120} style={{ width: 120 }} />
                </Form.Item>
                <Form.Item label="失败重试次数" name="request_retries" style={{ marginBottom: 0 }}>
                  <InputNumber min={0} max={10} style={{ width: 120 }} />
                </Form.Item>
              </Space>
            ),
          }]} />
        </Card>

        {/* ── Card 2: Monitor scope ── */}
        <Card size="small" title="监控范围" style={{ marginBottom: 16 }}>
          <Space size="large" wrap>
            <Form.Item label="新机VPS" name="vps_enabled" valuePropName="checked"><Switch /></Form.Item>
            <Form.Item label="轻量机" name="ls_enabled" valuePropName="checked"><Switch /></Form.Item>
            <Form.Item label="二手机" name="secondhand_enabled" valuePropName="checked"><Switch /></Form.Item>
          </Space>
          <Form.Item label="轻量机监控区域" name="ls_regions">
            <Select mode="multiple" options={LS_REGION_OPTIONS} placeholder="默认全部" />
          </Form.Item>

          <Divider style={{ margin: '12px 0' }}>筛选条件（仅影响推送，面板始终展示全部）</Divider>

          <Form.Item
            label={<span>关注的套餐 ID<Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>每行一个 plan_id，触发预警时高亮</Text></span>}
            name="watched_plans"
          >
            <Input.TextArea rows={3} placeholder="plan_id 每行一个" />
          </Form.Item>

          <Collapse ghost items={[{
            key: 'filters',
            label: '进阶筛选（区域 / 关键词 / 价格 / 配置下限）',
            children: (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                <Form.Item label="只看这些区域" name="filter_areas" tooltip="留空 = 全部区域">
                  <Select mode="tags" placeholder="如 HK、JP，可自定义" />
                </Form.Item>
                <Space size="large" wrap style={{ display: 'flex' }}>
                  <Form.Item label="名称包含" name="filter_name_include">
                    <Input placeholder="关键词" style={{ width: 200 }} />
                  </Form.Item>
                  <Form.Item label="名称排除" name="filter_name_exclude">
                    <Input placeholder="关键词" style={{ width: 200 }} />
                  </Form.Item>
                </Space>
                <Space size="large" wrap>
                  <Form.Item label="价格上限 (分)" name="filter_max_price" tooltip="0 = 不限">
                    <InputNumber min={0} step={100} style={{ width: 140 }} />
                  </Form.Item>
                  <Form.Item label="最低 CPU" name="filter_min_cpu">
                    <InputNumber min={0} style={{ width: 110 }} />
                  </Form.Item>
                  <Form.Item label="最低内存 (MB)" name="filter_min_memory_mb">
                    <InputNumber min={0} step={256} style={{ width: 130 }} />
                  </Form.Item>
                  <Form.Item label="最低流量 (GB)" name="filter_min_traffic_gb">
                    <InputNumber min={0} step={100} style={{ width: 130 }} />
                  </Form.Item>
                  <Form.Item label="最低评分" name="filter_min_score" tooltip="0 = 不限">
                    <InputNumber min={0} step={0.1} style={{ width: 120 }} />
                  </Form.Item>
                  <Form.Item label="最低星级" name="filter_min_star" tooltip="0 = 不限">
                    <InputNumber min={0} max={5} step={1} style={{ width: 120 }} />
                  </Form.Item>
                </Space>
              </div>
            ),
          }]} />
        </Card>

        {/* ── Card 3: Telegram ── */}
        <Card size="small" title="Telegram 通知" style={{ marginBottom: 16 }}>
          <Form.Item label="Bot Token" name="tg_bot_token">
            <Input.Password placeholder="123456:ABC..." autoComplete="off" />
          </Form.Item>
          <Form.Item label="Chat ID" name="tg_chat_id">
            <Input placeholder="-1001234567890 或个人 ID" />
          </Form.Item>
          <Button icon={<SendOutlined />} loading={testingTg} onClick={testTelegram} style={{ marginBottom: 16 }}>
            测试发送
          </Button>
          <Divider style={{ margin: '0 0 12px' }}>推送事件</Divider>
          <Space size="large" wrap>
            <Form.Item label="新上架" name="tg_notify_new_listing" valuePropName="checked"><Switch /></Form.Item>
            <Form.Item label="补货" name="tg_notify_restock" valuePropName="checked"><Switch /></Form.Item>
            <Form.Item label="下架/售罄" name="tg_notify_delisted" valuePropName="checked"><Switch /></Form.Item>
            <Form.Item label="价格变动" name="tg_notify_price_change" valuePropName="checked"><Switch /></Form.Item>
            <Form.Item label="购买成功" name="tg_notify_purchased" valuePropName="checked"><Switch /></Form.Item>
          </Space>
          <Divider style={{ margin: '0 0 12px' }} />
          <Form.Item label="通知范围" name="notify_scope">
            <Radio.Group>
              <Radio value="all">全部机器</Radio>
              <Radio value="watched">仅关注的套餐</Radio>
            </Radio.Group>
          </Form.Item>
          <Space size="large" wrap align="end">
            <Form.Item label="免打扰" name="dnd_enabled" valuePropName="checked"><Switch /></Form.Item>
            <Form.Item label="开始" name="dnd_start"><TimePicker format="HH:mm" minuteStep={5} /></Form.Item>
            <Form.Item label="结束" name="dnd_end"><TimePicker format="HH:mm" minuteStep={5} /></Form.Item>
            <Form.Item label="合并间隔 (分钟)" name="merge_interval_minutes" tooltip="同类通知在此间隔内只发一次，0 = 不合并">
              <InputNumber min={0} max={1440} style={{ width: 120 }} />
            </Form.Item>
          </Space>
        </Card>

        {/* ── Card 4: Generic webhook ── */}
        <Collapse style={{ marginBottom: 16 }} items={[{
          key: 'webhook',
          label: '其它通知渠道（Webhook）',
          children: (
            <>
              <Form.Item label="启用 Webhook" name="webhook_enabled" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item
                label={<span>Webhook URL<Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>POST JSON，可接 Server酱 / 企业微信 / Bark</Text></span>}
                name="webhook_url"
              >
                <Input placeholder="https://..." />
              </Form.Item>
              <Divider style={{ margin: '0 0 12px' }}>推送事件</Divider>
              <Space size="large" wrap>
                <Form.Item label="新上架" name="webhook_notify_new_listing" valuePropName="checked"><Switch /></Form.Item>
                <Form.Item label="补货" name="webhook_notify_restock" valuePropName="checked"><Switch /></Form.Item>
                <Form.Item label="下架/售罄" name="webhook_notify_delisted" valuePropName="checked"><Switch /></Form.Item>
                <Form.Item label="价格变动" name="webhook_notify_price_change" valuePropName="checked"><Switch /></Form.Item>
                <Form.Item label="购买成功" name="webhook_notify_purchased" valuePropName="checked"><Switch /></Form.Item>
              </Space>
            </>
          ),
        }]} />

        {/* ── Card 5: 版本信息 ── */}
        <Card size="small" title="版本信息" style={{ marginBottom: 16 }}>
          <Space size="large" wrap>
            <span><Text type="secondary">版本号：</Text><Text strong>{version?.version ?? '-'}</Text></span>
            <span><Text type="secondary">git_sha：</Text><Text code>{version?.git_sha ? version.git_sha.slice(0, 7) : '-'}</Text></span>
            <span><Text type="secondary">构建时间：</Text><Text>{version?.build_time || '-'}</Text></span>
          </Space>
        </Card>

        {/* ── Card 6: System ── */}
        <Collapse style={{ marginBottom: 16 }} items={[{
          key: 'system',
          label: '系统 / 日志 / 优选权重',
          children: (
            <>
              <Space size="large" wrap>
                <Form.Item label="事件历史保留 (天)" name="event_retention_days" tooltip="超期事件自动清理，0 = 永久保留">
                  <InputNumber min={0} max={3650} style={{ width: 140 }} />
                </Form.Item>
                <Form.Item label="时区" name="timezone">
                  <Select
                    style={{ width: 200 }}
                    options={['Asia/Shanghai', 'Asia/Hong_Kong', 'Asia/Tokyo', 'UTC', 'America/Los_Angeles'].map(t => ({ value: t, label: t }))}
                  />
                </Form.Item>
                <Form.Item label="日志级别" name="log_level" tooltip="排查问题时切到 DEBUG，可在「调试日志」页查看/下载">
                  <Select style={{ width: 140 }} options={[
                    { value: 'INFO', label: 'INFO（正常）' },
                    { value: 'DEBUG', label: 'DEBUG（详细）' },
                  ]} />
                </Form.Item>
              </Space>
              <Divider style={{ margin: '8px 0' }} plain>
                <Text type="secondary" style={{ fontSize: 12 }}>多候选优选权重（同价时按硬件打分高者优先）</Text>
              </Divider>
              <Space size="large" wrap>
                <Form.Item label="CPU 权重" name="score_w_cpu"><InputNumber min={0} step={0.5} style={{ width: 110 }} /></Form.Item>
                <Form.Item label="内存权重" name="score_w_mem"><InputNumber min={0} step={0.5} style={{ width: 110 }} /></Form.Item>
                <Form.Item label="硬盘权重" name="score_w_disk"><InputNumber min={0} step={0.1} style={{ width: 110 }} /></Form.Item>
                <Form.Item label="带宽权重" name="score_w_bw"><InputNumber min={0} step={0.5} style={{ width: 110 }} /></Form.Item>
                <Form.Item label="流量权重" name="score_w_traffic"><InputNumber min={0} step={0.01} style={{ width: 110 }} /></Form.Item>
              </Space>
            </>
          ),
        }]} />

        <Affix offsetBottom={0}>
          <div style={{ background: 'rgba(255,255,255,0.9)', backdropFilter: 'blur(4px)', padding: '12px 0', borderTop: '1px solid #f0f0f0' }}>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={saving}>
                保存设置
              </Button>
              <Button onClick={() => settings && form.setFieldsValue(settings)}>重置</Button>
            </Space>
          </div>
        </Affix>
      </Form>

      <Modal
        title="二次验证"
        open={_2faPending !== null}
        onCancel={() => { set2faPending(null); setLoggingIn(false) }}
        footer={[
          <Button key="cancel" onClick={() => { set2faPending(null); setLoggingIn(false) }}>
            取消
          </Button>,
          <Button key="submit" type="primary" loading={_2faSubmitting} onClick={submit2fa}>
            验证
          </Button>,
        ]}
        destroyOnClose
      >
        <div style={{ marginBottom: 12 }}>
          <Text type="secondary">
            Akile 要求二次验证，请输入你的验证器 App 中的 6 位动态码：
          </Text>
        </div>
        <Input
          placeholder="6 位验证码"
          value={_2faCode}
          onChange={e => set2faCode(e.target.value)}
          maxLength={6}
          autoComplete="one-time-code"
          style={{ width: 200 }}
          onPressEnter={submit2fa}
        />
      </Modal>

      <ChangePasswordCard />
    </div>
  )
}

function ChangePasswordCard() {
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)

  const onSubmit = async () => {
    try {
      const v = await form.validateFields()
      setSaving(true)
      await api.changePassword(v.old_password, v.new_password)
      message.success('密码已修改，下次登录请使用新密码')
      form.resetFields()
    } catch (e: any) {
      if (e?.errorFields) return
      message.error(e?.response?.data?.detail ?? '修改失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Collapse style={{ marginTop: 16, marginBottom: 80 }} items={[{
      key: 'password',
      label: '修改管理员密码',
      children: (
        <Form form={form} layout="vertical" style={{ maxWidth: 360 }}>
          <Form.Item label="原密码" name="old_password" rules={[{ required: true, message: '请输入原密码' }]}>
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Form.Item
            label="新密码"
            name="new_password"
            rules={[{ required: true, message: '请输入新密码' }, { min: 6, message: '至少 6 位' }]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item
            label="确认新密码"
            name="confirm"
            dependencies={['new_password']}
            rules={[
              { required: true, message: '请再次输入新密码' },
              ({ getFieldValue }) => ({
                validator: (_, value) =>
                  !value || getFieldValue('new_password') === value
                    ? Promise.resolve()
                    : Promise.reject(new Error('两次输入的密码不一致')),
              }),
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Button type="primary" onClick={onSubmit} loading={saving}>修改密码</Button>
        </Form>
      ),
    }]} />
  )
}

function splitLines(v: string | undefined): string[] {
  return (v ?? '').split(/[\n,]+/).map(s => s.trim()).filter(Boolean)
}
