import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { HomeData, Trip } from '../lib/types'
import { PLACE_ICONS, STATUS_LABEL } from '../lib/types'
import { daysBetween, fmtClock, fmtRange, plural, todayISO } from '../lib/format'
import { Card, Empty, Loading, PageHeader, useAsync } from '../components/ui'
import { Money } from '../components/Money'

export default function Home() {
  const { user } = useApp()
  const { data, error, loading } = useAsync(() => api.get<HomeData>(`${V1}/home`), [])
  if (loading && !data) return <Loading />
  if (error) return <div className="callout danger">{error}</div>
  if (!data) return null
  const { stats, current, next, upcoming, recent, today } = data
  const hour = new Date().getHours()
  const greet = hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening'
  return (
    <>
      <PageHeader title={`${greet}, ${(user?.name || '').split(' ')[0] || 'traveller'}`} sub={current ? `You are on the road: ${current.name}, day ${today?.day_number} of ${current.days}.` : next ? `Next up: ${next.name} in ${daysBetween(todayISO(), next.start_date)} days.` : 'Nothing planned yet. Where to next?'} actions={<Link className="btn primary" to="/trips?new=1">+ New trip</Link>} />
      <div className="stats">
        <Link className="stat" to="/trips"><div className="n">{stats.trips}</div><div className="l">trips</div></Link>
        <div className="stat"><div className="n">{stats.days}</div><div className="l">days travelled</div></div>
        <div className="stat"><div className="n">{stats.visited}<span className="muted" style={{ fontSize: 14 }}> / {stats.places}</span></div><div className="l">places visited</div></div>
        <div className="stat"><div className="n">{stats.destinations.length}</div><div className="l">destinations</div></div>
      </div>
      {current && today && (
        <Card title={<span>Today · {current.name}</span>} actions={<Link className="btn sm" to={`/trips/${current.id}/itinerary`}>Open itinerary</Link>} className="mb">
          <div className="overview-grid">
            <div>
              <div className="muted small mb-sm">Day {today.day_number} of {current.days}</div>
              {today.places.length === 0 ? <p className="muted">Nothing scheduled for today — a free day, or drag some ideas onto it.</p> : (
                <div className="day-items">
                  {today.places.map(p => (
                    <Link key={p.id} className="item" to={`/trips/${current.id}/itinerary`} style={{ color: 'inherit' }}>
                      <span className="time">{fmtClock(p.start_time)}</span>
                      <span className={`ico ${p.status === 'visited' ? 'visited' : ''}`}>{PLACE_ICONS[p.category] || '📍'}</span>
                      <span className="txt"><span className="title">{p.name}</span><span className="sub">{p.address || p.notes}</span></span>
                    </Link>
                  ))}
                </div>
              )}
            </div>
            <div className="grid" style={{ gap: 12 }}>
              {today.stay && <div><div className="muted small">Staying at</div><b>{today.stay.name}</b><div className="small muted">{today.stay.address}</div></div>}
              <div>
                <div className="muted small">Spent so far</div>
                <div style={{ fontSize: 22, fontWeight: 700 }}><Money minor={today.budget.actual} code={today.budget.currency} /></div>
                {today.budget.budget > 0 && <div className="small muted"><Money minor={today.budget.remaining} code={today.budget.currency} /> left of the budget · ~<Money minor={today.budget.per_day} code={today.budget.currency} /> a day</div>}
              </div>
            </div>
          </div>
        </Card>
      )}
      {!current && next && (
        <Card className="mb">
          <div className="next-up">
            <div className="countdown">{daysBetween(todayISO(), next.start_date)}<small>days until</small></div>
            <div className="grow">
              <h2><Link to={`/trips/${next.id}`}>{next.name}</Link></h2>
              <div className="muted">{next.destination} · {fmtRange(next.start_date, next.end_date)} · {plural(next.days, 'day')}</div>
              {next.counts && <div className="small muted mt-sm">{plural(next.counts.places, 'place')} planned · {next.counts.checklist_done}/{next.counts.checklist_items} checklist items done{next.counts.lodging === 0 && ' · no stay booked yet'}</div>}
            </div>
            <Link className="btn primary" to={`/trips/${next.id}`}>Plan</Link>
          </div>
        </Card>
      )}
      <div className="two-col">
        <Card title="Upcoming" actions={<Link className="btn ghost sm" to="/trips">All trips</Link>}>
          {upcoming.length === 0 ? <Empty title="No upcoming trips">Create one and start collecting ideas.</Empty> : <TripList trips={upcoming} />}
        </Card>
        <Card title="Past trips">
          {recent.length === 0 ? <Empty title="Nothing yet">Finished trips show up here with what they cost and where you went.</Empty> : <TripList trips={recent} />}
        </Card>
      </div>
    </>
  )
}

function TripList({ trips }: { trips: Trip[] }) {
  return (
    <table className="table compact">
      <tbody>
        {trips.map(t => (
          <tr key={t.id} className="clickable" onClick={() => (window.location.href = `/trips/${t.id}`)}>
            <td><Link to={`/trips/${t.id}`} onClick={e => e.stopPropagation()}><b>{t.name}</b></Link><div className="small muted">{t.destination}</div></td>
            <td className="muted small">{fmtRange(t.start_date, t.end_date)}</td>
            <td className="right"><span className="badge">{STATUS_LABEL[t.status]}</span></td>
            <td className="num small">{t.counts && t.counts.total > 0 ? <Money minor={t.counts.total} code={t.currency} compact /> : t.counts && t.counts.planned > 0 ? <span className="muted"><Money minor={t.counts.planned} code={t.currency} compact /></span> : ''}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
