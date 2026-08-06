package handler

import (
	"context"
	"net/http"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
)

type DemoUserService interface {
	List(context.Context) ([]domain.DemoUser, error)
}

type DemoUserHandler struct{ users DemoUserService }

type DemoUserResponse struct {
	ExternalUserID string `json:"external_user_id" binding:"required" format:"uuid"`
	DisplayName    string `json:"display_name" binding:"required"`
}

func NewDemoUserHandler(users DemoUserService) *DemoUserHandler {
	return &DemoUserHandler{users: users}
}

// List godoc
//
//	@Summary	List demo accounts available for local account selection
//	@Tags		demo
//	@Produce	json
//	@Success	200	{array}		DemoUserResponse
//	@Failure	500	{object}	middleware.ErrorResponse
//	@Router		/api/v1/demo/users [get]
func (handler *DemoUserHandler) List(c *gin.Context) {
	users, err := handler.users.List(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		return
	}
	response := make([]DemoUserResponse, 0, len(users))
	for _, user := range users {
		response = append(response, DemoUserResponse{
			ExternalUserID: string(user.ExternalUserID), DisplayName: user.DisplayName,
		})
	}
	c.JSON(http.StatusOK, response)
}

var _ = httpmiddleware.ErrorResponse{}
