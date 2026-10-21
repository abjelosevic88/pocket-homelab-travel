import { useEffect, useState } from 'react'
import { api, V1 } from '../../lib/api'
import { PLACE_ICONS } from '../../lib/types'
import { cap, fmtDate, fmtDuration, weekday } from '../../lib/format'
import { Field, Modal, Segmented, useToast } from '../../components/ui'
import { Icon } from '../../components/icons'
import { useTrip } from './ctx'

/** "✨ Suggest places": the model proposes popular places; picked ones become places. */
export function SuggestModal({ onClose }: { onClose: () => void }) {
  const { trip, ov, reload } = useTrip()
  const { toast } = useToast()
  const [interests, setInterests] = useState('')
  const [near, setNear] = useState('')
  const [count, setCount] = useState(8)
  const [busy, setBusy] = useState(false)
  const [res, setRes] = useState<{ name: string; category: string; why: string; area: string; lat: number | null; lng: number | null; address: string; website?: string; geocoded: boolean; duration_min: number; best_time: string; exists: boolean }[] | null>(null)
  const [sel, setSel] = useState<Set<number>>(new Set())
  const [adding, setAdding] = useState(false)
  const run = async () => {
    setBusy(true); setRes(null)
    try { const r = await api.post<{ suggestions: NonNullable<typeof res> }>(`${V1}/trips/${trip.id}/ai/suggest`, { interests, near, count }); setRes(r.suggestions); setSel(new Set(r.suggestions.map((_, i) => i).filter(i => !r.suggestions[i].exists))) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const add = async () => {
    if (!res) return
    setAdding(true)
    let n = 0
    for (const i of sel) {
      const s = res[i]
      try {
        await api.post(`${V1}/trips/${trip.id}/places`, { name: s.name, category: s.category, description: s.why, address: s.address, lat: s.lat, lng: s.lng, link: s.website || '', duration_min: s.duration_min || 0, notes: s.best_time && s.best_time !== 'any' ? `Best time: ${s.best_time}` : '', status: 'idea', tags: 'suggested' })
        n++
      } catch (e) { toast(`${s.name}: ${(e as Error).message}`, 'err') }
    }
    setAdding(false); toast(`Added ${n} place${n === 1 ? '' : 's'} as ideas`); await reload(); onClose()
  }
  return (
    <Modal title={<span className="row"><Icon name="sparkles" size={17} />Suggest places</span>} onClose={onClose} size="lg">
      {!res && (
        <div className="grid" style={{ gap: 14 }}>
          <p className="muted small">The model proposes popular, well-rated places for <b>{trip.destination || trip.name}</b> that are not in your plan yet ({ov.places.length} places known). Each suggestion is located with OpenStreetMap before it is shown.</p>
          <Field label="What are you into? (optional)"><input value={interests} onChange={e => setInterests(e.target.value)} placeholder="seafood, hidden beaches, kid-friendly, history, no hikes over 2 h…" autoFocus /></Field>
          <div className="two-col tight">
            <Field label="Around (optional)"><input value={near} onChange={e => setNear(e.target.value)} placeholder="the old town, near our hotel, Montmartre…" /></Field>
            <Field group label="How many"><Segmented value={count} onChange={setCount} options={[5, 8, 12].map(n => ({ value: n, label: String(n) }))} size="sm" /></Field>
          </div>
          <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={run} disabled={busy}>{busy ? 'Thinking and locating… ~15 s' : 'Suggest'}</button></div>
        </div>
      )}
      {res && (
        <div className="grid" style={{ gap: 10 }}>
          {res.length === 0 && <p className="muted">Nothing came back — try different interests.</p>}
          {res.map((s, i) => (
            <label key={i} className={`item ${s.exists ? 'ghost' : ''}`} style={{ cursor: 'pointer', alignItems: 'flex-start' }}>
              <input type="checkbox" checked={sel.has(i)} disabled={s.exists} onChange={e => setSel(x => { const n = new Set(x); if (e.target.checked) n.add(i); else n.delete(i); return n })} style={{ width: 'auto', marginTop: 4 }} />
              <span className="ico">{PLACE_ICONS[s.category] || '📍'}</span>
              <span className="txt">
                <span className="title">{s.name}{s.exists && <span className="badge">already in the plan</span>}{!s.geocoded && <span className="badge warn" title="OpenStreetMap did not find it; coordinates are the model's guess">approx. location</span>}</span>
                <span className="sub wrap">{s.why}</span>
                <span className="sub wrap muted">{[cap(s.category), s.area, s.duration_min ? fmtDuration(s.duration_min) : '', s.best_time && s.best_time !== 'any' ? `best ${s.best_time}` : '', s.address].filter(Boolean).join(' · ')}</span>
              </span>
            </label>
          ))}
          <div className="form-actions"><button type="button" className="btn ghost" onClick={() => setRes(null)}>Ask again</button><span className="spacer" /><button type="button" className="btn" onClick={onClose}>Close</button><button type="button" className="btn primary" onClick={add} disabled={adding || sel.size === 0}>{adding ? 'Adding…' : `Add ${sel.size} as ideas`}</button></div>
          <p className="muted small">Added places land in <b>Ideas</b> with the tag <code>suggested</code>; drag them onto days or let <i>Plan my days</i> do it. Double-check facts before you book anything.</p>
        </div>
      )}
    </Modal>
  )
}

/** "✨ Plan my days": the model assigns movable places to days and times; apply writes the itinerary. */
export function PlanModal({ onClose }: { onClose: () => void }) {
  const { trip, reload } = useTrip()
  const { toast } = useToast()
  const [pace, setPace] = useState<'relaxed' | '' | 'packed'>('')
  const [notes, setNotes] = useState('')
  const [reschedule, setReschedule] = useState(false)
  const [busy, setBusy] = useState(false)
  const [plan, setPlan] = useState<{ items: { place_id: number; name: string; day_date: string; start_time: string; duration_min: number; note: string }[]; summary: string; unplaced: number } | null>(null)
  const run = async () => {
    setBusy(true)
    try { setPlan(await api.post(`${V1}/trips/${trip.id}/ai/plan`, { pace, notes, reschedule })) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const apply = async () => {
    if (!plan) return
    setBusy(true)
    try { const r = await api.post<{ applied: number }>(`${V1}/trips/${trip.id}/ai/plan`, { apply: true, items: plan.items }); toast(`Scheduled ${r.applied} places`); await reload(); onClose() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const days = plan ? Array.from(new Set(plan.items.map(i => i.day_date))) : []
  return (
    <Modal title={<span className="row"><Icon name="sparkles" size={17} />Plan my days</span>} onClose={onClose} size="lg">
      {!plan ? (
        <div className="grid" style={{ gap: 14 }}>
          <p className="muted small">Unscheduled places get a day and a start time: nearby things on the same day, meals at meal times, light arrival and departure days. Booked places and transport stay where they are. You review the proposal before anything changes.</p>
          <Field group label="Pace"><Segmented value={pace} onChange={setPace} options={[{ value: 'relaxed', label: 'Relaxed' }, { value: '', label: 'Normal' }, { value: 'packed', label: 'Packed' }]} size="sm" /></Field>
          <Field label="Wishes (optional)"><input value={notes} onChange={e => setNotes(e.target.value)} placeholder="beach every afternoon, museums on a weekday, nothing before 9…" /></Field>
          <label className="row"><input type="checkbox" checked={reschedule} onChange={e => setReschedule(e.target.checked)} style={{ width: 'auto' }} /> Also move places that already have a day (except booked ones)</label>
          <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={run} disabled={busy}>{busy ? 'Planning…' : 'Propose a plan'}</button></div>
        </div>
      ) : (
        <div className="grid" style={{ gap: 12 }}>
          {plan.summary && <div className="callout">{plan.summary}</div>}
          {days.map(d => (
            <div key={d}>
              <h3 style={{ marginBottom: 6 }}>{weekday(d, true)}, {fmtDate(d, false)}</h3>
              <div className="day-items">{plan.items.filter(i => i.day_date === d).map(i => (
                <div key={i.place_id} className="item" style={{ cursor: 'default' }}>
                  <span className="time"><input type="time" value={i.start_time} onChange={e => setPlan(p => p && ({ ...p, items: p.items.map(x => x.place_id === i.place_id ? { ...x, start_time: e.target.value } : x) }))} style={{ padding: '2px 4px', fontSize: 12 }} /></span>
                  <span className="txt"><span className="title">{i.name}</span><span className="sub wrap">{[i.duration_min ? fmtDuration(i.duration_min) : '', i.note].filter(Boolean).join(' · ')}</span></span>
                  <button type="button" className="icon-btn" title="Leave unscheduled" aria-label="Leave unscheduled" onClick={() => setPlan(p => p && ({ ...p, items: p.items.filter(x => x.place_id !== i.place_id) }))}><Icon name="x" size={14} /></button>
                </div>
              ))}</div>
            </div>
          ))}
          {plan.unplaced > 0 && <p className="muted small">{plan.unplaced} place{plan.unplaced === 1 ? '' : 's'} left unscheduled on purpose — not enough days at this pace.</p>}
          <div className="form-actions"><button type="button" className="btn ghost" onClick={() => setPlan(null)}>Try again</button><span className="spacer" /><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={apply} disabled={busy || plan.items.length === 0}>{busy ? 'Applying…' : `Apply ${plan.items.length} slots`}</button></div>
        </div>
      )}
    </Modal>
  )
}

/** "✨ Write a recap": story / summary / caption from the places, journal and spending. */
export function RecapModal({ onClose }: { onClose: () => void }) {
  const { trip, reload } = useTrip()
  const { toast } = useToast()
  const [style, setStyle] = useState<'story' | 'summary' | 'social'>('story')
  const [busy, setBusy] = useState(false)
  const [text, setText] = useState('')
  const run = async () => {
    setBusy(true)
    try { const r = await api.post<{ text: string }>(`${V1}/trips/${trip.id}/ai/recap`, { style }); setText(r.text) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const save = async () => {
    setBusy(true)
    try { await api.post(`${V1}/trips/${trip.id}/journal`, { date: trip.end_date || undefined, title: style === 'social' ? 'Caption' : 'Recap', body: text, mood: '✨' }); toast('Saved to the journal'); await reload(); onClose() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  useEffect(() => { run() }, []) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Modal title={<span className="row"><Icon name="sparkles" size={17} />Trip recap</span>} onClose={onClose} size="lg">
      <div className="grid" style={{ gap: 12 }}>
        <div className="row"><Segmented value={style} onChange={v => { setStyle(v); setText('') }} options={[{ value: 'story', label: 'Story' }, { value: 'summary', label: 'Summary' }, { value: 'social', label: 'Caption' }]} size="sm" /><button type="button" className="btn sm" onClick={run} disabled={busy}>{busy ? 'Writing…' : text ? 'Rewrite' : 'Write'}</button></div>
        <textarea className="tall" value={text} onChange={e => setText(e.target.value)} placeholder={busy ? 'Writing from your places, journal and spending…' : ''} style={{ minHeight: 320 }} />
        <p className="muted small">Written only from what the trip knows: visited places, journal entries, spending, recorded distance. Edit freely, then save it as a journal entry.</p>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Close</button><button type="button" className="btn primary" onClick={save} disabled={busy || !text.trim()}>Save to journal</button></div>
      </div>
    </Modal>
  )
}

/** Shared "describe" helper for the place form. */
export async function describePlace(tripId: number, p: { place_id?: number; name: string; address?: string; category?: string }) {
  return api.post<{ description: string; tips: string; duration_min: number; best_time: string }>(`${V1}/trips/${tripId}/ai/describe`, p)
}

