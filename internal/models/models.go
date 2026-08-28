package models

// AppConfig represents the application configuration.
type AppConfig struct {
	DebugMode       bool   `json:"Debug_Mode"`
	LoggingFiles    bool   `json:"Logging_Files"`
	ShowConsole     bool   `json:"Show_Console"`
	CustomSteamPath string `json:"Custom_Steam_Path"`
	Language        string `json:"Language"`
	ProxyURL        string `json:"Proxy_URL"`
}

// ManifestInfo holds information about a single manifest from the API.
type ManifestInfo struct {
	AppID      string `json:"app_id"`
	DepotID    string `json:"depot_id"`
	DepotKey   string `json:"depot_key"`
	ManifestID string `json:"manifest_id"`
	Size       string `json:"size"`
	URL        string `json:"url"`
}

// SteamAppInfo holds game metadata.
type SteamAppInfo struct {
	AppID                 string `json:"app_id"`
	Name                  string `json:"name"`
	HeaderImage           string `json:"headerImage"`
	AccessToken           string `json:"accessToken"`
	DLCCount              int    `json:"dlc_count"`
	DepotCount            int    `json:"depot_count"`
	WorkshopDecryptionKey string `json:"workshop_decryption_key"`
}

// SteamAppManifestInfo groups main app and DLC manifests.
type SteamAppManifestInfo struct {
	MainApp []ManifestInfo `json:"mainapp"`
	DLCs    []ManifestInfo `json:"dlcs"`
}

// TaskResult holds the result of an unlock task.
type TaskResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// --- Frontend response types ---

type SimpleResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type BasicConfig struct {
	SteamPath string `json:"steam_path"`
	DebugMode bool   `json:"debug_mode"`
}

type ConfigResponse struct {
	Success bool        `json:"success"`
	Config  BasicConfig `json:"config"`
}

type DetailedConfig struct {
	SteamPath       string `json:"steam_path"`
	DebugMode       bool   `json:"debug_mode"`
	LoggingFiles    bool   `json:"logging_files"`
	ShowConsole     bool   `json:"show_console"`
	SteamPathExists bool   `json:"steam_path_exists"`
	Language        string `json:"language"`
	ProxyURL        string `json:"proxy_url"`
}

type DetailedConfigResponse struct {
	Success bool           `json:"success"`
	Config  DetailedConfig `json:"config"`
}

type UpdateConfigRequest struct {
	SteamPath    string `json:"steam_path"`
	DebugMode    bool   `json:"debug_mode"`
	LoggingFiles bool   `json:"logging_files"`
	ShowConsole  bool   `json:"show_console"`
	Language     string `json:"language"`
	ProxyURL     string `json:"proxy_url"`
}

type TaskStatusResponse struct {
	Status string      `json:"status"`
	Result *TaskResult `json:"result"`
}

// KernelSettings holds SteamTools registry settings.
type KernelSettings struct {
	ActivateUnlockMode bool `json:"activate_unlock_mode"`
	AlwaysStayUnlocked bool `json:"always_stay_unlocked"`
	NotUnlockDepot     bool `json:"not_unlock_depot"`
}

type KernelSettingsResponse struct {
	Success  bool           `json:"success"`
	Settings KernelSettings `json:"settings"`
	Message  string         `json:"message,omitempty"`
}

// --- Steam Store Search types ---

type StoreSearchResult struct {
	Total int               `json:"total"`
	Items []StoreSearchItem `json:"items"`
}

type StoreSearchItem struct {
	Type      string                `json:"type"`
	Name      string                `json:"name"`
	ID        int                   `json:"id"`
	TinyImage string                `json:"tiny_image"`
	Platforms *StoreSearchPlatforms `json:"platforms"`
	Price     *StoreSearchPrice     `json:"price"`
}

type StoreSearchPlatforms struct {
	Windows bool `json:"windows"`
	Mac     bool `json:"mac"`
	Linux   bool `json:"linux"`
}

type StoreSearchPrice struct {
	Currency        string `json:"currency"`
	Initial         int    `json:"initial"`
	Final           int    `json:"final"`
	DiscountPercent int    `json:"discount_percent"`
}

// --- Game Library types ---

type LibraryGame struct {
	AppID      int            `json:"app_id"`
	Name       string         `json:"name"`
	TinyImage  string         `json:"tiny_image"`
	LuaPath    string         `json:"lua_path"`
	DLCCount   int            `json:"dlc_count"`
	DepotCount int            `json:"depot_count"`
	Unlocked   bool           `json:"unlocked"`
	CreatedAt  string         `json:"created_at"`
	UpdatedAt  string         `json:"updated_at"`
	Depots     []LibraryDepot `json:"depots"`
	DLCs       []LibraryDepot `json:"dlcs"`
}

type LibraryDepot struct {
	AppID      string `json:"app_id"`
	DepotID    string `json:"depot_id"`
	DepotKey   string `json:"depot_key"`
	ManifestID string `json:"manifest_id"`
}
