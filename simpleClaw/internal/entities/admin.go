package entities

import (
	"time"

	"github.com/google/uuid"
)

// AdminUserListItem is a projected row of the admin users listing. Its computed
// fields (ClawsCount, HasActiveSubscription, HasIssues) are derived by the admin
// stats storage in a single query rather than mapped from a single ORM model.
type AdminUserListItem struct {
	ID                    uuid.UUID
	Email                 string
	Name                  string
	Nickname              string
	AvatarURL             string
	Role                  string
	BalanceMinor          int64
	HasActiveSubscription bool
	ClawsCount            int64
	HasIssues             bool
	CreatedAt             time.Time
}

// AdminUserCounts holds the aggregate user counters shown on the admin dashboard.
type AdminUserCounts struct {
	Total                  int64
	WithActiveSubscription int64
	WithIssues             int64
	Admins                 int64
}

// AdminUserSummary is the aggregated 360° summary embedded in the admin user
// detail card. SubscriptionStatus is empty and CurrentPeriodEnd is nil when the
// user has never subscribed.
type AdminUserSummary struct {
	ClawsTotal         int64
	ClawsRunning       int64
	ClawsError         int64
	SubscriptionStatus string
	CurrentPeriodEnd   *time.Time
	PaymentsTotalMinor int64
	UsageMonth         string
}

// AdminUserSort enumerates the accepted sort orders for the admin users listing.
// Values mirror the query strings the admin frontend sends.
type AdminUserSort string

const (
	AdminUserSortCreatedAtDesc AdminUserSort = "-created_at"
	AdminUserSortCreatedAtAsc  AdminUserSort = "created_at"
	AdminUserSortBalanceDesc   AdminUserSort = "-balance_minor"
	AdminUserSortBalanceAsc    AdminUserSort = "balance_minor"
	AdminUserSortEmailAsc      AdminUserSort = "email"
)

// AdminUserFilter captures the query for the admin users listing. Page and
// PageSize are 1-based and pre-clamped by the caller. Nil optional filters mean
// "no constraint".
type AdminUserFilter struct {
	Page                  int
	PageSize              int
	Sort                  AdminUserSort
	Query                 string
	Role                  string
	HasActiveSubscription *bool
	HasIssues             *bool
	RegisteredFrom        *time.Time
	RegisteredTo          *time.Time
}

// AdminUserPage is a single page of admin user rows plus total match count.
type AdminUserPage struct {
	Items    []AdminUserListItem
	Total    int64
	Page     int
	PageSize int
}
