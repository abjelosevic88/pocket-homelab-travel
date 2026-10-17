import { useState } from 'react'
import { Link } from 'react-router-dom'
import { V1 } from '../../lib/api'
import type { Photo } from '../../lib/types'
import { fmtDate, fmtISOTime, plural, weekday } from '../../lib/format'
import { Empty, Modal } from '../../components/ui'
import { useTrip } from './ctx'

export function Photos() {
  const { ov, trip, photos, extErrors, refreshExternal } = useTrip()
  const [open, setOpen] = useState<Photo | null>(null)
  const integ = ov.integrations.immich
  if (!integ.configured) return <Empty title="Immich is not connected">Add the Immich URL and an API key under <Link to="/settings?tab=integrations">Settings → Integrations</Link> and every photo taken during the trip shows up here, by day.</Empty>
  if (!trip.immich) return <Empty title="Photos are off for this trip">Turn them on under the trip's settings.</Empty>
  if (extErrors.immich) return <div className="callout danger">Immich: {extErrors.immich}</div>
  if (photos.length === 0) return <Empty title="No photos in this date range">{trip.start_date ? 'Immich has nothing taken between the trip dates (yet).' : 'Set the trip dates first.'}</Empty>
  const byDay = new Map<string, Photo[]>()
  for (const p of photos) { const d = p.taken_at.slice(0, 10); byDay.set(d, [...(byDay.get(d) || []), p]) }
  const idx = open ? photos.indexOf(open) : -1
  const step = (n: number) => { const p = photos[idx + n]; if (p) setOpen(p) }
  return (
    <>
      <div className="toolbar"><span className="muted">{plural(photos.length, 'photo')} from Immich across {byDay.size} days{photos.some(p => p.lat != null) && ' · the ones with GPS are on the map'}</span><span className="spacer" /><button className="btn ghost sm" onClick={refreshExternal}>↻ Refresh</button></div>
      {Array.from(byDay.entries()).map(([d, ps]) => (
        <div key={d} className="photo-day">
          <h3>{weekday(d, true)}, {fmtDate(d)} <span className="muted small">· {ps.length}</span></h3>
          <div className="photo-grid">{ps.map(p => <a key={p.id} href={`${integ.url}/photos/${p.id}`} onClick={e => { e.preventDefault(); setOpen(p) }}><img src={`${V1}/trips/${trip.id}/photos/${p.id}/thumb`} loading="lazy" alt={p.file_name} />{p.type === 'VIDEO' && <span className="vid">▶ {p.duration?.slice(3, 8)}</span>}{p.favorite && <span className="vid">♥</span>}</a>)}</div>
        </div>
      ))}
      {open && (
        <Modal title={<span>{fmtDate(open.taken_at.slice(0, 10))} {fmtISOTime(open.taken_at)} <span className="muted small">{open.city && `· ${open.city}`}</span></span>} onClose={() => setOpen(null)} size="lg">
          <div onKeyDown={e => { if (e.key === 'ArrowRight') step(1); if (e.key === 'ArrowLeft') step(-1) }} tabIndex={0}>
            <img className="lightbox-img" src={`${V1}/trips/${trip.id}/photos/${open.id}/thumb?size=preview`} alt={open.file_name} />
            <div className="row mt-sm"><button className="btn sm" onClick={() => step(-1)} disabled={idx <= 0}>← Previous</button><button className="btn sm" onClick={() => step(1)} disabled={idx >= photos.length - 1}>Next →</button><span className="spacer" /><span className="muted small">{open.file_name}</span><a className="btn sm" href={`${integ.url}/photos/${open.id}`} target="_blank" rel="noreferrer">Open in Immich</a></div>
          </div>
        </Modal>
      )}
    </>
  )
}
