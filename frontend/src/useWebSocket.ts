import { useEffect, useRef, useCallback } from 'react'
import { wsUrl } from './config'

export type WsMessage = {
  type: 'refresh' | 'events' | 'purchased'
  category?: string
  data?: unknown
  plan_id?: string
  result?: unknown
}

export function useWebSocket(onMessage: (msg: WsMessage) => void) {
  const wsRef = useRef<WebSocket | null>(null)
  const onMsgRef = useRef(onMessage)
  onMsgRef.current = onMessage

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return
    const ws = new WebSocket(wsUrl())
    ws.onmessage = (e) => {
      try {
        onMsgRef.current(JSON.parse(e.data))
      } catch {}
    }
    ws.onclose = () => {
      // reconnect after 3s
      setTimeout(connect, 3000)
    }
    wsRef.current = ws
  }, [])

  useEffect(() => {
    connect()
    // ping every 20s to keep alive
    const ping = setInterval(() => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send('ping')
      }
    }, 20_000)
    return () => {
      clearInterval(ping)
      wsRef.current?.close()
    }
  }, [connect])
}
