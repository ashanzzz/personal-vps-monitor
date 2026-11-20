import {
  Modal,
  Typography, Switch, Input, Tag, Checkbox, Alert,
} from 'antd'
import {
  SafetyCertificateOutlined, WarningOutlined,
} from '@ant-design/icons'
import type { SecondhandItem } from '../api'

const { Text } = Typography

interface SecondhandBuyModalProps {
  buyModalOpen: boolean
  setBuyModalOpen: (v: boolean) => void
  selectedItem: SecondhandItem | null
  safeMode: boolean
  setSafeMode: (v: boolean) => void
  forceBuyDepleted: boolean
  setForceBuyDepleted: (v: boolean) => void
  captcha: string
  setCaptcha: (v: string) => void
  submitting: boolean
  buyResult: any
  handleExecuteBuy: () => void
  isDepleted?: boolean
}

/**
 * 从 UnifiedSecondhandPage 拆出的弹窗。
 *
 * 拆分手法：props 名与原主组件里的 state 变量名保持一致，
 * 组件内按同名解构，弹窗内部 JSX 一行未改，不引入行为变化。
 */
export function SecondhandBuyModal({
  buyModalOpen,
  setBuyModalOpen,
  selectedItem,
  safeMode,
  setSafeMode,
  forceBuyDepleted,
  setForceBuyDepleted,
  captcha,
  setCaptcha,
  submitting,
  buyResult,
  handleExecuteBuy,
  isDepleted,
}: SecondhandBuyModalProps) {
  return (
    <>
      <Modal
        title={`二手转让抢单 - ${selectedItem?.title}`}
        open={buyModalOpen}
        onCancel={() => setBuyModalOpen(false)}
        onOk={handleExecuteBuy}
        confirmLoading={submitting}
        okText={safeMode ? '执行试算 (防误购)' : (isDepleted && !forceBuyDepleted ? '流量耗尽(需勾选强制)' : '确认立即付款抢单')}
        okButtonProps={{ disabled: !safeMode && isDepleted && !forceBuyDepleted }}
        cancelText="取消"
        width={560}
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14, padding: '10px 0' }}>
          <div>
            <Text type="secondary">交易市场：</Text>
            <Tag color={selectedItem?.platform === 'akile' ? 'blue' : 'green'} style={{ textTransform: 'uppercase' }}>
              {selectedItem?.platform === 'akile' ? 'Akile PushShop' : 'ACCK Exchange'}
            </Tag>
          </div>

          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <Text type="secondary">转让标价：</Text>
              <span className="mono-num" style={{ fontSize: 18, fontWeight: 700, color: '#0f172a' }}>
                ¥{selectedItem?.price_cny?.toFixed(2)}
              </span>
            </div>
            <div>
              <Text type="secondary">线路评估：</Text>
              <Tag color={selectedItem?.route?.tier === 'S' ? 'purple' : 'blue'}>
                {selectedItem?.route?.tier_label} ({selectedItem?.route?.primary_route})
              </Tag>
            </div>
          </div>

          {/* Traffic Warning Alert */}
          {isDepleted && (
            <Alert
              type="error"
              showIcon
              icon={<WarningOutlined />}
              message="【重大风控预警：可用流量已全部耗尽！】"
              description={
                <div style={{ fontSize: 12 }}>
                  <div>当前主机可用流量为 <b>0 GB</b>（总计 {selectedItem?.traffic?.total_gb}GB 已全被原号主用光）。</div>
                  <div>官方重置流量单价为 <b>¥{selectedItem?.traffic?.reset_price_cny}</b>。</div>
                  <div style={{ marginTop: 4, fontWeight: 600, color: '#b91c1c' }}>
                    真实买入总成本 = 挂售价 ¥{selectedItem?.price_cny} + 流量重置费 ¥{selectedItem?.traffic?.reset_price_cny} = <b>¥{selectedItem?.pricing_analysis?.real_total_cost_cny}</b>！
                  </div>
                </div>
              }
            />
          )}

          {/* Detailed Traffic Info */}
          <div style={{ padding: 10, background: '#f8fafc', borderRadius: 6, border: '1px solid #e2e8f0', fontSize: 12 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>已用流量：<b>{selectedItem?.traffic?.used_gb ?? 0} GB</b></span>
              <span>总额度：<b>{selectedItem?.traffic?.total_gb ?? 0} GB</b></span>
              <span>剩余可用：<b style={{ color: isDepleted ? '#dc2626' : '#15803d' }}>{selectedItem?.traffic?.remaining_gb ?? 0} GB</b></span>
            </div>
          </div>

          {selectedItem?.platform === 'akile' && !safeMode && (
            <div>
              <Text type="secondary">图形验证码（Akile 下单）：</Text>
              <Input
                placeholder="请输入 Akile 验证码（如需要）"
                value={captcha}
                onChange={(e) => setCaptcha(e.target.value)}
                style={{ marginTop: 4 }}
              />
            </div>
          )}

          {/* Safe Mode Switch */}
          <div style={{ padding: 12, background: safeMode ? '#eff6ff' : '#fffbeb', borderRadius: 6, border: `1px solid ${safeMode ? '#bfdbfe' : '#fde68a'}` }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
              <span style={{ fontWeight: 600, color: safeMode ? '#1d4ed8' : '#b45309', display: 'flex', alignItems: 'center', gap: 6 }}>
                <SafetyCertificateOutlined />
                防误购安全模式 (Safe Dry-Run)
              </span>
              <Switch checked={safeMode} onChange={setSafeMode} />
            </div>
            <p style={{ margin: 0, fontSize: 12, color: safeMode ? '#1e40af' : '#92400e' }}>
              {safeMode
                ? '已开启安全保护锁：点击确认仅对订单参数和接口合法性做试算校验，不发起真实资金扣除。'
                : '⚠️ 警告：保护锁已关闭！点击确认将直接从您的账户余额划扣并抢购该二手主机！'}
            </p>
          </div>

          {/* Force Buy Depleted Checkbox */}
          {!safeMode && isDepleted && (
            <div style={{ padding: '8px 12px', background: '#fffbeb', borderRadius: 6, border: '1px solid #fef3c7' }}>
              <Checkbox checked={forceBuyDepleted} onChange={(e) => setForceBuyDepleted(e.target.checked)}>
                <span style={{ fontSize: 12, color: '#b45309', fontWeight: 600 }}>
                  已知晓当期流量已用完（长期资产有效：下月续费自动恢复 {selectedItem?.traffic?.total_gb}G 满血，急用亦可花 ¥{selectedItem?.traffic?.reset_price_cny} 重置）
                </span>
              </Checkbox>
            </div>
          )}

          {buyResult && (
            <div style={{ padding: 10, background: '#f8fafc', borderRadius: 6, border: '1px solid #e2e8f0' }}>
              <div style={{ fontWeight: 600, marginBottom: 4, color: '#0f172a' }}>回执：</div>
              <pre style={{ margin: 0, fontSize: 11, maxHeight: 150, overflow: 'auto', fontFamily: 'monospace' }}>
                {JSON.stringify(buyResult, null, 2)}
              </pre>
            </div>
          )}
        </div>
      </Modal>
    </>
  )
}
