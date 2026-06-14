import { useState } from 'react'
import {
  Card, Switch, Slider, InputNumber, Button, Space, Select, Input,
  Collapse, Tag, Tooltip, Divider, Typography, Popconfirm, App as AntApp,
} from 'antd'
import { PlusOutlined, DeleteOutlined } from '@ant-design/icons'
import type { PageBuyConfig, BuyRule } from '../api'
import { ruleDefaults } from '../buyDefaults'

const { Text } = Typography

interface Props {
  pageLabel: string
  value: PageBuyConfig
  areas: string[]
  onChange: (next: PageBuyConfig) => void
}

// Refresh interval slider: 60–6000s, plus a custom numeric input that allows
// down to 5s for power users (with a warning).
export function BuyRulesEditor({ pageLabel, value, areas, onChange }: Props) {
  const { message } = AntApp.useApp()
  const [custom, setCustom] = useState(false)

  const patch = (p: Partial<PageBuyConfig>) => onChange({ ...value, ...p })

  const patchRule = (idx: number, p: Partial<BuyRule>) => {
    const rules = value.rules.map((r, i) => (i === idx ? { ...r, ...p } : r))
    patch({ rules })
  }

  const addRule = () => {
    const r = ruleDefaults()
    r.id = `rule_${Date.now()}_${Math.floor(Math.random() * 1000)}`
    patch({ rules: [...value.rules, r] })
  }

  const removeRule = (idx: number) => {
    patch({ rules: value.rules.filter((_, i) => i !== idx) })
  }

  return (
    <Card
      size="small"
      title={
        <Space>
          <span>{pageLabel} · 抢购设置</span>
          <Switch
            checkedChildren="开"
            unCheckedChildren="关"
            checked={value.enabled}
            onChange={v => patch({ enabled: v })}
          />
        </Space>
      }
      style={{ marginBottom: 16 }}
    >
      {/* Refresh interval */}
      <div style={{ marginBottom: 12 }}>
        <Space align="center" style={{ marginBottom: 4 }}>
          <Text style={{ color: '#aaa' }}>刷新间隔</Text>
          <Tag color="blue">{value.refresh_interval}s</Tag>
          <Switch
            size="small"
            checkedChildren="自定义"
            unCheckedChildren="滑块"
            checked={custom}
            onChange={c => {
              setCustom(c)
              if (c && value.refresh_interval < 5) patch({ refresh_interval: 5 })
            }}
          />
        </Space>
        {custom ? (
          <InputNumber
            min={5}
            max={86400}
            value={value.refresh_interval}
            addonAfter="秒"
            style={{ width: 160 }}
            onChange={v => {
              const n = Number(v) || 60
              if (n < 60) message.warning('过快刷新可能触发风控，请谨慎')
              patch({ refresh_interval: Math.max(5, n) })
            }}
          />
        ) : (
          <Slider
            min={60}
            max={6000}
            step={10}
            value={value.refresh_interval}
            onChange={v => patch({ refresh_interval: v })}
          />
        )}
      </div>

      <Divider style={{ margin: '8px 0' }} />

      {/* Page-level quota gates */}
      <Space wrap size="large" style={{ marginBottom: 8 }}>
        <span>
          <Text style={{ color: '#aaa', marginRight: 6 }}>单轮合计上限</Text>
          <InputNumber min={0} value={value.max_total_per_run} style={{ width: 80 }}
            onChange={v => patch({ max_total_per_run: Number(v) || 0 })} />
        </span>
        <span>
          <Text style={{ color: '#aaa', marginRight: 6 }}>每日合计上限</Text>
          <InputNumber min={0} value={value.max_total_daily} style={{ width: 80 }}
            onChange={v => patch({ max_total_daily: Number(v) || 0 })} />
        </span>
        <span>
          <Text style={{ color: '#aaa', marginRight: 6 }}>页价格上限(元/月)</Text>
          <InputNumber min={0} value={value.max_price / 100} style={{ width: 90 }}
            onChange={v => patch({ max_price: Math.round((Number(v) || 0) * 100) })} />
        </span>
        <span>
          <Text style={{ color: '#aaa', marginRight: 6 }}>周期</Text>
          <Select value={value.cycle} style={{ width: 90 }}
            onChange={v => patch({ cycle: v })}
            options={[{ value: 1, label: '月付' }, { value: 12, label: '年付' }]} />
        </span>
        <span>
          <Text style={{ color: '#aaa', marginRight: 6 }}>达上限</Text>
          <Select value={value.on_limit_reached} style={{ width: 120 }}
            onChange={v => patch({ on_limit_reached: v })}
            options={[
              { value: 'keep_watching', label: '持续补抢' },
              { value: 'stop', label: '停止' },
            ]} />
        </span>
      </Space>

      <Divider style={{ margin: '8px 0' }} />

      {/* Rules */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
        <Text strong style={{ color: '#ccc' }}>抢购规则 ({value.rules.length})</Text>
        <Button size="small" icon={<PlusOutlined />} onClick={addRule}>新增规则</Button>
      </div>

      {value.rules.length === 0 ? (
        <Text type="secondary" style={{ fontSize: 12 }}>暂无规则。点「新增规则」设置条件区间，刷到符合的机器会自动抢购。</Text>
      ) : (
        <Collapse
          size="small"
          items={value.rules.map((r, idx) => ({
            key: r.id || String(idx),
            label: (
              <Space>
                <Switch size="small" checked={r.enabled}
                  onClick={(_, e) => e.stopPropagation()}
                  onChange={v => patchRule(idx, { enabled: v })} />
                <Text style={{ color: '#ddd' }}>{r.label || `规则 ${idx + 1}`}</Text>
                {r.max_per_rule === 0
                  ? <Tag color="gold">不限量</Tag>
                  : <Tag color="green">限{r.max_per_rule}台</Tag>}
              </Space>
            ),
            extra: (
              <Popconfirm title="删除此规则?" onConfirm={() => removeRule(idx)}>
                <DeleteOutlined onClick={e => e.stopPropagation()} style={{ color: '#c66' }} />
              </Popconfirm>
            ),
            children: <RuleFields rule={r} areas={areas} onPatch={p => patchRule(idx, p)} />,
          }))}
        />
      )}
    </Card>
  )
}

function RuleFields({ rule, areas, onPatch }: {
  rule: BuyRule; areas: string[]; onPatch: (p: Partial<BuyRule>) => void
}) {
  const lbl = { color: '#999', fontSize: 12, display: 'block', marginBottom: 2 }
  return (
    <Space direction="vertical" style={{ width: '100%' }} size={8}>
      <Space wrap size="middle">
        <span>
          <span style={lbl}>标签名(如 jppro)</span>
          <Input value={rule.label} style={{ width: 130 }} placeholder="可选"
            onChange={e => onPatch({ label: e.target.value })} />
        </span>
        <span>
          <span style={lbl}>标题关键词</span>
          <Input value={rule.name_keyword} style={{ width: 130 }} placeholder="可选"
            onChange={e => onPatch({ name_keyword: e.target.value })} />
        </span>
        <span>
          <span style={lbl}>本规则上限(0=不限)</span>
          <InputNumber min={0} value={rule.max_per_rule} style={{ width: 100 }}
            onChange={v => onPatch({ max_per_rule: Number(v) || 0 })} />
        </span>
      </Space>

      <Space wrap size="middle">
        <span>
          <span style={lbl}>CPU 核(min-max)</span>
          <Space.Compact>
            <InputNumber min={0} value={rule.cpu_min} style={{ width: 70 }} placeholder="min"
              onChange={v => onPatch({ cpu_min: Number(v) || 0 })} />
            <InputNumber min={0} value={rule.cpu_max} style={{ width: 70 }} placeholder="max"
              onChange={v => onPatch({ cpu_max: Number(v) || 0 })} />
          </Space.Compact>
        </span>
        <span>
          <span style={lbl}>内存 MB(min-max)</span>
          <Space.Compact>
            <InputNumber min={0} value={rule.memory_min_mb} style={{ width: 80 }} placeholder="min"
              onChange={v => onPatch({ memory_min_mb: Number(v) || 0 })} />
            <InputNumber min={0} value={rule.memory_max_mb} style={{ width: 80 }} placeholder="max"
              onChange={v => onPatch({ memory_max_mb: Number(v) || 0 })} />
          </Space.Compact>
        </span>
        <span>
          <span style={lbl}>硬盘 GB(min-max)</span>
          <Space.Compact>
            <InputNumber min={0} value={rule.disk_min_gb} style={{ width: 70 }} placeholder="min"
              onChange={v => onPatch({ disk_min_gb: Number(v) || 0 })} />
            <InputNumber min={0} value={rule.disk_max_gb} style={{ width: 70 }} placeholder="max"
              onChange={v => onPatch({ disk_max_gb: Number(v) || 0 })} />
          </Space.Compact>
        </span>
      </Space>

      <Space wrap size="middle">
        <span>
          <span style={lbl}>带宽</span>
          <Space.Compact>
            <Select value={rule.bandwidth_mode} style={{ width: 90 }}
              onChange={v => onPatch({ bandwidth_mode: v })}
              options={[
                { value: 'any', label: '不限' },
                { value: 'min', label: '≥' },
                { value: 'exact', label: '=' },
              ]} />
            <InputNumber min={0} value={rule.bandwidth_mbps} style={{ width: 100 }} addonAfter="M"
              disabled={rule.bandwidth_mode === 'any'}
              onChange={v => onPatch({ bandwidth_mbps: Number(v) || 0 })} />
          </Space.Compact>
        </span>
        <span>
          <span style={lbl}>流量 ≥ GB/月</span>
          <InputNumber min={0} value={rule.traffic_min_gb} style={{ width: 100 }}
            onChange={v => onPatch({ traffic_min_gb: Number(v) || 0 })} />
        </span>
        <span>
          <span style={lbl}>续费价 ≤ 元/月</span>
          <InputNumber min={0} value={rule.price_max_monthly / 100} style={{ width: 100 }}
            onChange={v => onPatch({ price_max_monthly: Math.round((Number(v) || 0) * 100) })} />
        </span>
      </Space>

      <span>
        <span style={lbl}>地区(留空=全部)</span>
        <Select mode="multiple" value={rule.areas} style={{ width: '100%', maxWidth: 420 }}
          placeholder="全部地区"
          onChange={v => onPatch({ areas: v })}
          options={areas.map(a => ({ value: a, label: a }))} />
      </span>
      <Tooltip title="续费价统一换算成月付比较；年付填年价会自动按 /12 比较。留空或 0 表示该项不限制。">
        <Text type="secondary" style={{ fontSize: 11 }}>提示：所有条件留空/0 = 不限制。命中多台时优先抢便宜的，同价抢硬件更强的。</Text>
      </Tooltip>
    </Space>
  )
}
