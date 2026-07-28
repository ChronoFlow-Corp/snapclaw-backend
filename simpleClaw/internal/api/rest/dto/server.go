package dto

import "time"

type CreateServerRequest struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	URL       string `json:"url"`
	ProxyURL  string `json:"proxyUrl"`
	Status    string `json:"status"`
	SecretKey string `json:"secretKey"`
}

type UpdateServerRequest struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	URL       string `json:"url"`
	ProxyURL  string `json:"proxyUrl"`
	Status    string `json:"status"`
	SecretKey string `json:"secretKey"`
}

type ServerResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IP        string    `json:"ip"`
	URL       string    `json:"url"`
	ProxyURL  string    `json:"proxyUrl"`
	Status    string    `json:"status"`
	SecretKey string    `json:"secretKey"`
	MaxClaws  int       `json:"maxClaws"`
	CreatedAt time.Time `json:"createdAt"`
}
