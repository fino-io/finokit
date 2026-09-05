package messaging

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

var errConfigureSubscription = errors.New("configure subscription")

type failingSubscription struct {
	unsubscribed atomic.Bool
}

func (*failingSubscription) SetPendingLimits(int, int) error {
	return errConfigureSubscription
}

func (s *failingSubscription) Unsubscribe() error {
	s.unsubscribed.Store(true)
	return nil
}

func TestCompleteMessageUsesHandlerResultAsAcknowledgementPolicy(t *testing.T) {
	t.Run("core nats does not acknowledge", func(t *testing.T) {
		var acked atomic.Bool
		message := &SubMessage{}
		message.SetAck(func() { acked.Store(true) })

		completeMessage(message, nil, false)

		if acked.Load() {
			t.Fatal("expected core NATS message not to be acknowledged")
		}
	})

	t.Run("success acknowledges", func(t *testing.T) {
		var acked atomic.Bool
		message := &SubMessage{}
		message.SetAck(func() { acked.Store(true) })

		completeMessage(message, nil, true)

		if !acked.Load() {
			t.Fatal("expected successful handler to acknowledge message")
		}
	})

	t.Run("failure negatively acknowledges", func(t *testing.T) {
		var naked atomic.Bool
		message := &SubMessage{}
		message.SetNak(func() { naked.Store(true) })

		completeMessage(message, errors.New("handler failed"), true)

		if !naked.Load() {
			t.Fatal("expected failed handler to negatively acknowledge message")
		}
	})

	t.Run("explicit terminal action wins", func(t *testing.T) {
		var acked atomic.Bool
		message := &SubMessage{}
		message.SetTerm(func() {})
		message.SetAck(func() { acked.Store(true) })
		message.Term()

		completeMessage(message, nil, true)

		if acked.Load() {
			t.Fatal("expected completed message not to be acknowledged again")
		}
	})
}

func TestSubscriptionCarriesNATSConsumerSettings(t *testing.T) {
	subscription := &Subscription{
		Topic:             "demo.start-task",
		Pull:              true,
		AckWait:           5 * time.Minute,
		PullMaxWaiting:    8,
		PendingMsgLimit:   64,
		PendingBytesLimit: 1024,
	}

	if !subscription.Pull || subscription.AckWait != 5*time.Minute {
		t.Fatalf("unexpected pull subscription: %#v", subscription)
	}
	if subscription.PullMaxWaiting != 8 || subscription.PendingMsgLimit != 64 || subscription.PendingBytesLimit != 1024 {
		t.Fatalf("unexpected consumer limits: %#v", subscription)
	}
}

func TestConfigureSubscriptionCleansUpAfterPendingLimitFailure(t *testing.T) {
	subscription := &failingSubscription{}

	err := configureSubscription(subscription, &Subscription{PendingMsgLimit: 1})

	if !errors.Is(err, errConfigureSubscription) {
		t.Fatalf("expected pending limit error, got %v", err)
	}
	if !subscription.unsubscribed.Load() {
		t.Fatal("expected failed subscription to be unsubscribed")
	}
}

func TestConfigNormalized(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		var cfg *Config
		_, err := normalizeConfig(cfg)
		if !errors.Is(err, ErrNilConfig) {
			t.Fatalf("expect ErrNilConfig, got %v", err)
		}
	})

	t.Run("empty url", func(t *testing.T) {
		_, err := normalizeConfig(&Config{})
		if !errors.Is(err, ErrEmptyURL) {
			t.Fatalf("expect ErrEmptyURL, got %v", err)
		}
	})

	t.Run("trim url", func(t *testing.T) {
		cfg, err := normalizeConfig(&Config{URL: " nats://127.0.0.1:4222 "})
		if err != nil {
			t.Fatalf("normalized() failed: %v", err)
		}
		if cfg.URL != "nats://127.0.0.1:4222" {
			t.Fatalf("expect trimmed url, got %q", cfg.URL)
		}
	})
}

func TestNATSNewValidation(t *testing.T) {
	t.Run("new with config validate args", func(t *testing.T) {
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
}
