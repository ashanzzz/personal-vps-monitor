import { Tag, Tooltip } from 'antd'
import { SyncOutlined, CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import type { Machine } from '../api'
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
