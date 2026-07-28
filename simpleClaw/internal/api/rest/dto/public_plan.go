package dto

type PublicPlanVariantResponse struct {
	PlanID             string `json:"plan_id"`
	BillingAmountMinor int64  `json:"billing_amount_minor"`
	BalanceCreditMinor int64  `json:"balance_credit_minor"`
	Currency           string `json:"currency"`
}

type PublicPlanGroupResponse struct {
	Code    string                     `json:"code"`
	Name    string                     `json:"name"`
	Monthly *PublicPlanVariantResponse `json:"monthly,omitempty"`
	Yearly  *PublicPlanVariantResponse `json:"yearly,omitempty"`
}
