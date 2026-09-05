//go:build local

package messaging

import (
	"context"
	"fmt"
	"testing"
	"time"

	gonats "github.com/nats-io/nats.go"
)

func TestConfiguredSubscriptionRoundTrip(t *testing.T) {
	topic := fmt.Sprintf("fino2.messaging.%d", time.Now().UnixNano())
	config := &Config{
		URL: "nats://127.0.0.1:4222",
		Subscriptions: []*Subscription{{
			Name:  "fino2-test",
			Topic: topic,
			Endpoint: Endpoint{
				Service: "Agent",
				Method:  "start_task",
			},
		}},
	}

	client, err := NewWithConfig(config)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer client.Shutdown()

	subscriptions := client.Subscriptions()
	if len(subscriptions) != 1 || subscriptions[0].Endpoint.Method != "start_task" {
		t.Fatalf("unexpected subscriptions: %#v", subscriptions)
	}

	received := make(chan *SubMessage, 1)
	if err := client.Subscribe(subscriptions[0], func(ctx context.Context, _ *Subscription, message *SubMessage) error {
		if got := GetContextTopic(ctx); got != topic {
			t.Errorf("context topic = %q, want %q", got, topic)
		}
		if got := GetContextMessageId(ctx); got != "message-1" {
			t.Errorf("context message id = %q, want message-1", got)
		}
		received <- message
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err := client.Publish(context.Background(), topic, &Message{Id: "message-1", Data: `{"task":"demo"}`}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case message := <-received:
		if message.Data != `{"task":"demo"}` {
			t.Fatalf("message data = %q", message.Data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for NATS message")
	}
}

func TestConfiguredJetStreamCreatesStreamAndRoundTrips(t *testing.T) {
	suffix := time.Now().UnixNano()
	stream := fmt.Sprintf("FINO2_%d", suffix)
	topic := fmt.Sprintf("fino2.jetstream.%d", suffix)
	config := &Config{
		URL:       "nats://127.0.0.1:4222",
		JetStream: true,
		Streams: []*Stream{{
			Name:     stream,
			Subjects: []string{topic},
		}},
		Subscriptions: []*Subscription{{
			Name:  "fino2-jetstream-test",
			Topic: topic,
			Group: "fino2-jetstream-test",
			Pull:  true,
		}},
	}

	client, err := NewWithConfig(config)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer client.Shutdown()

	t.Cleanup(func() {
		nc, connectErr := gonats.Connect(config.URL)
		if connectErr != nil {
			return
		}
		defer nc.Close()
		js, jsErr := nc.JetStream()
		if jsErr == nil {
			_ = js.DeleteStream(stream)
		}
	})

	second, err := NewWithConfig(config)
	if err != nil {
		t.Fatalf("create second client with existing stream: %v", err)
	}
	second.Shutdown()

	received := make(chan *SubMessage, 1)
	if err := client.Subscribe(config.Subscriptions[0], func(_ context.Context, _ *Subscription, message *SubMessage) error {
		received <- message
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err := client.Publish(context.Background(), topic, &Message{Id: "jetstream-1", Data: `{}`}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case message := <-received:
		if message.Id != "jetstream-1" {
			t.Fatalf("message id = %q", message.Id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for JetStream message")
	}
}
