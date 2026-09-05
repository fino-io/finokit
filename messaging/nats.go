package messaging

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/nats-io/nats.go"

	"github.com/fino-io/finokit/logs"
)

const defaultFetchWait = time.Second

var (
	ErrEmptyURL            = errors.New("messaging nats: url is empty")
	ErrNilStream           = errors.New("messaging nats: stream is nil")
	ErrEmptyStreamName     = errors.New("messaging nats: stream name is empty")
	ErrEmptyStreamSubjects = errors.New("messaging nats: stream subjects are empty")
)

type natsQueue struct {
	conn          *nats.Conn
	js            nats.JetStreamContext
	subscriptions []*nats.Subscription
	shutdownCh    chan struct{}
	shutdownOnce  sync.Once
	subMu         sync.Mutex
}

type configurableSubscription interface {
	SetPendingLimits(int, int) error
	Unsubscribe() error
}

func newNATSQueue(cfg *Config) (*natsQueue, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	nc, err := nats.Connect(normalized.URL)
	if err != nil {
		logs.Errorw("failed to connect nats", "error", err)
		return nil, err
	}

	n := &natsQueue{
		conn:       nc,
		shutdownCh: make(chan struct{}),
	}

	if normalized.JetStream {
		js, err := nc.JetStream()
		if err != nil {
			nc.Close()
			return nil, err
		}
		n.js = js
		if err := ensureStreams(js, normalized.Streams); err != nil {
			nc.Close()
			return nil, err
		}
	}

	return n, nil
}

func ensureStreams(js nats.JetStreamContext, streams []*Stream) error {
	for _, stream := range streams {
		if stream == nil {
			return ErrNilStream
		}

		name := strings.TrimSpace(stream.Name)
		if name == "" {
			return ErrEmptyStreamName
		}

		subjects := make([]string, 0, len(stream.Subjects))
		for _, subject := range stream.Subjects {
			if subject = strings.TrimSpace(subject); subject != "" {
				subjects = append(subjects, subject)
			}
		}
		if len(subjects) == 0 {
			return ErrEmptyStreamSubjects
		}

		if err := ensureStream(js, name, subjects); err != nil {
			return fmt.Errorf("ensure stream %q: %w", name, err)
		}
	}
	return nil
}

func ensureStream(js nats.JetStreamContext, name string, subjects []string) error {
	info, err := js.AddStream(&nats.StreamConfig{Name: name, Subjects: subjects})
	if err == nil {
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return err
	}

	info, err = js.StreamInfo(name)
	if err != nil {
		return err
	}
	if slices.Equal(info.Config.Subjects, subjects) {
		return nil
	}

	config := info.Config
	config.Subjects = subjects
	_, err = js.UpdateStream(&config)
	return err
}

func (n *natsQueue) Publish(ctx context.Context, topic string, messages ...*Message) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	for _, message := range messages {
		if message == nil {
			return ErrNilMessage
		}
		publishCtx, span := startProducer(ctx, topic)
		bytes, err := jsoniter.Marshal(message)
		if err != nil {
			endSpan(span, err)
			logs.Errorw("marshal message error", "message", message, "error", err)
			return err
		}

		err = n.publish(publishCtx, topic, bytes, message.Id)
		endSpan(span, err)
		if err != nil {
			return logs.NewErrorw("publish message error", "topic", topic, "error", err)
		}
	}

	return nil
}

func (n *natsQueue) publish(ctx context.Context, topic string, payload []byte, messageID string) error {
	message := &nats.Msg{Subject: topic, Data: payload, Header: nats.Header{}}
	injectContext(ctx, message.Header)
	if n.js == nil {
		return n.conn.PublishMsg(message)
	}

	var options []nats.PubOpt
	if messageID != "" {
		options = append(options, nats.MsgId(messageID))
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		options = append(options, nats.Context(ctx))
	}
	_, err := n.js.PublishMsg(message, options...)
	return err
}

func (n *natsQueue) Subscribe(s *Subscription, h Handler) error {
	if h == nil {
		return ErrNilHandler
	}
	if err := s.Validate(); err != nil {
		return err
	}

	callback := func(raw *nats.Msg) {
		ctx := extractContext(context.Background(), raw.Header)
		ctx, span := startConsumer(ctx, s.Topic)
		defer span.End()

		msg := &SubMessage{}
		if err := jsoniter.Unmarshal(raw.Data, msg); err != nil {
			recordSpanError(span, err)
			logs.Warnw("Nats: failed to unmarshal data form topic", "topic", s.Topic, "error", err)
			if n.js != nil {
				if termErr := raw.Term(); termErr != nil {
					logs.Warnw("Nats: failed to term invalid message", "topic", s.Topic, "error", termErr)
				}
			}
			return
		}

		if n.js != nil {
			msg.SetAck(func() {
				if err := raw.Ack(); err != nil {
					logs.Warnw("Nats: failed to ack", "topic", s.Topic, "error", err)
				}
			})
			msg.SetNak(func() {
				if err := raw.Nak(); err != nil {
					logs.Warnw("Nats: failed to nak", "topic", s.Topic, "error", err)
				}
			})
			msg.SetTerm(func() {
				if err := raw.Term(); err != nil {
					logs.Warnw("Nats: failed to term", "topic", s.Topic, "error", err)
				}
			})
			msg.SetInProgress(func() {
				if err := raw.InProgress(); err != nil {
					logs.Warnw("Nats: failed to inProgress", "topic", s.Topic, "error", err)
				}
			})
		}

		ctx = WithTopic(ctx, s.Topic)
		ctx = WithMessage(ctx, msg)
		err := h(ctx, s, msg)
		if err != nil {
			recordSpanError(span, err)
		}
		completeMessage(msg, err, n.js != nil)
	}

	var (
		sub *nats.Subscription
		err error
	)
	if len(s.Group) > 0 {
		sub, err = n.queueSubscribe(s, callback)
	} else {
		sub, err = n.subscribe(s, callback)
	}
	if err != nil {
		return err
	}

	n.addSubscription(sub)
	return nil
}

func completeMessage(message *SubMessage, handlerErr error, acknowledge bool) {
	if message == nil || message.Done() || !acknowledge {
		return
	}
	if handlerErr != nil {
		message.Nak()
		return
	}
	message.Ack()
}

func (n *natsQueue) addSubscription(sub *nats.Subscription) {
	if n == nil || sub == nil {
		return
	}
	n.subMu.Lock()
	n.subscriptions = append(n.subscriptions, sub)
	n.subMu.Unlock()
}

func (n *natsQueue) queueSubscribe(s *Subscription, cb nats.MsgHandler) (*nats.Subscription, error) {
	var (
		sub *nats.Subscription
		err error
	)
	if n.js != nil {
		if s.Pull {
			sub, err = n.pullSubscribe(s, cb)
		} else {
			sub, err = n.js.QueueSubscribe(s.Topic, s.Group, cb)
		}
	} else {
		sub, err = n.conn.QueueSubscribe(s.Topic, s.Group, cb)
	}
	if err != nil {
		return nil, err
	}

	if err := configureSubscription(sub, s); err != nil {
		return nil, err
	}
	return sub, nil
}

func (n *natsQueue) subscribe(s *Subscription, cb nats.MsgHandler) (*nats.Subscription, error) {
	var (
		sub *nats.Subscription
		err error
	)
	if n.js != nil {
		sub, err = n.js.Subscribe(s.Topic, cb)
	} else {
		sub, err = n.conn.Subscribe(s.Topic, cb)
	}
	if err != nil {
		return nil, err
	}

	if err := configureSubscription(sub, s); err != nil {
		return nil, err
	}
	return sub, nil
}

func (n *natsQueue) pullSubscribe(s *Subscription, cb nats.MsgHandler) (*nats.Subscription, error) {
	durableName := strings.Join([]string{s.Group, s.Name}, "-")
	if durableName == "-" || durableName == "" {
		durableName = strings.ReplaceAll(s.Topic, ".", "-")
	}

	subOpts := []nats.SubOpt{}
	if s.PullMaxWaiting > 0 {
		subOpts = append(subOpts, nats.PullMaxWaiting(s.PullMaxWaiting))
	}
	if s.AckWait > 0 {
		subOpts = append(subOpts, nats.AckWait(s.AckWait))
	}

	sub, err := n.js.PullSubscribe(s.Topic, durableName, subOpts...)
	if err != nil {
		logs.Warnw("failed to pull subscribe the topic", "topic", s.Topic, "error", err)
		return nil, err
	}

	go func() {
		for {
			select {
			case <-n.shutdownCh:
				return
			default:
			}

			ms, err := sub.Fetch(1, nats.MaxWait(defaultFetchWait))
			if err != nil {
				if errors.Is(err, nats.ErrTimeout) {
					continue
				}
				if errors.Is(err, nats.ErrConnectionClosed) || errors.Is(err, nats.ErrBadSubscription) {
					return
				}
				logs.Warnw("failed to fetch the next message", "subscribe", durableName, "error", err)
				continue
			}

			for _, msg := range ms {
				cb(msg)
			}
		}
	}()

	return sub, nil
}

func configureSubscription(sub configurableSubscription, s *Subscription) error {
	if sub == nil {
		return nil
	}
	if err := setPendingLimit(sub, s); err != nil {
		return errors.Join(err, sub.Unsubscribe())
	}
	return nil
}

func setPendingLimit(sub configurableSubscription, s *Subscription) error {
	msgLimit := s.PendingMsgLimit
	bytesLimit := s.PendingBytesLimit
	if msgLimit != 0 || bytesLimit != 0 {
		if msgLimit == 0 {
			msgLimit = nats.DefaultSubPendingMsgsLimit
		}
		if bytesLimit == 0 {
			bytesLimit = nats.DefaultSubPendingBytesLimit
		}
		if err := sub.SetPendingLimits(msgLimit, bytesLimit); err != nil {
			logs.Warnw("failed to set pending limits", "name", s.Name, "topic", s.Topic,
				"msgLimit", msgLimit, "bytesLimit", bytesLimit, "error", err)
			return err
		}
	}
	return nil
}

func (n *natsQueue) Shutdown() {
	if n == nil {
		return
	}

	n.shutdownOnce.Do(func() {
		if n.shutdownCh != nil {
			close(n.shutdownCh)
		}

		n.subMu.Lock()
		for _, sub := range n.subscriptions {
			if sub != nil {
				if err := sub.Unsubscribe(); err != nil {
					logs.Warnw("failed to unsubscribe", "subject", sub.Subject, "error", err)
				}
			}
		}
		n.subscriptions = nil
		n.subMu.Unlock()

		if n.conn != nil {
			n.conn.Close()
		}
	})
}
