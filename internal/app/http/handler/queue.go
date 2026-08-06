package handler

import (
	"context"
	"net/http"
	"time"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/app/identity"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type QueueService interface {
	Join(context.Context, domain.ProductID, domain.ExternalUserID, domain.IdempotencyKey) (domain.JoinQueueResult, error)
	Current(context.Context, domain.ProductID, domain.ExternalUserID) (domain.CurrentQueueResult, error)
	Leave(context.Context, domain.ProductID, domain.ExternalUserID) error
}

type QueueHandler struct{ queue QueueService }

type QueueEntryResponse struct {
	AttemptID         string                   `json:"attempt_id" binding:"required" format:"uuid"`
	ProductID         string                   `json:"product_id" binding:"required" format:"uuid"`
	State             domain.QueueAttemptState `json:"state" binding:"required"`
	QueueSequence     int64                    `json:"queue_sequence" binding:"required"`
	PositionAhead     *int64                   `json:"position_ahead,omitempty"`
	DeadlineAt        *time.Time               `json:"deadline_at,omitempty" format:"date-time"`
	Position          *int64                   `json:"position,omitempty" minimum:"1"`
	TotalWaiting      *int64                   `json:"total_waiting,omitempty" minimum:"0"`
	ExpiresAt         *time.Time               `json:"expires_at,omitempty" format:"date-time"`
	NextAction        string                   `json:"next_action" binding:"required"`
	MessageCode       string                   `json:"message_code" binding:"required"`
	CreatedAt         time.Time                `json:"created_at" binding:"required" format:"date-time"`
	UpdatedAt         time.Time                `json:"updated_at" binding:"required" format:"date-time"`
	InvitedAt         *time.Time               `json:"invited_at,omitempty" format:"date-time"`
	CheckoutStartedAt *time.Time               `json:"checkout_started_at,omitempty" format:"date-time"`
	TerminalAt        *time.Time               `json:"terminal_at,omitempty" format:"date-time"`
	PurchasedAt       *time.Time               `json:"purchased_at,omitempty" format:"date-time"`
}

func NewQueueHandler(queue QueueService) *QueueHandler { return &QueueHandler{queue: queue} }

// Join godoc
//
//	@Summary	Join a product queue
//	@Tags		queue
//	@Produce	json
//	@Param		X-User-ID			header		string	true	"Canonical lowercase external user UUID"	format(uuid)
//	@Param		Idempotency-Key		header		string	true	"Scoped queue join idempotency key"			minlength(1)	maxlength(128)
//	@Param		productID			path		string	true	"Product UUID"								format(uuid)
//	@Success	200					{object}	QueueEntryResponse
//	@Success	201					{object}	QueueEntryResponse
//	@Failure	400,401,404,410,500	{object}	middleware.ErrorResponse
//	@Failure	409					{object}	middleware.ErrorResponse	"Conflict; disabled queues return error.code queue_disabled"
//	@Router		/api/v1/products/{productID}/queue-entries [post]
func (handler *QueueHandler) Join(c *gin.Context) {
	productID, userID, ok := parseProductAndIdentity(c)
	if !ok {
		return
	}
	key, err := domain.ParseIdempotencyKey(c.GetHeader("Idempotency-Key"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	result, err := handler.queue.Join(c.Request.Context(), productID, userID, key)
	if err != nil {
		_ = c.Error(err)
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	c.JSON(status, mapJoinQueueAttempt(result))
}

// Current godoc
//
//	@Summary	Get the current or latest queue entry
//	@Tags		queue
//	@Produce	json
//	@Param		X-User-ID		header		string	true	"Canonical lowercase external user UUID"	format(uuid)
//	@Param		productID		path		string	true	"Product UUID"								format(uuid)
//	@Success	200				{object}	QueueEntryResponse
//	@Failure	400,401,404,500	{object}	middleware.ErrorResponse
//	@Router		/api/v1/products/{productID}/queue-entry [get]
func (handler *QueueHandler) Current(c *gin.Context) {
	productID, userID, ok := parseProductAndIdentity(c)
	if !ok {
		return
	}
	result, err := handler.queue.Current(c.Request.Context(), productID, userID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, mapCurrentQueueAttempt(result))
}

// Leave godoc
//
//	@Summary	Cancel the current or latest queue entry
//	@Tags		queue
//	@Param		X-User-ID	header	string	true	"Canonical lowercase external user UUID"	format(uuid)
//	@Param		productID	path	string	true	"Product UUID"								format(uuid)
//	@Success	204
//	@Failure	400,401,404,409,500	{object}	middleware.ErrorResponse
//	@Router		/api/v1/products/{productID}/queue-entry [delete]
func (handler *QueueHandler) Leave(c *gin.Context) {
	productID, userID, ok := parseProductAndIdentity(c)
	if !ok {
		return
	}
	if err := handler.queue.Leave(c.Request.Context(), productID, userID); err != nil {
		_ = c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

func parseProductAndIdentity(c *gin.Context) (domain.ProductID, domain.ExternalUserID, bool) {
	userID, exists := identity.FromContext(c)
	if !exists {
		_ = c.Error(domain.ErrInvalidIdentity)
		return domain.ProductID{}, "", false
	}
	productID, err := domain.ParseProductID(c.Param("productID"))
	if err != nil {
		_ = c.Error(err)
		return domain.ProductID{}, "", false
	}
	return productID, userID, true
}

func mapQueueAttempt(attempt domain.QueueAttempt, positionAhead int64) QueueEntryResponse {
	nextAction, messageCode := queueClientState(attempt.State)
	response := QueueEntryResponse{
		AttemptID: uuid.UUID(attempt.ID).String(), ProductID: uuid.UUID(attempt.ProductID).String(),
		State: attempt.State, QueueSequence: attempt.QueueSequence, NextAction: nextAction, MessageCode: messageCode,
		CreatedAt: attempt.CreatedAt, UpdatedAt: attempt.UpdatedAt, InvitedAt: attempt.InvitedAt,
		CheckoutStartedAt: attempt.CheckoutStartedAt, TerminalAt: attempt.TerminalAt, PurchasedAt: attempt.PurchasedAt,
	}
	if attempt.State == domain.QueueAttemptWaiting {
		response.PositionAhead = &positionAhead
	}
	if attempt.State == domain.QueueAttemptInvited {
		response.DeadlineAt = attempt.InvitationDeadline
	}
	if attempt.State == domain.QueueAttemptCheckout {
		response.DeadlineAt = attempt.CheckoutDeadline
	}
	return response
}

func mapCurrentQueueAttempt(result domain.CurrentQueueResult) QueueEntryResponse {
	return mapQueueAttemptWithFrontendStatus(result.Attempt, result.PositionAhead, result.TotalWaiting)
}

func mapJoinQueueAttempt(result domain.JoinQueueResult) QueueEntryResponse {
	return mapQueueAttemptWithFrontendStatus(result.Attempt, result.PositionAhead, result.TotalWaiting)
}

func mapQueueAttemptWithFrontendStatus(
	attempt domain.QueueAttempt,
	positionAhead, totalWaiting int64,
) QueueEntryResponse {
	response := mapQueueAttempt(attempt, positionAhead)
	response.TotalWaiting = &totalWaiting

	switch attempt.State {
	case domain.QueueAttemptWaiting:
		position := positionAhead + 1
		response.Position = &position
	case domain.QueueAttemptInvited:
		response.ExpiresAt = attempt.InvitationDeadline
	case domain.QueueAttemptCheckout:
		response.ExpiresAt = attempt.CheckoutDeadline
	}

	return response
}

func queueClientState(state domain.QueueAttemptState) (string, string) {
	switch state {
	case domain.QueueAttemptWaiting:
		return "wait", "queue_waiting"
	case domain.QueueAttemptInvited:
		return "start_checkout", "checkout_available"
	case domain.QueueAttemptCheckout:
		return "complete_payment", "checkout_started"
	case domain.QueueAttemptPurchased:
		return "none", "purchased"
	case domain.QueueAttemptInviteExpired:
		return "join_queue", "invitation_expired"
	case domain.QueueAttemptCheckoutExpired:
		return "join_queue", "checkout_expired"
	case domain.QueueAttemptPaymentFailed:
		return "join_queue", "payment_failed"
	case domain.QueueAttemptCancelled:
		return "join_queue", "cancelled"
	case domain.QueueAttemptSoldOut:
		return "none", "sold_out"
	default:
		return "none", "unknown_state"
	}
}

var _ = httpmiddleware.ErrorResponse{}
