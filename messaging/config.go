package messaging

import (
	"strings"

	"github.com/fino-io/finokit/config"
)

const defaultConfigKey = "messaging"

type Config struct {
	URL           string          `json:"url" default:"nats://127.0.0.1:4222"`
	JetStream     bool            `json:"jetStream"`
	Streams       []*Stream       `json:"streams"`
	Subscriptions []*Subscription `json:"subscriptions"`
}

type Stream struct {
	Name     string   `json:"name"`
	Subjects []string `json:"subjects"`
}

func New() (*Client, error) {
	cfg := &Config{}
	if err := config.ScanFrom(cfg, defaultConfigKey); err != nil {
		return nil, err
	}
	return NewWithConfig(cfg)
}

func NewWithConfig(cfg *Config) (*Client, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	queue, err := newNATSQueue(normalized)
	if err != nil {
		return nil, err
	}

	return &Client{
		queue:         queue,
		subscriptions: normalized.Subscriptions,
	}, nil
}

func normalizeConfig(c *Config) (*Config, error) {
	if c == nil {
		return nil, ErrNilConfig
	}

	cfg := *c
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.URL == "" {
		return nil, ErrEmptyURL
	}

	return &cfg, nil
}
