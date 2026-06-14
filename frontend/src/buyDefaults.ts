import type { BuyRule, PageBuyConfig } from './api'

// Mirror of backend config.rule_defaults() / page_buy_defaults() so the UI can
// create new rules and fall back gracefully when a page config is missing.
export function ruleDefaults(): BuyRule {
  return {
    id: '',
    enabled: true,
    label: '',
    name_keyword: '',
    cpu_min: 0,
    cpu_max: 0,
    memory_min_mb: 0,
    memory_max_mb: 0,
    disk_min_gb: 0,
    disk_max_gb: 0,
    bandwidth_mode: 'any',
    bandwidth_mbps: 0,
    traffic_min_gb: 0,
    price_max_monthly: 0,
    areas: [],
    max_per_rule: 1,
  }
}

export function pageBuyDefaults(): PageBuyConfig {
  return {
    enabled: false,
    refresh_interval: 60,
    max_total_per_run: 1,
    max_total_daily: 1,
    max_price: 0,
    cycle: 1,
    on_limit_reached: 'keep_watching',
    rules: [],
  }
}
