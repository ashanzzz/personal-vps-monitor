/**
 * 三平台（Akile / ACCK / 懒猫云）共用的格式化与归一化工具。
 *
 * 抽出来的原因：show / timeAgo / pick / fmtDate / daysLeft 原本在
 * AccountPage、AcckAccountPage、LazyCatAccountPage、PlatformOverviewPage
 * 里各写了一遍，四份实现已经开始漂移（同名函数一个收 string 一个收 unix 秒，
 * 空值文案也不一致）。这里做唯一实现，页面只负责取值。
 */

/** 任意值转展示文本；null / undefined / 空串统一显示破折号，布尔转中文。 */
export function show(v: unknown): string {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'boolean') return v ? '是' : '否'
  return String(v)
}

/**
 * 把时间字段归一化成毫秒时间戳。
 *
 * 三个平台的时间字段有三种形态，必须在这里吸收掉：
 * - unix 秒（Akile / ACCK 的 due_time、expire_time）
 * - unix 毫秒（懒猫云部分字段）
 * - ISO8601 字符串（懒猫云的 expiresAt、缓存时间戳）
 */
export function toMs(v: unknown): number | null {
  if (v === null || v === undefined || v === '') return null
  if (typeof v === 'number') {
    if (!Number.isFinite(v) || v <= 0) return null
    // 1e11 秒约等于公元 5138 年，超过必是毫秒
    return v > 1e11 ? v : v * 1000
  }
  const t = new Date(String(v)).getTime()
  return Number.isNaN(t) ? null : t
}

/** 相对时间：刚刚 / N 分钟前 / N 小时前 / N 天前。空值返回「从未」。 */
export function timeAgo(iso: string | null | undefined): string {
  if (!iso) return '从未'
  const t = toMs(iso)
  if (t === null) return '未知'
  const sec = Math.max(0, Math.round((Date.now() - t) / 1000))
  if (sec < 60) return `${sec} 秒前`
  const min = Math.round(sec / 60)
  if (min < 60) return `${min} 分钟前`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr} 小时前`
  return `${Math.round(hr / 24)} 天前`
}

/** 取第一个有值的字段。三个平台的同义字段名差异很大，用它抹平。 */
export function pick<T extends object>(obj: T, keys: string[]): unknown {
  const o = obj as Record<string, unknown>
  for (const k of keys) {
    const v = o[k]
    if (v !== undefined && v !== null && v !== '') return v
  }
  return undefined
}

/** 日期（不含时分）：2026-12-14。兼容 unix 秒 / 毫秒 / ISO 字符串。 */
export function fmtDate(v: unknown): string {
  const t = toMs(v)
  if (t === null) return v === null || v === undefined || v === '' ? '—' : show(v)
  const d = new Date(t)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

/** 日期时间（到分钟）：2026-12-14 09:30。用于同步时间这类需要精确到分的场景。 */
export function fmtTime(v: unknown): string {
  const t = toMs(v)
  if (t === null) return v === null || v === undefined || v === '' ? '—' : show(v)
  const d = new Date(t)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

/** 距今天数（向上取整）。已过期为负数，无法解析返回 null。 */
export function daysLeft(v: unknown): number | null {
  const t = toMs(v)
  if (t === null) return null
  return Math.ceil((t - Date.now()) / 86400000)
}

/** 字节数转人类可读：1.46 TB / 727.3 MB。懒猫云流量字段是纯字节。 */
export function fmtBytes(bytes: unknown): string {
  const n = Number(bytes)
  if (!Number.isFinite(n) || n < 0) return '—'
  const gb = n / 1024 ** 3
  if (gb >= 1024) return `${(gb / 1024).toFixed(2)} TB`
  if (gb >= 1) return `${gb.toFixed(gb >= 100 ? 0 : 1)} GB`
  return `${(n / 1024 ** 2).toFixed(1)} MB`
}

/** 磁盘容量：输入单位是 MB（懒猫云和 ACCK 都是 MB，Akile 是 GB，调用方自己换算）。 */
export function fmtDiskMb(mb: unknown): string {
  const n = Number(mb)
  if (!Number.isFinite(n) || n <= 0) return '—'
  return n >= 1024 ? `${(n / 1024).toFixed(n % 1024 === 0 ? 0 : 1)}G` : `${n}M`
}

/** unix 秒时间戳是否落在本地日历的今天（签到状态判断）。 */
export function checkedInToday(ts: number | null | undefined): boolean {
  if (!ts) return false
  const then = new Date(ts * 1000)
  if (Number.isNaN(then.getTime())) return false
  const now = new Date()
  return then.getFullYear() === now.getFullYear()
    && then.getMonth() === now.getMonth()
    && then.getDate() === now.getDate()
}
