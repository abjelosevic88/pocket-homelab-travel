import { useMemo, useState } from 'react'
import { api, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import type { Expense, TripCategory, TripPaymentMode } from '../../lib/types'
import { fmtDate, groupBills, plural } from '../../lib/format'
import { Card, Confirm, Empty, Modal, useAsync, useToast } from '../../components/ui'
import { Icon } from '../../components/icons'
import { Select } from '../../components/Select'
import { Money } from '../../components/Money'
import { PersonDot } from '../../components/WithPeople'
import { useTrip } from './ctx'
import { CospendLinkCard } from './CospendLink'
import { CospendImport } from '../../components/CospendImport'

/**
 * The trip's shared money, as in Nextcloud Cospend: every spent cost is a bill
 * someone paid for some of the people here. Balances say who is up or down,
 * the plan says who pays whom, and the trash keeps deleted bills a while.
 */
export function Split() {
  const { ov, trip, canEdit, edit, reload } = useTrip()
  const { user } = useApp()
  const { toast } = useToast()
  const split = ov.split
  const [who, setWho] = useState('')
  const [cat, setCat] = useState('')
  const [settling, setSettling] = useState(false)
  const [lists, setLists] = useState(false)
  if (!split) return <Card><Empty icon="wallet" title="Nothing to split yet" /></Card>
  const people = split.people
  const name = (id: number | null) => people.find(p => p.id === id)?.name || '—'
  const person = (id: number | null) => people.find(p => p.id === id)
  const cats = new Map(split.categories.map(c => [c.key, c]))
  const modes = new Map(split.payment_modes.map(m => [m.id, m]))
  const bills = groupBills(ov.expenses.filter(e => e.kind === 'actual')).filter(e => (!who || e.payer_id === Number(who) || e.owers.includes(Number(who)) || e.parts.some(p => p.owers.includes(Number(who)))) && (!cat || e.category === cat))
  const me = people.find(p => p.user_id === user?.id)
  const myBalance = me ? split.balances.find(b => b.member_id === me.id) : undefined
  const maxAbs = Math.max(1, ...split.balances.map(b => Math.abs(b.balance)))
  const cur = split.currency
  const settle = async () => {
    try { await api.post(`${V1}/trips/${trip.id}/split/settle`); toast('Recorded — everyone is square'); await reload() } catch (e) { toast((e as Error).message, 'err') } finally { setSettling(false) }
  }
  return (
    <>
      <div className="stats">
        <div className="stat"><div className="n"><Money minor={split.total} code={cur} /></div><div className="l">spent together · {plural(split.bills, 'bill')}</div></div>
        {myBalance && <div className="stat"><div className={`n ${myBalance.balance > 0 ? 'good' : myBalance.balance < 0 ? 'bad' : ''}`}><Money minor={Math.abs(myBalance.balance)} code={cur} /></div><div className="l">{myBalance.balance > 0 ? 'you get back' : myBalance.balance < 0 ? 'you owe' : 'you are square'}</div></div>}
        {myBalance && <div className="stat"><div className="n"><Money minor={myBalance.spent} code={cur} /></div><div className="l">your share</div></div>}
        <div className="stat"><div className="n">{people.length}</div><div className="l">people</div></div>
      </div>

      <div className="two-col">
        <Card title="Balances" actions={<span className="muted small">paid − share</span>}>
          {people.length < 2 ? <p className="muted">Add the people you travel with (trip Settings → People) to split bills with them.</p> : (
            <div className="balances" role="list">
              {split.balances.map(b => {
                const p = person(b.member_id)
                const w = Math.abs(b.balance) / maxAbs * 50
                return (
                  <div key={b.member_id} className="bal-row" role="listitem" title={`${b.name}: paid ${(b.paid / 100).toFixed(2)}, share ${(b.spent / 100).toFixed(2)}`}>
                    <span className="bal-name"><PersonDot name={b.name} color={p?.color} size={22} />{b.name}</span>
                    <span className="bal-track"><i className="bal-mid" />{b.balance !== 0 && <span className={`bal-bar ${b.balance > 0 ? 'pos' : 'neg'}`} style={b.balance > 0 ? { left: '50%', width: `${w}%` } : { right: '50%', width: `${w}%` }} />}</span>
                    <span className={`bal-val ${b.balance > 0 ? 'good' : b.balance < 0 ? 'bad' : 'muted'}`}>{b.balance > 0 ? '+' : b.balance < 0 ? '−' : ''}<Money minor={Math.abs(b.balance)} code={cur} /></span>
                  </div>
                )
              })}
            </div>
          )}
        </Card>
        <Card title="Who pays whom" actions={canEdit && split.settlement.length > 0 && <button type="button" className="btn sm" onClick={() => setSettling(true)}><Icon name="check" size={14} />Mark as paid</button>}>
          {split.settlement.length === 0 ? <p className="muted">{split.bills ? 'Everyone is square.' : 'No bills yet.'}</p> : (
            <div className="day-items">
              {split.settlement.map((t, i) => (
                <div key={i} className="item settle-row">
                  <PersonDot name={name(t.from)} color={person(t.from)?.color} size={24} />
                  <span className="txt"><span className="title">{name(t.from)} <span className="muted">pays</span> {name(t.to)}</span></span>
                  <b><Money minor={t.amount} code={cur} /></b>
                </div>
              ))}
            </div>
          )}
          <p className="muted small mt-sm">The fewest payments that square everyone up. “Mark as paid” records them as reimbursements.</p>
        </Card>
      </div>

      <Card title={`Bills · ${bills.length}`} className="mb" actions={
        <div className="row" style={{ gap: 6 }}>
          {people.length > 1 && <Select value={who} onChange={setWho} options={people.map(p => ({ value: String(p.id), label: p.name }))} placeholder="Anyone" clearLabel="Anyone" ariaLabel="Filter by person" />}
          <Select value={cat} onChange={setCat} options={split.categories.map(c => ({ value: c.key, label: `${c.icon} ${c.name}` }))} placeholder="Any category" clearLabel="Any category" ariaLabel="Filter by category" />
          {canEdit && <button type="button" className="btn primary sm" onClick={() => edit('expense', null, { kind: 'actual', date: new Date().toISOString().slice(0, 10) })}><Icon name="plus" size={14} />Bill</button>}
        </div>}>
        {bills.length === 0 ? <Empty icon="wallet" title={ov.expenses.some(e => e.kind === 'actual') ? 'Nothing matches' : 'No bills yet'}>Add what you spend — who paid and for whom — and the balances follow. Paid stays and transport, and costs synced from Pocket Money, show up here too.</Empty> : (
          <table className="table compact bills">
            <thead><tr><th>When</th><th>What</th><th>Paid by</th><th>For</th><th className="num">Amount</th></tr></thead>
            <tbody>{bills.map(b => {
              const c = cats.get(b.category)
              const m = modes.get(b.payment_mode_id)
              return (
                <tr key={b.id} className="clickable" onClick={() => edit('expense', b)}>
                  <td className="muted nowrap">{fmtDate(b.date, false)}{b.bill_time && <span className="small"> {b.bill_time}</span>}</td>
                  <td><span title={c?.name}>{c?.icon || '📌'}</span> <b>{b.name}</b>{b.parts.length > 0 && <span className="badge" title={b.parts.map(p => `${name(p.owers[0])} +${(p.amount / 100).toFixed(2)}`).join(', ')}>uneven</span>}{b.source !== 'manual' && <span className="badge">{b.source === 'item' ? b.link_kind : b.source === 'money' ? 'Pocket Money' : b.source}</span>}{m && <span className="muted small"> · {m.icon} {m.name}</span>}</td>
                  <td className="nowrap"><PersonDot name={name(b.payer_id)} color={person(b.payer_id)?.color} size={18} /> {name(b.payer_id)}</td>
                  <td><span className="ower-dots" title={b.owers.map(name).join(', ')}>{b.owers.length === people.length && people.length > 1 ? <span className="muted small">everyone</span> : b.owers.map(id => <PersonDot key={id} name={name(id)} color={person(id)?.color} size={18} />)}{b.parts.length > 0 && <span className="muted small"> + {b.parts.map(p => name(p.owers[0]).split(' ')[0]).join(', ')}</span>}</span></td>
                  <td className="num nowrap"><Money minor={b.total} code={b.currency} />{b.currency !== cur && <div className="muted small"><Money minor={b.total_home} code={cur} /></div>}</td>
                </tr>
              )
            })}</tbody>
          </table>
        )}
      </Card>

      <div className="two-col">
        {canEdit && <CospendLinkCard />}
        <Card title="Categories & payment methods" actions={canEdit && <button type="button" className="btn ghost sm" onClick={() => setLists(true)}><Icon name="edit" size={13} />Edit</button>}>
          <div className="row wrap" style={{ gap: 6 }}>{split.categories.map(c => <span key={c.id} className="country-chip">{c.icon} {c.name}{split.by_category[c.key] ? <span className="muted"> · <Money minor={split.by_category[c.key]} code={cur} compact /></span> : null}</span>)}</div>
          <div className="row wrap mt-sm" style={{ gap: 6 }}>{split.payment_modes.map(m => <span key={m.id} className="country-chip">{m.icon} {m.name}</span>)}</div>
        </Card>
      </div>
      {split.trash > 0 && <Trash />}
      {canEdit && <Card title="Import from Nextcloud Cospend" className="mb"><p className="muted small">Bring an existing Cospend project's bills and people into this trip. Run it again later and it updates instead of doubling.</p><CospendImport tripId={trip.id} onDone={() => reload()} /></Card>}
      {settling && <Confirm title="Mark the plan as paid?" message={`${plural(split.settlement.length, 'reimbursement')} will be added so everyone is square.`} onCancel={() => setSettling(false)} onConfirm={settle} />}
      {lists && <ListsModal categories={split.categories} modes={split.payment_modes} onClose={() => { setLists(false); reload() }} />}
    </>
  )
}

/** Deleted bills: restore one, or empty the trash for good. */
function Trash() {
  const { trip, canEdit, reload, ov } = useTrip()
  const { toast } = useToast()
  const { data, reload: again } = useAsync(() => api.get<Expense[]>(`${V1}/trips/${trip.id}/split/trash`), [trip.id, ov.split?.trash])
  const [empty, setEmpty] = useState(false)
  const restore = async (e: Expense) => { try { await api.post(`${V1}/trips/${trip.id}/expenses/${e.id}/restore`); toast('Restored'); await reload(); again() } catch (err) { toast((err as Error).message, 'err') } }
  const purge = async () => { try { await api.del(`${V1}/trips/${trip.id}/split/trash`); toast('Trash emptied'); await reload() } catch (err) { toast((err as Error).message, 'err') } finally { setEmpty(false) } }
  return (
    <Card title={`Trash · ${data?.length ?? ov.split?.trash ?? 0}`} className="mb" actions={canEdit && <button type="button" className="btn ghost sm danger-text" onClick={() => setEmpty(true)}>Empty trash</button>}>
      <div className="day-items">{groupBills(data || []).map(e => (
        <div key={e.id} className="item">
          <span className="txt"><span className="title">{e.name}</span><span className="sub">{fmtDate(e.date, false)}</span></span>
          <Money minor={e.total} code={e.currency} />
          {canEdit && <button type="button" className="btn ghost sm" onClick={() => restore(e)}>Restore</button>}
        </div>
      ))}</div>
      {empty && <Confirm title="Empty the trash?" message="These bills are deleted for good, here and in MoneyBuster." onCancel={() => setEmpty(false)} onConfirm={purge} />}
    </Card>
  )
}

/** Add, rename and remove the trip's bill categories and payment methods. */
function ListsModal({ categories, modes, onClose }: { categories: TripCategory[]; modes: TripPaymentMode[]; onClose: () => void }) {
  const { trip } = useTrip()
  const { toast } = useToast()
  const [cats, setCats] = useState(categories)
  const [pms, setPms] = useState(modes)
  const [nc, setNc] = useState({ icon: '', name: '' })
  const [np, setNp] = useState({ icon: '', name: '' })
  const base = `${V1}/trips/${trip.id}/split`
  const run = async (fn: () => Promise<void>) => { try { await fn() } catch (e) { toast((e as Error).message, 'err') } }
  const sorted = useMemo(() => cats, [cats])
  return (
    <Modal title="Categories & payment methods" onClose={onClose} size="lg">
      <div className="two-col">
        <div className="grid" style={{ gap: 6 }}>
          <b>Categories</b>
          {sorted.map(c => (
            <div key={c.id} className="row" style={{ gap: 6 }}>
              <input value={c.icon} onChange={e => setCats(x => x.map(y => y.id === c.id ? { ...y, icon: e.target.value } : y))} onBlur={() => run(async () => { await api.put(`${base}/categories/${c.id}`, c) })} style={{ width: 52, textAlign: 'center' }} aria-label="Icon" />
              <input value={c.name} onChange={e => setCats(x => x.map(y => y.id === c.id ? { ...y, name: e.target.value } : y))} onBlur={() => run(async () => { await api.put(`${base}/categories/${c.id}`, c) })} className="grow" aria-label="Name" />
              {c.key !== 'reimbursement' && c.key !== 'other' && <button type="button" className="icon-btn" aria-label={`Remove ${c.name}`} onClick={() => run(async () => { await api.del(`${base}/categories/${c.id}`); setCats(x => x.filter(y => y.id !== c.id)) })}><Icon name="x" size={14} /></button>}
            </div>
          ))}
          <form className="row" style={{ gap: 6 }} onSubmit={e => { e.preventDefault(); if (nc.name.trim()) run(async () => { const c = await api.post<TripCategory>(`${base}/categories`, nc); setCats(x => [...x, c]); setNc({ icon: '', name: '' }) }) }}>
            <input value={nc.icon} onChange={e => setNc({ ...nc, icon: e.target.value })} placeholder="🏷" style={{ width: 52, textAlign: 'center' }} aria-label="New icon" />
            <input value={nc.name} onChange={e => setNc({ ...nc, name: e.target.value })} placeholder="New category" className="grow" aria-label="New category" />
            <button className="btn sm">Add</button>
          </form>
        </div>
        <div className="grid" style={{ gap: 6 }}>
          <b>Payment methods</b>
          {pms.map(m => (
            <div key={m.id} className="row" style={{ gap: 6 }}>
              <input value={m.icon} onChange={e => setPms(x => x.map(y => y.id === m.id ? { ...y, icon: e.target.value } : y))} onBlur={() => run(async () => { await api.put(`${base}/payment-modes/${m.id}`, m) })} style={{ width: 52, textAlign: 'center' }} aria-label="Icon" />
              <input value={m.name} onChange={e => setPms(x => x.map(y => y.id === m.id ? { ...y, name: e.target.value } : y))} onBlur={() => run(async () => { await api.put(`${base}/payment-modes/${m.id}`, m) })} className="grow" aria-label="Name" />
              <button type="button" className="icon-btn" aria-label={`Remove ${m.name}`} onClick={() => run(async () => { await api.del(`${base}/payment-modes/${m.id}`); setPms(x => x.filter(y => y.id !== m.id)) })}><Icon name="x" size={14} /></button>
            </div>
          ))}
          <form className="row" style={{ gap: 6 }} onSubmit={e => { e.preventDefault(); if (np.name.trim()) run(async () => { const m = await api.post<TripPaymentMode>(`${base}/payment-modes`, np); setPms(x => [...x, m]); setNp({ icon: '', name: '' }) }) }}>
            <input value={np.icon} onChange={e => setNp({ ...np, icon: e.target.value })} placeholder="💳" style={{ width: 52, textAlign: 'center' }} aria-label="New icon" />
            <input value={np.name} onChange={e => setNp({ ...np, name: e.target.value })} placeholder="New payment method" className="grow" aria-label="New payment method" />
            <button className="btn sm">Add</button>
          </form>
        </div>
      </div>
      <div className="form-actions mt"><span className="spacer" /><button type="button" className="btn primary" onClick={onClose}>Done</button></div>
    </Modal>
  )
}

