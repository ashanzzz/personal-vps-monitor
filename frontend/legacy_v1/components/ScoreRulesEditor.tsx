import {
  Card, InputNumber, Button, Space, Tooltip, Typography, Tag,
} from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import type { ScoreDimension, ScoreRules } from '../api'
import { SCORE_DIMENSION_META, defaultScoreRules } from '../scoreDefaults'

const { Text } = Typography

interface Props {
  pageLabel: string
  value: ScoreRules
  onChange: (next: ScoreRules) => void
}

// Per-page hardware scoring weights editor. Each list page (云/轻量/二手) keeps
// its own {dimension -> weight} map; the backend computes score = Σ(value×weight)
// and the "评分" column renders + sorts by it.
export function ScoreRulesEditor({ pageLabel, value, onChange }: Props) {
  const rules: ScoreRules = { ...defaultScoreRules(), ...(value ?? {}) }

  const patch = (key: ScoreDimension, weight: number) =>
    onChange({ ...rules, [key]: weight })

  const resetDefaults = () => onChange(defaultScoreRules())

  return (
    <Card
      size="small"
      title={
        <Space>
          <span>{pageLabel} · 评分规则</span>
          <Tooltip title="评分 = Σ(各维度数值 × 权重)。正权重越高越好；价格/延迟一般用负权重。延迟无数据时跳过。">
            <Tag color="blue">加权求和</Tag>
          </Tooltip>
        </Space>
      }
      extra={
        <Tooltip title="恢复默认权重">
          <Button size="small" icon={<ReloadOutlined />} onClick={resetDefaults}>恢复默认</Button>
        </Tooltip>
      }
      style={{ marginBottom: 16 }}
    >
      <Space wrap size="large">
        {SCORE_DIMENSION_META.map(meta => (
          <span key={meta.key}>
            <span style={{ color: '#999', fontSize: 12, display: 'block', marginBottom: 2 }}>
              {meta.label}
              <Text type="secondary" style={{ fontSize: 11, marginLeft: 4 }}>({meta.unit})</Text>
            </span>
            <Tooltip title={meta.hint}>
              <InputNumber
                value={rules[meta.key]}
                step={meta.step}
                style={{ width: 120 }}
                onChange={v => patch(meta.key, Number(v) || 0)}
              />
            </Tooltip>
          </span>
        ))}
      </Space>
    </Card>
  )
}
