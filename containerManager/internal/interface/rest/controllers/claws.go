package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"containermanager/internal/interface/rest/middleware"

	"containermanager/internal/entities"
	"containermanager/internal/interface/rest/controllers/dto"
	"containermanager/internal/pkg/logctx"
	"containermanager/internal/service"
	"containermanager/internal/service/commands"

	"github.com/go-chi/chi/v5"
)

type Claw struct {
	s   *service.Container
	key string
}

func NewClaw(s *service.Container, key string) *Claw {
	return &Claw{
		s:   s,
		key: key,
	}
}

func (c *Claw) Register(mux chi.Router) {
	mux.Use(middleware.Auth(c.key))
	mux.Post("/claws", c.CreateClaw)
	mux.Put("/claws", c.Update)
	mux.Get("/claws/start", c.Start)
	mux.Get("/claws/stop", c.Stop)
	mux.Get("/claws/config", c.ConfigArchive)
	mux.Post("/claws/config", c.RestoreConfig)
	mux.Delete("/claws", c.Delete)
	mux.Get("/approve", c.Approve)
}

func (c *Claw) CreateClaw(w http.ResponseWriter, r *http.Request) {
	var cfg dto.CreateClaw

	err := json.NewDecoder(r.Body).Decode(&cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	if cfg.UserID == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}

	if cfg.ClawID == "" {
		http.Error(w, "clawId is required", http.StatusBadRequest)
		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", cfg.UserID),
		slog.String("claw_id", cfg.ClawID),
	)

	cm := mapCreateClawToCommand(cfg)

	containerRecordID, err := c.s.Create(r.Context(), cm)
	if err != nil {
		log.Error("create claw failed", slog.Any("err", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	var res dto.CreateClawResponse

	res.ContainerID = containerRecordID

	json.NewEncoder(w).Encode(res)

	log.Info("claw created", slog.String("container_id", containerRecordID))
}

func (c *Claw) Start(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("userId")
	if q == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)

		return
	}

	clawID := r.URL.Query().Get("clawId")
	if clawID == "" {
		http.Error(w, "clawId is required", http.StatusBadRequest)

		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", q),
		slog.String("claw_id", clawID),
	)

	err := c.s.Start(r.Context(), commands.StartClaw{ClawID: clawID, UserID: q})
	if err != nil {
		log.Error("start claw failed", slog.Any("err", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	log.Info("claw started")
}

func (c *Claw) Approve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("userId")
	if q == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)

		return
	}

	clawID := r.URL.Query().Get("clawId")
	if clawID == "" {
		http.Error(w, "clawId is required", http.StatusBadRequest)

		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)

		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", q),
		slog.String("claw_id", clawID),
		slog.String("code", code),
	)

	err := c.s.Approve(clawID, q, code)
	if err != nil {
		log.Error("approve failed", slog.Any("err", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusOK)
	log.Info("approve succeeded")
}

func (c *Claw) Stop(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("userId")
	if q == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)

		return
	}

	clawID := r.URL.Query().Get("clawId")
	if clawID == "" {
		http.Error(w, "clawId is required", http.StatusBadRequest)

		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", q),
		slog.String("claw_id", clawID),
	)

	err := c.s.Stop(r.Context(), commands.StopClaw{ClawID: clawID, UserID: q})
	if err != nil {
		log.Error("stop claw failed", slog.Any("err", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info("claw stopped")
}

func (c *Claw) Update(w http.ResponseWriter, r *http.Request) {
	var cfg dto.UpdateClaw

	err := json.NewDecoder(r.Body).Decode(&cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	if cfg.UserID == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)

		return
	}

	if cfg.ClawID == "" {
		http.Error(w, "clawId is required", http.StatusBadRequest)

		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", cfg.UserID),
		slog.String("claw_id", cfg.ClawID),
	)

	cm := mapUpdateClawToCommand(cfg)

	err = c.s.Update(r.Context(), cm)
	if err != nil {
		log.Error("update claw failed", slog.Any("err", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusOK)
	log.Info("claw updated")
}

func (c *Claw) Delete(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("userId")
	if q == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)

		return
	}

	clawID := r.URL.Query().Get("clawId")
	if clawID == "" {
		http.Error(w, "clawId is required", http.StatusBadRequest)

		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", q),
		slog.String("claw_id", clawID),
	)

	deleteConfig := false
	if raw := r.URL.Query().Get("deleteConfig"); raw != "" {
		val, err := strconv.ParseBool(raw)
		if err != nil {
			http.Error(w, "deleteConfig must be a boolean", http.StatusBadRequest)
			return
		}
		deleteConfig = val
	}

	err := c.s.Delete(r.Context(), commands.DeleteClaw{
		ClawID:       clawID,
		UserID:       q,
		DeleteConfig: deleteConfig,
	})
	if err != nil {
		log.Error("delete claw failed", slog.Any("err", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info("claw deleted", slog.Bool("delete_config", deleteConfig))
}

func (c *Claw) ConfigArchive(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("userId")
	if q == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}

	clawID := r.URL.Query().Get("clawId")
	if clawID == "" {
		http.Error(w, "clawId is required", http.StatusBadRequest)
		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", q),
		slog.String("claw_id", clawID),
	)

	deleteAfter := false
	if raw := r.URL.Query().Get("deleteAfter"); raw != "" {
		val, err := strconv.ParseBool(raw)
		if err != nil {
			http.Error(w, "deleteAfter must be a boolean", http.StatusBadRequest)
			return
		}
		deleteAfter = val
	}

	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().
		Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"claw-config-%s-%s.tar\"", q, clawID))

	err := c.s.ConfigArchive(
		r.Context(),
		commands.ConfigArchive{UserID: q, ClawID: clawID, DeleteAfter: deleteAfter},
		w,
	)
	if err != nil {
		log.Error(
			"config archive failed",
			slog.Any("err", err),
			slog.Bool("delete_after", deleteAfter),
		)
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info("config archived", slog.Bool("delete_after", deleteAfter))
}

func (c *Claw) RestoreConfig(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("userId")
	if q == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}

	clawID := r.URL.Query().Get("clawId")
	if clawID == "" {
		http.Error(w, "clawId is required", http.StatusBadRequest)
		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", q),
		slog.String("claw_id", clawID),
	)

	err := c.s.RestoreConfig(r.Context(), commands.RestoreConfig{UserID: q, ClawID: clawID}, r.Body)
	if err != nil {
		log.Error("restore config failed", slog.Any("err", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.Info("config restored")
}

func mapCreateClawToCommand(d dto.CreateClaw) commands.CreateClaw {
	cm := commands.CreateClaw{
		Config: make([]entities.ClawConfig, 0),
	}

	cm.Config = mapConfig(d.ClawConfig)

	cm.Vars = d.Vars

	cm.UserID = d.UserID
	cm.ClawID = d.ClawID

	return cm
}

func mapUpdateClawToCommand(d dto.UpdateClaw) commands.UpdateClaw {
	cm := commands.UpdateClaw{
		Config: make([]entities.ClawConfig, 0),
	}

	cm.Config = mapConfig(d.ClawConfig)
	cm.UserID = d.UserID
	cm.ClawID = d.ClawID
	cm.Vars = d.Vars

	return cm
}

func mapConfig(d []dto.ClawConfig) []entities.ClawConfig {
	cm := make([]entities.ClawConfig, len(d))

	for i, c := range d {
		tmpCfg := entities.ClawConfig{
			Name: c.Name,
			Data: []byte(c.Data),
		}

		switch c.FileType {
		case entities.ClawConfigTypeJson:
			tmpCfg.FileType = entities.ClawConfigTypeJson
		case entities.ClawConfigTypeMd:
			tmpCfg.FileType = entities.ClawConfigTypeMd
		case entities.ClawConfigTypeDir:
			tmpCfg.FileType = entities.ClawConfigTypeDir
			tmpCfg.ClawConfig = mapConfig(c.ClawConfig)
		default:
			continue
		}

		cm[i] = tmpCfg
	}

	return cm
}
