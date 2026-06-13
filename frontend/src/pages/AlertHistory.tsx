import { AlertPanel } from '../components/AlertPanel'
import type { AlertEvent } from '../api'

interface Props {
  events: AlertEvent[]
}

export function AlertHistory({ events }: Props) {
  return (
    <div>
      <AlertPanel events={events} />
    </div>
  )
}
