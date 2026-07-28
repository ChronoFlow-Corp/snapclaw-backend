package commands

import "github.com/google/uuid"

type CreateServer struct {
	Name      string
	IP        string
	URL       string
	ProxyURL  string
	Status    string
	SecretKey string
}

type UpdateServer struct {
	ID        uuid.UUID
	Name      string
	IP        string
	URL       string
	ProxyURL  string
	Status    string
	SecretKey string
}
