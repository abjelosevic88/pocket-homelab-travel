import { useState } from 'react'
import { api, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import type { Checklist, ChecklistItem } from '../../lib/types'
import { cap } from '../../lib/format'
import { Card, Confirm, Empty, Field, Modal, Segmented, useToast } from '../../components/ui'
import { Icon } from '../../components/icons'
import { Select } from '../../components/Select'
import { useTrip } from './ctx'

export function Checklists() {
  const { ov, trip, canEdit, patch } = useTrip()
  const { toast } = useToast()
  const [adding, setAdding] = useState(false)
  const [del, setDel] = useState<Checklist | null>(null)
  const base = `${V1}/trips/${trip.id}/checklists`
  const update = (lists: Checklist[]) => patch({ checklists: lists })
  const toggle = async (c: Checklist, it: ChecklistItem) => {
    update(ov.checklists.map(l => l.id === c.id ? { ...l, items: l.items.map(x => x.id === it.id ? { ...x, done: !x.done } : x) } : l))
    try { await api.put(`${base}/items/${it.id}`, { done: !it.done }) } catch (e) { toast((e as Error).message, 'err') }
  }
  const add = async (c: Checklist, text: string) => {
    if (!text.trim()) return
    try { const items = await api.post<ChecklistItem[]>(`${base}/${c.id}/items`, { text }); update(ov.checklists.map(l => l.id === c.id ? { ...l, items: [...l.items, ...items] } : l)) } catch (e) { toast((e as Error).message, 'err') }
  }
  const remove = async (c: Checklist, it: ChecklistItem) => {
    update(ov.checklists.map(l => l.id === c.id ? { ...l, items: l.items.filter(x => x.id !== it.id) } : l))
    try { await api.del(`${base}/items/${it.id}`) } catch (e) { toast((e as Error).message, 'err') }
  }
  const rename = async (c: Checklist, it: ChecklistItem, text: string) => {
    if (!text.trim() || text === it.text) return
    update(ov.checklists.map(l => l.id === c.id ? { ...l, items: l.items.map(x => x.id === it.id ? { ...x, text } : x) } : l))
    try { await api.put(`${base}/items/${it.id}`, { text }) } catch (e) { toast((e as Error).message, 'err') }
  }
  const clearDone = async (c: Checklist) => {
    const done = c.items.filter(i => i.done)
    update(ov.checklists.map(l => l.id === c.id ? { ...l, items: l.items.filter(i => !i.done) } : l))
    for (const it of done) await api.del(`${base}/items/${it.id}`).catch(() => {})
  }
  const resetAll = async (c: Checklist) => {
    update(ov.checklists.map(l => l.id === c.id ? { ...l, items: l.items.map(i => ({ ...i, done: false })) } : l))
    for (const it of c.items.filter(i => i.done)) await api.put(`${base}/items/${it.id}`, { done: false }).catch(() => {})
  }
  const deleteList = async () => { if (!del) return; try { await api.del(`${base}/${del.id}`); update(ov.checklists.filter(l => l.id !== del.id)); setDel(null) } catch (e) { toast((e as Error).message, 'err') } }
  const total = ov.checklists.reduce((n, c) => n + c.items.length, 0), done = ov.checklists.reduce((n, c) => n + c.items.filter(i => i.done).length, 0)
  return (
    <>
      <div className="toolbar"><span className="muted">{total ? `${done} of ${total} done · ${Math.round(done / total * 100)}%` : 'Packing lists, to-dos before leaving, shopping — anything with a tick box.'}</span><span className="spacer" />{canEdit && <button type="button" className="btn primary sm" onClick={() => setAdding(true)}><Icon name="plus" size={14} />List</button>}</div>
      {ov.checklists.length === 0 ? <Empty icon="checklist" title="No checklists">Start from a template: beach, city, hiking, winter…</Empty> : (
        <div className="overview-grid">
          {ov.checklists.map(c => <ListCard key={c.id} c={c} canEdit={canEdit} onToggle={it => toggle(c, it)} onAdd={t => add(c, t)} onRemove={it => remove(c, it)} onRename={(it, t) => rename(c, it, t)} onClearDone={() => clearDone(c)} onReset={() => resetAll(c)} onDelete={() => setDel(c)} />)}
        </div>
      )}
      {adding && <NewListModal tripId={trip.id} onClose={() => setAdding(false)} onSaved={c => { setAdding(false); update([...ov.checklists, c]) }} />}
      {del && <Confirm title={`Delete “${del.name}”?`} message="The list and all its items are removed." onCancel={() => setDel(null)} onConfirm={deleteList} />}
    </>
  )
}

function ListCard({ c, canEdit, onToggle, onAdd, onRemove, onRename, onClearDone, onReset, onDelete }: { c: Checklist; canEdit: boolean; onToggle: (i: ChecklistItem) => void; onAdd: (t: string) => void; onRemove: (i: ChecklistItem) => void; onRename: (i: ChecklistItem, t: string) => void; onClearDone: () => void; onReset: () => void; onDelete: () => void }) {
  const [text, setText] = useState('')
  const [hideDone, setHideDone] = useState(false)
  const done = c.items.filter(i => i.done).length
  const items = hideDone ? c.items.filter(i => !i.done) : c.items
  return (
    <Card title={<><Icon name={c.kind === 'packing' ? 'luggage' : c.kind === 'shopping' ? 'tag' : 'checklist'} size={16} />{c.name} <span className="progress">{done}/{c.items.length}</span></>} actions={canEdit && <span className="row"><button type="button" className={`btn ghost icon-only sm ${hideDone ? 'active' : ''}`} onClick={() => setHideDone(h => !h)} title={hideDone ? 'Show ticked' : 'Hide ticked'} aria-label={hideDone ? 'Show ticked' : 'Hide ticked'} aria-pressed={hideDone}><Icon name={hideDone ? 'eye-off' : 'eye'} size={15} /></button>{done > 0 && <button type="button" className="btn ghost icon-only sm" onClick={onReset} title="Untick all" aria-label="Untick all"><Icon name="undo" size={15} /></button>}{done > 0 && <button type="button" className="btn ghost icon-only sm" onClick={onClearDone} title="Remove ticked" aria-label="Remove ticked"><Icon name="check-circle" size={15} /></button>}<button type="button" className="btn ghost icon-only sm danger-text" onClick={onDelete} title="Delete list" aria-label="Delete list"><Icon name="trash" size={15} /></button></span>}>
      {c.items.length > 0 && <div className="bar lg mb-sm"><span className="fill" style={{ width: `${done / c.items.length * 100}%`, background: done === c.items.length ? 'var(--success)' : undefined }} /></div>}
      {items.length === 0 && <p className="muted small">{c.items.length ? 'All done 🎉' : 'Empty — add items below, one per line.'}</p>}
      {items.map(it => (
        <div key={it.id} className={`checklist-item ${it.done ? 'done' : ''}`}>
          <input type="checkbox" checked={it.done} onChange={() => onToggle(it)} disabled={!canEdit} />
          <input className="inline-edit t" defaultValue={it.text} disabled={!canEdit} onBlur={e => onRename(it, e.target.value)} onKeyDown={e => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur() }} />
          {canEdit && <button type="button" className="icon-btn x" onClick={() => onRemove(it)} title="Remove" aria-label="Remove item"><Icon name="x" size={14} /></button>}
        </div>
      ))}
      {canEdit && <form className="row mt-sm" onSubmit={e => { e.preventDefault(); onAdd(text); setText('') }}><input value={text} onChange={e => setText(e.target.value)} placeholder="Add item… (paste several lines)" /><button className="btn sm">Add</button></form>}
    </Card>
  )
}

function NewListModal({ tripId, onClose, onSaved }: { tripId: number; onClose: () => void; onSaved: (c: Checklist) => void }) {
  const { meta } = useApp()
  const { toast } = useToast()
  const { settings } = useApp()
  const [f, setF] = useState({ name: 'Packing', kind: 'packing', template: 'essentials', items: '', ai: false, notes: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      if (f.ai) { const c = await api.post<Checklist>(`${V1}/trips/${tripId}/ai/packing`, { name: f.name, notes: f.notes, create: true }); toast(`List created with ${c.items.length} items`); onSaved(c); return }
 const c = await api.post<Checklist>(`${V1}/trips/${tripId}/checklists`, { name: f.name, kind: f.kind, template: f.template, items: f.items.split('\n').filter(Boolean) }); toast('List created'); onSaved(c) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="New checklist" onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required autoFocus /></Field>
        <Field group label="Kind"><Segmented value={f.kind} onChange={v => setF({ ...f, kind: v })} options={[{ value: 'packing', label: 'Packing' }, { value: 'todo', label: 'To-do' }, { value: 'shopping', label: 'Shopping' }, { value: 'custom', label: 'Other' }]} size="sm" /></Field>
        {settings?.openai_configured && <label className="row"><input type="checkbox" checked={f.ai} onChange={e => setF({ ...f, ai: e.target.checked })} style={{ width: 'auto' }} /> <Icon name="sparkles" size={14} />Generate it for this trip (destination, dates, weather, activities)</label>}
        {f.ai && <Field label="Who is going, anything special?"><input value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} placeholder="two adults and a toddler, we'll hike one day, no checked luggage" /></Field>}
        {!f.ai && <Field label="Start from a template" help="Items are added to the list; remove what you don't need."><Select value={f.template} onChange={v => setF({ ...f, template: v })} options={[{ value: '', label: 'Empty list' }, ...(meta?.packing_templates || []).map(t => ({ value: t, label: cap(t) }))]} /></Field>}
        {!f.ai && <Field label="Extra items" help="One per line."><textarea value={f.items} onChange={e => setF({ ...f, items: e.target.value })} rows={3} /></Field>}
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{busy ? (f.ai ? 'Writing the list…' : 'Creating…') : 'Create'}</button></div>
      </form>
    </Modal>
  )
}
