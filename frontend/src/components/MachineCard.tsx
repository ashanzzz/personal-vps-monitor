import { Tag, Popover, Button } from 'antd'
import {
  CheckCircleOutlined, CloseCircleOutlined,
  WifiOutlined, StopOutlined, ShoppingOutlined,
} from '@ant-design/icons'
import type { Machine, OfficialMatch } from '../api'
import { fmtPrice, fmtMemory, CAT_LABELS } from '../api'
import styles from './MachineCard.module.css'

interface Props {
  machine: Machine
  watched: boolean
  onWatch: (id: string, watched: boolean) => void
  onBuy?: (id: string) => void
  priceHistory?: { price: number; cycle: number }[]
  autoBuyEnabled?: boolean
}

export function MachineCard({ machine, watched, onWatch, onBuy, priceHistory, autoBuyEnabled }: Props) {
  const available = machine.available

  const cycleLabel = (c: number) => c === 1 ? '月付' : c === 12 ? '年付' : `${c}月`

  return (
    <div className={`${styles.card} ${available ? styles.available : styles.unavailable} ${watched ? styles.watched : ''}`}>
      <div className={styles.header}>
        <span className={styles.category}>{CAT_LABELS[machine.category] ?? machine.category}</span>
        <span className={styles.area}>{machine.area}{machine.node ? ` · ${machine.node}` : ''}</span>
        <span className={styles.status}>
          {machine.category === 'secondhand' && machine.ip_status === 'gfw' && (
            <Tag icon={<StopOutlined />} color="error">IP被墙</Tag>
          )}
          {machine.category === 'secondhand' && machine.ip_status === 'normal' && (
            <Tag icon={<WifiOutlined />} color="success">IP正常</Tag>
          )}
          {available
            ? <Tag icon={<CheckCircleOutlined />} color="success">有货{machine.stock > 0 ? ` ×${machine.stock}` : ''}</Tag>
            : <Tag icon={<CloseCircleOutlined />} color="error">无货</Tag>}
        </span>
      </div>

      <div className={styles.name} title={machine.name}>{machine.name}</div>

      <div className={styles.specs}>
        <span>{machine.cpu}核</span>
        <span>{fmtMemory(machine.memory_mb)}</span>
        <span>{machine.disk_gb}GB</span>
        <span>{machine.bandwidth_mbps >= 1000 ? `${machine.bandwidth_mbps / 1000}Gbps` : `${machine.bandwidth_mbps}Mbps`}</span>
        {machine.traffic_gb > 0 && <span>{machine.traffic_gb}GB流量</span>}
        {machine.ipv4 > 0 && <span>IPv4×{machine.ipv4}</span>}
      </div>

      <div className={styles.prices}>
        <strong className={styles.priceMain}>{fmtPrice(machine.price_monthly)}/月</strong>
        {machine.price_yearly && (
          <span className={styles.priceSub}>{fmtPrice(machine.price_yearly)}/年</span>
        )}
      </div>

      {priceHistory && priceHistory.length > 0 && (
        <div className={styles.history}>
          <span className={styles.historyLabel}>历史成交：</span>
          {priceHistory.map((h, i) => (
            <Tag key={i} color="default">{fmtPrice(h.price)}/{cycleLabel(h.cycle)}</Tag>
          ))}
        </div>
      )}

      <div className={styles.actions}>
        <Tag
          className={styles.watchTag}
          color={watched ? 'blue' : 'default'}
          onClick={() => onWatch(machine.plan_id, !watched)}
          style={{ cursor: 'pointer' }}
        >
          {watched ? '★ 已关注' : '☆ 关注'}
        </Tag>
        {machine.category === 'secondhand' && machine.official_match && (
          <Popover
            title="官方有同款全新"
            trigger="hover"
            content={<OfficialMatchContent match={machine.official_match} />}
          >
            <Tag
              color={machine.official_match.cheaper === 'official' ? 'gold' : 'cyan'}
              style={{ cursor: 'pointer' }}
            >
              🆕 官方全新{machine.official_match.cheaper === 'official' ? '更便宜' : '可对比'}
            </Tag>
          </Popover>
        )}
        {autoBuyEnabled && machine.category === 'secondhand' && available && onBuy && (
          <Tag color="green" style={{ cursor: 'pointer' }} onClick={() => onBuy(machine.plan_id)}>
            🛒 抢购
          </Tag>
        )}
      </div>

      <div className={styles.updated}>
        更新于 {machine.updated_at ? new Date(machine.updated_at).toLocaleString('zh-CN') : '—'}
      </div>
    </div>
  )
}

function OfficialMatchContent({ match }: { match: OfficialMatch }) {
  const delta = match.price_delta
  const absPrice = fmtPrice(Math.abs(delta))
  let verdict: React.ReactNode
  if (match.cheaper === 'official') {
    verdict = <span style={{ color: '#52c41a' }}>官方全新便宜 {absPrice}/月，建议直接买新的</span>
  } else if (match.cheaper === 'secondhand') {
    verdict = <span style={{ color: '#faad14' }}>这台二手便宜 {absPrice}/月</span>
  } else {
    verdict = <span style={{ color: '#999' }}>官方与二手同价</span>
  }
  return (
    <div style={{ maxWidth: 260 }}>
      <div style={{ marginBottom: 4 }}><strong>{match.name}</strong></div>
      <div style={{ fontSize: 12, color: '#999', marginBottom: 6 }}>
        {match.area} · {match.available ? '官方在售' : '官方暂时缺货'}
      </div>
      <div style={{ marginBottom: 6 }}>
        官方月付 <strong>{fmtPrice(match.price_monthly)}</strong>
      </div>
      <div style={{ marginBottom: 10 }}>{verdict}</div>
      <Button
        type="primary"
        size="small"
        icon={<ShoppingOutlined />}
        href={match.buy_url}
        target="_blank"
        rel="noopener noreferrer"
        block
      >
        去官方购买 →
      </Button>
    </div>
  )
}
