package service

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"groundclearance/internal/constants"
	"groundclearance/internal/model"
	"groundclearance/internal/repository"
	"groundclearance/internal/util"

	"gorm.io/gorm"
)

type ClearanceDecisionService struct {
	db             *gorm.DB
	repo           *repository.ClearanceDecisionRepository
	turnaroundRepo *repository.TurnaroundRepository
	checkRepo      *repository.SafetyCheckRepository
	unitRepo       *repository.GroundUnitRepository
	logger         *slog.Logger
}

func NewClearanceDecisionService(db *gorm.DB, repo *repository.ClearanceDecisionRepository,
	turnaroundRepo *repository.TurnaroundRepository, checkRepo *repository.SafetyCheckRepository,
	unitRepo *repository.GroundUnitRepository, logger *slog.Logger) *ClearanceDecisionService {
	return &ClearanceDecisionService{db: db, repo: repo, turnaroundRepo: turnaroundRepo, checkRepo: checkRepo, unitRepo: unitRepo, logger: logger}
}

func (s *ClearanceDecisionService) List(page, pageSize int, state string) ([]model.ClearanceDecision, int64, error) {
	state = strings.TrimSpace(state)
	if state != "" && !constants.IsValidClearanceState(state) {
		return nil, 0, util.NewAppError(constants.CodeValidationFailed, "invalid clearance state filter")
	}
	return s.repo.List(page, pageSize, state)
}

func (s *ClearanceDecisionService) Summary() (map[string]any, error) {
	return s.repo.Summary(time.Now())
}

func (s *ClearanceDecisionService) Get(id uint64) (*model.ClearanceDecision, error) {
	decision, err := s.repo.FindByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, util.NewAppError(constants.CodeNotFound, constants.MsgNotFound)
	}
	return decision, err
}

func (s *ClearanceDecisionService) Decide(turnaroundID, operatorID uint64, state, restrictions, reason string,
	evidence []string, requestID, operatorName, ip string) (*model.ClearanceDecision, error) {
	if !constants.IsValidClearanceState(state) || state == constants.ClearancePending {
		return nil, util.NewAppError(constants.CodeValidationFailed, "invalid clearance state")
	}
	reason = strings.TrimSpace(reason)
	restrictions = strings.TrimSpace(restrictions)
	if reason == "" {
		return nil, util.NewAppError(constants.CodeValidationFailed, "decision reason is required")
	}
	evidence, err := normalizeEvidence(evidence)
	if err != nil {
		return nil, err
	}
	if state == constants.ClearanceRestricted && restrictions == "" {
		return nil, util.NewAppError(constants.CodeValidationFailed, "restrictions are required")
	}
	if state == constants.ClearanceCleared {
		restrictions = ""
	}
	var decision *model.ClearanceDecision
	err = s.db.Transaction(func(tx *gorm.DB) error {
		turnaround, err := s.turnaroundRepo.FindByIDTx(tx, turnaroundID)
		if err != nil {
			return util.NewAppError(constants.CodeNotFound, constants.MsgNotFound)
		}
		current, err := s.repo.FindByTurnaroundTx(tx, turnaroundID)
		if err != nil {
			return err
		}
		if !allowedClearanceTransition(current.State, state) {
			return util.NewAppError(constants.CodeStateConflict, "clearance transition is not allowed")
		}
		if turnaround.Status == constants.TurnaroundCompleted {
			return util.NewAppError(constants.CodeStateConflict, "completed turnarounds cannot be changed")
		}
		checks, err := s.checkRepo.ListByTurnaroundTx(tx, turnaroundID)
		if err != nil {
			return err
		}
		if state != constants.ClearanceRevoked {
			if turnaround.Status != constants.TurnaroundChecking {
				return util.NewAppError(constants.CodeStateConflict, "turnaround must be in checking state before clearance")
			}
			if len(checks) == 0 {
				return util.NewAppError(constants.CodeStateConflict, "at least one safety check is required")
			}
			for _, check := range checks {
				if check.Result == constants.CheckPending {
					return util.NewAppError(constants.CodeStateConflict, "all safety checks must be reviewed before clearance")
				}
				if state == constants.ClearanceCleared && check.Result == constants.CheckFailed {
					return util.NewAppError(constants.CodeStateConflict, "failed checks prevent full clearance")
				}
			}
		}
		if state == constants.ClearanceCleared {
			for _, rawID := range turnaround.GroundUnitIDs {
				unitID, parseErr := strconv.ParseUint(rawID, 10, 64)
				if parseErr != nil || unitID == 0 {
					return util.NewAppError(constants.CodeStateConflict, "turnaround contains an invalid ground unit")
				}
				unit, findErr := s.unitRepo.FindByIDTx(tx, unitID)
				if findErr != nil || unit.State != constants.UnitAvailable {
					return util.NewAppError(constants.CodeStateConflict, "all assigned ground units must be available for full clearance")
				}
			}
		}
		previous := current.State
		current.PreviousState = previous
		current.State = state
		current.Restrictions = restrictions
		current.Reason = reason
		current.Evidence = model.JSONList(evidence)
		current.OperatorID = operatorID
		current.RequestID = requestID
		current.DecidedAt = time.Now()
		current.ReopenedFromAuditID = 0
		current.ReopenedFromReason = ""
		current.ReopenedAt = nil
		if err := s.repo.SaveTx(tx, current); err != nil {
			return err
		}
		turnaround.Status = constants.TurnaroundDecisioned
		turnaround.Version++
		if err := s.turnaroundRepo.SaveTx(tx, turnaround); err != nil {
			return err
		}
		detail, _ := json.Marshal(map[string]any{
			"previous_state": previous, "state": state, "reason": reason,
			"restrictions": restrictions, "evidence": evidence, "request_id": requestID,
		})
		audit := &model.AuditLog{OperatorID: operatorID, OperatorName: operatorName, Action: "CLEARANCE_TRANSITION",
			EntityType: "clearance", EntityID: strconv.FormatUint(current.ID, 10), Detail: string(detail), IP: ip, CreatedAt: time.Now()}
		if err := tx.Create(audit).Error; err != nil {
			return err
		}
		decision = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(constants.LogClearanceChanged, "turnaround_id", turnaroundID, "state", state, "operator_id", operatorID)
	return decision, nil
}

func allowedClearanceTransition(from, to string) bool {
	if from == constants.ClearancePending {
		return to == constants.ClearanceCleared || to == constants.ClearanceRestricted || to == constants.ClearanceRevoked
	}
	return (from == constants.ClearanceCleared || from == constants.ClearanceRestricted) && to == constants.ClearanceRevoked
}

// reconsiderationBlockers lists the conditions that still keep a revoked
// clearance on hold: pending or failed checks and equipment not back in
// service. It is pure so the same rules back unit tests.
func reconsiderationBlockers(checks []model.SafetyCheck, units []model.GroundUnit) []string {
	blockers := make([]string, 0)
	if len(checks) == 0 {
		blockers = append(blockers, "原检查没有任何检查项，无法确认检查结论")
	}
	for _, check := range checks {
		switch check.Result {
		case constants.CheckPending:
			blockers = append(blockers, "检查项 "+check.CheckCode+" 仍是待检查，需要现场完成检查")
		case constants.CheckFailed:
			blockers = append(blockers, "检查项 "+check.CheckCode+" 此前检查不通过，需要先处理故障并补检")
		}
	}
	for _, unit := range units {
		switch unit.State {
		case constants.UnitAvailable:
		case constants.UnitInspection:
			blockers = append(blockers, "关联设备 "+unit.UnitCode+" 仍在检查中，尚未恢复可用")
		case constants.UnitBlocked:
			blockers = append(blockers, "关联设备 "+unit.UnitCode+" 仍处于锁定状态，现场故障尚未恢复")
		case constants.UnitRetired:
			blockers = append(blockers, "关联设备 "+unit.UnitCode+" 已退役，需要从周转中替换该设备")
		default:
			blockers = append(blockers, "关联设备 "+unit.UnitCode+" 当前状态不是可用")
		}
	}
	return blockers
}

func (s *ClearanceDecisionService) loadReconsiderationContext(tx *gorm.DB, turnaroundID uint64) (*model.Turnaround, *model.ClearanceDecision, []model.SafetyCheck, []model.GroundUnit, error) {
	turnaround, err := s.turnaroundRepo.FindByIDTx(tx, turnaroundID)
	if err != nil {
		return nil, nil, nil, nil, util.NewAppError(constants.CodeNotFound, constants.MsgNotFound)
	}
	decision, err := s.repo.FindByTurnaroundTx(tx, turnaroundID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	checks, err := s.checkRepo.ListByTurnaroundTx(tx, turnaroundID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	units := make([]model.GroundUnit, 0, len(turnaround.GroundUnitIDs))
	for _, rawID := range turnaround.GroundUnitIDs {
		unitID, parseErr := strconv.ParseUint(rawID, 10, 64)
		if parseErr != nil || unitID == 0 {
			return nil, nil, nil, nil, util.NewAppError(constants.CodeStateConflict, "周转包含无效的关联设备: "+rawID)
		}
		unit, findErr := s.unitRepo.FindByIDTx(tx, unitID)
		if findErr != nil {
			return nil, nil, nil, nil, util.NewAppError(constants.CodeStateConflict, "关联设备不存在: #"+rawID)
		}
		units = append(units, *unit)
	}
	return turnaround, decision, checks, units, nil
}

// ReconsiderationEligibility is the read model behind the release desk entry:
// it tells the safety manager whether the revocation can be sent back and why
// the entry stays blocked while equipment or checks are outstanding.
func (s *ClearanceDecisionService) ReconsiderationEligibility(turnaroundID uint64) (map[string]any, error) {
	var result map[string]any
	err := s.db.Transaction(func(tx *gorm.DB) error {
		turnaround, decision, checks, units, err := s.loadReconsiderationContext(tx, turnaroundID)
		if err != nil {
			return err
		}
		blockers := reconsiderationBlockers(checks, units)
		stateReady := decision.State == constants.ClearanceRevoked && turnaround.Status == constants.TurnaroundDecisioned
		if decision.State != constants.ClearanceRevoked {
			blockers = append([]string{"当前放行状态不是已撤销，无需发起复议"}, blockers...)
		}
		if turnaround.Status != constants.TurnaroundDecisioned {
			blockers = append([]string{"周转当前不是已决定状态，不能对该决定发起复议"}, blockers...)
		}
		result = map[string]any{
			"turnaround_id":     turnaroundID,
			"clearance_id":      decision.ID,
			"clearance_state":   decision.State,
			"turnaround_status": turnaround.Status,
			"eligible":          stateReady && len(blockers) == 0,
			"blockers":          blockers,
			"revoked_reason":    decision.Reason,
			"revoked_at":        decision.DecidedAt,
			"pending_checks":    countCheckResults(checks, constants.CheckPending),
			"failed_checks":     countCheckResults(checks, constants.CheckFailed),
			"unit_states":       unitStateMap(units),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func countCheckResults(checks []model.SafetyCheck, result string) int {
	total := 0
	for _, check := range checks {
		if check.Result == result {
			total++
		}
	}
	return total
}

func unitStateMap(units []model.GroundUnit) map[string]string {
	states := make(map[string]string, len(units))
	for _, unit := range units {
		states[strconv.FormatUint(unit.ID, 10)] = unit.State
	}
	return states
}

// Reconsider sends a revoked clearance back to pending and returns the
// turnaround to checking once every assigned unit is restored and the
// original checks hold no pending or failed item. The original revocation
// audit row stays untouched; a separate reconsideration row is appended.
func (s *ClearanceDecisionService) Reconsider(turnaroundID, operatorID uint64, reason string, evidence []string,
	actor AuditContext) (*model.ClearanceDecision, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, util.NewAppError(constants.CodeValidationFailed, "reconsideration reason is required")
	}
	evidence, err := normalizeEvidence(evidence)
	if err != nil {
		return nil, err
	}
	var decision *model.ClearanceDecision
	err = s.db.Transaction(func(tx *gorm.DB) error {
		turnaround, current, checks, units, err := s.loadReconsiderationContext(tx, turnaroundID)
		if err != nil {
			return err
		}
		if current.State != constants.ClearanceRevoked {
			return util.NewAppError(constants.CodeStateConflict, "仅已撤销的放行决定可以发起复议")
		}
		if turnaround.Status != constants.TurnaroundDecisioned {
			return util.NewAppError(constants.CodeStateConflict, "周转当前不是已决定状态，无法复议")
		}
		if blockers := reconsiderationBlockers(checks, units); len(blockers) > 0 {
			return util.NewAppError(constants.CodeStateConflict, "暂不满足复议条件："+strings.Join(blockers, "；"))
		}
		revocationAudit, findErr := s.repo.FindLatestRevocationAuditTx(tx, current.ID)
		if findErr != nil {
			return util.NewAppError(constants.CodeStateConflict, "未找到该决定的撤销审计记录，无法发起复议")
		}
		revokedAt := current.DecidedAt
		revokedReason := current.Reason
		previous := current.State
		now := time.Now()
		current.PreviousState = previous
		current.State = constants.ClearancePending
		current.Restrictions = ""
		current.Reason = reason
		current.Evidence = model.JSONList(evidence)
		current.OperatorID = operatorID
		current.RequestID = actor.RequestID
		current.DecidedAt = now
		current.ReopenedFromAuditID = revocationAudit.ID
		current.ReopenedFromReason = revokedReason
		current.ReopenedAt = &now
		if err := s.repo.SaveTx(tx, current); err != nil {
			return err
		}
		turnaround.Status = constants.TurnaroundChecking
		turnaround.Version++
		if err := s.turnaroundRepo.SaveTx(tx, turnaround); err != nil {
			return err
		}
		if err := persistTransitionAudit(tx, actor, "CLEARANCE_RECONSIDERATION", "clearance", current.ID, map[string]any{
			"turnaround_id":  turnaroundID,
			"previous_state": previous, "state": constants.ClearancePending,
			"source_revocation_audit_id": revocationAudit.ID,
			"revoked_reason":             revokedReason, "revoked_decided_at": revokedAt,
			"reason": reason, "evidence": evidence,
		}); err != nil {
			return err
		}
		decision = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(constants.LogClearanceReconsidered, "turnaround_id", turnaroundID, "operator_id", operatorID)
	return decision, nil
}
