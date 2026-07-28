package entities

const (
	PlanIntervalMonthly = "monthly"
	PlanIntervalYearly  = "yearly"
)

func IsValidPlanInterval(value string) bool {
	switch value {
	case PlanIntervalMonthly, PlanIntervalYearly:
		return true
	default:
		return false
	}
}
