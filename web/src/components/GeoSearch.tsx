import { useEffect, useRef, useState } from 'react'
import { api, V1 } from '../lib/api'
import type { GeoResult } from '../lib/types'
import { useDebounce } from './ui'

/** Search box over Nominatim; picking a result reports it. */
export function GeoSearch({ onPick, placeholder = 'Search a place or address…', initial = '', autoFocus }: { onPick: (r: GeoResult) => void; placeholder?: string; initial?: string; autoFocus?: boolean }) {
  const [q, setQ] = useState(initial)
  const [res, setRes] = useState<GeoResult[]>([])
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [hl, setHl] = useState(0)
  const dq = useDebounce(q, 400)
  const root = useRef<HTMLDivElement>(null)
  const pickedQ = useRef<string | null>(null)
  useEffect(() => {
    if (pickedQ.current !== null && dq === pickedQ.current) { setRes([]); setOpen(false); return }
    if (dq.trim().length < 3) { setRes([]); return }
    // Links and coordinates go straight to the server; plain text waits for 3 characters.

    setBusy(true)
    api.get<GeoResult[]>(`${V1}/geo/search?q=${encodeURIComponent(dq)}&limit=8`).then(r => { setRes(r); setOpen(true); setHl(0) }).catch(() => setRes([])).finally(() => setBusy(false))
  }, [dq])
  useEffect(() => {
    const h = (e: MouseEvent) => { if (root.current && !root.current.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', h)
    return () => document.removeEventListener('mousedown', h)
  }, [])
  const pick = (r: GeoResult) => { pickedQ.current = r.name; setQ(r.name); setRes([]); setOpen(false); onPick(r) }
  return (
    <div ref={root} style={{ position: 'relative' }}>
      <div className="search" style={{ position: 'relative' }}>
        <input value={q} onChange={e => { pickedQ.current = null; setQ(e.target.value) }} placeholder={placeholder} autoFocus={autoFocus} autoComplete="off" onFocus={() => res.length && setOpen(true)}
          onKeyDown={e => {
            if (!open) return
            if (e.key === 'ArrowDown') { e.preventDefault(); setHl(h => Math.min(res.length - 1, h + 1)) }
            else if (e.key === 'ArrowUp') { e.preventDefault(); setHl(h => Math.max(0, h - 1)) }
            else if (e.key === 'Enter') { e.preventDefault(); if (res[hl]) pick(res[hl]) }
            else if (e.key === 'Escape') setOpen(false)
          }} />
        {busy && <span className="spinner sm" style={{ position: 'absolute', right: 10, top: 12 }} />}
      </div>
      {open && res.length > 0 && (
        <div className="search-results">
          {res.map((r, i) => <div key={i} className={`r ${i === hl ? 'hl' : ''}`} onMouseEnter={() => setHl(i)} onMouseDown={e => e.preventDefault()} onClick={() => pick(r)}><span>{r.name}{r.type && r.type !== 'yes' && <span className="muted small"> · {r.type.replace(/_/g, ' ')}</span>}</span><span className="sub">{r.display}</span></div>)}
        </div>
      )}
    </div>
  )
}
