import { useState } from 'react'
import { api, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import type { Expense, Trip } from '../../lib/types'
import { EXPENSE_ICONS } from '../../lib/types'
import { ago, cap, fmtDate, fmtMoney, todayISO } from '../../lib/format'
import { Card, Confirm, Empty, Field, Modal, Segmented, useToast } from '../../components/ui'
import { Select } from '../../components/Select'
import { AmountInput, Money } from '../../components/Money'
import { useTrip } from './ctx'

export function BudgetTab() {
  const { ov, trip, canEdit, edit, reload } = useTrip()
  const { currencies } = useApp()
  const { toast } = useToast()
  const [show, setShow] = useState<'all' | 'actual' | 'planned'>('all')
  const [syncing, setSyncing] = useState(false)
  const b = ov.budget
  const integ = ov.integrations.firefly
  const sync = async () => {
    setSyncing(true)
    try { const r = await api.post<{ synced: number }>(`${V1}/trips/${trip.id}/firefly/sync`); toast(`Synced ${r.synced} transactions from Firefly`); await reload() } catch (e) { toast((e as Error).message, 'err') } finally { setSyncing(false) }
  }
  const list = ov.expenses.filter(e => show === 'all' || e.kind === show)
  const max = Math.max(1, ...b.by_category.map(c => Math.max(c.planned, c.actual)))
  const maxDay = Math.max(1, ...b.by_day.map(d => d.actual))
  const today = todayISO()
  const dec = currencies.find(c => c.code === b.currency)?.decimals ?? 2
  return (
    <>
      <div className="stats">
        <div className="stat"><div className="n"><Money minor={b.total} code={b.currency} compact /></div><div className="l">total{b.bookings > 0 && <> · <Money minor={b.actual} code={b.currency} compact /> spent + bookings</>}{b.bookings === 0 && b.firefly_count > 0 && ` · ${b.firefly_count} from Firefly`}</div></div>
        <div className="stat"><div className="n"><Money minor={b.planned} code={b.currency} compact /></div><div className="l">planned</div></div>
        {trip.budget > 0 && <div className="stat"><div className={`n ${b.remaining < 0 ? 'bad' : ''}`}><Money minor={b.remaining} code={b.currency} compact /></div><div className="l">left of <Money minor={trip.budget} code={b.currency} compact /></div></div>}
        <div className="stat"><div className="n"><Money minor={b.per_day} code={b.currency} compact /></div><div className="l">per day{b.days_elapsed > 0 ? ` · ${b.days_elapsed} days so far` : trip.days ? ` · planned over ${trip.days}` : ''}</div></div>
      </div>
      <div className="two-col">
        <Card title="By category">
          {b.by_category.length === 0 ? <p className="muted">Nothing yet.</p> : <div className="cat-rows">{b.by_category.map(c => <div key={c.category} className="cat-row"><span>{EXPENSE_ICONS[c.category] || '•'} {cap(c.category)}</span><span className="bars"><span className="bar" title="spent"><span className="fill" style={{ width: `${c.actual / max * 100}%` }} /></span><span className="bar p" title="planned"><span className="fill" style={{ width: `${c.planned / max * 100}%` }} /></span></span><span className="right small"><Money minor={c.actual} code={b.currency} compact />{c.planned > 0 && <span className="muted"> / <Money minor={c.planned} code={b.currency} compact /></span>}</span></div>)}</div>}
          <div className="legend"><span style={{ '--d': 'var(--accent)' } as React.CSSProperties}>spent</span><span style={{ '--d': 'var(--accent-soft)' } as React.CSSProperties}>planned</span></div>
        </Card>
        <Card title="By day">
          {b.by_day.length === 0 ? <p className="muted">Spending by day appears once there are actual expenses with dates.</p> : <>
            <div className="spark">{b.by_day.map(d => <div key={d.date} className={`col ${d.date === today ? 'today' : ''}`} style={{ height: `${Math.max(3, d.actual / maxDay * 100)}%` }} data-l={`${fmtDate(d.date, false)}: ${fmtMoney(d.actual, b.currency, dec)} (${d.count})`} />)}</div>
            <div className="row small muted mt-sm"><span>{fmtDate(b.by_day[0].date, false)}</span><span className="spacer" /><span>{fmtDate(b.by_day[b.by_day.length - 1].date, false)}</span></div>
          </>}
          {Object.keys(b.by_currency).length > 1 && <p className="muted small mt-sm">Paid in {Object.entries(b.by_currency).map(([c, v]) => fmtMoney(v, c, currencies.find(x => x.code === c)?.decimals ?? 2)).join(', ')} — converted at the day's rate.</p>}
        </Card>
      </div>
      <Card className="mt" title="Expenses" actions={<span className="row">
        <Segmented value={show} onChange={setShow} options={[{ value: 'all', label: 'All' }, { value: 'actual', label: 'Spent' }, { value: 'planned', label: 'Planned' }]} size="sm" />
        {integ.configured && trip.firefly_tag && canEdit && <button className="btn sm" onClick={sync} disabled={syncing}>{syncing ? 'Syncing…' : '↻ Sync Firefly'}</button>}
        {canEdit && <button className="btn primary sm" onClick={() => edit('expense', null, { kind: trip.end_date && trip.end_date < today ? 'actual' : 'planned' })}>+ Cost</button>}
      </span>}>
        {integ.configured && !trip.firefly_tag && <div className="hint-strip">💡 Link a Firefly III tag under <a href="#" onClick={e => { e.preventDefault(); edit('trip', trip) }}>Edit trip</a> and every withdrawal with that tag lands here as spent, in its own currency.</div>}
        {integ.configured && trip.firefly_tag && <div className="hint-strip">Firefly tag <code>{trip.firefly_tag}</code>{integ.synced_at ? ` · synced ${ago(integ.synced_at)}` : ' · not synced yet'} · refreshes every 30 min while the trip is running.</div>}
        {list.length === 0 ? <Empty title="No expenses">Planned costs come from stays, transport and anything you add; what you really spend comes from Firefly or by hand.</Empty> : (
          <table className="table compact">
            <thead><tr><th>Date</th><th>What</th><th>Category</th><th className="num">Amount</th><th className="num">In {b.currency}</th></tr></thead>
            <tbody>{list.map(e => <tr key={e.id} className="clickable" onClick={() => edit('expense', e)}>
              <td className="muted small">{e.date ? fmtDate(e.date, false) : '—'}</td>
              <td><b>{e.name}</b> {e.source === 'firefly' && <span className="src-pill firefly">firefly</span>}{e.source === 'item' && <span className="src-pill">{e.link_kind}</span>}{e.kind === 'planned' && <span className="src-pill">planned</span>}{e.notes && <div className="small muted">{e.notes}</div>}</td>
              <td className="muted">{EXPENSE_ICONS[e.category] || ''} {cap(e.category)}</td>
              <td className="num"><Money minor={e.amount} code={e.currency} /></td>
              <td className="num">{e.currency !== b.currency ? <Money minor={e.amount_home} code={b.currency} /> : ''}</td>
            </tr>)}</tbody>
          </table>
        )}
      </Card>
    </>
  )
}

export function ExpenseModal({ trip, item, defaults = {}, onClose, onSaved }: { trip: Trip; item: Expense | null; defaults?: Record<string, unknown>; onClose: () => void; onSaved: () => void }) {
  const { meta } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<Partial<Expense>>(item ? { ...item } : { name: '', category: 'other', amount: 0, currency: trip.currency, date: todayISO(), kind: 'planned', notes: '', ...defaults })
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState(false)
  const set = <K extends keyof Expense>(k: K, v: Expense[K]) => setF(x => ({ ...x, [k]: v }))
  const base = `${V1}/trips/${trip.id}/expenses`
  const locked = item?.source === 'firefly' || item?.source === 'item'
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      const body: Record<string, unknown> = locked ? { category: f.category, notes: f.notes } : { ...f }
      if (item) await api.put(`${base}/${item.id}`, body); else await api.post(base, body)
      toast('Saved'); onSaved()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const remove = async () => { try { await api.del(`${base}/${item!.id}`); toast('Removed'); onSaved() } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Modal title={item ? 'Edit cost' : 'Add a cost'} onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        {item?.source === 'firefly' && <div className="callout">This comes from Firefly III (#{item.external_id}). Change the amount or date there; here you can only re-categorise it.</div>}
        {item?.source === 'item' && <div className="callout">This is the price of a {item.link_kind}. Edit it on the {item.link_kind} itself.</div>}
        <Field label="What"><input value={f.name || ''} onChange={e => set('name', e.target.value)} required autoFocus={!item} disabled={locked} /></Field>
        {!locked && <Field group label="Kind"><Segmented value={f.kind || 'planned'} onChange={v => set('kind', v as 'planned' | 'actual')} options={[{ value: 'planned', label: 'Planned (estimate)' }, { value: 'actual', label: 'Spent' }]} size="sm" /></Field>}
        <div className="two-col tight">
          <Field label="Amount"><AmountInput amount={f.amount ?? null} currency={f.currency || trip.currency} onAmount={m => set('amount', m || 0)} onCurrency={c => set('currency', c)} autoFocus={!!item && !locked} /></Field>
          <Field label="Date"><input type="date" value={f.date || ''} onChange={e => set('date', e.target.value)} disabled={locked} /></Field>
        </div>
        <Field label="Category"><Select value={f.category || 'other'} onChange={v => set('category', v)} options={(meta?.expense_categories || []).map(c => ({ value: c, label: `${EXPENSE_ICONS[c] || ''} ${cap(c)}` }))} /></Field>
        <Field label="Notes"><input value={f.notes || ''} onChange={e => set('notes', e.target.value)} /></Field>
        <div className="form-actions">
          {item && !locked && <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}>Delete</button>}
          <span className="spacer" /><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{item ? 'Save' : 'Add'}</button>
        </div>
      </form>
      {del && <Confirm title="Delete this cost?" message="It is removed from the budget." onCancel={() => setDel(false)} onConfirm={remove} />}
    </Modal>
  )
}
