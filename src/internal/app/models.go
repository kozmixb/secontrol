package app

import "time"

type Agent struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Host           string     `json:"host"`
	Port           int        `json:"port"`
	Username       string     `json:"username"`
	AuthType       string     `json:"authType"`
	Status         string     `json:"status"`
	LastError      string     `json:"lastError,omitempty"`
	LastSeen       *time.Time `json:"lastSeen,omitempty"`
	OS             string     `json:"os,omitempty"`
	Kernel         string     `json:"kernel,omitempty"`
	UptimeSeconds  int64      `json:"uptimeSeconds"`
	Load1          float64    `json:"load1"`
	MemoryTotal    int64      `json:"memoryTotal"`
	MemoryUsed     int64      `json:"memoryUsed"`
	DiskTotal      int64      `json:"diskTotal"`
	DiskUsed       int64      `json:"diskUsed"`
	CPUCount       int        `json:"cpuCount"`
	IsVM           bool       `json:"isVm"`
	Virtualization string     `json:"virtualization,omitempty"`
	AccessLevel    string     `json:"accessLevel"`
	ContainerCount int        `json:"containerCount"`
	RunningCount   int        `json:"runningCount"`
	CreatedAt      time.Time  `json:"createdAt"`
}

type agentInput struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	AuthType   string `json:"authType"`
	KeyID      int64  `json:"keyId"`
	Password   string `json:"password"`
	PrivateKey string `json:"privateKey"`
}

type SSHKey struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	PublicKey  string    `json:"publicKey"`
	UsageCount int       `json:"usageCount"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Container struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	Status  string `json:"status"`
	State   string `json:"state"`
	Ports   string `json:"ports"`
	Created string `json:"created"`
	Uptime  string `json:"uptime"`
}

type FleetContainer struct {
	Container
	AgentID         int64      `json:"agentId"`
	AgentName       string     `json:"agentName"`
	AgentHost       string     `json:"agentHost"`
	AgentAccess     string     `json:"agentAccess"`
	Version         string     `json:"version"`
	UpdateAvailable *bool      `json:"updateAvailable,omitempty"`
	ImageCheckedAt  *time.Time `json:"imageCheckedAt,omitempty"`
	ComposeProject  string     `json:"composeProject,omitempty"`
	ComposeService  string     `json:"composeService,omitempty"`
}

type ContainerDetails struct {
	ImageID         string   `json:"imageId"`
	LocalDigest     string   `json:"localDigest,omitempty"`
	RegistryDigest  string   `json:"registryDigest,omitempty"`
	UpdateAvailable *bool    `json:"updateAvailable,omitempty"`
	ImageCreated    string   `json:"imageCreated,omitempty"`
	ImageSize       int64    `json:"imageSize"`
	Platform        string   `json:"platform,omitempty"`
	RestartPolicy   string   `json:"restartPolicy,omitempty"`
	Health          string   `json:"health,omitempty"`
	ComposeProject  string   `json:"composeProject,omitempty"`
	ComposeService  string   `json:"composeService,omitempty"`
	Networks        []string `json:"networks"`
	Mounts          []string `json:"mounts"`
}
