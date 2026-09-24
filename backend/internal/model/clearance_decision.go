package model

import "time"

// ClearanceDecision is the current release state for one turnaround.
type ClearanceDecision struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TurnaroundID  uint64    `gorm:"not null;uniqueIndex" json:"turnaround_id"`
	State         string    `gorm:"size:30;not null;default:pending;index;check:chk_clearance_decisions_state,state IN ('pending','cleared','restricted','revoked')" json:"state"`
	PreviousState string    `gorm:"size:30;not null;default:''" json:"previous_state"`
	Restrictions  string    `gorm:"type:text" json:"restrictions"`
	Reason        string    `gorm:"type:text;not null" json:"reason"`
	Evidence      JSONList  `gorm:"type:jsonb" json:"evidence"`
	OperatorID    uint64    `gorm:"not null;index" json:"operator_id"`
	RequestID     string    `gorm:"size:64;index" json:"request_id"`
	DecidedAt     time.Time `json:"decided_at"`
	// Reconsideration snapshot: when a revoked decision is sent back to pending
	// these columns point at the original revocation audit entry and its reason.
	ReopenedFromAuditID uint64     `gorm:"not null;default:0" json:"reopened_from_audit_id"`
	ReopenedFromReason  string     `gorm:"type:text;not null;default:''" json:"reopened_from_reason"`
	ReopenedAt          *time.Time `json:"reopened_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (ClearanceDecision) TableName() string { return "clearance_decisions" }
