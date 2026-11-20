import React, { useEffect, useRef, useState, useCallback } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { Button, Space, Tag, Tooltip, message } from 'antd'
import { getToken } from '../api'
import {
  ReloadOutlined,
  ClearOutlined,
  ThunderboltOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  SyncOutlined,
  CodeOutlined,
} from '@ant-design/icons'

interface Props {
  instanceId: string
  instanceName?: string
}

export const LazyCatWebShellView: React.FC<Props> = ({ instanceId, instanceName }) => {
  const terminalRef = useRef<HTMLDivElement>(null)
  const xtermInstance = useRef<Terminal | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const wsRef = useRef<WebSocket | null>(null)

  const [connStatus, setConnStatus] = useState<'connecting' | 'connected' | 'disconnected' | 'error'>('connecting')

  const connectWebSocket = useCallback(() => {
    if (!instanceId) return

    // 关闭已存在的连接
    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }

    setConnStatus('connecting')

    const token = getToken()
    const tokenQuery = token ? `&token=${encodeURIComponent(token)}` : ''
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/api/lazycat/terminal/ws?instance_id=${encodeURIComponent(instanceId)}${tokenQuery}`

    const ws = new WebSocket(wsUrl)
    wsRef.current = ws

    ws.onopen = () => {
      setConnStatus('connected')
      if (xtermInstance.current) {
        xtermInstance.current.writeln('\x1b[32m[VPS-Hub WebShell] WebSocket 终端通道已就绪，已成功附加 Alpine PTY 会话。\x1b[0m\r\n')
        // 同步当前终端行列宽高至 Alpine PTY
        if (xtermInstance.current.cols && xtermInstance.current.rows) {
          ws.send(JSON.stringify({
            type: 'resize',
            cols: xtermInstance.current.cols,
            rows: xtermInstance.current.rows,
          }))
        }
      }
    }

    ws.onmessage = (event) => {
      // 过滤官方 WebSocket 握手 ACK 消息
      if (typeof event.data === 'string' && event.data.startsWith('{"type":"connected"')) {
        return
      }
      if (xtermInstance.current) {
        xtermInstance.current.write(event.data)
      }
    }

    ws.onerror = () => {
      setConnStatus('error')
      if (xtermInstance.current) {
        xtermInstance.current.writeln('\r\n\x1b[31m[VPS-Hub WebShell] 终端通道连接异常，请检查网络或实例状态。\x1b[0m\r\n')
      }
    }

    ws.onclose = (event) => {
      setConnStatus('disconnected')
      if (xtermInstance.current) {
        xtermInstance.current.writeln(`\r\n\x1b[33m[VPS-Hub WebShell] 终端会话已断开 (代码: ${event.code})。\x1b[0m\r\n`)
      }
    }
  }, [instanceId])

  useEffect(() => {
    if (!terminalRef.current) return

    // 初始化 xterm.js
    const term = new Terminal({
      cursorBlink: true,
      cursorStyle: 'block',
      fontSize: 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace, "Consolas"',
      theme: {
        background: '#0f172a', // Slate 900
        foreground: '#e2e8f0',
        cursor: '#38bdf8',
        selectionBackground: 'rgba(56, 189, 248, 0.3)',
        black: '#1e293b',
        red: '#f87171',
        green: '#4ade80',
        yellow: '#facc15',
        blue: '#60a5fa',
        magenta: '#c084fc',
        cyan: '#38bdf8',
        white: '#f8fafc',
      },
      convertEol: true,
      scrollback: 5000,
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(terminalRef.current)
    fitAddon.fit()

    xtermInstance.current = term
    fitAddonRef.current = fitAddon

    // 用户键盘输入转发至 WebSocket
    term.onData((data) => {
      if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
        wsRef.current.send(data)
      }
    })

    // 建立连接
    connectWebSocket()

    const handleResize = () => {
      try {
        fitAddon.fit()
        if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN && term.cols && term.rows) {
          wsRef.current.send(JSON.stringify({
            type: 'resize',
            cols: term.cols,
            rows: term.rows,
          }))
        }
      } catch {
        // ignore
      }
    }

    window.addEventListener('resize', handleResize)

    return () => {
      window.removeEventListener('resize', handleResize)
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
      term.dispose()
      xtermInstance.current = null
      fitAddonRef.current = null
    }
  }, [connectWebSocket])

  const sendCommand = (cmd: string) => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(cmd + '\n')
      if (xtermInstance.current) {
        xtermInstance.current.focus()
      }
    } else {
      message.warning('终端未处于连接状态，无法发送指令')
    }
  }

  const clearScreen = () => {
    if (xtermInstance.current) {
      xtermInstance.current.clear()
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 480 }}>
      {/* 顶部工具栏 */}
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          padding: '8px 12px',
          background: '#1e293b',
          borderTopLeftRadius: 8,
          borderTopRightRadius: 8,
          borderBottom: '1px solid #334155',
        }}
      >
        <Space size={10}>
          <span style={{ color: '#94a3b8', fontSize: 13, fontWeight: 600 }}>
            <CodeOutlined style={{ marginRight: 6, color: '#38bdf8' }} />
            {instanceName || `容器 #${instanceId}`} 实时交互 PTY 会话
          </span>

          {connStatus === 'connected' && (
            <Tag icon={<CheckCircleOutlined />} color="success" style={{ margin: 0 }}>
              PTY 会话活动中
            </Tag>
          )}
          {connStatus === 'connecting' && (
            <Tag icon={<SyncOutlined spin />} color="processing" style={{ margin: 0 }}>
              连接中...
            </Tag>
          )}
          {connStatus === 'disconnected' && (
            <Tag icon={<CloseCircleOutlined />} color="default" style={{ margin: 0 }}>
              已断开
            </Tag>
          )}
          {connStatus === 'error' && (
            <Tag icon={<CloseCircleOutlined />} color="error" style={{ margin: 0 }}>
              连接异常
            </Tag>
          )}
        </Space>

        <Space size={8}>
          <Tooltip title="快速读取当前实例家宽出口节点映射配置">
            <Button
              size="small"
              type="text"
              icon={<ThunderboltOutlined />}
              onClick={() => sendCommand('cat /root/nodes.json | jq . || cat /root/nodes.json')}
              style={{ color: '#cbd5e1' }}
            >
              nodes.json
            </Button>
          </Tooltip>

          <Tooltip title="查看容器进程列表">
            <Button
              size="small"
              type="text"
              onClick={() => sendCommand('ps aux')}
              style={{ color: '#cbd5e1' }}
            >
              ps
            </Button>
          </Tooltip>

          <Tooltip title="查看网络接口与 IP 分配">
            <Button
              size="small"
              type="text"
              onClick={() => sendCommand('ip a')}
              style={{ color: '#cbd5e1' }}
            >
              ip a
            </Button>
          </Tooltip>

          <Tooltip title="查看系统运行时间与负载">
            <Button
              size="small"
              type="text"
              onClick={() => sendCommand('uptime')}
              style={{ color: '#cbd5e1' }}
            >
              uptime
            </Button>
          </Tooltip>

          <Button
            size="small"
            type="text"
            icon={<ClearOutlined />}
            onClick={clearScreen}
            style={{ color: '#94a3b8' }}
          >
            清屏
          </Button>

          <Button
            size="small"
            type="primary"
            ghost
            icon={<ReloadOutlined />}
            onClick={connectWebSocket}
            style={{ borderColor: '#475569', color: '#e2e8f0' }}
          >
            重连
          </Button>
        </Space>
      </div>

      {/* 终端主体 */}
      <div
        ref={terminalRef}
        style={{
          flex: 1,
          height: 480,
          background: '#0f172a',
          padding: '8px 12px',
          borderBottomLeftRadius: 8,
          borderBottomRightRadius: 8,
          overflow: 'hidden',
        }}
      />
    </div>
  )
}
