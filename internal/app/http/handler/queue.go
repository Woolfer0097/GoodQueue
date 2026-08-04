package handler

import (
	"context"
	"net/http"
	"time"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type QueueService interface {
	Join(context.Context, domain.ProductID, domain.ExternalUserID, uuid.UUID) (domain.QueueEntry, error)
	Current(context.Context, domain.ProductID, domain.ExternalUserID) (domain.QueueEntry, error)
	Leave(context.Context, domain.ProductID, domain.ExternalUserID) (domain.QueueEntry, error)
}

type QueueHandler struct{ queue QueueService }

type QueueEntryResponse struct {
	EntryID      int64   `json:"entry_id" binding:"required" example:"42"`
	ProductID    string  `json:"product_id" binding:"required" format:"uuid"`
	Status       string  `json:"status" binding:"required" enums:"waiting,granted,purchased,cancelled,expired" example:"waiting"`
	Position     *int    `json:"position" binding:"required" extensions:"x-nullable" example:"3"`
	TotalWaiting *int    `json:"total_waiting" binding:"required" extensions:"x-nullable" example:"7"`
	ExpiresAt    *string `json:"expires_at" binding:"required" extensions:"x-nullable" format:"date-time"`
}

func NewQueueHandler(queue QueueService) *QueueHandler { return &QueueHandler{queue: queue} }

// Join godoc
//
//	@Summary		Join a product queue
//	@Description	Returns a stable waiting snapshot in mock API mode; no queue entry is persisted.
//	@Tags			queue
//	@Accept			json
//	@Produce		json
//	@Param			X-User-ID	header		string	true	"External user identity"	maxlength(255)
//	@Param			productID	path		string	true	"Product UUID"				format(uuid)
//	@Success		201			{object}	QueueEntryResponse
//	@Failure		400			{object}	ErrorResponse				"INVALID_PRODUCT_ID"
//	@Failure		401			{object}	ErrorResponse				"UNAUTHORIZED or INVALID_USER_ID"
//	@Failure		404			{object}	ErrorResponse				"PRODUCT_NOT_FOUND"
//	@Failure		500			{object}	ErrorResponse				"INTERNAL_ERROR"
//	@Failure		501			{object}	middleware.ErrorResponse	"Mock API disabled"
//	@Router			/api/v1/products/{productID}/queue-entries [post]
func (handler *QueueHandler) Join(c *gin.Context) {
	productID, userID, valid := queueRequestIdentity(c)
	if !valid {
		return
	}
	entry, err := handler.queue.Join(c.Request.Context(), productID, userID, uuid.Nil)
	if err != nil {
		handleAPIServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, queueEntryResponse(entry))
}

// Current godoc
//
//	@Summary		Get the current queue entry
//	@Description	Returns the stable queue snapshot selected by GOODQUEUE_MOCK_QUEUE_STATUS.
//	@Tags			queue
//	@Produce		json
//	@Param			X-User-ID	header		string	true	"External user identity"	maxlength(255)
//	@Param			productID	path		string	true	"Product UUID"				format(uuid)
//	@Success		200			{object}	QueueEntryResponse
//	@Failure		400			{object}	ErrorResponse				"INVALID_PRODUCT_ID"
//	@Failure		401			{object}	ErrorResponse				"UNAUTHORIZED or INVALID_USER_ID"
//	@Failure		404			{object}	ErrorResponse				"PRODUCT_NOT_FOUND"
//	@Failure		500			{object}	ErrorResponse				"INTERNAL_ERROR"
//	@Failure		501			{object}	middleware.ErrorResponse	"Mock API disabled"
//	@Router			/api/v1/products/{productID}/queue-entry [get]
func (handler *QueueHandler) Current(c *gin.Context) {
	productID, userID, valid := queueRequestIdentity(c)
	if !valid {
		return
	}
	entry, err := handler.queue.Current(c.Request.Context(), productID, userID)
	if err != nil {
		handleAPIServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, queueEntryResponse(entry))
}

// Leave godoc
//
//	@Summary		Leave the current product queue
//	@Description	Returns a stable cancelled snapshot in mock API mode; no state is persisted.
//	@Tags			queue
//	@Produce		json
//	@Param			X-User-ID	header		string	true	"External user identity"	maxlength(255)
//	@Param			productID	path		string	true	"Product UUID"				format(uuid)
//	@Success		200			{object}	QueueEntryResponse
//	@Failure		400			{object}	ErrorResponse				"INVALID_PRODUCT_ID"
//	@Failure		401			{object}	ErrorResponse				"UNAUTHORIZED or INVALID_USER_ID"
//	@Failure		404			{object}	ErrorResponse				"PRODUCT_NOT_FOUND"
//	@Failure		500			{object}	ErrorResponse				"INTERNAL_ERROR"
//	@Failure		501			{object}	middleware.ErrorResponse	"Mock API disabled"
//	@Router			/api/v1/products/{productID}/queue-entry [delete]
func (handler *QueueHandler) Leave(c *gin.Context) {
	productID, userID, valid := queueRequestIdentity(c)
	if !valid {
		return
	}
	entry, err := handler.queue.Leave(c.Request.Context(), productID, userID)
	if err != nil {
		handleAPIServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, queueEntryResponse(entry))
}

func queueRequestIdentity(c *gin.Context) (domain.ProductID, domain.ExternalUserID, bool) {
	productID, valid := parseProductID(c)
	if !valid {
		return domain.ProductID{}, "", false
	}
	userID, valid := requireUserID(c)
	if !valid {
		return domain.ProductID{}, "", false
	}
	return productID, userID, true
}

func queueEntryResponse(entry domain.QueueEntry) QueueEntryResponse {
	var expiresAt *string
	if entry.ExpiredAt != nil {
		formatted := entry.ExpiredAt.UTC().Format(time.RFC3339)
		expiresAt = &formatted
	}
	return QueueEntryResponse{
		EntryID:      entry.TicketID,
		ProductID:    uuid.UUID(entry.ProductID).String(),
		Status:       queueEntryStatusResponse(entry.Status),
		Position:     entry.Position,
		TotalWaiting: entry.TotalWaiting,
		ExpiresAt:    expiresAt,
	}
}

func queueEntryStatusResponse(status domain.QueueEntryStatus) string {
	switch status {
	case domain.QueueEntryRightIssued:
		return "granted"
	case domain.QueueEntryCompleted:
		return "purchased"
	default:
		return string(status)
	}
}

var _ = httpmiddleware.ErrorResponse{}
