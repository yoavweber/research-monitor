package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	domain "github.com/yoavweber/defi-monitor-backend/internal/domain/source"
	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
	"github.com/yoavweber/defi-monitor-backend/internal/http/common"
)

type SourceController struct{ uc domain.UseCase }

func NewSourceController(uc domain.UseCase) *SourceController {
	return &SourceController{uc: uc}
}

func (ctrl *SourceController) Create(c *gin.Context) {
	var req domain.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(shared.NewHTTPError(http.StatusBadRequest, "invalid json body", err))
		return
	}
	s, err := ctrl.uc.Create(c.Request.Context(), req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, common.Data(domain.ToResponse(s)))
}

func (ctrl *SourceController) List(c *gin.Context) {
	xs, err := ctrl.uc.List(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(domain.ToResponseList(xs)))
}

func (ctrl *SourceController) Get(c *gin.Context) {
	s, err := ctrl.uc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(domain.ToResponse(s)))
}

func (ctrl *SourceController) Update(c *gin.Context) {
	var req domain.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(shared.NewHTTPError(http.StatusBadRequest, "invalid json body", err))
		return
	}
	s, err := ctrl.uc.Update(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(domain.ToResponse(s)))
}

func (ctrl *SourceController) Delete(c *gin.Context) {
	if err := ctrl.uc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		_ = c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}
