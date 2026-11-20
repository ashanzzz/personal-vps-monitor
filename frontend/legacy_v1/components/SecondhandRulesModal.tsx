import {
  Button, Modal, Space,
} from 'antd'
import {
  SafetyCertificateOutlined, ThunderboltOutlined,
} from '@ant-design/icons'
import type { PageBuyConfig } from '../api'
import { BuyRulesEditor } from './BuyRulesEditor'

interface SecondhandRulesModalProps {
  rulesModalOpen: boolean
  setRulesModalOpen: (v: boolean) => void
  buyConfig: PageBuyConfig | null
  setBuyConfig: (v: PageBuyConfig) => void
  savingRules: boolean
  saveRules: () => void
}

/**
 * 从 UnifiedSecondhandPage 拆出的弹窗。
 *
 * 拆分手法：props 名与原主组件里的 state 变量名保持一致，
 * 组件内按同名解构，弹窗内部 JSX 一行未改，不引入行为变化。
 */
export function SecondhandRulesModal({
  rulesModalOpen,
  setRulesModalOpen,
  buyConfig,
  setBuyConfig,
  savingRules,
  saveRules
}: SecondhandRulesModalProps) {
  return (
    <>
      <Modal
        title={
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <ThunderboltOutlined style={{ color: '#0284c7' }} />
            <span>定向秒杀与自动抢购看门狗规则管理</span>
          </div>
        }
        open={rulesModalOpen}
        onCancel={() => setRulesModalOpen(false)}
        width={920}
        destroyOnClose
        footer={
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ fontSize: 12, color: '#94a3b8' }}>
              修改参数后点击保存，后台极速轮询看门狗（5s/次）将在下一周期自动加载生效
            </span>
            <Space>
              <Button onClick={() => setRulesModalOpen(false)}>取消</Button>
              <Button
                type="primary"
                icon={<SafetyCertificateOutlined />}
                loading={savingRules}
                onClick={saveRules}
                style={{ background: '#0284c7', borderColor: '#0284c7' }}
              >
                保存并立即生效
              </Button>
            </Space>
          </div>
        }
      >
        {buyConfig ? (
          <BuyRulesEditor
            pageLabel="双平台二手集市"
            value={buyConfig}
            onChange={setBuyConfig}
          />
        ) : (
          <div style={{ padding: 30, textAlign: 'center', color: '#94a3b8' }}>加载抢购配置中...</div>
        )}
      </Modal>
    </>
  )
}
