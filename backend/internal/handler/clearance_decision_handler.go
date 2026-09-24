package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"groundclearance/internal/constants"
	"groundclearance/internal/dto"
	"groundclearance/internal/middleware"
	"groundclearance/internal/service"

	"github.com/gin-gonic/gin"
)

type ClearanceDecisionHandler struct {
	svc    *service.ClearanceDecisionService
	logger *slog.Logger
}

func NewClearanceDecisionHandler(svc *service.ClearanceDecisionService, logger *slog.Logger) *ClearanceDecisionHandler {
	return &ClearanceDecisionHandler{svc: svc, logger: logger}
}

func (h *ClearanceDecisionHandler) List(c *gin.Context) {
	var query dto.PageQuery
	if !bindPageQuery(c, &query) {
		return
	}
	rows, total, err := h.svc.List(query.Page, query.PageSize, c.Query("state"))
	if err != nil {
		handleServiceError(c, h.logger, err, "clearance list")
		return
	}
	OK(c, pageResponse(rows, total, query.Page, query.PageSize))
}

func (h *ClearanceDecisionHandler) Summary(c *gin.Context) {
	result, err := h.svc.Summary()
	if err != nil {
		handleServiceError(c, h.logger, err, "clearance summary")
		return
	}
	OK(c, result)
}

func (h *ClearanceDecisionHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	row, err := h.svc.Get(id)
	if err != nil {
		handleServiceError(c, h.logger, err, "clearance get")
		return
	}
	OK(c, row)
}

func (h *ClearanceDecisionHandler) Decide(c *gin.Context) {
	var request dto.ClearanceDecisionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, err.Error())
		return
	}
	requestID, _ := c.Get("request_id")
	row, err := h.svc.Decide(request.TurnaroundID, middleware.GetUserID(c), request.State,
		request.Restrictions, request.Reason, request.Evidence, stringValue(requestID), middleware.GetPhone(c), c.ClientIP())
	if err != nil {
		handleServiceError(c, h.logger, err, "clearance decision")
		return
	}
	c.Set("audit_persisted", true)
	OKWithMessage(c, constants.MsgDecisionRecorded, row)
}

func (h *ClearanceDecisionHandler) ReconsiderationEligibility(c *gin.Context) {
	turnaroundID, err := strconv.ParseUint(c.Query("turnaround_id"), 10, 64)
	if err != nil || turnaroundID == 0 {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "turnaround_id is required")
		return
	}
	result, err := h.svc.ReconsiderationEligibility(turnaroundID)
	if err != nil {
		handleServiceError(c, h.logger, err, "clearance reconsideration eligibility")
		return
	}
	OK(c, result)
}

func (h *ClearanceDecisionHandler) Reconsider(c *gin.Context) {
	var request dto.ClearanceReconsiderationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, err.Error())
		return
	}
	row, err := h.svc.Reconsider(request.TurnaroundID, middleware.GetUserID(c), request.Reason,
		request.Evidence, requestAuditContext(c))
	if err != nil {
		handleServiceError(c, h.logger, err, "clearance reconsideration")
		return
	}
	c.Set("audit_persisted", true)
	OKWithMessage(c, constants.MsgReconsiderRecorded, row)
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}
