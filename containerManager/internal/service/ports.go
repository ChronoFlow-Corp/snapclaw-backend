package service

import (
	"errors"
	"strconv"
	"sync"
)

type Porter struct {
	used  map[string]struct{}
	mu    sync.Mutex
	start int
}

func NewPorter() *Porter {
	return &Porter{
		used:  make(map[string]struct{}),
		start: 10000,
	}
}

func (p *Porter) Acquire() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	port := p.start

	for {
		port += 20
		if port > 65535 {
			return "", errors.New("port out of range")
		}

		if _, exists := p.used[strconv.Itoa(port)]; !exists {
			p.used[strconv.Itoa(port)] = struct{}{}

			return strconv.Itoa(port), nil
		}
	}
}

func (p *Porter) Occupy(port string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.used[port] = struct{}{}
}

func (p *Porter) Release(port string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.used, port)
}
