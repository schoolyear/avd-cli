package lib

type AzAccount struct {
	EnvironmentName  string        `json:"environmentName"`
	HomeTenantId     string        `json:"homeTenantId"`
	SubscriptionId   string        `json:"id"`
	IsDefault        bool          `json:"isDefault"`
	ManagedByTenants []string      `json:"managedByTenants"`
	Name             string        `json:"name"`
	State            string        `json:"state"`
	TenantId         string        `json:"tenantId"`
	User             AzAccountUser `json:"user"`
}

type AzAccountUser struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type AzImageDefinition struct {
	Location string            `json:"location"`
	Tags     map[string]string `json:"tags"`
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`

	// properties missing
}
