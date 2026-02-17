package status

const (
	// Client phases
	ClientPhasePending = "Pending"
	ClientPhaseRunning = "Running"
	ClientPhaseFailed  = "Failed"
	ClientPhaseUnknown = "Unknown"

	// Upstream phases
	UpstreamPhasePending = "Pending"
	UpstreamPhaseActive  = "Active"
	UpstreamPhaseFailed  = "Failed"

	// Visitor phases
	VisitorPhasePending = "Pending"
	VisitorPhaseActive  = "Active"
	VisitorPhaseFailed  = "Failed"

	// Condition types
	ConditionTypeReady      = "Ready"
	ConditionTypeConfigSync = "ConfigSynced"

	// Condition reasons
	ReasonPodCreated         = "PodCreated"
	ReasonPodRunning         = "PodRunning"
	ReasonPodFailed          = "PodFailed"
	ReasonConfigMapUpdated   = "ConfigMapUpdated"
	ReasonConfigReloaded     = "ConfigReloaded"
	ReasonConfigReloadFailed = "ConfigReloadFailed"
)
