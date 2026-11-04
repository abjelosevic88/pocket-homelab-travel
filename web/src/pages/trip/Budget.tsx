import { useState } from 'react'
import { api, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import { PersonDot } from '../../components/WithPeople'
import type { Expense, Trip } from '../../lib/types'
import { EXPENSE_ICONS } from '../../lib/types'
import { ago, cap, fmtDate, fmtMoney, todayISO, groupBills } from '../../lib/format'
import { Card, Confirm, Empty, Field, Modal, Segmented, useToast } from '../../components/ui'
import { Icon } from '../../components/icons'
import { Select } from '../../components/Select'
import { AmountInput, Money, MoneyInput } from '../../components/Money'
import { useTrip } from './ctx'

export function BudgetTab() {
  const { ov, trip, canEdit, edit, reload, go } = useTrip()
  const { currencies } = useApp()
  const { toast } = useToast()
  const [show, setShow] = useState<'all' | 'actual' | 'planned'>('all')
  const [syncing, setSyncing] = useState(false)
  const b = ov.budget
  const integ = ov.integrations.firefly
  const sync = async () => {
    setSyncing(true)
    try { const r = await api.post<{ synced: number }>(`${V1}/trips/${trip.id}/money/sync`); toast(`Synced ${r.synced} transactions from Pocket Money`); await reload() } catch (e) { toast((e as Error).message, 'err') } finally { setSyncing(false) }
  }
  // money handed between people is not a cost: it lives on the Split tab
  const list = groupBills(ov.expenses.filter(e => e.category !== 'reimbursement' && (show === 'all' || e.kind === show)))
  // a planned cost that has been paid becomes a bill: a stay or leg is marked paid, anything else opens as a bill to say who paid
  const markPaid = async (e: Expense) => {
    if (e.source === 'item' && (e.link_kind === 'lodging' || e.link_kind === 'transport')) {
      try { await api.put(`${V1}/trips/${trip.id}/${e.link_kind}/${e.link_id}`, { paid: true }); toast(`Marked paid — it's a bill now, paid by you for everyone (change it on the Split tab)`); await reload() } catch (err) { toast((err as Error).message, 'err') }
      return
    }
    edit('expense', { ...e, kind: 'actual', date: e.date || todayISO() })
  }
  const max = Math.max(1, ...b.by_category.map(c => Math.max(c.planned, c.actual)))
  const maxDay = Math.max(1, ...b.by_day.map(d => d.actual))
  const today = todayISO()
  const dec = currencies.find(c => c.code === b.currency)?.decimals ?? 2
  return (
    <>
      <div className="stats">
        <div className="stat"><div className="n"><Money minor={b.total} code={b.currency} compact /></div><div className="l">total{b.bookings > 0 && <> · <Money minor={b.actual} code={b.currency} compact /> spent + bookings</>}{b.bookings === 0 && b.firefly_count > 0 && ` · ${b.firefly_count} from Pocket Money`}</div></div>
        <div className="stat"><div className="n"><Money minor={b.planned} code={b.currency} compact /></div><div className="l">planned</div></div>
        {trip.budget > 0 && <div className="stat"><div className={`n ${b.remaining < 0 ? 'bad' : ''}`}><Money minor={b.remaining} code={b.currency} compact /></div><div className="l">left of <Money minor={trip.budget} code={b.currency} compact /></div></div>}
        <div className="stat"><div className="n"><Money minor={b.per_day} code={b.currency} compact /></div><div className="l">per day{b.days_elapsed > 0 ? ` · ${b.days_elapsed} days so far` : trip.days ? ` · planned over ${trip.days}` : ''}</div></div>
      </div>
      <div className="two-col">
        <Card title="By category">
          {b.by_category.length === 0 ? <p className="muted">Nothing yet.</p> : <div className="cat-rows">{b.by_category.map(c => <div key={c.category} className="cat-row"><span>{EXPENSE_ICONS[c.category] || '•'} {cap(c.category)}</span><span className="bars"><span className="bar" title="spent"><span className="fill" style={{ width: `${c.actual / max * 100}%` }} /></span><span className="bar p" title="planned"><span className="fill" style={{ width: `${c.planned / max * 100}%` }} /></span></span><span className="right small"><Money minor={c.actual} code={b.currency} compact />{c.planned > 0 && <span className="muted"> / <Money minor={c.planned} code={b.currency} compact /></span>}</span></div>)}</div>}
          <div className="legend"><span style={{ '--d': 'var(--accent)' } as React.CSSProperties}>spent</span><span style={{ '--d': 'var(--accent-soft)' } as React.CSSProperties}>planned</span></div>
        </Card>
        <Card title="Paid vs still planned" actions={<button type="button" className="btn ghost sm" onClick={() => go('split')}>Who paid what →</button>}>
          {(() => {
            const open = b.planned // planned costs not paid yet (a paid one is a bill)
            const scale = Math.max(1, trip.budget, b.actual + open)
            return (
              <div className="pvp">
                <div className="pvp-bar" title="paid · still planned"><span className="paid" style={{ width: `${b.actual / scale * 100}%` }} /><span className="open" style={{ width: `${open / scale * 100}%` }} /></div>
                <dl className="pvp-rows">
                  <dt>Paid (bills)</dt><dd><Money minor={b.actual} code={b.currency} /></dd>
                  <dt>Still planned</dt><dd><Money minor={open} code={b.currency} /></dd>
                  <dt>Expected in all</dt><dd><Money minor={b.actual + open} code={b.currency} /></dd>
                  {trip.budget > 0 && <><dt>Budget</dt><dd className={b.actual + open > trip.budget ? 'bad' : ''}><Money minor={trip.budget} code={b.currency} /></dd></>}
                </dl>
                <p className="muted small" style={{ margin: 0 }}>A planned cost turns into a bill with “Paid” — then it counts as paid here and in the split. Money handed between people is not a cost and stays on the Split tab.</p>
                {Object.keys(b.by_currency).length > 1 && <p className="muted small" style={{ margin: 0 }}>Paid in {Object.entries(b.by_currency).map(([c, v]) => fmtMoney(v, c, currencies.find(x => x.code === c)?.decimals ?? 2)).join(', ')} — converted at the day's rate.</p>}
              </div>
            )
          })()}
        </Card>
      </div>
      <DayByDay />
      <Card className="mt" title="Expenses" actions={<span className="row">
        <Segmented value={show} onChange={setShow} options={[{ value: 'all', label: 'All' }, { value: 'actual', label: 'Spent' }, { value: 'planned', label: 'Planned' }]} size="sm" />
        {integ.configured && trip.firefly_tag && canEdit && <button type="button" className="btn sm" onClick={sync} disabled={syncing}><Icon name="refresh" size={13} />{syncing ? 'Syncing…' : 'Sync Pocket Money'}</button>}
        {canEdit && <button type="button" className="btn primary sm" onClick={() => edit('expense', null, { kind: trip.end_date && trip.end_date < today ? 'actual' : 'planned' })}><Icon name="plus" size={14} />Cost</button>}
      </span>}>
        {integ.configured && !trip.firefly_tag && <div className="hint-strip"><Icon name="info" size={14} />Set a Money tag under <a href="#" onClick={e => { e.preventDefault(); edit('trip', trip) }}>Edit trip</a> and every expense in Pocket Money with that tag lands here as spent, in its own currency.</div>}
        {integ.configured && trip.firefly_tag && <div className="hint-strip">Money tag <code>{trip.firefly_tag}</code>{integ.via === 'core' ? ' · through Pocket Core' : ''}{integ.synced_at ? ` · synced ${ago(integ.synced_at)}` : ' · not synced yet'} · refreshes every 30 min while the trip is running.</div>}
        {list.length === 0 ? <Empty icon="wallet" title="No expenses">Planned costs come from stays, transport and anything you add; what you really spend comes from Pocket Money or by hand.</Empty> : (
          <table className="table compact">
            <thead><tr><th>Date</th><th>What</th><th>Category</th><th className="num">Amount</th><th className="num">In {b.currency}</th></tr></thead>
            <tbody>{list.map(e => <tr key={e.id} className="clickable" onClick={() => edit('expense', e)}>
              <td className="muted small">{e.date ? fmtDate(e.date, false) : '—'}</td>
              <td><b>{e.name}</b> {e.source === 'money' && <span className="src-pill money">money</span>}{e.source === 'firefly' && <span className="src-pill firefly">firefly</span>}{e.source === 'item' && <span className="src-pill">{e.link_kind}</span>}{e.kind === 'planned' && <span className="src-pill">planned</span>}{e.notes && <div className="small muted">{e.notes}</div>}</td>
              <td className="muted">{EXPENSE_ICONS[e.category] || ''} {cap(e.category)}</td>
              <td className="num"><Money minor={e.total} code={e.currency} />{e.parts.length > 0 && <div className="small muted">uneven split</div>}{e.kind === 'planned' && canEdit && <div><button type="button" className="btn ghost sm paid-btn" onClick={ev => { ev.stopPropagation(); markPaid(e) }} title="It's paid — make it a bill (who paid, for whom)"><Icon name="check" size={12} />Paid</button></div>}</td>
              <td className="num">{e.currency !== b.currency ? <Money minor={e.total_home} code={b.currency} /> : ''}</td>
            </tr>)}</tbody>
          </table>
        )}
      </Card>
    </>
  )
}

export function ExpenseModal({ trip, item: picked, defaults = {}, onClose, onSaved }: { trip: Trip; item: Expense | null; defaults?: Record<string, unknown>; onClose: () => void; onSaved: () => void }) {
  const { meta } = useApp()
  const { ov } = useTrip()
  const { toast } = useToast()
  const people = ov.people || []
  const split = ov.split
  // an extra part of an uneven split opens its whole bill
  const item = picked?.parent_id ? ov.expenses.find(x => x.id === picked.parent_id) || picked : picked
  const parts = item ? ov.expenses.filter(x => x.parent_id === item.id) : []
  const [extras, setExtras] = useState<Record<number, number>>(() => Object.fromEntries(parts.map(p => [p.owers[0], p.amount])))
  const [uneven, setUneven] = useState(parts.length > 0)
  const [f, setF] = useState<Partial<Expense>>(item ? { ...item, amount: item.amount + parts.reduce((n, p) => n + p.amount, 0) } : { name: '', category: 'other', amount: 0, currency: trip.currency, date: todayISO(), kind: 'planned', notes: '', payer_id: null, owers: [], payment_mode_id: 0, bill_time: '', repeat: 'n', repeat_until: '', repeat_freq: 1, ...defaults })
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState(false)
  const set = <K extends keyof Expense>(k: K, v: Expense[K]) => setF(x => ({ ...x, [k]: v }))
  const base = `${V1}/trips/${trip.id}/expenses`
  const synced = item?.source === 'money' || item?.source === 'firefly'
  const locked = synced || item?.source === 'item'
  const bill = f.kind === 'actual'
  // a new bill: paid by the owner, for everyone taking part
  const owner = people.find(p => p.role === 'owner') || people[0]
  const payer = f.payer_id ?? owner?.id ?? null
  const owers = f.owers && f.owers.length ? f.owers : people.filter(p => p.active).map(p => p.id)
  const toggleOwer = (id: number) => set('owers', owers.includes(id) ? owers.filter(x => x !== id) : [...owers, id])
  const totalWeight = owers.reduce((n, id) => n + (people.find(p => p.id === id)?.weight || 1), 0)
  // uneven: each extra is owed on top; the rest is shared equally (by weight) by everyone ticked
  const extraSum = uneven ? Object.values(extras).reduce((n, v) => n + (v || 0), 0) : 0
  const rest = (f.amount || 0) - extraSum
  const shareOf = (id: number) => (owers.includes(id) && totalWeight > 0 ? Math.round(rest * (people.find(p => p.id === id)?.weight || 1) / totalWeight) : 0) + (uneven ? extras[id] || 0 : 0)
  const canUneven = bill && !locked && people.length > 1
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      if (uneven && rest < 0) { toast('The extra parts add up to more than the bill', 'err'); setBusy(false); return }
      const billPart = bill ? { payer_id: payer, owers, payment_mode_id: f.payment_mode_id || 0, bill_time: f.bill_time || '', ...(locked ? {} : { extras: uneven ? Object.entries(extras).filter(([, v]) => v > 0).map(([id, v]) => ({ member_id: Number(id), amount: v })) : [] }) } : {}
      const body: Record<string, unknown> = locked ? { category: f.category, notes: f.notes, ...billPart } : { ...f, ...billPart }
      if (item) await api.put(`${base}/${item.id}`, body); else await api.post(base, body)
      toast('Saved'); onSaved()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const remove = async () => { try { await api.del(`${base}/${item!.id}`); toast(bill ? 'Moved to the trash' : 'Removed'); onSaved() } catch (e) { toast((e as Error).message, 'err') } }
  const catOptions = split?.categories.length
    ? split.categories.map(c => ({ value: c.key, label: `${c.icon} ${c.name}` }))
    : (meta?.expense_categories || []).map(c => ({ value: c, label: `${EXPENSE_ICONS[c] || ''} ${cap(c)}` }))
  return (
    <Modal title={item ? (bill ? 'Edit bill' : 'Edit cost') : bill ? 'Add a bill' : 'Add a cost'} onClose={onClose} size={bill && people.length > 1 ? 'lg' : undefined}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        {synced && <div className="callout">This comes from {item?.source === 'money' ? 'Pocket Money' : 'Firefly III'} (#{item?.external_id}). Change the amount or date there; here you can re-categorise it and say who it was for.</div>}
        {item?.source === 'item' && <div className="callout">This is the price of a {item.link_kind}. Edit the amount on the {item.link_kind} itself{bill ? '; who paid and for whom are set here.' : '.'}{!bill && (item.link_kind === 'lodging' || item.link_kind === 'transport') && <> <button type="button" className="btn sm" style={{ marginLeft: 6 }} onClick={async () => { try { await api.put(`${V1}/trips/${trip.id}/${item.link_kind}/${item.link_id}`, { paid: true }); toast('Paid — it is a bill now'); onSaved() } catch (e) { toast((e as Error).message, 'err') } }}><Icon name="check" size={13} />It's paid</button></>}</div>}
        <Field label="What"><input value={f.name || ''} onChange={e => set('name', e.target.value)} required autoFocus={!item} disabled={locked} /></Field>
        {!locked && <Field group label="Kind"><Segmented value={f.kind || 'planned'} onChange={v => set('kind', v as 'planned' | 'actual')} options={[{ value: 'planned', label: 'Planned (estimate)' }, { value: 'actual', label: 'Spent (a bill)' }]} size="sm" /></Field>}
        <div className="two-col tight">
          <Field label="Amount"><AmountInput amount={f.amount ?? null} currency={f.currency || trip.currency} onAmount={m => set('amount', m || 0)} onCurrency={c => set('currency', c)} autoFocus={!!item && !locked} /></Field>
          <div className="two-col tight">
            <Field label="Date"><input type="date" value={f.date || ''} onChange={e => set('date', e.target.value)} disabled={locked} /></Field>
            {bill && <Field label="Time"><input type="time" value={f.bill_time || ''} onChange={e => set('bill_time', e.target.value)} /></Field>}
          </div>
        </div>
        {bill && people.length > 0 && <>
          <div className="two-col tight">
            <Field label="Paid by"><Select value={payer ? String(payer) : ''} onChange={v => set('payer_id', Number(v))} options={people.map(p => ({ value: String(p.id), label: p.name }))} /></Field>
            <Field label="Paid with"><Select value={f.payment_mode_id ? String(f.payment_mode_id) : ''} onChange={v => set('payment_mode_id', Number(v) || 0)} options={(split?.payment_modes || []).map(m => ({ value: String(m.id), label: `${m.icon} ${m.name}` }))} placeholder="—" clearLabel="Not said" /></Field>
          </div>
          {people.length > 1 && (
            <Field group label={<span className="row">For whom <span className="spacer" />{canUneven && <Segmented value={uneven ? 'uneven' : 'even'} onChange={v => setUneven(v === 'uneven')} options={[{ value: 'even', label: 'Equally' }, { value: 'uneven', label: 'Uneven' }]} size="sm" label="How to split" />}<button type="button" className="btn ghost sm" onClick={() => set('owers', people.map(p => p.id))}>Everyone</button><button type="button" className="btn ghost sm" onClick={() => payer && set('owers', [payer])}>Just the payer</button></span>}>
              {uneven && <p className="small muted" style={{ margin: '0 0 6px' }}>Put what someone had on top (their wine, a dessert) as their extra. What is left — <b><Money minor={Math.max(0, rest)} code={f.currency || trip.currency} /></b> — is shared equally by everyone ticked. Give everyone an extra and untick all for exact amounts.{rest < 0 && <span className="bad"> The extras are more than the bill.</span>}</p>}
              <div className={`owers ${uneven ? 'uneven' : ''}`}>
                {people.map(p => {
                  const on = owers.includes(p.id)
                  const share = shareOf(p.id)
                  return (
                    <div key={p.id} className={`ower ${on || (uneven && extras[p.id]) ? 'on' : ''}`}>
                      <label className="row grow" style={{ gap: 8, minWidth: 0, cursor: 'pointer' }}>
                        <input type="checkbox" checked={on} onChange={() => toggleOwer(p.id)} title={uneven ? 'Shares what is left' : undefined} />
                        <PersonDot name={p.name} color={p.color} size={20} />
                        <span className="grow ellipsis">{p.name}{p.weight !== 1 && <span className="muted small"> ×{p.weight}</span>}</span>
                      </label>
                      {uneven && <span className="extra-in" title={`${p.name}'s extra, on top of an equal share`}>+<MoneyInput value={extras[p.id] || null} code={f.currency || trip.currency} onChange={m => setExtras(x => ({ ...x, [p.id]: m || 0 }))} ariaLabel={`${p.name}'s extra`} /></span>}
                      {share > 0 && <span className="small muted nowrap"><Money minor={share} code={f.currency || trip.currency} /></span>}
                    </div>
                  )
                })}
              </div>
            </Field>
          )}
        </>}
        <Field label="Category"><Select value={f.category || 'other'} onChange={v => set('category', v)} options={catOptions} /></Field>
        <Field label="Notes"><input value={f.notes || ''} onChange={e => set('notes', e.target.value)} /></Field>
        <div className="form-actions">
          {item && !locked && <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}>Delete</button>}
          <span className="spacer" /><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{item ? 'Save' : 'Add'}</button>
        </div>
      </form>
      {del && <Confirm title={bill ? 'Move this bill to the trash?' : 'Delete this cost?'} message={bill ? 'It stops counting; you can restore it from the trash on the Split tab.' : 'It is removed from the budget.'} onCancel={() => setDel(false)} onConfirm={remove} />}
    </Modal>
  )
}

/**
 * What each day costs: paid (solid, by category) and still planned (hatched).
 * A stay is spread over its nights, a flight sits on the day it leaves,
 * dinner on the evening it was eaten; costs dated before or after the trip
 * get their own column.
 */
function DayByDay() {
  const { ov, trip } = useTrip()
  const days = ov.day_costs || []
  const cats = new Map((ov.split?.categories || []).map(c => [c.key, c]))
  const [hover, setHover] = useState<string | null>(null)
  if (!days.some(d => Object.keys(d.actual).length || Object.keys(d.planned).length)) return null
  const sum = (m: Record<string, number>) => Object.values(m).reduce((n, v) => n + v, 0)
  const max = Math.max(1, ...days.map(d => sum(d.actual) + sum(d.planned)))
  const used = Array.from(new Set(days.flatMap(d => [...Object.keys(d.actual), ...Object.keys(d.planned)]))).sort((a, b) => (cats.get(a)?.order ?? 99) - (cats.get(b)?.order ?? 99))
  const color = (k: string) => cats.get(k)?.color || '#94a3b8'
  const label = (d: string) => d === 'before' ? 'before' : d === 'after' ? 'after' : fmtDate(d, false)
  const paidDays = days.filter(d => d.date !== 'before' && d.date !== 'after' && sum(d.actual) > 0)
  const avg = paidDays.length ? Math.round(paidDays.reduce((n, d) => n + sum(d.actual), 0) / paidDays.length) : 0
  const h = days.find(d => d.date === hover)
  return (
    <Card title="Day by day" className="mt" actions={avg > 0 && <span className="muted small">⌀ <Money minor={avg} code={trip.currency} /> on a day with spending</span>}>
      <div className="dbd" onMouseLeave={() => setHover(null)}>
        {days.map((d, i) => {
          const a = sum(d.actual), p = sum(d.planned)
          return (
            <div key={d.date} className={`dbd-col ${hover === d.date ? 'hi' : ''} ${d.date === 'before' || d.date === 'after' ? 'edge' : ''}`} onMouseEnter={() => setHover(d.date)} tabIndex={0} onFocus={() => setHover(d.date)} aria-label={`${label(d.date)}: ${(a / 100).toFixed(2)} paid, ${(p / 100).toFixed(2)} planned`}>
              <div className="dbd-bar">
                {p > 0 && <div className="dbd-planned" style={{ height: `${p / max * 100}%` }} />}
                {used.filter(k => d.actual[k]).reverse().map(k => <div key={k} className="dbd-seg" style={{ height: `${d.actual[k] / max * 100}%`, background: color(k) }} />)}
              </div>
              <div className="dbd-x">{d.date === 'before' ? '←' : d.date === 'after' ? '→' : (days.length > 20 && i % 2 ? '' : d.date.slice(8, 10))}</div>
            </div>
          )
        })}
      </div>
      <div className="dbd-info">
        {h ? <><b>{label(h.date)}</b>{used.filter(k => h.actual[k] || h.planned[k]).map(k => <span key={k}>{cats.get(k)?.icon} {cats.get(k)?.name || k} <b><Money minor={(h.actual[k] || 0) + (h.planned[k] || 0)} code={trip.currency} /></b>{h.planned[k] ? <span className="muted"> planned</span> : null}</span>)}{!Object.keys(h.actual).length && !Object.keys(h.planned).length && <span className="muted">nothing</span>}</>
          : <span className="legend" style={{ margin: 0 }}>{used.map(k => <span key={k} className="key"><i className="sw" style={{ background: color(k) }} />{cats.get(k)?.name || k}</span>)}<span className="key"><i className="sw planned" />planned</span></span>}
      </div>
    </Card>
  )
}
