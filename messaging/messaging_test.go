package messaging

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fino-io/finokit/config"
)

type stubQueue struct {
	publishErr    error
	subscribeErr  error
	subscriptions []*Subscription
	handlers      []Handler
}

func (s *stubQueue) Publish(ctx context.Context, topic string, messages ...*Message) error {
	_ = ctx
	_ = topic
	_ = messages
	return s.publishErr
}

func (s *stubQueue) Subscribe(subscription *Subscription, handler Handler) error {
	s.subscriptions = append(s.subscriptions, subscription)
	s.handlers = append(s.handlers, handler)
	return s.subscribeErr
}

func (s *stubQueue) Shutdown() {}

func TestNew(t *testing.T) {
	t.Run("new client with injected queue", func(t *testing.T) {
		if _, err := NewWithQueue(nil); !errors.Is(err, ErrNilQueue) {
			t.Fatalf("expect ErrNilQueue, got %v", err)
		}

		client, err := NewWithQueue(&stubQueue{})
		if err != nil {
			t.Fatalf("new client failed: %v", err)
		}
		if client.queue == nil {
			t.Fatal("expect non-nil queue")
		}
	})

	t.Run("new with config validates before connecting", func(t *testing.T) {
		client, err := NewWithConfig(nil)
		if !errors.Is(err, ErrNilConfig) {
			t.Fatalf("expect ErrNilConfig, got %v", err)
		}
		if client != nil {
			t.Fatalf("expect nil client, got %#v", client)
		}

		client, err = NewWithConfig(&Config{})
		if !errors.Is(err, ErrEmptyURL) {
			t.Fatalf("expect ErrEmptyURL, got %v", err)
		}
		if client != nil {
			t.Fatalf("expect nil client, got %#v", client)
		}
	})

	t.Run("loads flattened config", func(t *testing.T) {
		loadTestConfig(t, "messaging.yaml", `messaging:
  url: ""
  jetStream: true
  streams:
    - name: demo
      subjects:
        - demo.>
  subscriptions:
    - name: agent-start-task
      topic: demo.start-task
      pull: true
      ackWait: 5m
      endpoint:
        service: Agent
        method: start_task
`)

		cfg := &Config{}
		if err := config.ScanFrom(cfg, defaultConfigKey); err != nil {
			t.Fatalf("scan config failed: %v", err)
		}
		if cfg.JetStream != true || len(cfg.Streams) != 1 || len(cfg.Subscriptions) != 1 {
			t.Fatalf("unexpected flattened config: %#v", cfg)
		}
		if cfg.Subscriptions[0].AckWait != 5*time.Minute {
			t.Fatalf("unexpected subscription: %#v", cfg.Subscriptions[0])
		}
		if _, err := New(); !errors.Is(err, ErrEmptyURL) {
			t.Fatalf("expect New() to validate URL, got %v", err)
		}
	})
}

func TestClientValidation(t *testing.T) {
	var nilClient *Client
	if err := nilClient.Publish(context.Background(), "topic", &Message{}); !errors.Is(err, ErrNilClient) {
		t.Fatalf("expect ErrNilClient, got %v", err)
	}
	if err := nilClient.Subscribe(&Subscription{Topic: "topic"}, func(context.Context, *Subscription, *SubMessage) error { return nil }); !errors.Is(err, ErrNilClient) {
		t.Fatalf("expect ErrNilClient, got %v", err)
	}

	client := &Client{}
	if err := client.Publish(context.Background(), "topic", &Message{}); !errors.Is(err, ErrNilQueue) {
		t.Fatalf("expect ErrNilQueue, got %v", err)
	}
	if err := client.Subscribe(&Subscription{Topic: "topic"}, func(context.Context, *Subscription, *SubMessage) error { return nil }); !errors.Is(err, ErrNilQueue) {
		t.Fatalf("expect ErrNilQueue, got %v", err)
	}

	client.queue = &stubQueue{}
	if err := client.Publish(context.Background(), " ", &Message{}); !errors.Is(err, ErrEmptyTopic) {
		t.Fatalf("expect ErrEmptyTopic, got %v", err)
	}
	if err := client.Subscribe(nil, func(context.Context, *Subscription, *SubMessage) error { return nil }); !errors.Is(err, ErrNilSubscription) {
		t.Fatalf("expect ErrNilSubscription, got %v", err)
	}
	if err := client.Subscribe(&Subscription{Topic: ""}, func(context.Context, *Subscription, *SubMessage) error { return nil }); !errors.Is(err, ErrEmptyTopic) {
		t.Fatalf("expect ErrEmptyTopic, got %v", err)
	}
	if err := client.Subscribe(&Subscription{Topic: "topic"}, nil); !errors.Is(err, ErrNilHandler) {
		t.Fatalf("expect ErrNilHandler, got %v", err)
	}

	publishErr := errors.New("publish err")
	client.queue = &stubQueue{publishErr: publishErr}
	if err := client.Publish(context.Background(), "topic", &Message{}); !errors.Is(err, publishErr) {
		t.Fatalf("expect publish error passthrough, got %v", err)
	}

	subscribeErr := errors.New("subscribe err")
	client.queue = &stubQueue{subscribeErr: subscribeErr}
	if err := client.Subscribe(&Subscription{Topic: "topic"}, func(context.Context, *Subscription, *SubMessage) error { return nil }); !errors.Is(err, subscribeErr) {
		t.Fatalf("expect subscribe error passthrough, got %v", err)
	}
}

func TestClientSubscribeService(t *testing.T) {
	queue := &stubQueue{}
	client, err := NewWithQueue(queue)
	if err != nil {
		t.Fatalf("NewWithQueue() failed: %v", err)
	}
	client.subscriptions = []*Subscription{
		{Topic: "user.create", Endpoint: Endpoint{Service: "UserService", Method: "create_user"}},
		{Topic: "user.update", Endpoint: Endpoint{Method: "update_user"}},
		{Topic: "other.create", Endpoint: Endpoint{Service: "OtherService", Method: "create_user"}},
	}

	createHandler := func(context.Context, *Subscription, *SubMessage) error { return nil }
	updateHandler := func(context.Context, *Subscription, *SubMessage) error { return nil }
	err = client.SubscribeService("UserService", map[string]Handler{
		"create_user": createHandler,
		"update_user": updateHandler,
	})
	if err != nil {
		t.Fatalf("SubscribeService() failed: %v", err)
	}
	if len(queue.subscriptions) != 2 {
		t.Fatalf("expect two subscriptions, got %d", len(queue.subscriptions))
	}
	if queue.subscriptions[0].Topic != "user.create" || queue.subscriptions[1].Topic != "user.update" {
		t.Fatalf("unexpected subscriptions: %#v", queue.subscriptions)
	}
}

func TestClientSubscribeServiceRejectsUnknownMethod(t *testing.T) {
	client, err := NewWithQueue(&stubQueue{})
	if err != nil {
		t.Fatalf("NewWithQueue() failed: %v", err)
	}
	client.subscriptions = []*Subscription{{
		Topic:    "user.missing",
		Endpoint: Endpoint{Service: "UserService", Method: "missing"},
	}}

	err = client.SubscribeService("UserService", nil)
	if !errors.Is(err, ErrUnknownEndpoint) {
		t.Fatalf("expect ErrUnknownEndpoint, got %v", err)
	}
}

func TestClientSubscribeServiceWrapsSubscriptionError(t *testing.T) {
	subscribeErr := errors.New("subscribe failed")
	client, err := NewWithQueue(&stubQueue{subscribeErr: subscribeErr})
	if err != nil {
		t.Fatalf("NewWithQueue() failed: %v", err)
	}
	client.subscriptions = []*Subscription{{
		Topic:    "user.create",
		Endpoint: Endpoint{Service: "UserService", Method: "create_user"},
	}}

	err = client.SubscribeService("UserService", map[string]Handler{
		"create_user": func(context.Context, *Subscription, *SubMessage) error { return nil },
	})
	if !errors.Is(err, subscribeErr) {
		t.Fatalf("expect subscribe error, got %v", err)
	}
	for _, value := range []string{"UserService", "create_user", "user.create"} {
		if !strings.Contains(err.Error(), value) {
			t.Fatalf("expect error %q to contain %q", err, value)
		}
	}
}

func TestJSONEndpoint(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}
	type endpointFunc func(context.Context, any) (any, error)

	endpointErr := errors.New("endpoint failed")
	var received *request
	endpoint := endpointFunc(func(_ context.Context, value any) (any, error) {
		received = value.(*request)
		return nil, endpointErr
	})
	handler := JSONEndpoint[request](endpoint)

	subscription := &Subscription{
		Topic:    "user.create",
		Endpoint: Endpoint{Service: "UserService", Method: "create_user"},
	}
	err := handler(context.Background(), subscription, &SubMessage{
		Message: Message{Data: `{"name":"alice"}`},
	})
	if !errors.Is(err, endpointErr) {
		t.Fatalf("expect endpoint error, got %v", err)
	}
	if received == nil || received.Name != "alice" {
		t.Fatalf("unexpected request: %#v", received)
	}
}

func TestJSONEndpointTerminatesInvalidMessage(t *testing.T) {
	called := false
	handler := JSONEndpoint[struct{}](func(context.Context, any) (any, error) {
		called = true
		return nil, nil
	})
	message := &SubMessage{Message: Message{Data: "{"}}
	var terminated atomic.Bool
	message.SetTerm(func() { terminated.Store(true) })

	err := handler(context.Background(), &Subscription{
		Topic:    "user.create",
		Endpoint: Endpoint{Method: "create_user"},
	}, message)
	if err == nil {
		t.Fatal("expect decode error")
	}
	for _, value := range []string{"user.create", "create_user"} {
		if !strings.Contains(err.Error(), value) {
			t.Fatalf("expect error %q to contain %q", err, value)
		}
	}
	if !terminated.Load() {
		t.Fatal("expect invalid message terminated")
	}
	if called {
		t.Fatal("expect endpoint not called")
	}
}

func TestContextHelpers(t *testing.T) {
	msg := &SubMessage{
		Message: Message{
			Id:         "id-1",
			Attributes: map[string]any{"k": "v"},
		},
	}

	ctx := WithTopic(nil, "topic-1")
	ctx = WithMessage(ctx, msg)
	if topic := GetContextTopic(ctx); topic != "topic-1" {
		t.Fatalf("expect topic-1, got %q", topic)
	}
	if got := GetContextMessage(ctx); got != msg {
		t.Fatalf("expect same message pointer")
	}
	if id := GetContextMessageId(ctx); id != "id-1" {
		t.Fatalf("expect id-1, got %q", id)
	}
	attrs := GetContextMessageAttributes(ctx)
	if attrs["k"] != "v" {
		t.Fatalf("expect attrs[k]=v, got %#v", attrs["k"])
	}
}

func TestSubMessageAckOnlyOnce(t *testing.T) {
	var ack, nak, term, inProgress atomic.Int64
	msg := &SubMessage{}
	msg.SetAck(func() { ack.Add(1) })
	msg.SetNak(func() { nak.Add(1) })
	msg.SetTerm(func() { term.Add(1) })
	msg.SetInProgress(func() { inProgress.Add(1) })

	msg.Ack()
	msg.Nak()
	msg.Term()
	msg.InProgress()

	if ack.Load() != 1 {
		t.Fatalf("expect ack 1, got %d", ack.Load())
	}
	if nak.Load() != 0 {
		t.Fatalf("expect nak 0, got %d", nak.Load())
	}
	if term.Load() != 0 {
		t.Fatalf("expect term 0, got %d", term.Load())
	}
	if inProgress.Load() != 1 {
		t.Fatalf("expect inProgress 1, got %d", inProgress.Load())
	}
	if !msg.Done() {
		t.Fatal("expect message marked done after ack")
	}
}

func loadTestConfig(t *testing.T, filename, body string) {
	t.Helper()

	if err := config.InitDefault(config.WithWatcherDisabled()); err != nil {
		t.Fatalf("InitDefault() failed: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}
	if err := config.LoadPath(filepath.Join(dir, "config")); err != nil {
		t.Fatalf("LoadPath() failed: %v", err)
	}
}
