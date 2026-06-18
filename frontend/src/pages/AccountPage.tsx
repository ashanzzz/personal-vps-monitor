import { useCallback, useEffect, useState } from 'react'
import {
  Card, Statistic, Row, Col, Button, Space, Tag, Empty, Table, Spin,
  Descriptions, Alert, Typography, App as AntApp, Switch, Modal, TimePicker,
} from 'antd'
import {
  ReloadOutlined, WalletOutlined, CloudServerOutlined, CheckCircleTwoTone,
  CloseCircleTwoTone, GoldOutlined, SettingOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { api, fmtPrice } from '../api'
import type { AccountInfo, MyServer, AppSettings } from '../api'

const { Text } = Typography

// Render a value that may be string/number/bool into something printable.
function show(v: unknown): string {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'boolean') return v ? '是' : '否'
  return String(v)
}

// Human "N ago" for the cache timestamp. Input is ISO8601 UTC.
function timeAgo(iso: string | null | undefined): string {
  if (!iso) return '从未'
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return '未知'
  const sec = Math.max(0, Math.round((Date.now() - then) / 1000))
  if (sec < 60) return `${sec} 秒前`
  const min = Math.round(sec / 60)
  if (min < 60) return `${min} 分钟前`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr} 小时前`
  return `${Math.round(hr / 24)} 天前`
}

// Pick the first present key from a server object (Akile field names vary).
function pick(s: MyServer, keys: string[]): unknown {
  for (const k of keys) if (s[k] !== undefined && s[k] !== null && s[k] !== '') return s[k]
  return undefined
}

// Whether last_checkin_time (unix seconds) falls on the local calendar today.
function checkedInToday(ts: number | null | undefined): boolean {
  if (!ts) return false
  const then = new Date(ts * 1000)
  if (Number.isNaN(then.getTime())) return false
  const now = new Date()
  return then.getFullYear() === now.getFullYear()
    && then.getMonth() === now.getMonth()
    && then.getDate() === now.getDate()
}

export function AccountPage() {
  const { message } = AntApp.useApp()
  const [data, setData] = useState<AccountInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [checkingIn, setCheckingIn] = useState(false)
  const [settings, setSettings] = useState<AppSettings | null>(null)
  const [checkinModalOpen, setCheckinModalOpen] = useState(false)
  const [checkinStart, setCheckinStart] = useState<dayjs.Dayjs | null>(null)
  const [checkinEnd, setCheckinEnd] = useState<dayjs.Dayjs | null>(null)
  const [savingCheckin, setSavingCheckin] = useState(false)

  // Load settings for auto-checkin controls
  const loadSettings = useCallback(async () => {
    try {
      const s = await api.getSettings()
      setSettings(s)
      setCheckinStart(s.checkin_time_start ? dayjs(s.checkin_time_start, 'HH:mm') : dayjs('08:00', 'HH:mm'))
      setCheckinEnd(s.checkin_time_end ? dayjs(s.checkin_time_end, 'HH:mm') : dayjs('09:00', 'HH:mm'))
    } catch { /* ignore */ }
  }, [])

  // Initial load reads the cached snapshot — instant, no upstream call.
  const load = useCallback(async () => {
    setLoading(true)
    try {
      setData(await api.getAccount())
    } catch {
      message.error('加载账户信息失败')
    } finally {
      setLoading(false)
    }
  }, [message])

  // Manual refresh forces a live fetch and updates the cache.
  const refresh = useCallback(async () => {
    setRefreshing(true)
    try {
      setData(await api.refreshAccount())
      message.success('已刷新账户数据')
    } catch {
      message.error('刷新账户数据失败')
    } finally {
      setRefreshing(false)
    }
  }, [message])

  // Manual daily checkin. On success, refresh the cache so the AK coin
  // balance and last_checkin_time reflect the new state.
  const checkin = useCallback(async () => {
    setCheckingIn(true)
    try {
      const res = await api.checkin()
      const msg = res.status_msg || '签到完成'
      if (res.status_code === 0) {
        if (res.ak_coin_earned) {
          message.success(`✅ 签到成功！获得 ${res.ak_coin_earned} AK 币`)
        } else {
          message.success(msg)
        }
      } else {
        message.info(msg)
      }
      // Pull fresh account data so AK coin / last_checkin_time update.
      try { setData(await api.refreshAccount()) } catch { /* keep current */ }
    } catch {
      message.error('签到失败')
    } finally {
      setCheckingIn(false)
    }
  }, [message])

  useEffect(() => { load(); loadSettings() }, [load, loadSettings])

  // Toggle auto-checkin
  const toggleAutoCheckin = useCallback(async (checked: boolean) => {
    try {
      const updated = await api.saveSettings({ auto_checkin_enabled: checked })
      setSettings(updated)
      message.success(checked ? '自动签到已开启' : '自动签到已关闭')
    } catch {
      message.error('保存失败')
    }
  }, [message])

  // Save checkin time range
  const saveCheckinTime = useCallback(async () => {
    if (!checkinStart || !checkinEnd) return
    setSavingCheckin(true)
    try {
      const start = checkinStart.format('HH:mm')
      const end = checkinEnd.format('HH:mm')
      const updated = await api.saveSettings({
        checkin_time_start: start,
        checkin_time_end: end,
      })
      setSettings(updated)
      setCheckinModalOpen(false)
      message.success(`签到时间已更新：${start} – ${end}`)
    } catch {
      message.error('保存失败')
    } finally {
      setSavingCheckin(false)
    }
  }, [checkinStart, checkinEnd, message])

  if (loading && !data) {
    return <Spin size="large" style={{ display: 'block', margin: '120px auto' }} />
  }

  const profile = data?.profile ?? null
  const servers = data?.servers ?? []

  // Build columns dynamically from the union of server keys, but prefer a
  // curated set of common Akile fields when present.
  const PREFERRED: { title: string; keys: string[] }[] = [
    { title: '名称', keys: ['name', 'hostname', 'title'] },
    { title: '区域', keys: ['area', 'region', 'node', 'location'] },
    { title: 'IP', keys: ['ip', 'ipv4', 'public_ip', 'main_ip'] },
    { title: '状态', keys: ['status', 'state'] },
    { title: '到期', keys: ['expired_at', 'expire_time', 'due_time', 'end_time'] },
  ]
  const columns: ColumnsType<MyServer> = servers.length
    ? PREFERRED.map(p => ({
        title: p.title,
        key: p.title,
        render: (_: unknown, s: MyServer) => show(pick(s, p.keys)),
      }))
    : []

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Space size="middle" align="center">
          <Text strong style={{ color: '#ccc', fontSize: 16 }}>我的账户</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>
            数据更新于 {timeAgo(data?.updated_at)}
          </Text>
        </Space>
        <Space wrap>
          <Button
            type="primary"
            icon={<GoldOutlined />}
            loading={checkingIn}
            disabled={checkedInToday(profile?.last_checkin_time)}
            onClick={checkin}
          >
            {checkedInToday(profile?.last_checkin_time) ? '今日已签到' : '签到'}
          </Button>
          <Button type="primary" ghost icon={<ReloadOutlined />} loading={refreshing} onClick={refresh}>强制刷新</Button>
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
            <Switch
              size="small"
              checked={settings?.auto_checkin_enabled ?? true}
              onChange={toggleAutoCheckin}
            />
            <Text type="secondary" style={{ fontSize: 12, whiteSpace: 'nowrap' }}>自动签到</Text>
          </span>
          <Button
            size="small"
            type="text"
            icon={<SettingOutlined />}
            onClick={() => setCheckinModalOpen(true)}
            style={{ color: '#888' }}
          />
        </Space>
      </div>

      {data && !data.configured && (
        <Alert type="warning" showIcon style={{ marginBottom: 16 }}
          message="尚未配置 Akile Token" description="请到「设置 → API 与连接」填入 token 后再查看账户信息。" />
      )}

      {data?.errors?.length ? (
        <Alert type="error" showIcon style={{ marginBottom: 16 }}
          message="部分账户数据获取失败" description={data.errors.join('; ')} />
      ) : null}

      {/* Balance + AK coin + profile */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col xs={24} sm={8}>
          <Card size="small">
            <Statistic
              title={<Space><WalletOutlined />账户余额</Space>}
              value={profile?.money != null ? fmtPrice(profile.money) : '—'}
              valueStyle={{ color: '#52c41a' }}
            />
            {profile?.money != null && (
              <Text type="secondary" style={{ fontSize: 12 }}>{profile.money} 分</Text>
            )}
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card size="small">
            <Statistic
              title={<Space><GoldOutlined />AK 币</Space>}
              value={profile?.ak_coin != null ? profile.ak_coin : '—'}
              suffix={profile?.ak_coin != null ? 'AK 币' : undefined}
              valueStyle={{ color: '#faad14' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card size="small" title="账户信息">
            {profile ? (
              <Descriptions size="small" column={{ xs: 1, sm: 2 }}>
                <Descriptions.Item label="用户名">{show(profile.username)}</Descriptions.Item>
                <Descriptions.Item label="邮箱">{show(profile.email)}</Descriptions.Item>
                <Descriptions.Item label="角色">{show(profile.role)}</Descriptions.Item>
                <Descriptions.Item label="Telegram">{show(profile.telegram_id)}</Descriptions.Item>
                <Descriptions.Item label="实名">
                  {profile.is_real_name
                    ? <span><CheckCircleTwoTone twoToneColor="#52c41a" /> 已实名</span>
                    : <span><CloseCircleTwoTone twoToneColor="#faad14" /> 未实名</span>}
                </Descriptions.Item>
                <Descriptions.Item label="二次验证">
                  {profile.is_ga ? <Tag color="green">已开启</Tag> : <Tag>未开启</Tag>}
                </Descriptions.Item>
              </Descriptions>
            ) : <Empty description="无账户信息" />}
          </Card>
        </Col>
      </Row>

      {/* My servers */}
      <Card size="small" title={<Space><CloudServerOutlined />我的服务器 ({servers.length})</Space>}>
        {servers.length === 0 ? (
          <Empty description="当前没有服务器" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        ) : (
          <Table<MyServer>
            rowKey={(s, i) => String(pick(s, ['id', 'server_id', 'uuid']) ?? i)}
            size="small"
            columns={columns}
            dataSource={servers}
            pagination={false}
            expandable={{
              expandedRowRender: s => (
                <pre style={{ margin: 0, fontSize: 12, color: '#aaa', whiteSpace: 'pre-wrap' }}>
                  {JSON.stringify(s, null, 2)}
                </pre>
              ),
            }}
          />
        )}
      </Card>

      {/* Auto-checkin time range modal */}
      <Modal
        title="自动签到设置"
        open={checkinModalOpen}
        onCancel={() => setCheckinModalOpen(false)}
        onOk={saveCheckinTime}
        confirmLoading={savingCheckin}
        okText="保存"
        cancelText="取消"
      >
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <div>
            <Text type="secondary" style={{ fontSize: 12 }}>
              每天在设定的时间范围内随机选择一个时刻自动签到领取 AK 币，避免固定时间被识别。
            </Text>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <Text>开始时间</Text>
            <TimePicker
              format="HH:mm"
              minuteStep={5}
              value={checkinStart}
              onChange={setCheckinStart}
              style={{ width: 140 }}
            />
            <Text>结束时间</Text>
            <TimePicker
              format="HH:mm"
              minuteStep={5}
              value={checkinEnd}
              onChange={setCheckinEnd}
              style={{ width: 140 }}
            />
          </div>
          <Text type="secondary" style={{ fontSize: 12 }}>
            💡 例如设置 08:00 – 09:00，系统每天会在 08:00 到 09:00 之间随机选一个时间点自动签到。
          </Text>
        </Space>
      </Modal>
    </div>
  )
}
