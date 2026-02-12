package models

import "encoding/json"

// DurationRule defines the mapping from duration to a specific ReasonID.
type DurationRule struct {
	Label    string
	MaxHours float64
	ReasonID int
}

// ─── PENDING OUTAGES ───

type FilteredData struct {
	ID        int    `json:"id"`
	Field     string `json:"field"`
	Type      string `json:"type"`
	Operator  string `json:"operator"`
	Value     string `json:"value"`
	FromValue string `json:"fromValue"`
	ToValue   string `json:"toValue"`
}

type PendingRequest struct {
	OrderBy      *string        `json:"orderBy"`
	Order        *string        `json:"order"`
	FilteredData []FilteredData `json:"filteredData"`
	Offset       int            `json:"offset"`
	Limit        int            `json:"limit"`
}

type Outage struct {
	ID                 string `json:"id"`
	InterruptionType   string `json:"interruption_type"`
	OutageType         int    `json:"outage_type"`
	OutageTypeName     string `json:"outage_type_name"`
	FeederID           int    `json:"feeder_id"`
	FeederName         string `json:"feeder_name"`
	FeederCategory     string `json:"feeder_category"`
	OutageOccurDate    string `json:"outage_occur_date"`
	OutageOccurTime    string `json:"outage_occur_time"`
	OutageRestoreDate  string `json:"outage_restore_date"`
	OutageRestoreTime  string `json:"outage_restore_time"`
	SSName             string `json:"ss_name"`
	DiscomCircleName   string `json:"discom_circle_name"`
	DiscomDivisionName string `json:"discom_division_name"`
	CompanyName        string `json:"company_name"`
	SubdivisionName    string `json:"subdivision_name"`
}

type PendingResponse struct {
	TotalRecords int      `json:"total_records"`
	Data         []Outage `json:"data"`
}

// ─── REASON DETAIL ───

type GeoFeatureProperties struct {
	Hlt string `json:"hlt"` // Pole type (e.g., "HT Pole")
	ID  int    `json:"id"`  // Pole ID
}

type GeoFeatureGeometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

type GeoFeature struct {
	Type       string               `json:"type"`
	Geometry   GeoFeatureGeometry   `json:"geometry"`
	LocStr     string               `json:"loc_str"`
	Properties GeoFeatureProperties `json:"properties"`
}

type GeoFeatureCollection struct {
	Type     string       `json:"type"`
	Features []GeoFeature `json:"features"`
}

type RowToJSONWrapper struct {
	RowToJSON GeoFeatureCollection `json:"row_to_json"`
}

type ReasonDetailResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		OutageData         json.RawMessage   `json:"outageData"`
		FeederPointGeoJson []json.RawMessage `json:"feederPointGeoJson"` // Mixed types: arrays and objects
	} `json:"data"`
}

// ─── SUBMIT REASON ───

type ReasonPayloadItem struct {
	LocID    int `json:"loc_id"`
	ReasonID int `json:"reason_id"`
}

// ─── LOGIN ───

type LoginRequest struct {
	CompanyName string `json:"companyName"`
	EmpNo       string `json:"empNo"`
	Password    string `json:"password"`
	AppName     string `json:"appName"`
}

type LoginResponse struct {
	User struct {
		AuthToken string `json:"auth_token"`
	} `json:"user"`
}
