package models

// Aircraft represents aircraft information from the aircraft database
// All fields correspond to columns in the aircraft-database-complete CSV file
type Aircraft struct {
	ICAO24              string // Primary key - 6 hex digit ICAO address
	Timestamp           string // Timestamp from database
	ACARS               string // ACARS capability
	ADSB                string // ADS-B capability
	Built               string // Year built
	CategoryDescription string // Aircraft category description
	Country             string // Country of registration
	Engines             string // Number of engines
	FirstFlightDate     string // First flight date
	FirstSeen           string // First seen date
	ICAOAircraftClass   string // ICAO aircraft class
	LineNumber          string // Line number
	ManufacturerICAO    string // Manufacturer ICAO code
	ManufacturerName    string // Manufacturer name
	Model               string // Aircraft model
	Modes               string // Mode S codes
	NextReg             string // Next registration
	Notes               string // Notes
	Operator            string // Operator name
	OperatorCallsign    string // Operator callsign
	OperatorIATA        string // Operator IATA code
	OperatorICAO        string // Operator ICAO code
	Owner               string // Owner name
	PrevReg             string // Previous registration
	RegUntil            string // Registration until date
	Registered          string // Registration date
	Registration        string // Aircraft registration (e.g., N12345)
	SelCal              string // SELCAL code
	SerialNumber        string // Serial number
	Status              string // Status
	TypeCode            string // Aircraft type code
	VDL                 string // VDL capability
}
