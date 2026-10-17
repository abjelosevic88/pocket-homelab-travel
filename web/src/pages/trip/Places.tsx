import { useEffect, useState } from 'react'
import { api, uploadForm, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import type { GeoResult, Place, PlaceStatus, Trip } from '../../lib/types'
import { PLACE_ICONS, PLACE_STATUS_LABEL } from '../../lib/types'
import { appleMaps, cap, dateRange, fmtClock, fmtDate, gmaps, stars } from '../../lib/format'
import { Confirm, Empty, Field, Modal, Segmented, useToast } from '../../components/ui'
import { Select } from '../../components/Select'
import { AmountInput, Money } from '../../components/Money'
import { GeoSearch } from '../../components/GeoSearch'
import { MapView } from '../../components/Map'
import { useTrip } from './ctx'
import { SuggestModal, describePlace } from './ai'

export function Places() {
  const { ov, trip, canEdit, edit } = useTrip()
  const { meta } = useApp()
  const [q, setQ] = useState('')
  const [cat, setCat] = useState('')
  const [status, setStatus] = useState('')
  const [view, setView] = useState<'cards' | 'table'>('cards')
  const [suggest, setSuggest] = useState(false)
  const list = ov.places.filter(p => (!q || `${p.name} ${p.address} ${p.tags} ${p.notes}`.toLowerCase().includes(q.toLowerCase())) && (!cat || p.category === cat) && (!status || p.status === status))
  const cats = Array.from(new Set(ov.places.map(p => p.category)))
  return (
    <>
      <div className="toolbar">
        <div className="search"><span className="ico">⌕</span><input value={q} onChange={e => setQ(e.target.value)} placeholder="Filter places…" /></div>
        <Select value={cat} onChange={setCat} options={cats.map(c => ({ value: c, label: `${PLACE_ICONS[c] || ''} ${cap(c)}` }))} placeholder="All categories" clearLabel="All categories" />
        <Select value={status} onChange={setStatus} options={Object.entries(PLACE_STATUS_LABEL).map(([v, l]) => ({ value: v, label: l }))} placeholder="Any status" clearLabel="Any status" />
        <Segmented value={view} onChange={setView} options={[{ value: 'cards', label: '▦' }, { value: 'table', label: '☰' }]} size="sm" />
        {canEdit && ov.integrations.ai.configured && <button className="btn" onClick={() => setSuggest(true)}>✨ Suggest</button>}
        {canEdit && <button className="btn primary" onClick={() => edit('place', null, {})}>+ Place</button>}
      </div>
      {suggest && <SuggestModal onClose={() => setSuggest(false)} />}
      {list.length === 0 ? <Empty title={ov.places.length ? 'Nothing matches' : 'No places yet'}>{!ov.places.length && (ov.integrations.ai.configured ? 'Add the sights, restaurants and beaches you want to see, or press ✨ Suggest for popular places at the destination.' : 'Add the sights, restaurants and beaches you want to see. Use the map tab to discover what is nearby.')}</Empty> : view === 'cards' ? (
        <div className="pool">
          {list.map(p => (
            <div key={p.id} className="place-card" onClick={() => edit('place', p)}>
              <div className="img" style={p.has_image ? { backgroundImage: `url(${V1}/trips/${trip.id}/places/${p.id}/image?v=${p.updated_at})` } : undefined}>{!p.has_image && (PLACE_ICONS[p.category] || '📍')}</div>
              <div className="pc-body">
                <h4><span className={`status-dot ${p.status}`} /> {p.name}</h4>
                <div className="sub">{cap(p.category)}{p.day_date ? ` · ${fmtDate(p.day_date, false)}${p.start_time ? ' ' + fmtClock(p.start_time) : ''}` : ' · unscheduled'}</div>
                {p.address && <div className="sub">{p.address}</div>}
                <div className="row small" style={{ gap: 8 }}>{p.rating ? <span className="stars">{stars(p.rating)}</span> : null}{p.price > 0 && <Money minor={p.price} code={p.currency || trip.currency} />}{p.status === 'booked' && <span className="badge warn">booked</span>}</div>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <table className="table compact">
          <thead><tr><th /><th>Name</th><th>Category</th><th>When</th><th>Status</th><th className="num">Price</th></tr></thead>
          <tbody>{list.map(p => <tr key={p.id} className="clickable" onClick={() => edit('place', p)}><td>{PLACE_ICONS[p.category] || '📍'}</td><td><b>{p.name}</b><div className="small muted">{p.address}</div></td><td className="muted">{cap(p.category)}</td><td className="muted">{p.day_date ? `${fmtDate(p.day_date, false)} ${fmtClock(p.start_time)}` : '—'}</td><td><span className={`status-dot ${p.status}`} /> {PLACE_STATUS_LABEL[p.status]}</td><td className="num">{p.price > 0 && <Money minor={p.price} code={p.currency || trip.currency} />}</td></tr>)}</tbody>
        </table>
      )}
    </>
  )
}

const RATINGS = [1, 2, 3, 4, 5]

export function PlaceModal({ trip, item, defaults = {}, onClose, onSaved }: { trip: Trip; item: Place | null; defaults?: Record<string, unknown>; onClose: () => void; onSaved: () => void }) {
  const { meta } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<Partial<Place>>(item ? { ...item } : { name: '', category: 'sight', status: 'idea', currency: trip.currency, price: 0, day_date: '', start_time: '', duration_min: 0, address: '', link: '', phone: '', notes: '', description: '', tags: '', booking_ref: '', lat: null, lng: null, rating: null, ...defaults })
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState(false)
  const [tab, setTab] = useState<'main' | 'more'>('main')
  const [wikiBusy, setWikiBusy] = useState(false)
  const [aiBusy, setAiBusy] = useState(false)
  const { settings } = useApp()
  const set = <K extends keyof Place>(k: K, v: Place[K]) => setF(x => ({ ...x, [k]: v }))
  const base = `${V1}/trips/${trip.id}/places`
  useEffect(() => { if (f.day_date && f.status === 'idea') set('status', 'planned') }, [f.day_date]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      const body = { ...f, price: f.price || 0, duration_min: f.duration_min || 0 }
      if (item) await api.put(`${base}/${item.id}`, body); else await api.post(base, body)
      toast(item ? 'Place saved' : 'Place added'); onSaved()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const remove = async () => { try { await api.del(`${base}/${item!.id}`); toast('Place removed'); onSaved() } catch (e) { toast((e as Error).message, 'err') } }
  const pick = (r: GeoResult) => { setF(x => ({ ...x, name: x.name || r.name, address: r.display, lat: r.lat, lng: r.lng, link: x.link || r.website || '', phone: x.phone || r.phone || '' })) }
  const wiki = async () => {
    setWikiBusy(true)
    try { const w = await api.get<{ extract: string; url: string }>(`${V1}/geo/wiki?q=${encodeURIComponent(f.name || '')}`); set('description', w.extract); if (!f.link) set('link', w.url) } catch (e) { toast((e as Error).message, 'err') } finally { setWikiBusy(false) }
  }
  const describe = async () => {
    setAiBusy(true)
    try { const d = await describePlace(trip.id, { place_id: item?.id, name: f.name || '', address: f.address, category: f.category }); setF(x => ({ ...x, description: d.description, notes: x.notes ? x.notes + '\n\n' + d.tips : d.tips, duration_min: x.duration_min || d.duration_min })) } catch (e) { toast((e as Error).message, 'err') } finally { setAiBusy(false) }
  }
  const upload = async (file: File) => {
    if (!item) return
    try { await uploadForm(`${base}/${item.id}/image`, file); toast('Image saved'); onSaved() } catch (e) { toast((e as Error).message, 'err') }
  }
  const days = dateRange(trip.start_date, trip.end_date)
  const marker = f.lat != null && f.lng != null ? [{ id: 'p', lat: f.lat, lng: f.lng!, label: f.name || '', icon: PLACE_ICONS[f.category || 'sight'] }] : []
  return (
    <Modal title={item ? 'Edit place' : 'Add a place'} onClose={onClose} size="lg">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <div className="tabs sub"><button type="button" className={tab === 'main' ? 'active' : ''} onClick={() => setTab('main')}>Details</button><button type="button" className={tab === 'more' ? 'active' : ''} onClick={() => setTab('more')}>Location, notes & photo</button></div>
        {tab === 'main' && <>
          <div className="two-col tight">
            <Field label="Name"><input value={f.name || ''} onChange={e => set('name', e.target.value)} required autoFocus={!item} maxLength={120} /></Field>
            <Field label="Category"><Select value={f.category || 'sight'} onChange={v => set('category', v)} options={(meta?.place_categories || []).map(c => ({ value: c, label: `${PLACE_ICONS[c] || ''} ${cap(c)}` }))} /></Field>
          </div>
          {!item && <Field label="Find it on the map" help="Search OpenStreetMap; picking a result fills the address and coordinates."><GeoSearch onPick={pick} /></Field>}
          <div className="two-col tight">
            <Field label="Day"><Select value={f.day_date || ''} onChange={v => set('day_date', v)} options={days.map((d, i) => ({ value: d, label: `Day ${i + 1} · ${fmtDate(d, false)}` }))} placeholder="Unscheduled (idea)" clearLabel="Unscheduled" /></Field>
            <div className="two-col tight">
              <Field label="Time"><input type="time" value={f.start_time || ''} onChange={e => set('start_time', e.target.value)} /></Field>
              <Field label="Duration (min)"><input type="number" min={0} step={15} value={f.duration_min || ''} onChange={e => set('duration_min', Number(e.target.value))} placeholder="90" /></Field>
            </div>
          </div>
          <Field group label="Status"><Segmented value={f.status || 'idea'} onChange={v => set('status', v as PlaceStatus)} options={Object.entries(PLACE_STATUS_LABEL).map(([v, l]) => ({ value: v, label: l }))} size="sm" /></Field>
          <div className="two-col tight">
            <Field label="Price / entry fee"><AmountInput amount={f.price ?? null} currency={f.currency || trip.currency} onAmount={m => set('price', m || 0)} onCurrency={c => set('currency', c)} /></Field>
            <Field group label="Rating"><div className="rating-pick">{RATINGS.map(r => <button type="button" key={r} className={(f.rating || 0) >= r ? 'on' : ''} onClick={() => set('rating', f.rating === r ? null : r)}>★</button>)}</div></Field>
          </div>
          <div className="two-col tight">
            <Field label="Link"><input type="url" value={f.link || ''} onChange={e => set('link', e.target.value)} placeholder="https://" /></Field>
            <Field label="Booking reference"><input value={f.booking_ref || ''} onChange={e => set('booking_ref', e.target.value)} /></Field>
          </div>
        </>}
        {tab === 'more' && <>
          <Field label="Address"><input value={f.address || ''} onChange={e => set('address', e.target.value)} /></Field>
          <Field label="Move the pin" help="Click the map to set the coordinates; search above to jump somewhere."><GeoSearch onPick={pick} placeholder="Search to re-centre…" /><MapView className="sm" markers={marker} center={f.lat != null ? [f.lat, f.lng!] : trip.lat != null ? [trip.lat, trip.lng!] : undefined} zoom={13} onClick={(lat, lng) => setF(x => ({ ...x, lat, lng }))} fitKey={`${f.lat}:${f.lng}`} /></Field>
          <div className="two-col tight">
            <Field label="Phone"><input value={f.phone || ''} onChange={e => set('phone', e.target.value)} /></Field>
            <Field label="Tags" help="Comma separated."><input value={f.tags || ''} onChange={e => set('tags', e.target.value)} placeholder="sunset, kids, rainy day" /></Field>
          </div>
          <Field label={<span className="row">Description <button type="button" className="btn ghost sm" onClick={wiki} disabled={wikiBusy || !f.name}>{wikiBusy ? 'Looking up…' : 'Fill from Wikipedia'}</button>{settings?.openai_configured && <button type="button" className="btn ghost sm" onClick={describe} disabled={aiBusy || !f.name}>{aiBusy ? 'Writing…' : '✨ Describe + tips'}</button>}</span>}><textarea value={f.description || ''} onChange={e => set('description', e.target.value)} rows={4} /></Field>
          <Field label="Notes" help="Tips, opening hours, what to order."><textarea value={f.notes || ''} onChange={e => set('notes', e.target.value)} rows={3} /></Field>
          {item && <Field label="Photo"><div className="row">{item.has_image && <img src={`${V1}/trips/${trip.id}/places/${item.id}/image?v=${item.updated_at}`} alt="" style={{ height: 60, borderRadius: 8 }} />}<label className="btn sm">Upload image…<input type="file" accept="image/*" hidden onChange={e => e.target.files?.[0] && upload(e.target.files[0])} /></label></div></Field>}
        </>}
        <div className="form-actions">
          {item && <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}>Delete</button>}
          {item && f.lat != null && <><a className="btn ghost sm" href={gmaps(f.lat, f.lng!)} target="_blank" rel="noreferrer">Google Maps</a><a className="btn ghost sm" href={appleMaps(f.lat, f.lng!)} target="_blank" rel="noreferrer">Apple Maps</a></>}
          <span className="spacer" />
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={busy}>{item ? 'Save' : 'Add place'}</button>
        </div>
      </form>
      {del && <Confirm title={`Remove ${item?.name}?`} message="The place and its photo are deleted from this trip." onCancel={() => setDel(false)} onConfirm={remove} />}
    </Modal>
  )
}
