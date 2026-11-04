import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Stats, StatsTrip } from '../lib/types'
import { countryName, flag } from '../lib/countries'
import { fmtDate, fmtRange, plural, todayISO } from '../lib/format'
import { Card, Empty, Loading, PageHeader, Segmented, useAsync, useToast } from '../components/ui'
import { Icon } from '../components/icons'

/** Where you went, for how long and when: countries with days, a timeline per year, days away per year. */
export default function StatsPage() {
  const { data, error, loading, reload } = useAsync(() => api.get<Stats>(`${V1}/stats`), [])
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const detect = async () => {
    setBusy(true)
    try { const r = await api.post<{ detected: number; missing: number }>(`${V1}/countries/detect`); toast(r.missing ? `Found ${r.detected}, ${r.missing} still to look up` : `Countries found for ${plural(r.detected, 'spot')}`); await reload() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  if (loading && !data) return <Loading />
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return null
  const { totals } = data
  return (
    <>
      <PageHeader title="Insights" sub="Where you went, for how long, and when" />
      {totals.pending_lookups > 0 && (
        <div className="hint-strip"><Icon name="compass" size={14} />Looking up the country of {plural(totals.pending_lookups, 'stay or place', 'stays and places')} from their coordinates — the numbers fill in as they are found.<span className="spacer" /><button type="button" className="btn sm" onClick={detect} disabled={busy}>{busy ? 'Looking up…' : 'Look up now'}</button></div>
      )}
      <div className="stats">
        <div className="stat"><div className="n">{totals.countries}</div><div className="l">countries visited</div></div>
        <div className="stat"><div className="n">{totals.days}</div><div className="l">days away</div></div>
        <Link className="stat" to="/trips"><div className="n">{totals.trips}</div><div className="l">trips taken</div></Link>
        <div className="stat"><div className="n">{totals.places_visited}</div><div className="l">places visited</div></div>
      </div>
      {data.trips.length === 0 ? <Card><Empty icon="globe" title="Nothing to look back on yet">Trips with dates show up here: the countries you were in, for how many days, and when.</Empty></Card> : <>
        <CountryChart data={data} />
        <Timeline trips={data.trips} />
        <YearChart data={data} />
        <TripTable trips={data.trips} />
      </>}
    </>
  )
}

/** Days per country: one bar each, longest first. Days of upcoming trips extend the bar, hatched. */
function CountryChart({ data }: { data: Stats }) {
  const [mode, setMode] = useState<'days' | 'trips'>('days')
  const rows = useMemo(() => [...data.countries].sort((a, b) => mode === 'days' ? (b.days - a.days) || (b.planned_days - a.planned_days) : (b.trips.length - a.trips.length) || (b.days - a.days)), [data.countries, mode])
  if (rows.length === 0) return <Card title="Countries" className="mb"><p className="muted">No countries known yet. They come from the coordinates of stays, places and trip destinations.</p></Card>
  const max = Math.max(1, ...rows.map(c => mode === 'days' ? c.days + c.planned_days : c.trips.length))
  const anyPlanned = mode === 'days' && rows.some(c => c.planned_days > 0)
  return (
    <Card title={`Countries · ${rows.filter(c => c.days > 0).length}`} className="mb" actions={<Segmented value={mode} onChange={setMode} options={[{ value: 'days', label: 'Days' }, { value: 'trips', label: 'Trips' }]} size="sm" label="Measure" />}>
      {anyPlanned && <div className="legend" style={{ marginTop: 0 }}><span className="key"><i className="sw spent" />days there</span><span className="key"><i className="sw planned" />upcoming</span></div>}
      <div className="hbars" role="list">
        {rows.map(c => {
          const v = mode === 'days' ? c.days : c.trips.length
          const tip = `${countryName(c.code)} — ${plural(c.days, 'day')}${c.planned_days ? ` (+${c.planned_days} upcoming)` : ''} on ${plural(c.trips.length, 'trip')}${c.first ? ` · first ${fmtDate(c.first)}` : ''}${c.last && c.last !== c.first ? ` · last ${fmtDate(c.last)}` : ''}`
          return (
            <div key={c.code} className="hbar" role="listitem" title={tip}>
              <span className="hbar-label"><span className="flag">{flag(c.code)}</span>{countryName(c.code)}</span>
              <span className="hbar-track">
                <span className="hbar-fill" style={{ width: `${v / max * 100}%` }} />
                {mode === 'days' && c.planned_days > 0 && <span className="hbar-fill planned" style={{ width: `${c.planned_days / max * 100}%` }} />}
              </span>
              <span className="hbar-value">{mode === 'days' ? <>{c.days}{c.planned_days > 0 && <span className="muted"> +{c.planned_days}</span>}</> : c.trips.length}</span>
            </div>
          )
        })}
      </div>
    </Card>
  )
}

const MONTHS = ['J', 'F', 'M', 'A', 'M', 'J', 'J', 'A', 'S', 'O', 'N', 'D']
const dayOfYear = (d: Date) => Math.floor((Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()) - Date.UTC(d.getUTCFullYear(), 0, 1)) / 864e5)
const daysIn = (y: number) => (y % 4 === 0 && y % 100 !== 0) || y % 400 === 0 ? 366 : 365

/** One row per year, January to December; each trip a bar where it fell, with the flags of its countries. */
function Timeline({ trips }: { trips: StatsTrip[] }) {
  const navigate = useNavigate()
  const [hover, setHover] = useState<{ t: StatsTrip; x: number; y: number } | null>(null)
  const years = useMemo(() => {
    const by = new Map<number, { t: StatsTrip; from: number; to: number }[]>()
    for (const t of trips) {
      const s = new Date(t.start_date + 'T00:00:00Z'), e = new Date(t.end_date + 'T00:00:00Z')
      for (let y = s.getUTCFullYear(); y <= e.getUTCFullYear(); y++) {
        const from = y === s.getUTCFullYear() ? dayOfYear(s) : 0
        const to = y === e.getUTCFullYear() ? dayOfYear(e) + 1 : daysIn(y)
        if (!by.has(y)) by.set(y, [])
        by.get(y)!.push({ t, from: from / daysIn(y), to: to / daysIn(y) })
      }
    }
    return [...by.entries()].sort((a, b) => b[0] - a[0])
  }, [trips])
  const today = todayISO()
  const ty = Number(today.slice(0, 4)), tpos = dayOfYear(new Date(today + 'T00:00:00Z')) / daysIn(ty)
  return (
    <Card title="Timeline" className="mb">
      <div className="tl" onMouseLeave={() => setHover(null)}>
        <div className="tl-row tl-axis"><span className="tl-year" /><span className="tl-track">{MONTHS.map((m, i) => <span key={i} style={{ left: `${i / 12 * 100}%` }}>{m}</span>)}</span></div>
        {years.map(([y, items]) => (
          <div key={y} className="tl-row">
            <span className="tl-year">{y}</span>
            <span className="tl-track">
              {MONTHS.map((_, i) => <i key={i} className="tl-grid" style={{ left: `${i / 12 * 100}%` }} />)}
              {y === ty && <i className="tl-today" style={{ left: `${tpos * 100}%` }} title="Today" />}
              {items.map(({ t, from, to }) => (
                <button type="button" key={t.id} className={`tl-bar ${t.upcoming ? 'upcoming' : ''}`} style={{ left: `${from * 100}%`, width: `max(6px, ${(to - from) * 100}%)`, '--c': t.color || 'var(--accent)' } as React.CSSProperties}
                  onClick={() => navigate(`/trips/${t.id}`)} onMouseEnter={e => { const r = (e.currentTarget.closest('.tl') as HTMLElement).getBoundingClientRect(), b = e.currentTarget.getBoundingClientRect(); setHover({ t, x: b.left - r.left + b.width / 2, y: b.top - r.top }) }} onFocus={e => { const r = (e.currentTarget.closest('.tl') as HTMLElement).getBoundingClientRect(), b = e.currentTarget.getBoundingClientRect(); setHover({ t, x: b.left - r.left + b.width / 2, y: b.top - r.top }) }} onBlur={() => setHover(null)} aria-label={`${t.name}, ${fmtRange(t.start_date, t.end_date)}`}>
                  {(to - from) > 0.06 && <span>{t.countries.slice(0, 3).map(c => flag(c.code)).join('')}</span>}
                </button>
              ))}
            </span>
          </div>
        ))}
        {hover && (
          <div className="tl-tip" style={{ left: hover.x, top: hover.y }}>
            <b>{hover.t.name}</b>
            <div className="muted small">{fmtRange(hover.t.start_date, hover.t.end_date)} · {plural(hover.t.days, 'day')}{hover.t.upcoming ? ' · upcoming' : ''}</div>
            {hover.t.countries.length > 0 && <div className="small">{hover.t.countries.map(c => `${flag(c.code)} ${countryName(c.code)}${c.days ? ` ${c.days}d` : ''}`).join(' · ')}</div>}
          </div>
        )}
      </div>
    </Card>
  )
}

/** Days away per calendar year, with the countries that were new that year. */
function YearChart({ data }: { data: Stats }) {
  if (data.years.length === 0) return null
  const max = Math.max(1, ...data.years.map(y => y.days))
  return (
    <Card title="Days away per year" className="mb">
      <div className="vbars" role="list">
        {data.years.map(y => (
          <div key={y.year} className="vbar" role="listitem" title={`${y.year}: ${plural(y.days, 'day')} away on ${plural(y.trips, 'trip')}${y.countries.length ? ` · ${y.countries.map(countryName).join(', ')}` : ''}${y.new_countries.length ? ` · new: ${y.new_countries.map(countryName).join(', ')}` : ''}`}>
            <span className="vbar-value">{y.days}</span>
            <span className="vbar-track"><span className="vbar-fill" style={{ height: `${y.days / max * 100}%` }} /></span>
            <span className="vbar-label">{y.year}</span>
            <span className="vbar-new">{y.new_countries.map(flag).join('')}</span>
          </div>
        ))}
      </div>
      <p className="muted small mt-sm">Flags under a year are countries visited for the first time that year.</p>
    </Card>
  )
}

/** Every trip with its countries and days — the same numbers as the charts, as a table. */
function TripTable({ trips }: { trips: StatsTrip[] }) {
  const navigate = useNavigate()
  return (
    <Card title="Trips by country" className="mb" flush>
      <table className="table compact">
        <thead><tr><th>Trip</th><th>When</th><th className="num">Days</th><th>Countries (days)</th><th className="num">Places visited</th></tr></thead>
        <tbody>{[...trips].reverse().map(t => (
          <tr key={t.id} className="clickable" onClick={() => navigate(`/trips/${t.id}`)}>
            <td><b>{t.name}</b>{t.upcoming && <span className="badge">upcoming</span>}</td>
            <td className="muted">{fmtRange(t.start_date, t.end_date)}</td>
            <td className="num">{t.days}</td>
            <td>{t.countries.length ? t.countries.map(c => <span key={c.code} className="country-chip" title={countryName(c.code)}>{flag(c.code)} {c.code}{c.days ? ` ${c.days}` : ''}</span>) : <span className="muted">—</span>}</td>
            <td className="num">{t.visited}/{t.places}</td>
          </tr>
        ))}</tbody>
      </table>
    </Card>
  )
}
