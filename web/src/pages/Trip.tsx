import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { DayWeather, Expense, JournalEntry, Lodging, Overview, Photo, Place, Route, TimelineDay, Transport, Trip } from '../lib/types'
import { STATUS_LABEL } from '../lib/types'
import { daysBetween, fmtRange, plural, todayISO } from '../lib/format'
import { Loading, Tabs, useToast } from '../components/ui'
import { TripCtx, type EditKind } from './trip/ctx'
import { OverviewTab } from './trip/Overview'
import { Itinerary } from './trip/Itinerary'
import { Places, PlaceModal } from './trip/Places'
import { MapTab } from './trip/MapTab'
import { Stay, LodgingModal, TransportModal } from './trip/Stay'
import { BudgetTab, ExpenseModal } from './trip/Budget'
import { Checklists } from './trip/Lists'
import { Journal, JournalModal } from './trip/Journal'
import { Photos } from './trip/Photos'
import { Documents } from './trip/Documents'
import { TripSettings, TripModal } from './trip/TripSettings'

const TABS = [
  { id: 'overview', label: 'Overview' }, { id: 'itinerary', label: 'Itinerary' }, { id: 'places', label: 'Places' }, { id: 'map', label: 'Map' },
  { id: 'stay', label: 'Stay & transport' }, { id: 'budget', label: 'Budget' }, { id: 'lists', label: 'Checklists' }, { id: 'journal', label: 'Journal' },
  { id: 'photos', label: 'Photos' }, { id: 'documents', label: 'Documents' }, { id: 'settings', label: 'Settings' },
]

export default function TripPage() {
  const { id, tab = 'overview' } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const [ov, setOv] = useState<Overview | null>(null)
  const [err, setErr] = useState('')
  const [weather, setWeather] = useState<DayWeather[]>([])
  const [route, setRoute] = useState<Route | null>(null)
  const [timeline, setTimeline] = useState<TimelineDay[]>([])
  const [photos, setPhotos] = useState<Photo[]>([])
  const [extErrors, setExtErrors] = useState<Record<string, string>>({})
  const [editing, setEditing] = useState<{ kind: EditKind; item?: unknown; defaults?: Record<string, unknown> } | null>(null)
  const base = `${V1}/trips/${id}`

  const reload = useCallback(async () => {
    try { setOv(await api.get<Overview>(`${base}/overview`)); setErr('') } catch (e) { setErr((e as Error).message) }
  }, [base])
  useEffect(() => { reload() }, [reload])

  const loadExternal = useCallback(async (refresh = false) => {
    const q = refresh ? '?refresh=1' : ''
    const errs: Record<string, string> = {}
    await Promise.all([
      api.get<DayWeather[]>(`${base}/weather${q}`).then(setWeather).catch(e => { errs.weather = (e as Error).message }),
      api.get<Route>(`${base}/dawarich/route${q}`).then(setRoute).catch(e => { errs.dawarich = (e as Error).message }),
      api.get<TimelineDay[]>(`${base}/dawarich/timeline${q}`).then(setTimeline).catch(e => { errs.dawarich = (e as Error).message }),
      api.get<Photo[]>(`${base}/photos${q}`).then(setPhotos).catch(e => { errs.immich = (e as Error).message }),
    ])
    setExtErrors(errs)
  }, [base])
  useEffect(() => { if (ov) loadExternal() }, [ov?.trip.id, ov?.trip.start_date, ov?.trip.end_date]) // eslint-disable-line react-hooks/exhaustive-deps

  const refreshExternal = useCallback(async () => {
    try { await api.post(`${base}/refresh`); await reload(); await loadExternal(true); toast('Refreshed') } catch (e) { toast((e as Error).message, 'err') }
  }, [base, reload, loadExternal, toast])

  const patch = useCallback((p: Partial<Overview>) => setOv(o => o ? { ...o, ...p } : o), [])
  const go = useCallback((t: string) => navigate(`/trips/${id}/${t}`), [navigate, id])
  const edit = useCallback((kind: EditKind, item?: unknown, defaults?: Record<string, unknown>) => setEditing({ kind, item, defaults }), [])

  const state = useMemo(() => ov ? ({
    ov, trip: ov.trip, canEdit: ov.trip.role !== 'viewer', isOwner: ov.trip.role === 'owner',
    reload, patch, weather, route, timeline, photos, extErrors, refreshExternal, edit, go,
  }) : null, [ov, reload, patch, weather, route, timeline, photos, extErrors, refreshExternal, edit, go])

  if (err) return <div className="callout danger">{err} — <Link to="/trips">back to trips</Link></div>
  if (!state || !ov) return <Loading />
  const t = ov.trip
  const today = todayISO()
  const running = t.start_date && t.start_date <= today && t.end_date >= today
  const until = t.start_date > today ? daysBetween(today, t.start_date) : 0
  const heroStyle = t.has_cover ? { backgroundImage: `url(${V1}/trips/${t.id}/cover?v=${t.updated_at})` } : { background: `linear-gradient(135deg, ${t.color || '#0284c7'} 0%, #0f172a 100%)` }
  const close = () => setEditing(null)
  const afterSave = async () => { close(); await reload() }
  return (
    <TripCtx.Provider value={state}>
      <div className={`hero ${t.has_cover ? 'has-cover' : ''}`} style={heroStyle}>
        <div className="hero-body">
          <div>
            <div className="row" style={{ gap: 8, marginBottom: 6 }}><span className="status-pill">{running ? `Day ${daysBetween(t.start_date, today) + 1} of ${t.days}` : until ? `In ${until} days` : STATUS_LABEL[t.status]}</span>{t.archived && <span className="status-pill">Archived</span>}</div>
            <h1>{t.name}</h1>
            <div className="meta-line"><span>{t.destination || 'No destination yet'}</span><span>{fmtRange(t.start_date, t.end_date)}</span>{t.days > 0 && <span>{plural(t.days, 'day')}</span>}{t.members && t.members.length > 1 && <span>👥 {t.members.map(m => m.name.split(' ')[0]).join(', ')}</span>}</div>
          </div>
          <div className="row">
            {state.canEdit && <button className="btn ghost" onClick={() => edit('place', null, {})}>+ Place</button>}
            {state.canEdit && <button className="btn ghost" onClick={() => edit('expense', null, { kind: running || t.end_date < today ? 'actual' : 'planned' })}>+ Cost</button>}
            <button className="btn ghost" onClick={() => edit('trip', t)}>Edit</button>
          </div>
        </div>
      </div>
      <Tabs tabs={TABS} value={tab} onChange={go} />
      <div className="mt">
        {tab === 'overview' && <OverviewTab />}
        {tab === 'itinerary' && <Itinerary />}
        {tab === 'places' && <Places />}
        {tab === 'map' && <MapTab />}
        {tab === 'stay' && <Stay />}
        {tab === 'budget' && <BudgetTab />}
        {tab === 'lists' && <Checklists />}
        {tab === 'journal' && <Journal />}
        {tab === 'photos' && <Photos />}
        {tab === 'documents' && <Documents />}
        {tab === 'settings' && <TripSettings />}
      </div>
      {editing?.kind === 'place' && <PlaceModal trip={t} item={editing.item as Place | null} defaults={editing.defaults} onClose={close} onSaved={afterSave} />}
      {editing?.kind === 'lodging' && <LodgingModal trip={t} item={editing.item as Lodging | null} defaults={editing.defaults} onClose={close} onSaved={afterSave} />}
      {editing?.kind === 'transport' && <TransportModal trip={t} item={editing.item as Transport | null} defaults={editing.defaults} onClose={close} onSaved={afterSave} />}
      {editing?.kind === 'expense' && <ExpenseModal trip={t} item={editing.item as Expense | null} defaults={editing.defaults} onClose={close} onSaved={afterSave} />}
      {editing?.kind === 'journal' && <JournalModal trip={t} item={editing.item as JournalEntry | null} defaults={editing.defaults} onClose={close} onSaved={afterSave} />}
      {editing?.kind === 'trip' && <TripModal trip={editing.item as Trip} onClose={close} onSaved={afterSave} />}
    </TripCtx.Provider>
  )
}
