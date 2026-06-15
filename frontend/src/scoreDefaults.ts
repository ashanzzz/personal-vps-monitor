import type { ScoreDimension, ScoreRules } from './api'

// Mirror of backend scoring.DEFAULT_SCORE_RULES so the UI can render the editor
// and fall back gracefully when a page's score rules are missing.
export function defaultScoreRules(): ScoreRules {
  return {
    cpu: 50,
    memory: 20,
    disk: 1,
    bandwidth: 0,
    traffic: 0,
    price_monthly: -1,
    sale_price: -1,
    latency: -1,
  }
}

// Display order + labels for the scoring dimensions, with the unit each weight
// applies to. Negative-by-default dimensions (price/latency) noted in hints.
export const SCORE_DIMENSION_META: {
  key: ScoreDimension
  label: string
  unit: string
  step: number
  hint?: string
}[] = [
  { key: 'cpu',           label: 'CPU',   unit: '每核',     step: 1 },
  { key: 'memory',        label: '内存',  unit: '每 GB',    step: 1 },
  { key: 'disk',          label: '硬盘',  unit: '每 GB',    step: 0.5 },
  { key: 'bandwidth',     label: '带宽',  unit: '每 Mbps',  step: 0.1 },
  { key: 'traffic',       label: '流量',  unit: '每 GB',    step: 0.01 },
  { key: 'price_monthly', label: '月付价', unit: '每 ¥/月',  step: 0.5, hint: '一般填负值，越便宜评分越高' },
  { key: 'sale_price',    label: '售价',  unit: '每 ¥',     step: 0.5, hint: '二手售价，一般填负值；无数据时跳过' },
  { key: 'latency',       label: '延迟',  unit: '每 ms',    step: 0.5, hint: '一般填负值；无延迟数据时跳过该项' },
]
