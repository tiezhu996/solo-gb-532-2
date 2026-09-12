package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"sonar-survey-coverage-planner/backend/internal/dto"
	"sonar-survey-coverage-planner/backend/internal/service"
	"sonar-survey-coverage-planner/backend/pkg/api"
)

type RunReplayHandler struct{ service *service.RunReplayService }

func NewRunReplayHandler(service *service.RunReplayService) *RunReplayHandler {
	return &RunReplayHandler{service: service}
}

func (h *RunReplayHandler) List(c *gin.Context) {
	page, size := pageQuery(c)
	items, total, err := h.service.List(dto.RunReplayQuery{RunID: uintQuery(c, "run_id"), Page: page, PageSize: size})
	if err != nil {
		writeServiceError(c, err, "运行回放")
		return
	}
	api.Page(c, items, page, size, total)
}

func (h *RunReplayHandler) Get(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	view, err := h.service.Get(id)
	if err != nil {
		writeServiceError(c, err, "运行回放")
		return
	}
	api.Success(c, http.StatusOK, view)
}

func (h *RunReplayHandler) Create(c *gin.Context) {
	var request dto.CreateReplayRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		api.BindError(c, err)
		return
	}
	view, err := h.service.Create(request, actorFrom(c))
	if err != nil {
		writeServiceError(c, err, "运行回放")
		return
	}
	api.Success(c, http.StatusCreated, view)
}
