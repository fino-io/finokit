//go:build local
// +build local

package nats

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	gonats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"

	"github.com/fino-io/finokit/config"
	"github.com/fino-io/finokit/logs"
	"github.com/fino-io/finokit/messaging"
)

func Test_PublishStartTask(t *testing.T) {
	traceID := uuid.New().String()
	logs.Infow("publish start task", "traceId", traceID)
	err := PublishStartTask(context.Background(), &startTaskRequest{Name: "task-" + traceID, Ids: []string{"1", "2"}})
	assert.NoError(t, err)
}

func Test_PublishStopTask(t *testing.T) {
	traceID := uuid.New().String()
	logs.Infow("publish stop task", "traceId", traceID)
	err := PublishStopTask(context.Background(), &stopTaskRequest{Name: "task-" + traceID})
	assert.NoError(t, err)
}

func Test_Subscription(t *testing.T) {
	cfg := mustLoadLocalConfig(t)
	client := mustGetMessaging(t)

	for _, spec := range cfg.Subscriptions {
		spec := spec
		if spec.Endpoint.Service != "" && spec.Endpoint.Service != "Agent" {
			continue
		}

		if err := client.Subscribe(spec, func(ctx context.Context, s *messaging.Subscription, m *messaging.SubMessage) error {
			switch spec.Endpoint.Method {
			case "start_task":
				request := &startTaskRequest{}
				if err := jsoniter.ConfigFastest.UnmarshalFromString(m.Data, request); err != nil {
					m.Term()
					return err
				}
				if err := startTask(ctx, request); err != nil {
					m.Nak()
					return err
				}
			case "stop_tasks":
				request := &stopTaskRequest{}
				if err := jsoniter.ConfigFastest.UnmarshalFromString(m.Data, request); err != nil {
					m.Term()
					return err
				}
				if err := stopTask(ctx, request); err != nil {
					m.Nak()
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("subscribe %s failed: %v", spec.Topic, err)
		}
	}

	select {}
}

type startTaskRequest struct {
	Name string
	Ids  []string
}

func startTask(ctx context.Context, request *startTaskRequest) error {
	logs.Infow("starting task", "name", request.Name, "topic", messaging.GetContextTopic(ctx))
	return nil
}

type stopTaskRequest struct {
	Name string
}

func stopTask(ctx context.Context, request *stopTaskRequest) error {
	logs.Infow("stopping task", "name", request.Name, "topic", messaging.GetContextTopic(ctx))
	return nil
}

const (
	StartTaskTopic = "demo.start-task"
	StopTasksTopic = "demo.stop-tasks"
)

var (
	messageClient *messaging.Client
	natsClient    *Nats
	clientOnce    sync.Once
)

func mustLoadLocalConfig(t *testing.T) *messaging.Config {
	t.Helper()

	cfg := &messaging.Config{}
	if err := config.ScanFrom(cfg, "messaging"); err != nil {
		t.Fatalf("failed to load messaging config: %v", err)
	}
	return cfg
}

func initLocalMessaging() {
	clientOnce.Do(func() {
		cfg := &messaging.Config{}
		if err := config.ScanFrom(cfg, "messaging"); err != nil {
			panic(err)
		}

		queue, err := NewWithConfig(&cfg.Nats)
		if err != nil {
			panic(err)
		}

		wrapped, err := messaging.NewWithQueue(queue)
		if err != nil {
			panic(err)
		}
		messageClient = wrapped
		natsClient = queue
	})
}

func mustGetMessaging(t *testing.T) *Nats {
	t.Helper()

	initLocalMessaging()
	return natsClient
}

func PublishStartTask(ctx context.Context, task *startTaskRequest) error {
	initLocalMessaging()
	data, err := jsoniter.ConfigFastest.MarshalToString(task)
	if err != nil {
		return err
	}

	return messageClient.Publish(ctx, StartTaskTopic, &messaging.Message{Id: "1", Data: data})
}

func PublishStopTask(ctx context.Context, task *stopTaskRequest) error {
	initLocalMessaging()
	data, err := jsoniter.ConfigFastest.MarshalToString(task)
	if err != nil {
		return err
	}

	return messageClient.Publish(ctx, StopTasksTopic, &messaging.Message{Id: "2", Data: data})
}

func Test_ClearStream(t *testing.T) {
	cfg := mustLoadLocalConfig(t)

	nc, err := gonats.Connect(cfg.Nats.URL)
	if err != nil {
		t.Fatalf("failed to connect nats: %v", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("failed to get jetstream: %v", err)
	}

	streamNames := js.StreamNames()
	for name := range streamNames {
		logs.Infow("stream", "name", name)
	}
}

func TestNatsSubscribeValidation(t *testing.T) {
	queue := &Nats{}

	if err := queue.Subscribe(nil, func(context.Context, *messaging.Subscription, *messaging.SubMessage) error { return nil }); !errors.Is(err, messaging.ErrNilSubscription) {
		t.Fatalf("expect ErrNilSubscription, got %v", err)
	}

	if err := queue.Subscribe(&messaging.Subscription{}, func(context.Context, *messaging.Subscription, *messaging.SubMessage) error { return nil }); !errors.Is(err, messaging.ErrEmptyTopic) {
		t.Fatalf("expect ErrEmptyTopic, got %v", err)
	}

	if err := queue.Subscribe(&messaging.Subscription{Topic: "topic"}, nil); !errors.Is(err, messaging.ErrNilHandler) {
		t.Fatalf("expect ErrNilHandler, got %v", err)
	}
}
