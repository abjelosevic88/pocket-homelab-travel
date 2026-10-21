import { useState } from 'react'
import { api, uploadForm, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import type { GeoResult, Lodging, Transport, Trip } from '../../lib/types'
import { LODGING_ICONS, TRANSPORT_ICONS } from '../../lib/types'
import { appleMaps, cap, daysBetween, fmtDate, fmtDateTime, fmtKm, gmaps, todayISO } from '../../lib/format'
import { Card, Confirm, Empty, Field, Modal, useToast } from '../../components/ui'
import { Icon } from '../../components/icons'
import { Select } from '../../components/Select'
import { AmountInput, Money } from '../../components/Money'
import { GeoSearch } from '../../components/GeoSearch'
import { useTrip } from './ctx'

export function Stay() {
  const { ov, trip, canEdit, edit } = useTrip()
  const { settings } = useApp()
  const today = todayISO()
  const unit = settings?.distance_unit || 'km'
  return (
    <div className="two-col">
      <Card title="Where you sleep" actions={canEdit && <button type="button" className="btn primary sm" onClick={() => edit('lodging', null, {})}><Icon name="plus" size={14} />Stay</button>}>
        {ov.lodging.length === 0 ? <Empty icon="bed" title="No stays yet">Hotels, apartments, a friend's couch — with check-in dates they show up in the itinerary and the calendar export.</Empty> : (
          <div className="day-items">
            {ov.lodging.map(l => {
              const now = l.check_in.slice(0, 10) <= today && today < l.check_out.slice(0, 10)
              const nights = daysBetween(l.check_in.slice(0, 10), l.check_out.slice(0, 10))
              return (
                <div key={l.id} className="item" onClick={() => edit('lodging', l)}>
                  <span className="ico lodging">{LODGING_ICONS[l.type] || '🛏'}</span>
                  <span className="txt">
                    <span className="title">{l.name}{now && <span className="badge cat">current stay</span>}{l.paid && <span className="badge">paid</span>}</span>
                    <span className="sub wrap">{fmtDate(l.check_in, false)} → {fmtDate(l.check_out, false)}{nights > 0 && ` · ${nights} night${nights === 1 ? '' : 's'}`}{l.address && ` · ${l.address}`}</span>
                    <span className="sub wrap">{[l.reservation_number && `Reservation ${l.reservation_number}`, l.phone, l.price > 0 && <Money key="m" minor={l.price} code={l.currency || trip.currency} />].filter(Boolean).map((x, i) => <span key={i}>{i > 0 && ' · '}{x}</span>)}</span>
                  </span>
                  <span className="row" onClick={e => e.stopPropagation()}>{l.link && <a className="icon-btn" href={l.link} target="_blank" rel="noreferrer" title="Booking" aria-label="Booking"><Icon name="link" size={15} /></a>}{l.lat != null && <a className="icon-btn" href={gmaps(l.lat, l.lng!)} target="_blank" rel="noreferrer" title="Directions" aria-label="Directions"><Icon name="navigation" size={15} /></a>}</span>
                </div>
              )
            })}
          </div>
        )}
      </Card>
      <Card title="How you get there" actions={canEdit && <button type="button" className="btn primary sm" onClick={() => edit('transport', null, {})}><Icon name="plus" size={14} />Leg</button>}>
        {ov.transport.length === 0 ? <Empty icon="plane" title="No transport yet">Flights, trains, ferries and drives. Booking references and seats live here, and the calendar export carries them.</Empty> : (
          <div className="day-items">
            {ov.transport.map(t => (
              <div key={t.id} className="item" onClick={() => edit('transport', t)}>
                <span className="ico transport">{TRANSPORT_ICONS[t.type] || '➡️'}</span>
                <span className="txt">
                  <span className="title">{t.name || `${t.from_name} → ${t.to_name}`}{t.paid && <span className="badge">paid</span>}</span>
                  <span className="sub wrap">{t.depart_at && fmtDateTime(t.depart_at)}{t.arrive_at && ` → ${fmtDateTime(t.arrive_at, false)}`}{t.name && (t.from_name || t.to_name) && ` · ${t.from_name} → ${t.to_name}`}</span>
                  <span className="sub wrap">{[t.carrier, t.number, t.booking_ref && `ref ${t.booking_ref}`, t.seat && `seat ${t.seat}`, t.distance_km > 0 && fmtKm(t.distance_km, unit), t.price > 0 && <Money key="m" minor={t.price} code={t.currency || trip.currency} />].filter(Boolean).map((x, i) => <span key={i}>{i > 0 && ' · '}{x}</span>)}</span>
                </span>
                {t.link && <a className="icon-btn" href={t.link} target="_blank" rel="noreferrer" title="Booking" aria-label="Booking" onClick={e => e.stopPropagation()}><Icon name="link" size={15} /></a>}
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}

export function LodgingModal({ trip, item, defaults = {}, onClose, onSaved }: { trip: Trip; item: Lodging | null; defaults?: Record<string, unknown>; onClose: () => void; onSaved: () => void }) {
  const { meta } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<Partial<Lodging>>(item ? { ...item, check_in: item.check_in.slice(0, 10), check_out: item.check_out.slice(0, 10) } : { name: '', type: 'hotel', address: '', check_in: trip.start_date, check_out: trip.end_date, reservation_number: '', link: '', phone: '', price: 0, currency: trip.currency, paid: false, notes: '', lat: null, lng: null, rating: null, ...defaults })
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState(false)
  const set = <K extends keyof Lodging>(k: K, v: Lodging[K]) => setF(x => ({ ...x, [k]: v }))
  const base = `${V1}/trips/${trip.id}/lodging`
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (item) await api.put(`${base}/${item.id}`, f); else await api.post(base, f); toast(item ? 'Stay saved' : 'Stay added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const remove = async () => { try { await api.del(`${base}/${item!.id}`); toast('Stay removed'); onSaved() } catch (e) { toast((e as Error).message, 'err') } }
  const pick = (r: GeoResult) => setF(x => ({ ...x, name: x.name || r.name, address: r.display, lat: r.lat, lng: r.lng, link: x.link || r.website || '', phone: x.phone || r.phone || '' }))
  const upload = async (file: File) => { if (!item) return; try { await uploadForm(`${base}/${item.id}/image`, file); toast('Image saved'); onSaved() } catch (e) { toast((e as Error).message, 'err') } }
  const nights = daysBetween(f.check_in || '', f.check_out || '')
  return (
    <Modal title={item ? 'Edit stay' : 'Add a stay'} onClose={onClose} size="lg">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <div className="two-col tight">
          <Field label="Name"><input value={f.name || ''} onChange={e => set('name', e.target.value)} required autoFocus={!item} /></Field>
          <Field label="Type"><Select value={f.type || 'hotel'} onChange={v => set('type', v)} options={(meta?.lodging_types || []).map(t => ({ value: t, label: `${LODGING_ICONS[t] || ''} ${cap(t)}` }))} /></Field>
        </div>
        <Field label="Find the address"><GeoSearch onPick={pick} placeholder="Search hotel or address…" /></Field>
        <Field label="Address"><input value={f.address || ''} onChange={e => set('address', e.target.value)} /></Field>
        <div className="two-col tight">
          <Field label="Check-in"><input type="date" value={f.check_in || ''} onChange={e => set('check_in', e.target.value)} /></Field>
          <Field label="Check-out" help={nights > 0 ? `${nights} night${nights === 1 ? '' : 's'}` : undefined}><input type="date" value={f.check_out || ''} onChange={e => set('check_out', e.target.value)} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Total price" help="Becomes a planned cost in the budget."><AmountInput amount={f.price ?? null} currency={f.currency || trip.currency} onAmount={m => set('price', m || 0)} onCurrency={c => set('currency', c)} /></Field>
          <Field label="Reservation number"><input value={f.reservation_number || ''} onChange={e => set('reservation_number', e.target.value)} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Booking link"><input type="url" value={f.link || ''} onChange={e => set('link', e.target.value)} placeholder="https://" /></Field>
          <Field label="Phone"><input value={f.phone || ''} onChange={e => set('phone', e.target.value)} /></Field>
        </div>
        <label className="row"><input type="checkbox" checked={!!f.paid} onChange={e => set('paid', e.target.checked)} style={{ width: 'auto' }} /> Paid already</label>
        <Field label="Notes" help="Door codes, parking, breakfast times."><textarea value={f.notes || ''} onChange={e => set('notes', e.target.value)} rows={3} /></Field>
        {item && <Field label="Photo"><div className="row">{item.has_image && <img src={`${V1}/trips/${trip.id}/lodging/${item.id}/image?v=${item.updated_at}`} alt="" style={{ height: 60, borderRadius: 8 }} />}<label className="btn sm"><Icon name="upload" size={14} />Upload image…<input type="file" accept="image/*" hidden onChange={e => e.target.files?.[0] && upload(e.target.files[0])} /></label></div></Field>}
        <div className="form-actions">
          {item && <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}>Delete</button>}
          {item && f.lat != null && <><a className="btn ghost sm" href={gmaps(f.lat, f.lng!)} target="_blank" rel="noreferrer"><Icon name="external" size={13} />Google Maps</a><a className="btn ghost sm" href={appleMaps(f.lat, f.lng!)} target="_blank" rel="noreferrer"><Icon name="external" size={13} />Apple Maps</a></>}
          <span className="spacer" /><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{item ? 'Save' : 'Add stay'}</button>
        </div>
      </form>
      {del && <Confirm title={`Remove ${item?.name}?`} message="The stay and its planned cost are deleted." onCancel={() => setDel(false)} onConfirm={remove} />}
    </Modal>
  )
}

export function TransportModal({ trip, item, defaults = {}, onClose, onSaved }: { trip: Trip; item: Transport | null; defaults?: Record<string, unknown>; onClose: () => void; onSaved: () => void }) {
  const { meta } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<Partial<Transport>>(item ? { ...item } : { type: 'flight', name: '', carrier: '', number: '', from_name: '', to_name: '', depart_at: trip.start_date ? trip.start_date + 'T08:00' : '', arrive_at: '', booking_ref: '', seat: '', link: '', price: 0, currency: trip.currency, paid: false, distance_km: 0, notes: '', from_lat: null, from_lng: null, to_lat: null, to_lng: null, ...defaults })
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState(false)
  const set = <K extends keyof Transport>(k: K, v: Transport[K]) => setF(x => ({ ...x, [k]: v }))
  const base = `${V1}/trips/${trip.id}/transport`
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (item) await api.put(`${base}/${item.id}`, f); else await api.post(base, f); toast(item ? 'Leg saved' : 'Leg added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const remove = async () => { try { await api.del(`${base}/${item!.id}`); toast('Leg removed'); onSaved() } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Modal title={item ? 'Edit transport' : 'Add transport'} onClose={onClose} size="lg">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <div className="two-col tight">
          <Field label="Type"><Select value={f.type || 'flight'} onChange={v => set('type', v)} options={(meta?.transport_types || []).map(t => ({ value: t, label: `${TRANSPORT_ICONS[t] || ''} ${cap(t)}` }))} /></Field>
          <Field label="Name" help="Optional — defaults to from → to."><input value={f.name || ''} onChange={e => set('name', e.target.value)} placeholder="Flight to Lisbon" autoFocus={!item} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="From"><GeoSearch initial={f.from_name} onPick={r => setF(x => ({ ...x, from_name: r.name, from_lat: r.lat, from_lng: r.lng }))} placeholder="Airport, station, city…" /></Field>
          <Field label="To"><GeoSearch initial={f.to_name} onPick={r => setF(x => ({ ...x, to_name: r.name, to_lat: r.lat, to_lng: r.lng }))} placeholder="Airport, station, city…" /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Departs"><input type="datetime-local" value={f.depart_at || ''} onChange={e => set('depart_at', e.target.value)} /></Field>
          <Field label="Arrives"><input type="datetime-local" value={f.arrive_at || ''} onChange={e => set('arrive_at', e.target.value)} /></Field>
        </div>
        <div className="three-col tight">
          <Field label="Carrier"><input value={f.carrier || ''} onChange={e => set('carrier', e.target.value)} placeholder="Ryanair, ÖBB…" /></Field>
          <Field label="Flight / train no."><input value={f.number || ''} onChange={e => set('number', e.target.value)} /></Field>
          <Field label="Seat"><input value={f.seat || ''} onChange={e => set('seat', e.target.value)} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Price" help="Becomes a planned cost in the budget."><AmountInput amount={f.price ?? null} currency={f.currency || trip.currency} onAmount={m => set('price', m || 0)} onCurrency={c => set('currency', c)} /></Field>
          <Field label="Booking reference"><input value={f.booking_ref || ''} onChange={e => set('booking_ref', e.target.value)} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Link"><input type="url" value={f.link || ''} onChange={e => set('link', e.target.value)} placeholder="https://" /></Field>
          <Field label="Distance (km)" help="Filled from the coordinates when left empty."><input type="number" min={0} step="0.1" value={f.distance_km || ''} onChange={e => set('distance_km', Number(e.target.value))} /></Field>
        </div>
        <label className="row"><input type="checkbox" checked={!!f.paid} onChange={e => set('paid', e.target.checked)} style={{ width: 'auto' }} /> Paid already</label>
        <Field label="Notes"><textarea value={f.notes || ''} onChange={e => set('notes', e.target.value)} rows={2} /></Field>
        <div className="form-actions">
          {item && <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}>Delete</button>}
          <span className="spacer" /><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{item ? 'Save' : 'Add leg'}</button>
        </div>
      </form>
      {del && <Confirm title="Remove this leg?" message="The leg and its planned cost are deleted." onCancel={() => setDel(false)} onConfirm={remove} />}
    </Modal>
  )
}
