import { useState } from 'react'
import { api, V1 } from '../../lib/api'
import type { JournalEntry, Trip } from '../../lib/types'
import { dateRange, fmtDate, todayISO, weekday } from '../../lib/format'
import { Card, Confirm, Empty, Field, Modal, useToast } from '../../components/ui'
import { Icon } from '../../components/icons'
import { Select } from '../../components/Select'
import { useTrip } from './ctx'

const MOODS = ['😊', '🤩', '😌', '😴', '🥾', '🍷', '🌧', '😅', '🤒', '😍']

export function Journal() {
  const { ov, trip, canEdit, edit } = useTrip()
  const today = todayISO()
  const inRange = trip.start_date && trip.start_date <= today && trip.end_date >= today
  const byDate = new Map<string, JournalEntry[]>()
  for (const j of ov.journal) byDate.set(j.date, [...(byDate.get(j.date) || []), j])
  const dates = Array.from(byDate.keys()).sort()
  return (
    <div className="wrap-sm">
      <div className="toolbar"><span className="muted">A few lines a day beats a blank page later. Entries from everyone on the trip, in order.</span><span className="spacer" />{canEdit && <button type="button" className="btn primary sm" onClick={() => edit('journal', null, { date: inRange ? today : trip.end_date || today })}><Icon name="plus" size={14} />Entry</button>}</div>
      {dates.length === 0 ? <Empty icon="book" title="No entries yet">{canEdit && 'Write the first one — what you saw, ate, the thing that went wrong.'}</Empty> : dates.map(d => (
        <Card key={d} className="mb" title={<span>{weekday(d, true)}, {fmtDate(d)}{trip.start_date && d >= trip.start_date && <span className="muted small"> · day {dateRange(trip.start_date, d).length}</span>}</span>}>
          {byDate.get(d)!.map(j => (
            <div key={j.id} className="journal-entry" onClick={() => canEdit && edit('journal', j)} style={{ cursor: canEdit ? 'pointer' : 'default' }}>
              <div className="j-head">{j.mood && <span style={{ fontSize: 18 }}>{j.mood}</span>}{j.title && <b>{j.title}</b>}<span className="muted small">{j.user_name}</span></div>
              <div className="j-body">{j.body}</div>
            </div>
          ))}
        </Card>
      ))}
    </div>
  )
}

export function JournalModal({ trip, item, defaults = {}, onClose, onSaved }: { trip: Trip; item: JournalEntry | null; defaults?: Record<string, unknown>; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState<Partial<JournalEntry>>(item ? { ...item } : { date: todayISO(), title: '', body: '', mood: '', ...defaults })
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState(false)
  const base = `${V1}/trips/${trip.id}/journal`
  const days = dateRange(trip.start_date, trip.end_date)
  const dayOpts = days.map((d, i) => ({ value: d, label: `Day ${i + 1} · ${fmtDate(d, false)}` }))
  if (f.date && !days.includes(f.date)) dayOpts.push({ value: f.date, label: fmtDate(f.date) })
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (item) await api.put(`${base}/${item.id}`, f); else await api.post(base, f); toast('Saved'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const remove = async () => { try { await api.del(`${base}/${item!.id}`); onSaved() } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Modal title={item ? 'Edit entry' : 'New journal entry'} onClose={onClose} size="lg">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <div className="two-col tight">
          <Field label="Day">{days.length ? <Select value={f.date || ''} onChange={v => setF({ ...f, date: v })} options={dayOpts} /> : <input type="date" value={f.date || ''} onChange={e => setF({ ...f, date: e.target.value })} />}</Field>
          <Field label="Title (optional)"><input value={f.title || ''} onChange={e => setF({ ...f, title: e.target.value })} placeholder="Gaudí day" /></Field>
        </div>
        <Field group label="Mood"><div className="mood-pick">{MOODS.map(m => <button type="button" key={m} className={f.mood === m ? 'on' : ''} onClick={() => setF({ ...f, mood: f.mood === m ? '' : m })}>{m}</button>)}</div></Field>
        <Field label="What happened"><textarea className="tall" value={f.body || ''} onChange={e => setF({ ...f, body: e.target.value })} autoFocus placeholder="Walked the walls before the crowds…" /></Field>
        <div className="form-actions">{item && <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}>Delete</button>}<span className="spacer" /><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
      {del && <Confirm title="Delete this entry?" message="It is gone for good." onCancel={() => setDel(false)} onConfirm={remove} />}
    </Modal>
  )
}
