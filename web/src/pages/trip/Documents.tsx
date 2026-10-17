import { useState } from 'react'
import { api, uploadForm, V1 } from '../../lib/api'
import type { Doc } from '../../lib/types'
import { cap, fmtBytes, fmtDate } from '../../lib/format'
import { Card, Confirm, Empty, useToast } from '../../components/ui'
import { Select } from '../../components/Select'
import { useTrip } from './ctx'

const KINDS = ['ticket', 'booking', 'insurance', 'id', 'map', 'other']

export function Documents() {
  const { ov, trip, canEdit, patch } = useTrip()
  const { toast } = useToast()
  const [kind, setKind] = useState('other')
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState<Doc | null>(null)
  const base = `${V1}/trips/${trip.id}/documents`
  const upload = async (files: FileList | null) => {
    if (!files?.length) return
    setBusy(true)
    try { const added: Doc[] = []; for (const f of Array.from(files)) added.push(await uploadForm<Doc>(base, f, { kind })); patch({ documents: [...ov.documents, ...added] }); toast(`Uploaded ${added.length}`) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const remove = async () => { if (!del) return; try { await api.del(`${base}/${del.id}`); patch({ documents: ov.documents.filter(d => d.id !== del.id) }); setDel(null) } catch (e) { toast((e as Error).message, 'err') } }
  const rename = async (d: Doc, name: string) => { if (!name.trim() || name === d.name) return; try { const nd = await api.put<Doc>(`${base}/${d.id}`, { name }); patch({ documents: ov.documents.map(x => x.id === d.id ? nd : x) }) } catch (e) { toast((e as Error).message, 'err') } }
  const setDocKind = async (d: Doc, k: string) => { try { const nd = await api.put<Doc>(`${base}/${d.id}`, { kind: k }); patch({ documents: ov.documents.map(x => x.id === d.id ? nd : x) }) } catch (e) { toast((e as Error).message, 'err') } }
  const ext = (d: Doc) => (d.name.split('.').pop() || d.mime.split('/').pop() || 'file').slice(0, 4)
  return (
    <Card title={`Documents · ${ov.documents.length}`} actions={canEdit && <span className="row"><Select value={kind} onChange={setKind} options={KINDS.map(k => ({ value: k, label: cap(k) }))} /><label className="btn primary sm">{busy ? 'Uploading…' : 'Upload…'}<input type="file" multiple hidden onChange={e => upload(e.target.files)} /></label></span>}>
      <p className="muted small">Tickets, booking confirmations, insurance, passport scans. Stored on the server next to the database and backed up with it.</p>
      {ov.documents.length === 0 ? <Empty title="No documents yet" /> : ov.documents.map(d => (
        <div key={d.id} className="doc-row">
          <a className="ext" href={`${base}/${d.id}/file`} target="_blank" rel="noreferrer">{ext(d)}</a>
          <div className="grow">
            <input className="inline-edit" defaultValue={d.name} disabled={!canEdit} onBlur={e => rename(d, e.target.value)} onKeyDown={e => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur() }} style={{ fontWeight: 600 }} />
            <div className="small muted">{fmtBytes(d.bytes)} · {fmtDate(d.created_at)}</div>
          </div>
          {canEdit ? <Select value={d.kind} onChange={k => setDocKind(d, k)} options={KINDS.map(k => ({ value: k, label: cap(k) }))} className="toolbar" /> : <span className="badge">{d.kind}</span>}
          <a className="btn ghost sm" href={`${base}/${d.id}/file?download=1`}>↓</a>
          {canEdit && <button className="btn ghost sm danger-text" onClick={() => setDel(d)}>✕</button>}
        </div>
      ))}
      {del && <Confirm title={`Delete ${del.name}?`} message="The file is removed from the server." onCancel={() => setDel(null)} onConfirm={remove} />}
    </Card>
  )
}
