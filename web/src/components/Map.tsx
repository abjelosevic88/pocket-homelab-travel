import { useEffect, useRef } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { useApp } from '../lib/app-context'

export interface Marker { id: string; lat: number; lng: number; label: string; icon?: string; kind?: string; number?: number; popup?: string; onClick?: () => void }
export interface Line { points: [number, number][]; color?: string; dashed?: boolean; weight?: number; noFit?: boolean; opacity?: number; tooltip?: string }

const OSM_ATTR = '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
const ESRI_ATTR = 'Tiles &copy; Esri'
export const TILES: Record<string, { label: string; url: string; attr: string; overlay?: string; maxZoom?: number }> = {
  osm: { label: 'OpenStreetMap', url: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png', attr: OSM_ATTR },
  hot: { label: 'OSM Humanitarian (clean, good labels)', url: 'https://{s}.tile.openstreetmap.fr/hot/{z}/{x}/{y}.png', attr: OSM_ATTR + ', style <a href="https://www.hotosm.org/">HOT</a>' },
  cyclosm: { label: 'CyclOSM (paths, trails, terrain)', url: 'https://{s}.tile-cyclosm.openstreetmap.fr/cyclosm/{z}/{x}/{y}.png', attr: OSM_ATTR + ', <a href="https://www.cyclosm.org/">CyclOSM</a>' },
  topo: { label: 'OpenTopoMap (hiking)', url: 'https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png', attr: OSM_ATTR + ', <a href="https://opentopomap.org">OpenTopoMap</a>', maxZoom: 17 },
  'esri-streets': { label: 'Esri World Street', url: 'https://server.arcgisonline.com/ArcGIS/rest/services/World_Street_Map/MapServer/tile/{z}/{y}/{x}', attr: ESRI_ATTR },
  'esri-topo': { label: 'Esri World Topo', url: 'https://server.arcgisonline.com/ArcGIS/rest/services/World_Topo_Map/MapServer/tile/{z}/{y}/{x}', attr: ESRI_ATTR },
  'esri-satellite': { label: 'Esri Satellite (with labels)', url: 'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}', attr: ESRI_ATTR + ', Maxar, Earthstar Geographics', overlay: 'https://server.arcgisonline.com/ArcGIS/rest/services/Reference/World_Boundaries_and_Places/MapServer/tile/{z}/{y}/{x}' },
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
  const tiles = TILES[settings?.map_tiles || 'osm'] || TILES.osm // 'carto' from older settings falls back to OSM
  const lastFit = useRef('')
  useEffect(() => {
    if (!ref.current || map.current) return
    const m = L.map(ref.current, { zoomControl: true, attributionControl: true, scrollWheelZoom: true })
    m.setView(center || [45, 15], zoom || 5)
    L.tileLayer(tiles.url, { attribution: tiles.attr, maxZoom: tiles.maxZoom || 19 }).addTo(m)
    if (tiles.overlay) L.tileLayer(tiles.overlay, { maxZoom: tiles.maxZoom || 19, pane: 'tilePane' }).addTo(m)
    layer.current = L.layerGroup().addTo(m)
    map.current = m
    // the sidebar opening or closing resizes the map without a render: keep the tiles filling it
    const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => m.invalidateSize()) : null
    ro?.observe(ref.current)
    return () => { ro?.disconnect(); m.remove(); map.current = null }
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
