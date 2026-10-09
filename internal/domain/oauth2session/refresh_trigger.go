package oauth2session

// RefreshTrigger identifies why a session refresh was requested.
type RefreshTrigger string

const (
	RefreshTriggerOnDemand   RefreshTrigger = "on-demand"
	RefreshTriggerBackground RefreshTrigger = "background"
	RefreshTriggerSweep      RefreshTrigger = "sweep"
)
