package constants

// TimezoneOption represents a timezone choice for UI display
type TimezoneOption struct {
	ID          string // Display ID like "1", "2", etc.
	Name        string // Display name like "US/Eastern (UTC-5)"
	IANAName    string // IANA timezone name like "America/New_York"
	Description string // Helpful description
}

// CommonTimezones contains frequently used timezones
var CommonTimezones = []TimezoneOption{
	{ID: "1", Name: "UTC", IANAName: "UTC", Description: "Coordinated Universal Time"},
	{ID: "2", Name: "US/Eastern", IANAName: "America/New_York", Description: "US Eastern Time (New York)"},
	{ID: "3", Name: "US/Central", IANAName: "America/Chicago", Description: "US Central Time (Chicago)"},
	{ID: "4", Name: "US/Mountain", IANAName: "America/Denver", Description: "US Mountain Time (Denver)"},
	{ID: "5", Name: "US/Pacific", IANAName: "America/Los_Angeles", Description: "US Pacific Time (Los Angeles)"},
	{ID: "6", Name: "Europe/London", IANAName: "Europe/London", Description: "UK Time (London)"},
	{ID: "7", Name: "Europe/Paris", IANAName: "Europe/Paris", Description: "Central European Time (Paris, Berlin)"},
	{ID: "8", Name: "Asia/Tokyo", IANAName: "Asia/Tokyo", Description: "Japan Time (Tokyo)"},
	{ID: "9", Name: "Asia/Shanghai", IANAName: "Asia/Shanghai", Description: "China Time (Shanghai, Beijing)"},
	{ID: "10", Name: "Asia/Kolkata", IANAName: "Asia/Kolkata", Description: "India Time (Mumbai, Delhi)"},
	{ID: "11", Name: "Australia/Sydney", IANAName: "Australia/Sydney", Description: "Australian Eastern Time (Sydney)"},
	{ID: "12", Name: "More timezones...", IANAName: "", Description: "See all available timezones"},
}

// AllTimezones contains comprehensive list of IANA timezones grouped by region
var AllTimezones = []TimezoneOption{
	// UTC
	{ID: "1", Name: "UTC", IANAName: "UTC", Description: "Coordinated Universal Time"},

	// Americas - North America
	{ID: "2", Name: "US/Eastern", IANAName: "America/New_York", Description: "US Eastern (New York, Toronto)"},
	{ID: "3", Name: "US/Central", IANAName: "America/Chicago", Description: "US Central (Chicago, Houston)"},
	{ID: "4", Name: "US/Mountain", IANAName: "America/Denver", Description: "US Mountain (Denver, Phoenix)"},
	{ID: "5", Name: "US/Pacific", IANAName: "America/Los_Angeles", Description: "US Pacific (Los Angeles, Seattle)"},
	{ID: "6", Name: "US/Alaska", IANAName: "America/Anchorage", Description: "Alaska Time (Anchorage)"},
	{ID: "7", Name: "US/Hawaii", IANAName: "Pacific/Honolulu", Description: "Hawaii Time (Honolulu)"},
	{ID: "8", Name: "Canada/Atlantic", IANAName: "America/Halifax", Description: "Atlantic Time (Halifax)"},
	{ID: "9", Name: "Canada/Newfoundland", IANAName: "America/St_Johns", Description: "Newfoundland Time (St. John's)"},
	{ID: "10", Name: "Mexico/Central", IANAName: "America/Mexico_City", Description: "Mexico Central (Mexico City)"},

	// Americas - Central America
	{ID: "11", Name: "Central America", IANAName: "America/Guatemala", Description: "Central America (Guatemala, Costa Rica)"},

	// Americas - South America
	{ID: "12", Name: "Brazil/East", IANAName: "America/Sao_Paulo", Description: "Brazil Eastern (São Paulo, Rio)"},
	{ID: "13", Name: "Argentina", IANAName: "America/Argentina/Buenos_Aires", Description: "Argentina (Buenos Aires)"},
	{ID: "14", Name: "Chile", IANAName: "America/Santiago", Description: "Chile (Santiago)"},
	{ID: "15", Name: "Colombia", IANAName: "America/Bogota", Description: "Colombia (Bogotá)"},
	{ID: "16", Name: "Peru", IANAName: "America/Lima", Description: "Peru (Lima)"},
	{ID: "17", Name: "Venezuela", IANAName: "America/Caracas", Description: "Venezuela (Caracas)"},

	// Europe - Western
	{ID: "18", Name: "UK", IANAName: "Europe/London", Description: "UK (London, Edinburgh)"},
	{ID: "19", Name: "Ireland", IANAName: "Europe/Dublin", Description: "Ireland (Dublin)"},
	{ID: "20", Name: "Portugal", IANAName: "Europe/Lisbon", Description: "Portugal (Lisbon)"},
	{ID: "21", Name: "Iceland", IANAName: "Atlantic/Reykjavik", Description: "Iceland (Reykjavik)"},

	// Europe - Central
	{ID: "22", Name: "Central Europe", IANAName: "Europe/Paris", Description: "France, Germany, Spain, Italy"},
	{ID: "23", Name: "Netherlands", IANAName: "Europe/Amsterdam", Description: "Netherlands (Amsterdam)"},
	{ID: "24", Name: "Belgium", IANAName: "Europe/Brussels", Description: "Belgium (Brussels)"},
	{ID: "25", Name: "Switzerland", IANAName: "Europe/Zurich", Description: "Switzerland (Zurich)"},
	{ID: "26", Name: "Austria", IANAName: "Europe/Vienna", Description: "Austria (Vienna)"},
	{ID: "27", Name: "Poland", IANAName: "Europe/Warsaw", Description: "Poland (Warsaw)"},
	{ID: "28", Name: "Czech Republic", IANAName: "Europe/Prague", Description: "Czech Republic (Prague)"},
	{ID: "29", Name: "Sweden", IANAName: "Europe/Stockholm", Description: "Sweden (Stockholm)"},
	{ID: "30", Name: "Norway", IANAName: "Europe/Oslo", Description: "Norway (Oslo)"},
	{ID: "31", Name: "Denmark", IANAName: "Europe/Copenhagen", Description: "Denmark (Copenhagen)"},

	// Europe - Eastern
	{ID: "32", Name: "Greece", IANAName: "Europe/Athens", Description: "Greece (Athens)"},
	{ID: "33", Name: "Turkey", IANAName: "Europe/Istanbul", Description: "Turkey (Istanbul)"},
	{ID: "34", Name: "Ukraine", IANAName: "Europe/Kiev", Description: "Ukraine (Kyiv)"},
	{ID: "35", Name: "Romania", IANAName: "Europe/Bucharest", Description: "Romania (Bucharest)"},
	{ID: "36", Name: "Finland", IANAName: "Europe/Helsinki", Description: "Finland (Helsinki)"},

	// Europe - Russia
	{ID: "37", Name: "Russia/Moscow", IANAName: "Europe/Moscow", Description: "Russia Moscow (Moscow, St. Petersburg)"},
	{ID: "38", Name: "Russia/Samara", IANAName: "Europe/Samara", Description: "Russia Samara"},
	{ID: "39", Name: "Russia/Yekaterinburg", IANAName: "Asia/Yekaterinburg", Description: "Russia Yekaterinburg"},
	{ID: "40", Name: "Russia/Novosibirsk", IANAName: "Asia/Novosibirsk", Description: "Russia Novosibirsk"},
	{ID: "41", Name: "Russia/Vladivostok", IANAName: "Asia/Vladivostok", Description: "Russia Vladivostok"},

	// Africa
	{ID: "42", Name: "South Africa", IANAName: "Africa/Johannesburg", Description: "South Africa (Johannesburg, Cape Town)"},
	{ID: "43", Name: "Egypt", IANAName: "Africa/Cairo", Description: "Egypt (Cairo)"},
	{ID: "44", Name: "Nigeria", IANAName: "Africa/Lagos", Description: "Nigeria (Lagos)"},
	{ID: "45", Name: "Kenya", IANAName: "Africa/Nairobi", Description: "Kenya (Nairobi)"},
	{ID: "46", Name: "Morocco", IANAName: "Africa/Casablanca", Description: "Morocco (Casablanca)"},

	// Middle East
	{ID: "47", Name: "Israel", IANAName: "Asia/Jerusalem", Description: "Israel (Jerusalem, Tel Aviv)"},
	{ID: "48", Name: "Saudi Arabia", IANAName: "Asia/Riyadh", Description: "Saudi Arabia (Riyadh)"},
	{ID: "49", Name: "UAE", IANAName: "Asia/Dubai", Description: "UAE (Dubai, Abu Dhabi)"},
	{ID: "50", Name: "Qatar", IANAName: "Asia/Qatar", Description: "Qatar (Doha)"},

	// Asia - South & Southeast
	{ID: "51", Name: "India", IANAName: "Asia/Kolkata", Description: "India (Mumbai, Delhi, Bangalore)"},
	{ID: "52", Name: "Pakistan", IANAName: "Asia/Karachi", Description: "Pakistan (Karachi, Lahore)"},
	{ID: "53", Name: "Bangladesh", IANAName: "Asia/Dhaka", Description: "Bangladesh (Dhaka)"},
	{ID: "54", Name: "Sri Lanka", IANAName: "Asia/Colombo", Description: "Sri Lanka (Colombo)"},
	{ID: "55", Name: "Nepal", IANAName: "Asia/Kathmandu", Description: "Nepal (Kathmandu)"},
	{ID: "56", Name: "Thailand", IANAName: "Asia/Bangkok", Description: "Thailand (Bangkok)"},
	{ID: "57", Name: "Vietnam", IANAName: "Asia/Ho_Chi_Minh", Description: "Vietnam (Ho Chi Minh, Hanoi)"},
	{ID: "58", Name: "Singapore", IANAName: "Asia/Singapore", Description: "Singapore"},
	{ID: "59", Name: "Malaysia", IANAName: "Asia/Kuala_Lumpur", Description: "Malaysia (Kuala Lumpur)"},
	{ID: "60", Name: "Indonesia/Jakarta", IANAName: "Asia/Jakarta", Description: "Indonesia West (Jakarta)"},
	{ID: "61", Name: "Philippines", IANAName: "Asia/Manila", Description: "Philippines (Manila)"},

	// Asia - East
	{ID: "62", Name: "China", IANAName: "Asia/Shanghai", Description: "China (Beijing, Shanghai, Shenzhen)"},
	{ID: "63", Name: "Hong Kong", IANAName: "Asia/Hong_Kong", Description: "Hong Kong"},
	{ID: "64", Name: "Taiwan", IANAName: "Asia/Taipei", Description: "Taiwan (Taipei)"},
	{ID: "65", Name: "Japan", IANAName: "Asia/Tokyo", Description: "Japan (Tokyo, Osaka)"},
	{ID: "66", Name: "South Korea", IANAName: "Asia/Seoul", Description: "South Korea (Seoul)"},

	// Oceania
	{ID: "67", Name: "Australia/Sydney", IANAName: "Australia/Sydney", Description: "Australia East (Sydney, Melbourne)"},
	{ID: "68", Name: "Australia/Brisbane", IANAName: "Australia/Brisbane", Description: "Australia Queensland (Brisbane)"},
	{ID: "69", Name: "Australia/Adelaide", IANAName: "Australia/Adelaide", Description: "Australia Central (Adelaide)"},
	{ID: "70", Name: "Australia/Perth", IANAName: "Australia/Perth", Description: "Australia West (Perth)"},
	{ID: "71", Name: "New Zealand", IANAName: "Pacific/Auckland", Description: "New Zealand (Auckland, Wellington)"},
	{ID: "72", Name: "Fiji", IANAName: "Pacific/Fiji", Description: "Fiji"},

	// Pacific Islands
	{ID: "73", Name: "Samoa", IANAName: "Pacific/Apia", Description: "Samoa"},
	{ID: "74", Name: "Tahiti", IANAName: "Pacific/Tahiti", Description: "French Polynesia (Tahiti)"},
	{ID: "75", Name: "Guam", IANAName: "Pacific/Guam", Description: "Guam"},
}

// GetTimezoneByIANAName returns a TimezoneOption by its IANA name
func GetTimezoneByIANAName(ianaName string) *TimezoneOption {
	for _, tz := range AllTimezones {
		if tz.IANAName == ianaName {
			return &tz
		}
	}
	return nil
}

// GetDefaultTimezone returns the system's default timezone or UTC
func GetDefaultTimezone() string {
	// Try to detect system timezone, fallback to UTC
	return "UTC"
}
