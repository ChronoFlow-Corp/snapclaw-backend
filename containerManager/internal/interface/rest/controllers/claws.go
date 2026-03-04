package controllers

import (
	"encoding/json"
	"net/http"

	"containermanager/internal/entities"
	"containermanager/internal/interface/rest/controllers/dto"
	"containermanager/internal/service"
	"containermanager/internal/service/commands"

	"github.com/go-chi/chi"
)

type Claw struct {
	s *service.Container
}

func NewClaw(s *service.Container) *Claw {
	return &Claw{
		s: s,
	}
}

func (c *Claw) Register(mux chi.Router) {
	mux.Post("/claws", c.CreateClaw)
	mux.Get("/claws/start", c.Start)
	mux.Get("/claws/stop", c.Stop)
	mux.Delete("/claws", c.Delete)
}

func (c *Claw) CreateClaw(w http.ResponseWriter, r *http.Request) {
	var cfg dto.CreateClaw

	err := json.NewDecoder(r.Body).Decode(&cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	cm := mapCreateClawToCommand(cfg)

	cId, err := c.s.Create(r.Context(), cm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	var res dto.CreateClawResponse

	res.ContainerID = cId

	json.NewEncoder(w).Encode(res)
}

func (c *Claw) Start(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("userId")
	if q == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)

		return
	}

	cID := r.URL.Query().Get("containerId")
	if cID == "" {
		http.Error(w, "containerId is required", http.StatusBadRequest)
	}

	err := c.s.Start(r.Context(), commands.StartClaw{ContainerID: cID, UserID: q})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}
}

func (c *Claw) Stop(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("userId")
	if q == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)

		return
	}

	cID := r.URL.Query().Get("containerId")
	if cID == "" {
		http.Error(w, "containerId is required", http.StatusBadRequest)
	}

	err := c.s.Stop(r.Context(), commands.StopClaw{ContainerID: cID, UserID: q})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (c *Claw) Delete(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("userId")
	if q == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)

		return
	}

	cID := r.URL.Query().Get("containerId")
	if cID == "" {
		http.Error(w, "containerId is required", http.StatusBadRequest)
	}

	err := c.s.Delete(r.Context(), commands.DeleteClaw{ContainerID: cID, UserID: q})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func mapCreateClawToCommand(d dto.CreateClaw) commands.CreateClaw {
	cm := commands.CreateClaw{
		Config: make([]entities.ClawConfig, 0),
	}

	cm.Config = mapConfig(d.ClawConfig)

	cm.UserID = d.UserID

	return cm
}

func mapConfig(d []dto.ClawConfig) []entities.ClawConfig {
	cm := make([]entities.ClawConfig, len(d))

	for i, c := range d {
		tmpCfg := entities.ClawConfig{}

		switch c.FileType {
		case entities.ClawConfigTypeJson:
			tmpCfg.FileType = entities.ClawConfigTypeJson
		case entities.ClawConfigTypeMd:
			tmpCfg.FileType = entities.ClawConfigTypeMd
		case entities.ClawConfigTypeDir:
			tmpCfg.FileType = entities.ClawConfigTypeDir
			tmpCfg.Name = c.Name
			cm[i].ClawConfig = mapConfig(c.ClawConfig)
		default:
			continue
		}

		tmpCfg.Data = []byte(c.Data)
		tmpCfg.Name = c.Name
		cm[i] = tmpCfg
	}

	return cm
}
