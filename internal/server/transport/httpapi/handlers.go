package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/momaek/tolato/internal/server/app/usecase"
	infraauth "github.com/momaek/tolato/internal/server/infra/auth"
	"github.com/momaek/tolato/internal/shared/errs"
	"github.com/momaek/tolato/internal/shared/types"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Handler struct {
	logger   *zap.Logger
	auth     infraauth.Service
	usecases usecase.Services
	db       *pgxpool.Pool
	redis    *goredis.Client
}

func (h Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	errs.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := dependenciesReady(ctx, h.db, h.redis); err != nil {
		errs.WriteError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	errs.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req types.LoginRequest
	if err := errs.DecodeJSON(r, &req); err != nil {
		errs.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.auth.Login(req.Username, req.Password)
	if err != nil {
		errs.WriteError(w, http.StatusUnauthorized, err.Error())
		return
	}

	errs.WriteJSON(w, http.StatusOK, types.LoginResponse{User: user})
}

func (h Handler) Me(w http.ResponseWriter, r *http.Request) {
	errs.WriteJSON(w, http.StatusOK, types.LoginResponse{User: h.auth.CurrentUser()})
}

func (h Handler) ListNodes(w http.ResponseWriter, r *http.Request) {
	resp, err := h.usecases.ListNodes.Execute(r.Context())
	if err != nil {
		errs.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	errs.WriteJSON(w, http.StatusOK, resp)
}

func (h Handler) GetNode(w http.ResponseWriter, r *http.Request) {
	resp, err := h.usecases.GetNode.Execute(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		errs.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	errs.WriteJSON(w, http.StatusOK, resp)
}

func (h Handler) GenerateTaskPlan(w http.ResponseWriter, r *http.Request) {
	var req types.TaskPlanRequest
	if err := errs.DecodeJSON(r, &req); err != nil {
		errs.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.usecases.GenerateTaskPlan.Execute(r.Context(), req)
	if err != nil {
		errs.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	errs.WriteJSON(w, http.StatusOK, resp)
}

func (h Handler) ApproveTask(w http.ResponseWriter, r *http.Request) {
	resp, err := h.usecases.ApproveTask.Execute(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		errs.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	errs.WriteJSON(w, http.StatusOK, resp)
}

func (h Handler) RejectTask(w http.ResponseWriter, r *http.Request) {
	resp, err := h.usecases.RejectTask.Execute(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		errs.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	errs.WriteJSON(w, http.StatusOK, resp)
}

func (h Handler) CancelTask(w http.ResponseWriter, r *http.Request) {
	resp, err := h.usecases.CancelTask.Execute(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		errs.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	errs.WriteJSON(w, http.StatusOK, resp)
}

func (h Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	resp, err := h.usecases.GetTask.Execute(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		errs.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	errs.WriteJSON(w, http.StatusOK, resp)
}

func (h Handler) ListTaskExecutions(w http.ResponseWriter, r *http.Request) {
	resp, err := h.usecases.ListTaskExecutions.Execute(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		errs.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	errs.WriteJSON(w, http.StatusOK, resp)
}

func (h Handler) ListAudits(w http.ResponseWriter, r *http.Request) {
	resp, err := h.usecases.ListAuditEvents.Execute(r.Context(), r.URL.Query().Get("task_id"))
	if err != nil {
		errs.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	errs.WriteJSON(w, http.StatusOK, resp)
}

func (h Handler) EnrollAgent(w http.ResponseWriter, r *http.Request) {
	var req types.EnrollRequest
	if err := errs.DecodeJSON(r, &req); err != nil {
		errs.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.usecases.RegisterNode.Execute(r.Context(), req)
	if err != nil {
		errs.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	errs.WriteJSON(w, http.StatusOK, resp)
}
