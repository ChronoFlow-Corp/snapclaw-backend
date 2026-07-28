package dto

import "time"

// AdminUserListItemResponse is a row in the admin users listing. Field names are
// snake_case to match the admin console's user DTO.
type AdminUserListItemResponse struct {
	ID                    string    `json:"id"`
	Email                 string    `json:"email"`
	Name                  string    `json:"name"`
	Nickname              string    `json:"nickname"`
	AvatarURL             string    `json:"avatar_url"`
	Role                  string    `json:"role"`
	BalanceMinor          int64     `json:"balance_minor"`
	HasActiveSubscription bool      `json:"has_active_subscription"`
	ClawsCount            int64     `json:"claws_count"`
	HasIssues             bool      `json:"has_issues"`
	CreatedAt             time.Time `json:"created_at"`
}

// AdminUserListResponse is the paginated envelope for the users listing.
type AdminUserListResponse struct {
	Items    []AdminUserListItemResponse `json:"items"`
	Total    int64                       `json:"total"`
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
}

// AdminUserCountsResponse holds the dashboard overview counters.
type AdminUserCountsResponse struct {
	Total                  int64 `json:"total"`
	WithActiveSubscription int64 `json:"with_active_subscription"`
	WithIssues             int64 `json:"with_issues"`
	Admins                 int64 `json:"admins"`
}

// AdminUserSummaryResponse is the 360° summary embedded in the user detail card.
// SubscriptionStatus and CurrentPeriodEnd are null (not omitted) when the user
// has never subscribed.
type AdminUserSummaryResponse struct {
	ClawsTotal         int64      `json:"claws_total"`
	ClawsRunning       int64      `json:"claws_running"`
	ClawsError         int64      `json:"claws_error"`
	SubscriptionStatus *string    `json:"subscription_status"`
	CurrentPeriodEnd   *time.Time `json:"current_period_end"`
	PaymentsTotalMinor int64      `json:"payments_total_minor"`
	UsageMonth         string     `json:"usage_month"`
}

// AdminUserDetailResponse is the full admin user detail payload.
type AdminUserDetailResponse struct {
	ID           string                   `json:"id"`
	Email        string                   `json:"email"`
	Name         string                   `json:"name"`
	Nickname     string                   `json:"nickname"`
	AvatarURL    string                   `json:"avatar_url"`
	Role         string                   `json:"role"`
	BalanceMinor int64                    `json:"balance_minor"`
	HasIssues    bool                     `json:"has_issues"`
	CreatedAt    time.Time                `json:"created_at"`
	Summary      AdminUserSummaryResponse `json:"summary"`
}

// AdminClawResponse is the user-facing claw response enriched with the admin-only
// context (owning server, creation time, enabled capabilities). MonthlyBudgetUsd
// is not persisted in a readable form and is therefore always omitted for now.
type AdminClawResponse struct {
	ClawResponse
	ServerID         string    `json:"serverId,omitempty"`
	MonthlyBudgetUsd *float64  `json:"monthlyBudgetUsd,omitempty"`
	Capabilities     []string  `json:"capabilities"`
	CreatedAt        time.Time `json:"createdAt"`
}

// AdminPaymentResponse is the admin-facing view of a payment. Card/authorization
// secrets carried by the payment entity are deliberately dropped.
type AdminPaymentResponse struct {
	ID          string    `json:"id"`
	Purpose     string    `json:"purpose"`
	Status      string    `json:"status"`
	Paid        bool      `json:"paid"`
	Amount      string    `json:"amount"`
	Currency    string    `json:"currency"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// AdminBalanceEntryResponse is a single balance ledger entry.
type AdminBalanceEntryResponse struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	AmountMinor int64     `json:"amount_minor"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// AdminIntegrationResponse is an account integration; secret payloads are dropped.
type AdminIntegrationResponse struct {
	ID                string    `json:"id"`
	Capability        string    `json:"capability"`
	Provider          string    `json:"provider"`
	ExternalAccountID string    `json:"external_account_id"`
	DisplayName       string    `json:"display_name"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
}

// AdminTelegramBotResponse is a managed Telegram bot. ManagedBotUsername and
// ChannelID are null when not yet assigned.
type AdminTelegramBotResponse struct {
	ID                 string    `json:"id"`
	Status             string    `json:"status"`
	ManagedBotUsername *string   `json:"managed_bot_username"`
	ChannelID          *string   `json:"channel_id"`
	LastError          string    `json:"last_error"`
	CreatedAt          time.Time `json:"created_at"`
}
