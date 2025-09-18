package lib

type AzAccount struct {
	SubscriptionId string `json:"id"`
	Name           string `json:"name"`
	State          string `json:"state"`
	TenantId       string `json:"tenantId"`

	// NOTE: There are a some other fields here that we do not parse
	// and `managedByTenants []string` in particular was giving us problems where we
	// were runnig into unmarshalling errors because Go was receiving a struct instead of
	// a string.
	// Hoever we do not use those, so we opted to nuke them altogether.
}

type AzImageDefinition struct {
	Location string            `json:"location"`
	Tags     map[string]string `json:"tags"`
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`

	// properties missing
}
