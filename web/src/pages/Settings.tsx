import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, Settings as SettingsT, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Card, Confirm, Field, Loading, Modal, PageHeader, Segmented, Tabs, useAsync, useToast } from '../components/ui'
import { Select } from '../components/Select'
import { CurrencySelect } from '../components/Money'
import { Avatar } from '../components/people'

type Tab = 'general' | 'integrations' | 'import' | 'users' | 'profile' | 'api' | 'data' | 'about'

export default function Settings() {
  const [tab, setTab] = useState<Tab>(() => (new URLSearchParams(window.location.search).get('tab') as Tab) || 'general')
  return (
    <>
      <PageHeader title="Settings" />
      <Tabs tabs={[{ id: 'general', label: 'General' }, { id: 'integrations', label: 'Integrations' }, { id: 'import', label: 'Import' }, { id: 'users', label: 'Users' }, { id: 'profile', label: 'Profile' }, { id: 'api', label: 'API tokens' }, { id: 'data', label: 'Backup & data' }, { id: 'about', label: 'About' }]} value={tab} onChange={setTab} />
      <div className="mt">
        {tab === 'general' && <General />}
        {tab === 'integrations' && <Integrations />}
        {tab === 'import' && <Import />}
        {tab === 'users' && <Users />}
        {tab === 'profile' && <Profile />}
        {tab === 'api' && <Tokens />}
        {tab === 'data' && <Data />}
        {tab === 'about' && <About />}
      </div>
    </>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <div className="two-col">
      <Card title="Preferences">
        <div className="grid" style={{ gap: 14 }}>
          <Field label="App title"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} disabled={!isAdmin} /></Field>
          <Field label="Home currency" help="The default for new trips and budgets."><CurrencySelect value={f.home_currency} onChange={c => setF({ ...f, home_currency: c })} disabled={!isAdmin} /></Field>
          <div className="two-col tight">
            <Field label="Date format"><Select value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} disabled={!isAdmin} /></Field>
            <Field label="Time format"><Select value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} disabled={!isAdmin} /></Field>
          </div>
          <div className="two-col tight">
            <Field group label="Distances"><Segmented value={f.distance_unit} onChange={v => setF({ ...f, distance_unit: v as 'km' | 'mi' })} options={[{ value: 'km', label: 'km' }, { value: 'mi', label: 'miles' }]} size="sm" /></Field>
            <Field label="Map tiles"><Select value={f.map_tiles} onChange={v => setF({ ...f, map_tiles: v })} options={[{ value: 'osm', label: 'OpenStreetMap' }, { value: 'carto', label: 'CARTO Voyager' }, { value: 'topo', label: 'OpenTopoMap' }]} disabled={!isAdmin} /></Field>
          </div>
          <label className="row"><input type="checkbox" checked={f.fx_auto} onChange={e => setF({ ...f, fx_auto: e.target.checked })} style={{ width: 'auto' }} disabled={!isAdmin || f.fx_provider === 'none'} /> Look up exchange rates automatically (Frankfurter / open.er-api.com){f.fx_provider === 'none' && <span className="muted small"> — disabled on the server</span>}</label>
          {isAdmin ? <div><button className="btn primary" onClick={() => save()} disabled={busy}>Save</button></div> : <p className="muted small">Only an admin can change these.</p>}
        </div>
      </Card>
      <Card title="How it fits together">
        <table className="table compact"><tbody>
          <tr><th>Trips</th><td className="muted">Dates, a home currency and a budget. Everything else hangs off a trip.</td></tr>
          <tr><th>Places</th><td className="muted">Ideas until you drag them onto a day. Tick them off as you go.</td></tr>
          <tr><th>Budget</th><td className="muted">Stays and transport add their price as planned cost; Firefly brings the real spending in.</td></tr>
          <tr><th>Recorded</th><td className="muted">Dawarich draws where you actually went; Immich shows what you photographed.</td></tr>
          <tr><th>Outside</th><td className="muted">Places search and discover use OpenStreetMap; the weather is Open-Meteo. No keys needed.</td></tr>
        </tbody></table>
      </Card>
    </div>
  )
}

function Integrations() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  const { toast } = useToast()
  const [test, setTest] = useState<Record<string, string>>({})
  if (!f) return <Loading />
  const run = async (which: 'firefly' | 'dawarich' | 'immich', url: string, key: string) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const block = (which: 'firefly' | 'dawarich' | 'immich', title: string, urlKey: 'firefly_url' | 'dawarich_url' | 'immich_url', keyKey: 'firefly_token' | 'dawarich_api_key' | 'immich_api_key', what: string, help: string) => (
    <Card title={title} actions={<span className={`badge ${f[`${which}_configured` as 'firefly_configured'] ? 'cat' : ''}`}>{f[`${which}_configured` as 'firefly_configured'] ? 'connected' : 'not set'}</span>}>
      <p className="muted small">{what}</p>
      <div className="grid" style={{ gap: 12 }}>
        <Field label="URL" help={help}><input value={f[urlKey]} onChange={e => setF({ ...f, [urlKey]: e.target.value })} placeholder="http://host.docker.internal:8080" disabled={!isAdmin} /></Field>
        <Field label={which === 'firefly' ? 'Personal access token' : 'API key'} help="Stored on the server; shown masked. Leave as is to keep the current one."><input value={f[keyKey]} onChange={e => setF({ ...f, [keyKey]: e.target.value })} disabled={!isAdmin} autoComplete="off" /></Field>
        {which === 'immich' && <Field label="Public URL (optional)" help="What the browser opens for “Open in Immich” when the API URL above is internal (e.g. host.docker.internal)."><input value={f.immich_public_url} onChange={e => setF({ ...f, immich_public_url: e.target.value })} placeholder="https://photos.example.com" disabled={!isAdmin} /></Field>}
        <div className="row"><button className="btn sm" onClick={() => run(which, f[urlKey], f[keyKey])} disabled={!f[urlKey]}>Test connection</button><span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span></div>
      </div>
    </Card>
  )
  return (
    <div className="grid" style={{ gap: 16 }}>
      {block('firefly', '💶 Firefly III', 'firefly_url', 'firefly_token', 'What the trip really cost. Tag the withdrawals in Firefly (e.g. "lisbon 2027") and set the same tag on the trip; they appear as spent, in their own currency, converted at the day\'s rate. Synced every 30 minutes while a trip is running or upcoming, and on demand.', 'Create the token under Firefly → Options → Profile → OAuth → Personal access tokens.')}
      {block('dawarich', '🛰 Dawarich', 'dawarich_url', 'dawarich_api_key', 'Where you actually went. The recorded route is drawn on the map, each day shows the distance and the detected stops, and the GPX export can include the track.', 'Dawarich → Account → API key. Behind HTTPS use the public name; plain http works too.')}
      {block('immich', '📷 Immich', 'immich_url', 'immich_api_key', 'Every photo and video taken between the trip dates, grouped by day; the geotagged ones land on the map. Thumbnails are proxied so the browser never sees the key.', 'Immich → Account settings → API keys.')}
      {isAdmin && <div><button className="btn primary" onClick={() => save()} disabled={busy}>Save integrations</button></div>}
    </div>
  )
}

function Import() {
  const { toast } = useToast()
  const navigate = useNavigate()
  const [al, setAL] = useState({ url: '', username: '', password: '' })
  const [cols, setCols] = useState<{ id: string; name: string; start_date: string; end_date: string; location_count: number; is_archived: boolean }[] | null>(null)
  const [sel, setSel] = useState<string[]>([])
  const [images, setImages] = useState(true)
  const [busy, setBusy] = useState('')
  const [result, setResult] = useState<{ results: { collection: string; trip_id?: number; name?: string; counts?: Record<string, number>; error?: string }[] } | null>(null)
  const list = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy('list')
    try { const c = await api.post<typeof cols>(`${V1}/import/adventurelog/collections`, al); setCols(c); setSel((c || []).filter(x => !x.is_archived).map(x => x.id)) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') }
  }
  const run = async () => {
    setBusy('import')
    try { setResult(await api.post(`${V1}/import/adventurelog`, { ...al, collections: sel, images })) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') }
  }
  const importJSON = async (file: File) => {
    setBusy('json')
    try { const t = await api.post<{ id: number; name: string }>(`${V1}/import/trip`, JSON.parse(await file.text())); toast(`Imported ${t.name}`); navigate(`/trips/${t.id}`) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') }
  }
  return (
    <div className="two-col">
      <Card title="From AdventureLog">
        <p className="muted small">Collections become trips; locations, lodging, transportation, notes and checklists come along, with their images. Re-running skips what is already here. Legs that AdventureLog imported from Dawarich are left out — the route is live here.</p>
        <form onSubmit={list} className="grid" style={{ gap: 12 }}>
          <Field label="AdventureLog URL"><input value={al.url} onChange={e => setAL({ ...al, url: e.target.value })} placeholder="https://adventurelog.example.com" required /></Field>
          <div className="two-col tight">
            <Field label="Username"><input value={al.username} onChange={e => setAL({ ...al, username: e.target.value })} required autoComplete="off" /></Field>
            <Field label="Password"><input type="password" value={al.password} onChange={e => setAL({ ...al, password: e.target.value })} required autoComplete="new-password" /></Field>
          </div>
          <div><button className="btn" disabled={busy === 'list'}>{busy === 'list' ? 'Connecting…' : 'List collections'}</button></div>
        </form>
        {cols && (
          <div className="mt">
            {cols.length === 0 ? <p className="muted">No collections found.</p> : <>
              <table className="table compact"><tbody>{cols.map(c => <tr key={c.id}><td style={{ width: 28 }}><input type="checkbox" checked={sel.includes(c.id)} onChange={e => setSel(s => e.target.checked ? [...s, c.id] : s.filter(x => x !== c.id))} style={{ width: 'auto' }} /></td><td><b>{c.name}</b>{c.is_archived && <span className="badge" style={{ marginLeft: 6 }}>archived</span>}</td><td className="muted small">{c.start_date} → {c.end_date}</td><td className="muted small num">{c.location_count} places</td></tr>)}</tbody></table>
              <label className="row mt-sm"><input type="checkbox" checked={images} onChange={e => setImages(e.target.checked)} style={{ width: 'auto' }} /> Download images (slower)</label>
              <div className="mt-sm"><button className="btn primary" onClick={run} disabled={busy === 'import' || sel.length === 0}>{busy === 'import' ? 'Importing… this can take a minute' : `Import ${sel.length} collection${sel.length === 1 ? '' : 's'}`}</button></div>
            </>}
          </div>
        )}
        {result && <div className="callout success mt">{result.results.map((r, i) => <div key={i}>{r.error ? <span className="bad">✕ {r.collection}: {r.error}</span> : <span>✓ <a href={`/trips/${r.trip_id}`}>{r.name}</a> — {Object.entries(r.counts || {}).map(([k, v]) => `${v} ${k}`).join(', ') || 'nothing new'}</span>}</div>)}</div>}
      </Card>
      <Card title="From a JSON backup">
        <p className="muted small">A trip exported from Pocket Travel (trip settings → JSON backup) imports as a new trip owned by you.</p>
        <label className="btn">{busy === 'json' ? 'Importing…' : 'Choose file…'}<input type="file" accept="application/json,.json" hidden onChange={e => e.target.files?.[0] && importJSON(e.target.files[0])} /></label>
      </Card>
    </div>
  )
}

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Card title="Users" actions={isAdmin && <button className="btn primary sm" onClick={() => setEdit({ role: 'member' })}>+ Add user</button>}>
      <p className="muted">Everyone signs in with their own account. A trip is visible to its owner and the companions added under the trip's settings; admins see everything and manage users and integrations.</p>
      <table className="table compact">
        <thead><tr><th /><th>Name</th><th>Email</th><th>Role</th><th /></tr></thead>
        <tbody>
          {users.map(u => (
            <tr key={u.id}>
              <td style={{ width: 32 }}><Avatar user={u} /></td>
              <td>{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge" style={{ marginLeft: 6 }}>you</span>}</td>
              <td className="muted">{u.email}</td>
              <td><span className={`badge ${u.role === 'admin' ? 'cat' : ''}`}>{u.role}</span></td>
              <td className="right">{isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm title={`Remove ${del.name || del.email}?`} message="Their account and the trips they own are deleted." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Card>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit user' : 'Add user'} onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add user'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Card title="Profile">
      <form onSubmit={save} className="grid" style={{ gap: 14, maxWidth: 480 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
        <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        {status?.auth_mode === 'local' && <>
          <hr />
          <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
          <Field label="New password" help="At least 8 characters. Leave empty to keep the current one."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
        </>}
        <div><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Card>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <Card title="API tokens">
      <p className="muted">Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and change your trips, feed a Homepage widget (<code>GET /api/v1/dashboard</code>) and authorise a calendar subscription (<code>/cal/&lt;trip&gt;.ics?token=…</code>). See <code>docs/api.md</code>.</p>
      <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage widget)" /><button className="btn primary">Create</button></form>
      {fresh && <div className="callout success mb"><b>Copy it now — it is shown once.</b><pre className="mono" style={{ margin: '8px 0 0' }}>{fresh}</pre></div>}
      {data && data.length > 0 ? (
        <table className="table compact">
          <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
          <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
        </table>
      ) : <p className="muted">No tokens yet.</p>}
      {del && <Confirm title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
    </Card>
  )
}

function Data() {
  return (
    <div className="two-col">
      <Card title="Backup">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>) with every trip. Uploaded documents and images live next to it under <code>files/</code>. Restore by stopping the app and replacing <code>pocket-travel.db</code> in the data directory.</p>
        <a className="btn primary" href={`${V1}/backup`}>Download database</a>
      </Card>
      <Card title="Per-trip exports">
        <p className="muted">Each trip has its own exports under its Settings tab: a calendar (.ics) you can subscribe to, GPX for maps and watches, and a JSON backup that imports again under Settings → Import.</p>
      </Card>
    </div>
  )
}

function About() {
  const { data } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const counts = (data?.counts || {}) as Record<string, number>
  return (
    <Card title="About">
      <p><b>Pocket Travel</b> is a self-hosted trip planner: itinerary, places, stays, transport, budget, packing lists and a journal — with the real spending from Firefly III, the recorded route from Dawarich and the photos from Immich on the same page. One binary, one SQLite file, one volume.</p>
      <p className="muted small">Open source (MIT) — <a href="https://github.com/abjelosevic88/pocket-homelab-travel" target="_blank" rel="noreferrer">github.com/abjelosevic88/pocket-homelab-travel</a>. Sibling projects: Pocket Habits, Pocket Inventory, Pocket Bookshelf, Pocket Split, Pocket Clips, Pocket Subtitles and Pocket Invoicing. Maps © OpenStreetMap contributors; weather by Open-Meteo.</p>
      {data && (
        <table className="table compact mt">
          <tbody>
            <tr><th>Version</th><td className="mono">{String(data.version)}</td></tr>
            <tr><th>Runtime</th><td className="mono">{String(data.go)} · {String(data.os)}/{String(data.arch)}</td></tr>
            <tr><th>Auth mode</th><td className="mono">{String(data.auth_mode)}</td></tr>
            <tr><th>Database</th><td className="mono">{String(data.db_path)} · {fmtBytes(Number(data.db_bytes))} · files {fmtBytes(Number(data.files_bytes))}</td></tr>
            <tr><th>Timezone</th><td className="mono">{String(data.timezone)}</td></tr>
            <tr><th>Uptime</th><td className="mono">{String(data.uptime)}</td></tr>
            <tr><th>Rows</th><td className="mono">{counts.trips} trips · {counts.places} places · {counts.expenses} expenses · {counts.journal} journal · {counts.documents} documents · {counts.users} users</td></tr>
          </tbody>
        </table>
      )}
    </Card>
  )
}
