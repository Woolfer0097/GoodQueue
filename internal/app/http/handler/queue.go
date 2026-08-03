package handler

import (
	"context"
	"time"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type QueueService interface {
	Join(context.Context, domain.ProductID, domain.ExternalUserID, uuid.UUID) (domain.QueueEntry, error)
	Current(context.Context, domain.ProductID, domain.ExternalUserID) (domain.QueueEntry, error)
	Leave(context.Context, domain.ProductID, domain.ExternalUserID) error
}

var _ = httpmiddleware.ErrorResponse{}

type QueueHandler struct{ queue QueueService }

type JoinQueueRequest struct {
	IdempotencyKey string `json:"idempotency_key" binding:"required" format:"uuid" example:"7ae799c1-0dfa-4248-b80b-4e60e61f431d"`
}

type QueueEntryResponse struct {
	TicketID      int64                   `json:"ticket_id" binding:"required" example:"42"`
	ProductID     string                  `json:"product_id" binding:"required" format:"uuid"`
	Status        domain.QueueEntryStatus `json:"status" binding:"required" enums:"waiting,right_issued,completed,cancelled,expired" example:"waiting"`
	JoinedAt      time.Time               `json:"joined_at" binding:"required" format:"date-time"`
	RightIssuedAt *time.Time              `json:"right_issued_at,omitempty" format:"date-time"`
	CompletedAt   *time.Time              `json:"completed_at,omitempty" format:"date-time"`
	CancelledAt   *time.Time              `json:"cancelled_at,omitempty" format:"date-time"`
	ExpiredAt     *time.Time              `json:"expired_at,omitempty" format:"date-time"`
}

func NewQueueHandler(queue QueueService) *QueueHandler { return &QueueHandler{queue: queue} }

// Join godoc
//
//	@Summary		Join a product queue
//	@Description	Reserved business contract; returns 501 without parsing input or changing PostgreSQL.
//	@Tags			queue
//	@Accept			json
//	@Produce		json
//	@Param			X-User-ID	header		string				false	"Trusted external user identity supplied by upstream authentication"	maxlength(255)
//	@Param			productID	path		string				true	"Product UUID"															format(uuid)
//	@Param			request		body		JoinQueueRequest	true	"Queue join idempotency key; future persistence is unique per external user"
//	@Success		201			{object}	QueueEntryResponse
//	@Failure		501			{object}	middleware.ErrorResponse
//	@Router			/api/v1/products/{productID}/queue-entries [post]
func (handler *QueueHandler) Join(c *gin.Context) {
	_, err := handler.queue.Join(c.Request.Context(), domain.ProductID{}, "", uuid.Nil)
	_ = c.Error(err)
}

// Current godoc
//
//	@Summary		Get the current queue entry
//	@Description	Reserved business contract; returns 501 without querying PostgreSQL.
//	@Tags			queue
//	@Produce		json
//	@Param			X-User-ID	header		string	false	"Trusted external user identity supplied by upstream authentication"	maxlength(255)
//	@Param			productID	path		string	true	"Product UUID"															format(uuid)
//	@Success		200			{object}	QueueEntryResponse
//	@Failure		501			{object}	middleware.ErrorResponse
//	@Router			/api/v1/products/{productID}/queue-entry [get]
func (handler *QueueHandler) Current(c *gin.Context) {
	_, err := handler.queue.Current(c.Request.Context(), domain.ProductID{}, "")
	_ = c.Error(err)
}

// Leave godoc
//
//	@Summary		Leave the current product queue
//	@Description	Reserved business contract; returns 501 without mutating PostgreSQL.
//	@Tags			queue
//	@Produce		json
//	@Param			X-User-ID	header	string	false	"Trusted external user identity supplied by upstream authentication"	maxlength(255)
//	@Param			productID	path	string	true	"Product UUID"															format(uuid)
//	@Success		204
//	@Failure		501	{object}	middleware.ErrorResponse
//	@Router			/api/v1/products/{productID}/queue-entry [delete]
func (handler *QueueHandler) Leave(c *gin.Context) {
	_ = c.Error(handler.queue.Leave(c.Request.Context(), domain.ProductID{}, ""))
}
