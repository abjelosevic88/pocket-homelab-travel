import { useEffect, useRef } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { useApp } from '../lib/app-context'

export interface Marker { id: string; lat: number; lng: number; label: string; icon?: string; kind?: string; number?: number; popup?: string; onClick?: () => void }
export interface Line { points: [number, number][]; color?: string; dashed?: boolean; weight?: number; noFit?: boolean; opacity?: number; tooltip?: string }

const TILES: Record<string, { url: string; attr: string }> = {
  osm: { url: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png', attr: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>' },
  carto: { url: 'https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png', attr: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> &copy; <a href="https://carto.com/">CARTO</a>' },
  topo: { url: 'https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png', attr: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>, <a href="https://opentopomap.org">OpenTopoMap</a>' },
}

function pinIcon(m: Marker): L.DivIcon {
  const cls = m.number != null ? 'pin num' : `pin ${m.kind || ''}`
  const inner = m.number != null ? String(m.number) : (m.icon || '📍')
  return L.divIcon({ className: '', html: `<div class="${cls}"><span>${inner}</span></div>`, iconSize: [28, 28], iconAnchor: m.number != null ? [12, 12] : [6, 26], popupAnchor: m.number != null ? [0, -10] : [8, -24] })
}

/** A Leaflet map with pins and lines; fits to content unless a center is given. */
export function MapView({ markers, lines = [], className = '', center, zoom, onClick, fitKey }: { markers: Marker[]; lines?: Line[]; className?: string; center?: [number, number]; zoom?: number; onClick?: (lat: number, lng: number) => void; fitKey?: string }) {
  const ref = useRef<HTMLDivElement>(null)
  const map = useRef<L.Map | null>(null)
  const layer = useRef<L.LayerGroup | null>(null)
  const { settings } = useApp()
  const tiles = TILES[settings?.map_tiles || 'osm'] || TILES.osm
  const lastFit = useRef('')
  useEffect(() => {
    if (!ref.current || map.current) return
    const m = L.map(ref.current, { zoomControl: true, attributionControl: true, scrollWheelZoom: true })
    m.setView(center || [45, 15], zoom || 5)
    L.tileLayer(tiles.url, { attribution: tiles.attr, maxZoom: 19 }).addTo(m)
    layer.current = L.layerGroup().addTo(m)
    map.current = m
    return () => { m.remove(); map.current = null }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const m = map.current
    if (!m) return
    m.off('click')
    if (onClick) m.on('click', e => onClick(e.latlng.lat, e.latlng.lng))
  }, [onClick])
  useEffect(() => {
    const m = map.current, g = layer.current
    if (!m || !g) return
    g.clearLayers()
    const bounds: [number, number][] = []
    for (const ln of lines) {
      if (ln.points.length < 2) continue
      const pl = L.polyline(ln.points, { color: ln.color || '#0284c7', weight: ln.weight || 3, opacity: ln.opacity ?? .85, dashArray: ln.dashed ? '6 6' : undefined, lineJoin: 'round', lineCap: 'round' }).addTo(g)
      if (ln.tooltip) pl.bindTooltip(ln.tooltip, { sticky: true })
      if (!ln.noFit) bounds.push(...ln.points)
    }
    for (const mk of markers) {
      const marker = L.marker([mk.lat, mk.lng], { icon: pinIcon(mk), title: mk.label }).addTo(g)
      if (mk.popup) marker.bindPopup(mk.popup)
      if (mk.onClick) marker.on('click', mk.onClick)
      bounds.push([mk.lat, mk.lng])
    }
    const key = fitKey ?? `${markers.length}:${lines.map(l => l.points.length).join(',')}`
    if (bounds.length && key !== lastFit.current) {
      lastFit.current = key
      if (bounds.length === 1) m.setView(bounds[0], zoom || 13)
      else m.fitBounds(L.latLngBounds(bounds), { padding: [30, 30], maxZoom: 15 })
    } else if (!bounds.length && center) m.setView(center, zoom || 10)
    setTimeout(() => m.invalidateSize(), 50)
  }, [markers, lines, center, zoom, fitKey])
  useEffect(() => { const t = setTimeout(() => map.current?.invalidateSize(), 200); return () => clearTimeout(t) })
  return <div ref={ref} className={`map ${className}`} />
}
