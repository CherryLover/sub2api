const DAY_MS = 24 * 60 * 60 * 1000

export interface MonthlyProrationState {
  prorate?: boolean | null
  proratedLimit?: number | null
  proratedUntil?: string | null
}

export interface FixedMonthlyPeriod {
  start: Date
  end: Date
}

export interface MonthlyProrationEstimate extends FixedMonthlyPeriod {
  effectiveLimit: number
  remainingDays: number
  totalDays: number
}

interface ZonedDateParts {
  year: number
  month: number
  day: number
  hour: number
  minute: number
  second: number
}

const dateTimeFormatters = new Map<string, Intl.DateTimeFormat>()

function getZonedDateParts(date: Date, timeZone: string): ZonedDateParts {
  let formatter = dateTimeFormatters.get(timeZone)
  if (!formatter) {
    formatter = new Intl.DateTimeFormat('en-US', {
      timeZone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hourCycle: 'h23',
    })
    dateTimeFormatters.set(timeZone, formatter)
  }

  const values: Record<string, number> = {}
  for (const part of formatter.formatToParts(date)) {
    if (part.type !== 'literal') values[part.type] = Number(part.value)
  }
  return {
    year: values.year,
    month: values.month,
    day: values.day,
    hour: values.hour,
    minute: values.minute,
    second: values.second,
  }
}

function zonedDateTimeToDate(
  year: number,
  monthIndex: number,
  day: number,
  hour: number,
  timeZone: string,
): Date {
  const targetWallTime = Date.UTC(year, monthIndex, day, hour)
  let guess = targetWallTime

  // Convert a wall-clock time in an IANA timezone to UTC. A second pass handles
  // offsets that change between the initial UTC guess and the actual instant.
  for (let i = 0; i < 3; i += 1) {
    const parts = getZonedDateParts(new Date(guess), timeZone)
    const representedWallTime = Date.UTC(
      parts.year,
      parts.month - 1,
      parts.day,
      parts.hour,
      parts.minute,
      parts.second,
    )
    const offset = representedWallTime - Math.floor(guess / 1000) * 1000
    const nextGuess = targetWallTime - offset
    if (nextGuess === guess) break
    guess = nextGuess
  }

  return new Date(guess)
}

function daysInMonth(year: number, monthIndex: number): number {
  return new Date(Date.UTC(year, monthIndex + 1, 0)).getUTCDate()
}

function monthlyResetInstant(
  year: number,
  monthIndex: number,
  resetDay: number,
  resetHour: number,
  timeZone: string,
): Date {
  const day = Math.min(Math.max(Math.trunc(resetDay), 1), daysInMonth(year, monthIndex))
  const hour = Math.min(Math.max(Math.trunc(resetHour), 0), 23)
  return zonedDateTimeToDate(year, monthIndex, day, hour, timeZone)
}

export function resolveFixedMonthlyPeriod(
  now: Date,
  resetDay: number,
  resetHour: number,
  timeZone: string,
): FixedMonthlyPeriod | null {
  try {
    const zonedNow = getZonedDateParts(now, timeZone)
    const resets: Date[] = []

    for (let monthOffset = -2; monthOffset <= 2; monthOffset += 1) {
      const anchor = new Date(Date.UTC(zonedNow.year, zonedNow.month - 1 + monthOffset, 1))
      resets.push(monthlyResetInstant(
        anchor.getUTCFullYear(),
        anchor.getUTCMonth(),
        resetDay,
        resetHour,
        timeZone,
      ))
    }

    resets.sort((left, right) => left.getTime() - right.getTime())
    const nowMs = now.getTime()
    const start = [...resets].reverse().find(reset => reset.getTime() <= nowMs)
    const end = resets.find(reset => reset.getTime() > nowMs)
    if (!start || !end || end.getTime() <= start.getTime()) return null
    return { start, end }
  } catch {
    return null
  }
}

export function roundMonthlyQuota(value: number): number {
  return Math.round((value + Number.EPSILON) * 100) / 100
}

export function prorateMonthlyLimit(
  limit: number,
  cycleStart: Date,
  cycleEnd: Date,
  from: Date,
): number {
  const duration = cycleEnd.getTime() - cycleStart.getTime()
  if (!Number.isFinite(limit) || limit <= 0 || duration <= 0) return 0

  const remaining = Math.min(
    Math.max(cycleEnd.getTime() - from.getTime(), 0),
    duration,
  )
  return roundMonthlyQuota(limit * (remaining / duration))
}

export function estimateMonthlyProration(
  limit: number,
  resetDay: number,
  resetHour: number,
  timeZone: string,
  now: Date,
): MonthlyProrationEstimate | null {
  if (!Number.isFinite(limit) || limit <= 0) return null
  const period = resolveFixedMonthlyPeriod(now, resetDay, resetHour, timeZone)
  if (!period) return null

  const effectiveLimit = prorateMonthlyLimit(limit, period.start, period.end, now)
  const remainingMs = Math.max(0, period.end.getTime() - now.getTime())
  const totalMs = Math.max(0, period.end.getTime() - period.start.getTime())

  return {
    ...period,
    effectiveLimit,
    remainingDays: Math.max(0, Math.ceil(remainingMs / DAY_MS)),
    totalDays: Math.max(1, Math.round(totalMs / DAY_MS)),
  }
}

export function isMonthlyProrationActive(
  state: MonthlyProrationState,
  now: Date = new Date(),
): boolean {
  if (state.prorate !== true) return false
  if (state.proratedLimit == null || state.proratedLimit < 0) return false
  if (!state.proratedUntil) return false
  const untilMs = Date.parse(state.proratedUntil)
  return Number.isFinite(untilMs) && untilMs > now.getTime()
}

export function getMonthlyEffectiveLimit(
  monthlyLimit: number | null | undefined,
  state: MonthlyProrationState,
  now: Date = new Date(),
): number | null {
  if (isMonthlyProrationActive(state, now)) {
    return state.proratedLimit ?? monthlyLimit ?? null
  }
  return monthlyLimit ?? null
}

export function formatMonthlyPeriodTime(value: string | Date, timeZone: string): string {
  const date = value instanceof Date ? value : new Date(value)
  if (!Number.isFinite(date.getTime())) return '-'

  const parts = new Intl.DateTimeFormat('en-GB', {
    timeZone,
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(date)
  const values: Record<string, string> = {}
  for (const part of parts) {
    if (part.type !== 'literal') values[part.type] = part.value
  }
  return `${values.month}/${values.day} ${values.hour}:${values.minute}`
}
