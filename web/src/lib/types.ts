export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }
export interface Currency { code: string; name: string; decimals: number }

export type TripStatus = 'planning' | 'booked' | 'active' | 'completed' | 'cancelled'
export interface TripMember { user_id: number; name: string; email: string; role: string }
export interface TripCounts { places: number; visited: number; lodging: number; transport: number; documents: number; journal: number; checklist_items: number; checklist_done: number; planned: number; actual: number; total: number }
export interface Trip {
  id: number; owner_id: number; name: string; destination: string; description: string
  start_date: string; end_date: string; status: TripStatus; currency: string; budget: number
  color: string; has_cover: boolean; link: string; notes: string; lat: number | null; lng: number | null; timezone: string
  firefly_tag: string; dawarich: boolean; immich: boolean; archived: boolean; external_id: string
  created_at: string; updated_at: string; days: number; role?: string; members?: TripMember[]; counts?: TripCounts
}
export type PlaceStatus = 'idea' | 'planned' | 'booked' | 'visited' | 'skipped'
export interface Place {
  id: number; trip_id: number; name: string; category: string; description: string; address: string
  lat: number | null; lng: number | null; link: string; phone: string; rating: number | null; price: number; currency: string
  day_date: string; start_time: string; duration_min: number; status: PlaceStatus; visited_at: string; booking_ref: string
  tags: string; notes: string; has_image: boolean; sort_order: number; external_id: string; created_at: string; updated_at: string
}
export interface Lodging {
  id: number; trip_id: number; name: string; type: string; address: string; lat: number | null; lng: number | null
  check_in: string; check_out: string; reservation_number: string; link: string; phone: string; price: number; currency: string
  paid: boolean; rating: number | null; notes: string; has_image: boolean; external_id: string; created_at: string; updated_at: string
}
export interface Transport {
  id: number; trip_id: number; type: string; name: string; carrier: string; number: string
  from_name: string; from_lat: number | null; from_lng: number | null; to_name: string; to_lat: number | null; to_lng: number | null
  depart_at: string; arrive_at: string; booking_ref: string; seat: string; link: string; price: number; currency: string; paid: boolean
  distance_km: number; notes: string; external_id: string; created_at: string; updated_at: string
}
export interface Expense {
  id: number; trip_id: number; name: string; category: string; amount: number; currency: string; amount_home: number; fx_rate: number
  date: string; kind: 'planned' | 'actual'; source: 'manual' | 'firefly' | 'item'; external_id: string; link_kind: string; link_id: number
  notes: string; created_at: string; updated_at: string
}
export interface ChecklistItem { id: number; checklist_id: number; text: string; done: boolean; assignee_id: number | null; sort_order: number; created_at: string }
export interface Checklist { id: number; trip_id: number; name: string; kind: string; sort_order: number; created_at: string; items: ChecklistItem[] }
export interface JournalEntry { id: number; trip_id: number; user_id: number; user_name: string; date: string; title: string; body: string; mood: string; lat: number | null; lng: number | null; created_at: string; updated_at: string }
export interface Doc { id: number; trip_id: number; user_id: number; name: string; mime: string; bytes: number; kind: string; link_kind: string; link_id: number; created_at: string }
export interface Activity { id: number; trip_id: number; user_id: number | null; user_name: string; action: string; subject: string; created_at: string }

export interface CategoryTotal { category: string; planned: number; actual: number }
export interface Budget {
  currency: string; budget: number; planned: number; actual: number; remaining: number; per_day: number; total: number; bookings: number
  by_category: CategoryTotal[]; by_day: { date: string; actual: number; count: number }[]; by_currency: Record<string, number>
  top: Expense[]; firefly_count: number; days: number; days_elapsed: number; sources: Record<string, number>
}
export interface Overview {
  trip: Trip; places: Place[]; lodging: Lodging[]; transport: Transport[]; expenses: Expense[]; checklists: Checklist[]
  journal: JournalEntry[]; documents: Doc[]; budget: Budget
  integrations: { firefly: { configured: boolean; tag: string; synced_at: string }; dawarich: { configured: boolean; enabled: boolean }; immich: { configured: boolean; enabled: boolean; url: string }; ai: { configured: boolean; model: string } }
}
export interface DayWeather { date: string; code: number; label: string; tmax: number; tmin: number; precip_mm: number; wind_kmh: number; sunrise?: string; sunset?: string; source: 'forecast' | 'archive' }
export interface Route { points: [number, number, number][]; raw_count: number; distance_km: number; days_km: Record<string, number>; bounds?: [number, number, number, number] }
export interface TimelineEntry { type: 'journey' | 'visit'; track_id?: number; visit_id?: number; name?: string; status?: string; started_at: string; ended_at: string; duration: number; distance: number; dominant_mode?: string; avg_speed?: number; elevation_gain?: number; latitude?: number; longitude?: number; point_count?: number }
export interface TimelineDay { date: string; summary: { total_distance: number; distance_unit: string; places_visited: number; time_moving_minutes: number; time_stationary_minutes: number; mode_distances: Record<string, number> }; entries: TimelineEntry[] }
export interface Photo { id: string; type: 'IMAGE' | 'VIDEO'; file_name: string; taken_at: string; lat: number | null; lng: number | null; city: string; country: string; thumbhash: string; favorite: boolean; duration?: string }
export interface GeoResult { name: string; display: string; lat: number; lng: number; type: string; category: string; country: string; city: string; website?: string; phone?: string; hours?: string; source: string; osm_id?: string }
export interface Settings {
  title: string; home_currency: string; date_format: string; time_format: string; distance_unit: 'km' | 'mi'; fx_auto: boolean; fx_provider: string; map_tiles: string
  firefly_url: string; firefly_token: string; firefly_configured: boolean
  dawarich_url: string; dawarich_api_key: string; dawarich_configured: boolean
  immich_url: string; immich_public_url: string; immich_api_key: string; immich_configured: boolean
  openai_api_key: string; openai_model: string; openai_base_url: string; openai_configured: boolean
  max_upload_bytes: number; setup_complete: boolean
}
export interface Meta { place_categories: string[]; expense_categories: string[]; discover_categories: string[]; packing_templates: string[]; lodging_types: string[]; transport_types: string[] }
export interface HomeData {
  stats: { trips: number; upcoming: number; days: number; places: number; visited: number; destinations: string[]; next_trip_id: number }
  current: Trip | null; next: Trip | null; upcoming: Trip[]; recent: Trip[]
  today?: { date: string; day_number: number; places: Place[]; stay: Lodging | null; budget: Budget }
}

export const PLACE_ICONS: Record<string, string> = { sight: '🏛', museum: '🖼', nature: '🌿', beach: '🏖', viewpoint: '🌄', food: '🍽', cafe: '☕', bar: '🍸', shop: '🛍', activity: '🎟', event: '🎭', nightlife: '🌙', wellness: '🧖', transport: '🚏', other: '📍', historic: '🏰', lodging: '🛏' }
export const TRANSPORT_ICONS: Record<string, string> = { flight: '✈️', train: '🚆', bus: '🚌', car: '🚗', ferry: '⛴', taxi: '🚕', bike: '🚲', walk: '🚶', other: '➡️' }
export const LODGING_ICONS: Record<string, string> = { hotel: '🏨', apartment: '🏢', hostel: '🛏', house: '🏡', camping: '⛺', friends: '🫶', other: '🛏' }
export const EXPENSE_ICONS: Record<string, string> = { transport: '🚗', lodging: '🛏', food: '🍽', activities: '🎟', shopping: '🛍', fees: '🏧', fuel: '⛽', groceries: '🛒', insurance: '🛡', other: '💶' }
export const STATUS_LABEL: Record<string, string> = { planning: 'Planning', booked: 'Booked', active: 'On the road', completed: 'Completed', cancelled: 'Cancelled' }
export const PLACE_STATUS_LABEL: Record<string, string> = { idea: 'Idea', planned: 'Planned', booked: 'Booked', visited: 'Visited', skipped: 'Skipped' }
export const WX_ICON = (code: number) => code === 0 ? '☀️' : code <= 2 ? '🌤' : code === 3 ? '☁️' : code <= 49 ? '🌫' : code <= 59 ? '🌦' : code <= 69 ? '🌧' : code <= 79 ? '🌨' : code <= 84 ? '🌦' : code <= 94 ? '🌨' : '⛈'
