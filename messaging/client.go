package messaging

import (
	"context"
	"fmt"
	"strings"
)

type Client struct {
	queue         Queue
	subscriptions []*Subscription
}

func NewWithQueue(queue Queue) (*Client, error) {
	if queue == nil {
		return nil, ErrNilQueue
	}
	return &Client{queue: queue}, nil
}

func (c *Client) Close() error {
	if c == nil || c.queue == nil {
		return nil
	}
	return c.queue.Close()
}

func (c *Client) Subscriptions() []*Subscription {
	if c == nil {
		return nil
	}
	return c.subscriptions
}

func (c *Client) Subscribe(subscription *Subscription, handler Handler) error {
	if c == nil {
		return ErrNilClient
	}
	if c.queue == nil {
		return ErrNilQueue
	}
	if err := subscription.Validate(); err != nil {
		return err
	}
	if handler == nil {
		return ErrNilHandler
	}
	return c.queue.Subscribe(subscription, handler)
}

func (c *Client) SubscribeService(service string, handlers map[string]Handler) error {
	if c == nil {
		return ErrNilClient
	}
	if c.queue == nil {
		return ErrNilQueue
	}

	for _, subscription := range c.subscriptions {
		if subscription == nil {
			return ErrNilSubscription
		}

		endpoint := subscription.Endpoint
		if endpoint.Service != "" && endpoint.Service != service {
			continue
		}

		handler, ok := handlers[endpoint.Method]
		if !ok {
			return fmt.Errorf("%w: service %q method %q", ErrUnknownEndpoint, service, endpoint.Method)
		}
		if err := c.Subscribe(subscription, handler); err != nil {
			return fmt.Errorf(
				"messaging: subscribe service %q method %q topic %q: %w",
				service,
				endpoint.Method,
				subscription.Topic,
				err,
			)
		}
	}
	return nil
}

func (c *Client) Publish(ctx context.Context, topic string, messages ...*Message) error {
	if c == nil {
		return ErrNilClient
	}
	if c.queue == nil {
		return ErrNilQueue
	}
	if strings.TrimSpace(topic) == "" {
		return ErrEmptyTopic
	}
	return c.queue.Publish(ctx, topic, messages...)
}
