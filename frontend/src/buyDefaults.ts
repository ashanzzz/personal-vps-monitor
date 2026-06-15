import type { BuyRule, PageBuyConfig, RuleSpec } from './api'

// Mirror of backend config.rule_defaults() / page_buy_defaults() so the UI can
// create new rules and fall back gracefully when a page config is missing.
export function specDefaults(): RuleSpec {
  return {
    cpu: { min: 0, max: 0 },
    memory: { min: 0, max: 0 },
    disk: { min: 0, max: 0 },
    bandwidth: { min: 0, max: 0 },
    traffic: { min: 0, max: 0 },
  }
}

export function ruleDefaults(): BuyRule {
  return {
    id: '',
    enabled: true,
    label: '',
    tags_include: [],
    spec: specDefaults(),
    price_type: 'monthly',
    price_max: 0,
    sale_price_max: 0,
    max_buy: 1,
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
