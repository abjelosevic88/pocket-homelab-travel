import { useState } from 'react'
import { api, uploadForm, V1 } from '../../lib/api'
import type { Doc } from '../../lib/types'
import { cap, fmtBytes, fmtDate } from '../../lib/format'
import { Card, Confirm, Empty, useToast } from '../../components/ui'
import { Icon } from '../../components/icons'
import { Select } from '../../components/Select'
import { useTrip } from './ctx'

const KINDS = ['ticket', 'booking', 'insurance', 'id', 'map', 'other']
const KIND_ICON: Record<string, string> = { ticket: '🎫', booking: '🛏', insurance: '🛡', id: '🪪', map: '🗺', other: '📄' }

export function Documents() {
  const { ov, trip, canEdit, patch } = useTrip()
  const { toast } = useToast()
  const [kind, setKind] = useState('other')
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState<Doc | null>(null)
  const [drag, setDrag] = useState(false)
  const base = `${V1}/trips/${trip.id}/documents`
  const upload = async (files: FileList | File[] | null) => {
    if (!files || !files.length) return
    setBusy(true)
    try { const added: Doc[] = []; for (const f of Array.from(files)) added.push(await uploadForm<Doc>(base, f, { kind })); patch({ documents: [...ov.documents, ...added] }); toast(`Uploaded ${added.length}`) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const remove = async () => { if (!del) return; try { await api.del(`${base}/${del.id}`); patch({ documents: ov.documents.filter(d => d.id !== del.id) }); setDel(null) } catch (e) { toast((e as Error).message, 'err') } }
  const save = async (d: Doc, body: Partial<Doc>) => { try { const nd = await api.put<Doc>(`${base}/${d.id}`, body); patch({ documents: ov.documents.map(x => x.id === d.id ? nd : x) }) } catch (e) { toast((e as Error).message, 'err') } }
  const ext = (d: Doc) => (d.name.includes('.') ? d.name.split('.').pop()! : d.mime.split('/').pop() || 'file').slice(0, 4)
  const linked = (d: Doc) => {
    if (d.link_kind === 'lodging') return ov.lodging.find(l => l.id === d.link_id)?.name
    if (d.link_kind === 'place') return ov.places.find(p => p.id === d.link_id)?.name
    if (d.link_kind === 'transport') { const t = ov.transport.find(t => t.id === d.link_id); return t && (t.name || `${t.from_name} → ${t.to_name}`) }
    return undefined
  }
  const groups = KINDS.filter(k => ov.documents.some(d => d.kind === k)).map(k => ({ kind: k, docs: ov.documents.filter(d => d.kind === k) }))
  return (
    <Card title={`Documents · ${ov.documents.length}`} actions={canEdit && <span className="row"><span className="muted small">Upload as</span><Select className="doc-kind" value={kind} onChange={setKind} options={KINDS.map(k => ({ value: k, label: `${KIND_ICON[k]} ${cap(k)}` }))} /><label className="btn primary sm"><Icon name="upload" size={14} />{busy ? 'Uploading…' : 'Upload…'}<input type="file" multiple hidden onChange={e => upload(e.target.files)} /></label></span>}>
      <div className={`dropzone ${drag ? 'over' : ''}`} onDragOver={e => { if (canEdit) { e.preventDefault(); setDrag(true) } }} onDragLeave={() => setDrag(false)} onDrop={e => { e.preventDefault(); setDrag(false); if (canEdit) upload(e.dataTransfer.files) }}>
        {ov.documents.length === 0 ? <Empty icon="file" title="No documents yet">{canEdit ? 'Tickets, booking confirmations, insurance, passport scans — drop files here or use Upload. They are stored next to the database and backed up with it.' : 'Tickets, bookings and insurance would show up here.'}</Empty> : groups.map(g => (
          <div key={g.kind} className="doc-group">
            <h3>{KIND_ICON[g.kind]} {cap(g.kind)} <span className="muted small">· {g.docs.length}</span></h3>
            {g.docs.map(d => (
              <div key={d.id} className="doc-row">
                <a className="ext" href={`${base}/${d.id}/file`} target="_blank" rel="noreferrer" title="Open">{ext(d)}</a>
                <div className="doc-main">
                  {canEdit ? <input className="inline-edit doc-name" defaultValue={d.name} onBlur={e => e.target.value.trim() && e.target.value !== d.name && save(d, { name: e.target.value.trim() })} onKeyDown={e => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur() }} /> : <a className="doc-name" href={`${base}/${d.id}/file`} target="_blank" rel="noreferrer">{d.name}</a>}
                  <div className="small muted">{fmtBytes(d.bytes)} · {fmtDate(d.created_at)}{linked(d) && <> · <span title={d.link_kind} style={{ display: 'inline-flex', alignItems: 'center', gap: 3, verticalAlign: 'bottom' }}><Icon name={d.link_kind === 'lodging' ? 'bed' : d.link_kind === 'place' ? 'pin' : 'plane'} size={12} />{linked(d)}</span></>}</div>
                </div>
                {canEdit ? <Select className="doc-kind" value={d.kind} onChange={k => save(d, { kind: k })} options={KINDS.map(k => ({ value: k, label: `${KIND_ICON[k]} ${cap(k)}` }))} /> : <span className="badge">{d.kind}</span>}
                <a className="btn ghost icon-only sm" href={`${base}/${d.id}/file?download=1`} title="Download" aria-label="Download"><Icon name="download" size={15} /></a>
                {canEdit && <button type="button" className="btn ghost icon-only sm danger-text" onClick={() => setDel(d)} title="Delete" aria-label="Delete"><Icon name="trash" size={15} /></button>}
              </div>
            ))}
          </div>
        ))}
        {ov.documents.length > 0 && canEdit && <p className="muted small mt" style={{ margin: '12px 0 0' }}>Drop files anywhere on this card to upload them as “{cap(kind)}”. Click a name to rename it.</p>}
      </div>
      {del && <Confirm title={`Delete ${del.name}?`} message="The file is removed from the server." onCancel={() => setDel(null)} onConfirm={remove} />}
    </Card>
  )
}
