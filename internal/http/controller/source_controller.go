package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/source"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
)

type SourceController struct{ uc domain.UseCase }

func NewSourceController(uc domain.UseCase) *SourceController {
	return &SourceController{uc: uc}
}

// Create godoc
//
// @Summary      Create a source
// @Tags         Sources
// @Accept       json
// @Produce      json
// @Param        source  body      domain.CreateRequest  true  "Source to create"
// @Success      201     {object}  SourceEnvelope        "Created source"
// @Failure      400     {object}  common.ErrorEnvelope  "Invalid JSON body or validation error"
// @Failure      401     {object}  common.ErrorEnvelope  "Missing or invalid API token"
// @Failure      409     {object}  common.ErrorEnvelope  "Source URL already exists"
// @Security     APIToken
// @Router       /sources [post]
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

// List godoc
//
// @Summary      List all sources
// @Tags         Sources
// @Produce      json
// @Success      200  {object}  SourceListEnvelope    "Sources"
// @Failure      401  {object}  common.ErrorEnvelope  "Missing or invalid API token"
// @Security     APIToken
// @Router       /sources [get]
func (ctrl *SourceController) List(c *gin.Context) {
	xs, err := ctrl.uc.List(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(domain.ToResponseList(xs)))
}

// Get godoc
//
// @Summary      Get a source by ID
// @Tags         Sources
// @Produce      json
// @Param        id   path      string                true  "Source ID"
// @Success      200  {object}  SourceEnvelope        "Source"
// @Failure      401  {object}  common.ErrorEnvelope  "Missing or invalid API token"
// @Failure      404  {object}  common.ErrorEnvelope  "Source not found"
// @Security     APIToken
// @Router       /sources/{id} [get]
func (ctrl *SourceController) Get(c *gin.Context) {
	s, err := ctrl.uc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(domain.ToResponse(s)))
}

// Update godoc
//
// @Summary      Update a source
// @Tags         Sources
// @Accept       json
// @Produce      json
// @Param        id      path      string                true  "Source ID"
// @Param        source  body      domain.UpdateRequest  true  "Fields to update"
// @Success      200     {object}  SourceEnvelope        "Updated source"
// @Failure      400     {object}  common.ErrorEnvelope  "Invalid JSON body or validation error"
// @Failure      401     {object}  common.ErrorEnvelope  "Missing or invalid API token"
// @Failure      404     {object}  common.ErrorEnvelope  "Source not found"
// @Security     APIToken
// @Router       /sources/{id} [patch]
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

// Delete godoc
//
// @Summary      Delete a source
// @Tags         Sources
// @Param        id   path  string  true  "Source ID"
// @Success      204  "Deleted"
// @Failure      401  {object}  common.ErrorEnvelope  "Missing or invalid API token"
// @Failure      404  {object}  common.ErrorEnvelope  "Source not found"
// @Security     APIToken
// @Router       /sources/{id} [delete]
func (ctrl *SourceController) Delete(c *gin.Context) {
	if err := ctrl.uc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		_ = c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}
