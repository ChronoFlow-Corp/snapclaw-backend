package entities

import (
	"time"

	"github.com/google/uuid"
)

type Server struct {
	ID        uuid.UUID
	Name      string
	IP        string
	URL       string
	ProxyURL  string
	Status    string
	SecretKey string
	CreatedAt time.Time
}

func NewServer(name, ip, url, proxyURL, status, secretKey string) Server {
	return Server{
		ID:        uuid.New(),
		Name:      name,
		IP:        ip,
		URL:       url,
		ProxyURL:  proxyURL,
		Status:    status,
		SecretKey: secretKey,
		CreatedAt: time.Now(),
	}
}
