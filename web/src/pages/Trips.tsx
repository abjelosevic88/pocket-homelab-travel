import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Trip } from '../lib/types'
import { STATUS_LABEL } from '../lib/types'
import { daysBetween, fmtRange, plural, todayISO } from '../lib/format'
import { Empty, Loading, PageHeader, Segmented, useAsync } from '../components/ui'
import { Money } from '../components/Money'
import { TripModal } from './trip/TripSettings'
import { Icon } from '../components/icons'
import { useApp } from '../lib/app-context'

type Filter = 'all' | 'upcoming' | 'past' | 'archived'

export default function Trips() {
  const [params, setParams] = useSearchParams()
  const [filter, setFilter] = useState<Filter>('all')
  const [q, setQ] = useState('')
  const [creating, setCreating] = useState(params.get('new') === '1')
  const navigate = useNavigate()
  const { reloadTrips } = useApp()
  const { data, loading, error, reload } = useAsync(() => api.get<Trip[]>(`${V1}/trips?archived=${filter === 'archived' ? '1' : '0'}`), [filter])
  useEffect(() => { if (params.get('new') === '1') { setCreating(true); params.delete('new'); setParams(params, { replace: true }) } }, [params, setParams])
  const today = todayISO()
  const list = (data || []).filter(t => {
    if (q && !`${t.name} ${t.destination}`.toLowerCase().includes(q.toLowerCase())) return false
    if (filter === 'upcoming') return !t.start_date || t.end_date >= today
    if (filter === 'past') return t.end_date && t.end_date < today
    return true
  })
  return (
    <>
      <PageHeader title="Trips" sub={data ? plural(data.length, 'trip') : ''} actions={<button type="button" className="btn primary" onClick={() => setCreating(true)}><Icon name="plus" size={15} />New trip</button>} />
      <div className="toolbar">
        <div className="search"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Search trips…" aria-label="Search trips" /></div>
        <Segmented value={filter} onChange={setFilter} options={[{ value: 'all', label: 'All' }, { value: 'upcoming', label: 'Upcoming' }, { value: 'past', label: 'Past' }, { value: 'archived', label: 'Archived' }]} size="sm" label="Show" />
      </div>
      {loading && !data ? <Loading /> : error ? <div className="callout danger">{error}</div> : list.length === 0 ? (
        <Empty icon="luggage" title={q ? 'No trips match' : filter === 'archived' ? 'Nothing archived' : 'No trips yet'}>{!q && filter !== 'archived' && 'Create your first trip, or import your collections from AdventureLog under Settings → Import.'}</Empty>
      ) : (
        <div className="trip-grid">
          {list.map(t => <TripCard key={t.id} t={t} />)}
        </div>
      )}
      {creating && <TripModal onClose={() => setCreating(false)} onSaved={t => { setCreating(false); reload(); reloadTrips(); navigate(`/trips/${t.id}`) }} />}
    </>
  )
}

export function TripCard({ t }: { t: Trip }) {
  const today = todayISO()
  const running = t.start_date && t.start_date <= today && t.end_date >= today
  const until = t.start_date > today ? daysBetween(today, t.start_date) : 0
  const style = { '--c': t.color || '#0284c7', backgroundImage: t.has_cover ? `url(${V1}/trips/${t.id}/cover)` : undefined } as React.CSSProperties
  const c = t.counts
  return (
    <Link to={`/trips/${t.id}`} className="trip-card">
      <div className="cover" style={style}>
        <span className="badge">{running ? `Day ${daysBetween(t.start_date, today) + 1} of ${t.days}` : until ? `in ${until} days` : STATUS_LABEL[t.status]}</span>
        {t.days > 0 && <span className="days">{plural(t.days, 'day')}</span>}
      </div>
      <div className="body">
        <h3>{t.name}</h3>
        <div className="muted small">{t.destination || '—'} · {fmtRange(t.start_date, t.end_date)}</div>
        {c && <div className="facts">
          <span title="Places (visited)"><Icon name="pin" size={13} />{c.visited > 0 ? `${c.visited}/${c.places}` : c.places}</span>
          {c.lodging > 0 && <span title="Stays"><Icon name="bed" size={13} />{c.lodging}</span>}
          {c.transport > 0 && <span title="Transport legs"><Icon name="plane" size={13} />{c.transport}</span>}
          {c.checklist_items > 0 && <span title="Checklists"><Icon name="checklist" size={13} />{c.checklist_done}/{c.checklist_items}</span>}
          {(c.total > 0 || c.planned > 0) && <span className="spacer right" title={c.total > 0 ? "Spent plus booked stays and transport" : "Planned"}><Money minor={c.total > 0 ? c.total : c.planned} code={t.currency} compact />{c.total === 0 && <span className="muted"> planned</span>}</span>}
        </div>}
        {t.budget > 0 && c && <div className={`budget-bar ${c.actual > t.budget ? 'over' : ''}`} style={{ height: 6 }}><div className="actual" style={{ width: `${Math.min(100, c.actual / t.budget * 100)}%` }} /><div className="planned" style={{ width: `${Math.max(0, Math.min(100 - c.actual / t.budget * 100, (c.planned - c.actual) / t.budget * 100))}%` }} /></div>}
      </div>
    </Link>
  )
}
