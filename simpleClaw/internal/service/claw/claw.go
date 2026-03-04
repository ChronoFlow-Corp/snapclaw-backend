package claw

import "context"

type Service struct{}

func NewClaw() *Service {
	return &Service{}
}

func (c *Service) Create(ctx context.Context) error {
	const op = "service.Service.Create"
}

func (c *Service) Start() {
	const op = "service.Service.Start"
}

func (c *Service) Stop() {
	const op = "service.Service.Stop"
}

func (c *Service) Delete() {
	const op = "service.Service.Delete"
}
