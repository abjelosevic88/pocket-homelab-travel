package store

// User is an account that can sign in. The first user is the admin; admins
// manage other users and the shared settings. Trips are shared with members.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Trip statuses.
const (
	StatusPlanning  = "planning"
	StatusBooked    = "booked"
	StatusActive    = "active"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
)

// ValidStatus reports whether s is a trip status.
func ValidStatus(s string) bool {
	switch s {
	case StatusPlanning, StatusBooked, StatusActive, StatusCompleted, StatusCancelled:
		return true
	}
	return false
}

// Trip is one journey.
type Trip struct {
	ID          int64    `json:"id"`
	OwnerID     int64    `json:"owner_id"`
	Name        string   `json:"name"`
	Destination string   `json:"destination"`
	Description string   `json:"description"`
	StartDate   string   `json:"start_date"`
	EndDate     string   `json:"end_date"`
	Status      string   `json:"status"`
	Currency    string   `json:"currency"`
	Budget      int64    `json:"budget"`
	Color       string   `json:"color"`
	CoverPath   string   `json:"-"`
	HasCover    bool     `json:"has_cover"`
	Link        string   `json:"link"`
	Notes       string   `json:"notes"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
	Timezone    string   `json:"timezone"`
	FireflyTag  string   `json:"firefly_tag"`
	Dawarich    bool     `json:"dawarich"`
	Immich      bool     `json:"immich"`
	Archived    bool     `json:"archived"`
	ExternalID  string   `json:"external_id"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`

	// Derived
	Days    int          `json:"days"`
	Role    string       `json:"role,omitempty"` // the requesting user's role
	Members []TripMember `json:"members,omitempty"`
	Counts  *TripCounts  `json:"counts,omitempty"`
}

// TripMember is a user with access to a trip.
type TripMember struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

// TripCounts summarises a trip for list cards.
type TripCounts struct {
	Places    int   `json:"places"`
	Visited   int   `json:"visited"`
	Lodging   int   `json:"lodging"`
	Transport int   `json:"transport"`
	Documents int   `json:"documents"`
	Journal   int   `json:"journal"`
	Checklist int   `json:"checklist_items"`
	Checked   int   `json:"checklist_done"`
	Planned   int64 `json:"planned"` // minor units, trip currency
	Actual    int64 `json:"actual"`
	Total     int64 `json:"total"` // actual + unpaid booked stays/transport
}

// Place categories.
var PlaceCategories = []string{"sight", "museum", "nature", "beach", "viewpoint", "food", "cafe", "bar", "shop", "activity", "event", "nightlife", "wellness", "transport", "other"}

// Place is somewhere to go.
type Place struct {
	ID          int64    `json:"id"`
	TripID      int64    `json:"trip_id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Address     string   `json:"address"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
	Link        string   `json:"link"`
	Phone       string   `json:"phone"`
	Rating      *float64 `json:"rating"`
	Price       int64    `json:"price"`
	Currency    string   `json:"currency"`
	DayDate     string   `json:"day_date"`
	StartTime   string   `json:"start_time"`
	DurationMin int      `json:"duration_min"`
	Status      string   `json:"status"`
	VisitedAt   string   `json:"visited_at"`
	BookingRef  string   `json:"booking_ref"`
	Tags        string   `json:"tags"`
	Notes       string   `json:"notes"`
	ImagePath   string   `json:"-"`
	HasImage    bool     `json:"has_image"`
	SortOrder   int      `json:"sort_order"`
	ExternalID  string   `json:"external_id"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// Lodging is where you sleep.
type Lodging struct {
	ID                int64    `json:"id"`
	TripID            int64    `json:"trip_id"`
	Name              string   `json:"name"`
	Type              string   `json:"type"`
	Address           string   `json:"address"`
	Lat               *float64 `json:"lat"`
	Lng               *float64 `json:"lng"`
	CheckIn           string   `json:"check_in"`
	CheckOut          string   `json:"check_out"`
	ReservationNumber string   `json:"reservation_number"`
	Link              string   `json:"link"`
	Phone             string   `json:"phone"`
	Price             int64    `json:"price"`
	Currency          string   `json:"currency"`
	Paid              bool     `json:"paid"`
	Rating            *float64 `json:"rating"`
	Notes             string   `json:"notes"`
	ImagePath         string   `json:"-"`
	HasImage          bool     `json:"has_image"`
	ExternalID        string   `json:"external_id"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

// Transport is how you get from A to B.
type Transport struct {
	ID         int64    `json:"id"`
	TripID     int64    `json:"trip_id"`
	Type       string   `json:"type"`
	Name       string   `json:"name"`
	Carrier    string   `json:"carrier"`
	Number     string   `json:"number"`
	FromName   string   `json:"from_name"`
	FromLat    *float64 `json:"from_lat"`
	FromLng    *float64 `json:"from_lng"`
	ToName     string   `json:"to_name"`
	ToLat      *float64 `json:"to_lat"`
	ToLng      *float64 `json:"to_lng"`
	DepartAt   string   `json:"depart_at"`
	ArriveAt   string   `json:"arrive_at"`
	BookingRef string   `json:"booking_ref"`
	Seat       string   `json:"seat"`
	Link       string   `json:"link"`
	Price      int64    `json:"price"`
	Currency   string   `json:"currency"`
	Paid       bool     `json:"paid"`
	DistanceKm float64  `json:"distance_km"`
	Notes      string   `json:"notes"`
	ExternalID string   `json:"external_id"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

// Expense categories.
var ExpenseCategories = []string{"transport", "lodging", "food", "activities", "shopping", "fees", "fuel", "groceries", "insurance", "other"}

// Expense is money planned or spent on a trip.
type Expense struct {
	ID         int64   `json:"id"`
	TripID     int64   `json:"trip_id"`
	Name       string  `json:"name"`
	Category   string  `json:"category"`
	Amount     int64   `json:"amount"`
	Currency   string  `json:"currency"`
	AmountHome int64   `json:"amount_home"`
	FXRate     float64 `json:"fx_rate"`
	Date       string  `json:"date"`
	Kind       string  `json:"kind"`   // planned | actual
	Source     string  `json:"source"` // manual | firefly | item
	ExternalID string  `json:"external_id"`
	LinkKind   string  `json:"link_kind"`
	LinkID     int64   `json:"link_id"`
	Notes      string  `json:"notes"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// Checklist groups items (packing, to-do before leaving, shopping…).
type Checklist struct {
	ID        int64           `json:"id"`
	TripID    int64           `json:"trip_id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	SortOrder int             `json:"sort_order"`
	CreatedAt string          `json:"created_at"`
	Items     []ChecklistItem `json:"items"`
}

// ChecklistItem is one line of a checklist.
type ChecklistItem struct {
	ID          int64  `json:"id"`
	ChecklistID int64  `json:"checklist_id"`
	Text        string `json:"text"`
	Done        bool   `json:"done"`
	AssigneeID  *int64 `json:"assignee_id"`
	SortOrder   int    `json:"sort_order"`
	CreatedAt   string `json:"created_at"`
}

// JournalEntry is a diary note written during (or after) the trip.
type JournalEntry struct {
	ID        int64    `json:"id"`
	TripID    int64    `json:"trip_id"`
	UserID    int64    `json:"user_id"`
	UserName  string   `json:"user_name"`
	Date      string   `json:"date"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Mood      string   `json:"mood"`
	Lat       *float64 `json:"lat"`
	Lng       *float64 `json:"lng"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// Document is an uploaded file.
type Document struct {
	ID        int64  `json:"id"`
	TripID    int64  `json:"trip_id"`
	UserID    int64  `json:"user_id"`
	Name      string `json:"name"`
	Filename  string `json:"-"`
	Mime      string `json:"mime"`
	Bytes     int64  `json:"bytes"`
	Kind      string `json:"kind"`
	LinkKind  string `json:"link_kind"`
	LinkID    int64  `json:"link_id"`
	CreatedAt string `json:"created_at"`
}

// Activity is one line of the trip's change log.
type Activity struct {
	ID        int64  `json:"id"`
	TripID    int64  `json:"trip_id"`
	UserID    *int64 `json:"user_id"`
	UserName  string `json:"user_name"`
	Action    string `json:"action"`
	Subject   string `json:"subject"`
	CreatedAt string `json:"created_at"`
}

// Settings are preferences stored as key/value pairs, shared by everyone.
type Settings struct {
	Title           string `json:"title"`
	HomeCurrency    string `json:"home_currency"`
	DateFormat      string `json:"date_format"`
	TimeFormat      string `json:"time_format"`
	DistanceUnit    string `json:"distance_unit"` // km | mi
	FXAuto          bool   `json:"fx_auto"`
	FireflyURL      string `json:"firefly_url"`
	FireflyToken    string `json:"firefly_token"`
	DawarichURL     string `json:"dawarich_url"`
	DawarichAPIKey  string `json:"dawarich_api_key"`
	ImmichURL       string `json:"immich_url"`
	ImmichAPIKey    string `json:"immich_api_key"`
	ImmichPublicURL string `json:"immich_public_url"` // what the browser opens; defaults to the API URL
	OpenAIKey       string `json:"openai_api_key"`
	OpenAIModel     string `json:"openai_model"`
	OpenAIBaseURL   string `json:"openai_base_url"` // any OpenAI-compatible endpoint
	MapTiles        string `json:"map_tiles"`       // osm | carto | topo
	SetupComplete   bool   `json:"setup_complete"`
}
