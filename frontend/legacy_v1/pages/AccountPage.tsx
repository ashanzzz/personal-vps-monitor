import { useCallback, useEffect, useState } from 'react'
import {
  Card, Statistic, Row, Col, Button, Space, Tag, Empty, Table, Spin,
  Descriptions, Alert, Typography, App as AntApp, Switch, Modal, TimePicker,
  Radio, Badge,
} from 'antd'
import {
  ReloadOutlined, WalletOutlined, CloudServerOutlined, CheckCircleTwoTone,
  CloseCircleTwoTone, GoldOutlined, SettingOutlined, HistoryOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { api, fmtPrice } from '../api'
import type { AccountInfo, MyServer, AppSettings } from '../api'
import { show, timeAgo, pick, checkedInToday } from '../lib/format'

const { Text } = Typography





export function AccountPage() {
  const { message } = AntApp.useApp()
  const [data, setData] = useState<AccountInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [checkingIn, setCheckingIn] = useState(false)
  const [settings, setSettings] = useState<AppSettings | null>(null)
  const [checkinModalOpen, setCheckinModalOpen] = useState(false)
  const [checkinMode, setCheckinMode] = useState<'fixed' | 'random'>('fixed')
  const [checkinFixedTime, setCheckinFixedTime] = useState<dayjs.Dayjs | null>(dayjs('08:00', 'HH:mm'))
  const [checkinStart, setCheckinStart] = useState<dayjs.Dayjs | null>(null)
  const [checkinEnd, setCheckinEnd] = useState<dayjs.Dayjs | null>(null)
  const [savingCheckin, setSavingCheckin] = useState(false)
  const [checkinStatusData, setCheckinStatusData] = useState<any>(null)
  const [historyModalOpen, setHistoryModalOpen] = useState(false)

  // Load checkin status & history
  const loadCheckinStatus = useCallback(async () => {
    try {
      const res = await api.getCheckinStatus()
      setCheckinStatusData(res)
    } catch { /* ignore */ }
  }, [])

  // Load settings for auto-checkin controls
  const loadSettings = useCallback(async () => {
    try {
      const s = await api.getSettings()
      setSettings(s)
      setCheckinMode(s.checkin_mode || 'fixed')
      setCheckinFixedTime(s.checkin_fixed_time ? dayjs(s.checkin_fixed_time, 'HH:mm') : dayjs('08:00', 'HH:mm'))
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

  useEffect(() => { load(); loadSettings(); loadCheckinStatus() }, [load, loadSettings, loadCheckinStatus])

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

  // Save checkin time range / fixed time
  const saveCheckinTime = useCallback(async () => {
    setSavingCheckin(true)
    try {
      const payload: Partial<AppSettings> = {
        checkin_mode: checkinMode,
      }
      if (checkinMode === 'fixed') {
        if (!checkinFixedTime) return
        const fixed = checkinFixedTime.format('HH:mm')
        payload.checkin_fixed_time = fixed
      } else {
        if (!checkinStart || !checkinEnd) return
        const start = checkinStart.format('HH:mm')
        const end = checkinEnd.format('HH:mm')
        payload.checkin_time_start = start
        payload.checkin_time_end = end
      }
      const updated = await api.saveSettings(payload)
      setSettings(updated)
      setCheckinModalOpen(false)
      loadCheckinStatus()
      message.success('签到配置已更新')
    } catch {
      message.error('保存失败')
    } finally {
      setSavingCheckin(false)
    }
  }, [checkinMode, checkinFixedTime, checkinStart, checkinEnd, loadCheckinStatus, message])

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
          <Button
            icon={<HistoryOutlined />}
            onClick={() => { loadCheckinStatus(); setHistoryModalOpen(true) }}
          >
            打卡状态
            {checkinStatusData?.status?.akile?.checked_in ? (
              <Badge status="success" style={{ marginLeft: 6 }} />
            ) : (
              <Badge status="warning" style={{ marginLeft: 6 }} />
            )}
          </Button>
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
        title="每日自动签到设置"
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
              设置每天自动打卡的时间规则。支持每天固定时刻打卡，或在时间窗口内随机打卡以模拟真实访问。
            </Text>
          </div>

          <div>
            <Text strong style={{ marginRight: 12 }}>打卡模式：</Text>
            <Radio.Group
              value={checkinMode}
              onChange={e => setCheckinMode(e.target.value)}
              buttonStyle="solid"
            >
              <Radio.Button value="fixed">固定时间打卡</Radio.Button>
              <Radio.Button value="random">随机时间窗口</Radio.Button>
            </Radio.Group>
          </div>

          {checkinMode === 'fixed' ? (
            <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
              <Text>打卡时间：</Text>
              <TimePicker
                format="HH:mm"
                minuteStep={1}
                value={checkinFixedTime}
                onChange={setCheckinFixedTime}
                style={{ width: 140 }}
              />
              <Text type="secondary" style={{ fontSize: 12 }}>
                每天将在该时刻准时打卡
              </Text>
            </div>
          ) : (
            <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
              <Text>开始时间：</Text>
              <TimePicker
                format="HH:mm"
                minuteStep={5}
                value={checkinStart}
                onChange={setCheckinStart}
                style={{ width: 120 }}
              />
              <Text>结束时间：</Text>
              <TimePicker
                format="HH:mm"
                minuteStep={5}
                value={checkinEnd}
                onChange={setCheckinEnd}
                style={{ width: 120 }}
              />
            </div>
          )}

          <Text type="secondary" style={{ fontSize: 12 }}>
            💡 提示：该配置将同时作用于 AkileCloud 与 ACCK 平台。后台每天会自动执行签到并记录打卡状态。
          </Text>
        </Space>
      </Modal>

      {/* Checkin Status & History Modal */}
      <Modal
        title={<Space><HistoryOutlined />双平台打卡状态与历史记录</Space>}
        open={historyModalOpen}
        onCancel={() => setHistoryModalOpen(false)}
        footer={[
          <Button key="close" onClick={() => setHistoryModalOpen(false)}>关闭</Button>,
          <Button key="refresh" icon={<ReloadOutlined />} onClick={loadCheckinStatus}>刷新状态</Button>,
        ]}
        width={720}
      >
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <Row gutter={16}>
            <Col span={12}>
              <Card size="small" title="AkileCloud 今日打卡">
                <Space orientation="vertical" size={4}>
                  <div>
                    <Text strong>今日状态：</Text>
                    {checkinStatusData?.status?.akile?.checked_in ? (
                      <Tag color="success">已打卡完成</Tag>
                    ) : (
                      <Tag color="warning">今日未打卡 / 待执行</Tag>
                    )}
                  </div>
                  <div>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {checkinStatusData?.status?.akile?.last_log?.message || '今日尚无打卡日志'}
                    </Text>
                  </div>
                </Space>
              </Card>
            </Col>
            <Col span={12}>
              <Card size="small" title="ACCK 今日打卡">
                <Space orientation="vertical" size={4}>
                  <div>
                    <Text strong>今日状态：</Text>
                    {checkinStatusData?.status?.acck?.checked_in ? (
                      <Tag color="success">已打卡完成</Tag>
                    ) : (
                      <Tag color="warning">今日未打卡 / 待执行</Tag>
                    )}
                  </div>
                  <div>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {checkinStatusData?.status?.acck?.last_log?.message || '今日尚无打卡日志'}
                    </Text>
                  </div>
                </Space>
              </Card>
            </Col>
          </Row>

          <div>
            <Text strong>最近打卡执行日志</Text>
            <Table
              size="small"
              rowKey="id"
              dataSource={checkinStatusData?.history || []}
              pagination={{ pageSize: 5 }}
              columns={[
                {
                  title: '平台',
                  dataIndex: 'platform',
                  key: 'platform',
                  render: (p: string) => <Tag color={p === 'akile' ? 'blue' : 'purple'}>{p.toUpperCase()}</Tag>,
                },
                { title: '日期', dataIndex: 'checkin_date', key: 'checkin_date', width: 110 },
                {
                  title: '状态',
                  dataIndex: 'status',
                  key: 'status',
                  width: 100,
                  render: (s: string) => {
                    if (s === 'success') return <Tag color="success">打卡成功</Tag>
                    if (s === 'already_signed') return <Tag color="processing">已签过</Tag>
                    return <Tag color="error">失败</Tag>
                  },
                },
                { title: '说明', dataIndex: 'message', key: 'message', ellipsis: true },
                {
                  title: '执行时间',
                  dataIndex: 'created_at',
                  key: 'created_at',
                  width: 160,
                  render: (t: string) => t ? dayjs(t).format('YYYY-MM-DD HH:mm:ss') : '—',
                },
              ]}
            />
          </div>
        </Space>
      </Modal>
    </div>
  )
}
