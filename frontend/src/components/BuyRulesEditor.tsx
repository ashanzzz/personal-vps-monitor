import { useState } from 'react'
import {
  Card, Switch, Slider, InputNumber, Button, Space, Select, Input,
  Collapse, Tag, Tooltip, Divider, Typography, Popconfirm, App as AntApp,
} from 'antd'
import { PlusOutlined, DeleteOutlined } from '@ant-design/icons'
import type { PageBuyConfig, BuyRule, SpecDimension } from '../api'
import { ruleDefaults } from '../buyDefaults'

const { Text } = Typography

// Spec dimensions rendered as min-max range inputs. memory is in MB.
const SPEC_META: { key: SpecDimension; label: string; addon?: string }[] = [
  { key: 'cpu', label: 'CPU 核' },
  { key: 'memory', label: '内存', addon: 'MB' },
  { key: 'disk', label: '硬盘', addon: 'GB' },
  { key: 'bandwidth', label: '带宽', addon: 'M' },
  { key: 'traffic', label: '流量', addon: 'GB' },
]

interface Props {
  pageLabel: string
  value: PageBuyConfig
  onChange: (next: PageBuyConfig) => void
}

// Refresh interval slider: 60–6000s, plus a custom numeric input that allows
// down to 5s for power users (with a warning). Each rule is an independent
// filter + per-rule count cap; the page's scoring rules order candidates.
export function BuyRulesEditor({ pageLabel, value, onChange }: Props) {
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
        <Text type="secondary" style={{ fontSize: 12 }}>暂无规则。点「新增规则」设置条件，刷到符合的机器会按评分从高到低自动抢购。</Text>
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
                {r.max_buy === 0
                  ? <Tag color="gold">不限量</Tag>
                  : <Tag color="green">限{r.max_buy}台</Tag>}
              </Space>
            ),
            extra: (
              <Popconfirm title="删除此规则?" onConfirm={() => removeRule(idx)}>
                <DeleteOutlined onClick={e => e.stopPropagation()} style={{ color: '#c66' }} />
              </Popconfirm>
            ),
            children: <RuleFields rule={r} onPatch={p => patchRule(idx, p)} />,
          }))}
        />
      )}
    </Card>
  )
}

function RuleFields({ rule, onPatch }: {
  rule: BuyRule; onPatch: (p: Partial<BuyRule>) => void
}) {
  const lbl = { color: '#999', fontSize: 12, display: 'block', marginBottom: 2 }

  const patchSpec = (dim: SpecDimension, side: 'min' | 'max', v: number) =>
    onPatch({ spec: { ...rule.spec, [dim]: { ...rule.spec[dim], [side]: v } } })

  return (
    <Space direction="vertical" style={{ width: '100%' }} size={8}>
      <Space wrap size="middle">
        <span>
          <span style={lbl}>标签名(显示用,如 jppro)</span>
          <Input value={rule.label} style={{ width: 130 }} placeholder="可选"
            onChange={e => onPatch({ label: e.target.value })} />
        </span>
        <span>
          <span style={lbl}>目标网站/平台</span>
          <Select value={rule.platform || 'all'} style={{ width: 120 }}
            onChange={v => onPatch({ platform: v })}
            options={[
              { value: 'all', label: '全部平台' },
              { value: 'akile', label: 'Akile Push' },
              { value: 'acck', label: 'ACCK' },
            ]} />
        </span>
        <span>
          <span style={lbl}>本规则上限(0=不限)</span>
          <InputNumber min={0} value={rule.max_buy} style={{ width: 100 }}
            onChange={v => onPatch({ max_buy: Number(v) || 0 })} />
        </span>
      </Space>

      <Space wrap size="middle" style={{ width: '100%' }}>
        <span style={{ flex: 1, minWidth: 260 }}>
          <span style={lbl}>标签包含(匹配名称/节点,任一命中即可;留空=不限)</span>
          <Select mode="tags" value={rule.tags_include} style={{ width: '100%' }}
            placeholder="如 元旦快乐、HKLite-One,回车添加"
            tokenSeparators={[',', ' ']}
            open={false}
            onChange={v => onPatch({ tags_include: v })} />
        </span>
        <span style={{ width: 180 }}>
          <span style={lbl}>API 快速检索前缀 (可选)</span>
          <Input value={rule.search_prefix || ''} placeholder="如 HKLite-One"
            onChange={e => onPatch({ search_prefix: e.target.value })} />
        </span>
      </Space>

      <Space wrap size="middle">
        {SPEC_META.map(s => (
          <span key={s.key}>
            <span style={lbl}>{s.label}{s.addon ? `(${s.addon})` : ''} min-max</span>
            <Space.Compact>
              <InputNumber min={0} value={rule.spec[s.key].min} style={{ width: 80 }} placeholder="min"
                onChange={v => patchSpec(s.key, 'min', Number(v) || 0)} />
              <InputNumber min={0} value={rule.spec[s.key].max} style={{ width: 80 }} placeholder="max"
                onChange={v => patchSpec(s.key, 'max', Number(v) || 0)} />
            </Space.Compact>
          </span>
        ))}
      </Space>

      <Space wrap size="middle">
        <span>
          <span style={lbl}>续费周期</span>
          <Select value={rule.price_type} style={{ width: 100 }}
            onChange={v => onPatch({ price_type: v })}
            options={[
              { value: 'monthly', label: '月付' },
              { value: 'yearly', label: '年付' },
            ]} />
        </span>
        <span>
          <span style={lbl}>续费价 ≤ 元({rule.price_type === 'yearly' ? '年' : '月'})</span>
          <InputNumber min={0} value={rule.price_max / 100} style={{ width: 110 }}
            onChange={v => onPatch({ price_max: Math.round((Number(v) || 0) * 100) })} />
        </span>
        <span>
          <span style={lbl}>售价 ≤ 元</span>
          <InputNumber min={0} value={rule.sale_price_max / 100} style={{ width: 110 }}
            onChange={v => onPatch({ sale_price_max: Math.round((Number(v) || 0) * 100) })} />
        </span>
      </Space>

      <Tooltip title="所有条件留空/0 = 不限制。续费价按所选周期(月付/年付)比较;售价独立。命中多台时按本页评分规则从高到低抢,达本规则上限即停。">
        <Text type="secondary" style={{ fontSize: 11 }}>提示：所有条件留空/0 = 不限制。命中多台时按评分从高到低抢，评分规则在上方「评分规则」配置。</Text>
      </Tooltip>
    </Space>
  )
}

