package domain

import (
	"net/netip"
	"time"

	"github.com/google/uuid"
)

const (
	SystemRoleUser     = "USER"
	SystemRoleAdmin    = "SYSTEM_ADMIN"
	UserStatusActive   = "ACTIVE"
	UserStatusDisabled = "DISABLED"
	UserStatusLocked   = "LOCKED"
	UserStatusDeleted  = "DELETED"
	DefaultLocale      = "zh-CN"
	DefaultTimezone    = "Asia/Shanghai"
	DefaultPage        = 1
	DefaultPageSize    = 50
	MaximumPageSize    = 200
)

type User struct {
	ID                   uuid.UUID
	Username             string
	UsernameNormalized   string
	EmployeeNo           *string
	EmployeeNoNormalized *string
	DisplayName          string
	Email                *string
	EmailNormalized      *string
	Phone                *string
	SystemRole           string
	Status               string
	Locale               string
	Timezone             string
	LastLoginAt          *time.Time
	CreatedByUserID      *uuid.UUID
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time
	RowVersion           int64
}

type NewUser struct {
	ID                   uuid.UUID
	CredentialID         uuid.UUID
	Username             string
	UsernameNormalized   string
	EmployeeNo           *string
	EmployeeNoNormalized *string
	DisplayName          string
	Email                *string
	EmailNormalized      *string
	Phone                *string
	SystemRole           string
	Locale               string
	Timezone             string
	PasswordHash         string
	CreatedByUserID      uuid.UUID
	CreatedAt            time.Time
}

type UserChanges struct {
	EmployeeNo  *string
	DisplayName *string
	Email       *string
	Phone       *string
	SystemRole  *string
	Locale      *string
	Timezone    *string
	RowVersion  int64
}

type ListFilter struct {
	Page       int
	PageSize   int
	Status     *string
	SystemRole *string
}

type ListResult struct {
	Items    []User
	Total    int64
	Page     int
	PageSize int
}

type Session struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	DeviceID     *string
	IPAddress    *netip.Addr
	UserAgent    *string
	Status       string
	ExpiresAt    time.Time
	LastSeenAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	RevokedAt    *time.Time
	RevokeReason *string
	RowVersion   int64
}

type SessionListResult struct {
	Items    []Session
	Total    int64
	Page     int
	PageSize int
}

type IdempotencyRecord struct {
	RequestHash      []byte
	Status           string
	ResultResourceID *uuid.UUID
}

type Event struct {
	ID               uuid.UUID
	AggregateID      uuid.UUID
	AggregateVersion int64
	Type             string
	Payload          []byte
	DeduplicationKey string
	CorrelationID    uuid.UUID
	CreatedAt        time.Time
}
