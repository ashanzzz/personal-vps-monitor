import { useEffect, useState } from 'react'
import {
  Card, Form, Input, InputNumber, Switch, Select, Button,
  Divider, message, Typography, Slider,
} from 'antd'
import { SaveOutlined, SendOutlined } from '@ant-design/icons'
import type { AppSettings } from '../api'
import { api } from '../api'

const { Text } = Typography

const LS_REGION_OPTIONS = ['HK', 'JP', 'SG', 'TW', 'US', 'DE', 'MO', 'KR'].map(r => ({ value: r, label: r }))

interface Props {
  settings: AppSettings | null
  onSaved: (s: AppSettings) => void
}

export function SettingsPage({ settings, onSaved }: Props) {
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const [testingTg, setTestingTg] = useState(false)

  // Sync when settings load
  useEffect(() => {
    if (settings) {
      form.setFieldsValue({
        ...settings,
        watched_plans: (settings.watched_plans ?? []).join('\n'),
        auto_buy_plans: (settings.auto_buy_plans ?? []).join('\n'),
      })
    }
  }, [settings, form])

  const handleSave = async () => {
    try {
      const vals = await form.validateFields()
      setSaving(true)
      const payload = {
        ...vals,
        watched_plans: (vals.watched_plans ?? '')
          .split(/[\n,]+/)
          .map((s: string) => s.trim())
          .filter(Boolean),
        auto_buy_plans: (vals.auto_buy_plans ?? '')
          .split(/[\n,]+/)
          .map((s: string) => s.trim())
          .filter(Boolean),
      }
      const saved = await api.saveSettings(payload)
      onSaved(saved)
      message.success('设置已保存')
    } catch (e: any) {
      message.error(e?.response?.data?.detail ?? '保存失败')
    } finally {
      setSaving(false)
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
    <div style={{ maxWidth: 720, margin: '0 auto' }}>
      <Form form={form} layout="vertical" onFinish={handleSave}>
        {/* API */}
        <Card size="small" title="API 设置" style={{ marginBottom: 16 }}>
          <Form.Item label="Akile Token" name="akile_token" rules={[{ required: true, message: '必填' }]}>
            <Input.Password placeholder="your-token" autoComplete="off" />
          </Form.Item>

          <Form.Item
            label={
              <span>
                刷新间隔 (秒)
                <Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>
                  建议 30–300 秒，太短容易触发限频
                </Text>
              </span>
            }
            name="refresh_interval"
          >
            <Slider min={15} max={600} step={5} marks={{ 30: '30s', 60: '1min', 300: '5min', 600: '10min' }} />
          </Form.Item>
        </Card>

        {/* Monitor scope */}
        <Card size="small" title="监控范围" style={{ marginBottom: 16 }}>
          <Form.Item label="启用新机VPS监控" name="vps_enabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label="启用轻量机监控" name="ls_enabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label="轻量机监控区域" name="ls_regions">
            <Select mode="multiple" options={LS_REGION_OPTIONS} placeholder="默认全部" />
          </Form.Item>
          <Form.Item label="启用二手机监控" name="secondhand_enabled" valuePropName="checked">
            <Switch />
          </Form.Item>

          <Divider style={{ margin: '12px 0' }} />

          <Form.Item
            label={
              <span>
                关注的套餐 ID
                <Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>
                  每行一个 plan_id，触发预警时会高亮显示
                </Text>
              </span>
            }
            name="watched_plans"
          >
            <Input.TextArea rows={4} placeholder="plan_id 每行一个" />
          </Form.Item>
        </Card>

        {/* Telegram */}
        <Card size="small" title="Telegram 通知" style={{ marginBottom: 16 }}>
          <Form.Item label="Bot Token" name="tg_bot_token">
            <Input.Password placeholder="123456:ABC..." autoComplete="off" />
          </Form.Item>
          <Form.Item label="Chat ID" name="tg_chat_id">
            <Input placeholder="-1001234567890 或个人 ID" />
          </Form.Item>
          <Button
            icon={<SendOutlined />}
            loading={testingTg}
            onClick={testTelegram}
            style={{ marginBottom: 16 }}
          >
            测试发送
          </Button>
          <Divider style={{ margin: '0 0 12px' }} />
          <Form.Item label="新上架" name="tg_notify_new_listing" valuePropName="checked"><Switch /></Form.Item>
          <Form.Item label="补货" name="tg_notify_restock" valuePropName="checked"><Switch /></Form.Item>
          <Form.Item label="下架/售罄" name="tg_notify_delisted" valuePropName="checked"><Switch /></Form.Item>
          <Form.Item label="价格变动" name="tg_notify_price_change" valuePropName="checked"><Switch /></Form.Item>
          <Form.Item label="购买成功" name="tg_notify_purchased" valuePropName="checked"><Switch /></Form.Item>
        </Card>

        {/* Auto-buy */}
        <Card size="small" title="自动抢购（实验性）" style={{ marginBottom: 16 }}>
          <Form.Item label="启用自动购买" name="auto_buy_enabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item
            label={
              <span>
                自动购买套餐 ID
                <Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>每行一个 plan_id</Text>
              </span>
            }
            name="auto_buy_plans"
          >
            <Input.TextArea rows={3} placeholder="只有二手机支持自动购买" />
          </Form.Item>
          <Form.Item label="最高可接受价格 (分)" name="auto_buy_max_price">
            <InputNumber min={0} step={100} addonAfter="分" style={{ width: 180 }} />
          </Form.Item>
          <Form.Item label="购买周期 (月)" name="auto_buy_cycle">
            <Select options={[1,3,6,12].map(v => ({ value: v, label: `${v} 个月` }))} style={{ width: 140 }} />
          </Form.Item>
        </Card>

        <Form.Item>
          <Button
            type="primary"
            htmlType="submit"
            icon={<SaveOutlined />}
            loading={saving}
            block
          >
            保存设置
          </Button>
        </Form.Item>
      </Form>
    </div>
  )
}
