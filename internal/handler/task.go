package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"cronApi/internal/auth"
	"cronApi/internal/errcode"
	"cronApi/internal/logger"
	"cronApi/internal/response"
	"cronApi/internal/service/runner"
	tasksvc "cronApi/internal/service/task"
)

// TaskHandler 联调任务接口。
type TaskHandler struct {
	svc *tasksvc.TaskService
}

// NewTaskHandler 构造。
func NewTaskHandler(s *tasksvc.TaskService) *TaskHandler { return &TaskHandler{svc: s} }

// Create 创建任务 POST /api/v1/tasks
func (h *TaskHandler) Create(c *gin.Context) {
	var in tasksvc.CreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, errcode.Wrapf(errcode.BindError, err, "参数校验失败: %v", err))
		return
	}
	v, err := h.svc.Save(c.Request.Context(), auth.MustUserID(c), &in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, v)
}

// Update 更新任务 PUT /api/v1/tasks/:id
func (h *TaskHandler) Update(c *gin.Context) {
	id, err := idParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var in tasksvc.CreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, errcode.Wrapf(errcode.BindError, err, "参数校验失败: %v", err))
		return
	}
	v, err := h.svc.Update(c.Request.Context(), id, auth.MustUserID(c), &in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, v)
}

type toggleReq struct {
	Enabled bool `json:"enabled"`
}

// Toggle 启用/停用任务 PATCH /api/v1/tasks/:id/enabled
func (h *TaskHandler) Toggle(c *gin.Context) {
	id, err := idParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req toggleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.Wrap(errcode.BindError, err))
		return
	}
	v, err := h.svc.SetEnabled(c.Request.Context(), id, req.Enabled)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, v)
}

// List 任务列表 GET /api/v1/tasks
func (h *TaskHandler) List(c *gin.Context) {
	page, size := pageQuery(c)
	list, total, err := h.svc.List(c.Request.Context(), auth.MustUserID(c), page, size)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.Page{List: list, Total: total, Page: page, PageSize: size})
}

// Get 任务详情 GET /api/v1/tasks/:id
func (h *TaskHandler) Get(c *gin.Context) {
	id, err := idParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	v, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, v)
}

// Delete 删除任务 DELETE /api/v1/tasks/:id
func (h *TaskHandler) Delete(c *gin.Context) {
	id, err := idParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Run 立即执行 POST /api/v1/tasks/:id/run（同步返回完整报告）
func (h *TaskHandler) Run(c *gin.Context) {
	id, err := idParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	// 透传当前调用者登录态，供步骤 auth=inherit 时复用
	headers := map[string]string{}
	if v := c.GetHeader("Authorization"); v != "" {
		headers["Authorization"] = v
	}
	run, err := h.svc.RunNow(c.Request.Context(), id, runner.TaskRunInput{
		Trigger:     "manual",
		TraceID:     logger.TraceIDFromContext(c.Request.Context()),
		AuthHeaders: headers,
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, run)
}

// ListRuns 执行记录 GET /api/v1/tasks/:id/runs
func (h *TaskHandler) ListRuns(c *gin.Context) {
	id, err := idParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	page, size := pageQuery(c)
	list, total, err := h.svc.ListRuns(c.Request.Context(), id, page, size)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.Page{List: list, Total: total, Page: page, PageSize: size})
}

// GetRun 执行详情 GET /api/v1/tasks/:id/runs/:run_id
func (h *TaskHandler) GetRun(c *gin.Context) {
	id, err := idParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	runID, convErr := strconv.ParseInt(c.Param("run_id"), 10, 64)
	if convErr != nil || runID <= 0 {
		response.Fail(c, errcode.New(errcode.InvalidParam).WithMessage("run_id 非法"))
		return
	}
	run, err := h.svc.GetRun(c.Request.Context(), id, runID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, run)
}
