package messaging

import (
	"strings"

	"github.com/fino-io/finokit/config"
)

const (
	DriverNATS       = "nats"
	defaultConfigKey = "messaging"
)

type Config struct {
	Driver        string          `json:"driver"`
	Nats          NatsConfig      `json:"nats"`
	Subscriptions []*Subscription `json:"subscriptions"`
}

type NatsConfig struct {
	URL       string        `json:"url" default:"nats://127.0.0.1:4222"`
	JetStream bool          `json:"jetStream"`
	Streams   []*NatsStream `json:"streams"`
}

type NatsStream struct {
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
	queue, err := buildQueue(cfg)
	if err != nil {
		return nil, err
	}

	if queue == nil {
		return nil, ErrNilQueue
	}
	return &Client{
		queue:         queue,
		subscriptions: cfg.Subscriptions,
	}, nil
}

func (c *Config) normalized() (*Config, error) {
	if c == nil {
		return nil, ErrNilConfig
	}

	cfg := *c
	cfg.Driver = strings.ToLower(strings.TrimSpace(cfg.Driver))
	if cfg.Driver == "" {
		cfg.Driver = DriverNATS
	}

	return &cfg, nil
}
