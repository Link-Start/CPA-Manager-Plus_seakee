package model

type AdminCredential struct {
	Version     int    `json:"version"`
	Salt        string `json:"salt"`
	KeyHash     string `json:"keyHash"`
	Iterations  int    `json:"iterations"`
	CreatedAtMS int64  `json:"createdAtMs"`
	RotatedAtMS int64  `json:"rotatedAtMs,omitempty"`
	Source      string `json:"source,omitempty"`
}

type BootstrapState struct {
	Version            int            `json:"version"`
	Status             string         `json:"status"`
	SetupStep          string         `json:"setupStep"`
	DeploymentMode     DeploymentMode `json:"deploymentMode"`
	BootstrapRequired  bool           `json:"bootstrapRequired"`
	CPAReady           bool           `json:"cpaReady"`
	AdminReady         bool           `json:"adminReady"`
	ProjectInitialized bool           `json:"projectInitialized"`
	DataKeyReady       bool           `json:"dataKeyReady"`
	MigratedLegacy     bool           `json:"migratedLegacy"`
	HasHistoricalData  bool           `json:"hasHistoricalData"`
	UpdatedAtMS        int64          `json:"updatedAtMs"`
}

type BootstrapCredential struct {
	Version      int    `json:"version"`
	Salt         string `json:"salt"`
	TokenHash    string `json:"tokenHash"`
	CreatedAtMS  int64  `json:"createdAtMs"`
	ExpiresAtMS  int64  `json:"expiresAtMs"`
	ConsumedAtMS int64  `json:"consumedAtMs,omitempty"`
}
