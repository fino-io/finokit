package redis

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/samber/lo"
)

const (
	maxCounter = (1 << 8) - 1

	counterKeyExpiration = 10 * time.Minute
)

var ErrNilClient = errors.New("redis client is nil")

type Generator struct {
	client    goredis.UniversalClient
	serverIDs []int64
	namespace string
	provider  io.Closer
}

func NewWithClient(client goredis.UniversalClient, serverIDs []int64) (*Generator, error) {
	if client == nil {
		return nil, ErrNilClient
	}
	if len(serverIDs) == 0 {
		return nil, ErrEmptyServerIDs
	}

	return &Generator{
		client:    client,
		serverIDs: append([]int64(nil), serverIDs...),
	}, nil
}

func (g *Generator) Close() error {
	if g == nil || g.provider == nil {
		return nil
	}
	return g.provider.Close()
}

func (g *Generator) GenID(ctx context.Context) (int64, error) {
	ids, err := g.GenMultiIDs(ctx, 1)
	if err != nil {
		return 0, fmt.Errorf("failed to generate id: %w", err)
	}
	return ids[0], nil
}

func (g *Generator) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	const maxTimeAddrTimes = 8

	leftNum := int64(counts)
	lastMs := int64(0)
	ids := make([]int64, 0, counts)
	serverID, err := g.pickServerID()
	if err != nil {
		return nil, fmt.Errorf("failed to pick server id: %w", err)
	}

	for idx := int64(0); leftNum > 0 && idx < maxTimeAddrTimes; idx++ {
		ms := lo.Ternary(g.timeMS() > lastMs, g.timeMS(), lastMs)
		if ms <= lastMs {
			ms++
		}

		lastMs = ms
		redisKey := g.counterKey(g.namespace, serverID, ms)

		counter, err := g.incrBy(ctx, redisKey, leftNum)
		if err != nil {
			return nil, err
		}

		var start, end int64

		start = counter - leftNum
		if start == 0 {
			g.expire(ctx, redisKey)
		}

		if start > maxCounter {
			continue
		} else if counter < leftNum {
			return nil, fmt.Errorf("recycling of counting space occurs, ms=%v", ms)
		}

		if counter > maxCounter {
			end = maxCounter + 1
			leftNum = counter - maxCounter - 1
		} else {
			end = counter
			leftNum = 0
		}

		seconds := ms / 1000
		millis := ms % 1000

		if seconds&0xFFFFFFFF != seconds {
			return nil, fmt.Errorf("seconds more than 32 bits, seconds=%v", seconds)
		}

		if serverID&0x3FFF != serverID {
			return nil, fmt.Errorf("server id more than 14 bits, serverID=%v", serverID)
		}

		for i := start; i < end; i++ {
			id := (seconds)<<32 + (millis)<<22 + i<<14 + serverID
			ids = append(ids, id)
		}
	}

	if len(ids) < counts || leftNum != 0 {
		return nil, fmt.Errorf("IDs num not enough, ns=%v, expect=%v, gotten=%v, lastMs=%v", g.namespace, counts, len(ids), lastMs)
	}

	return ids, nil
}

func (g *Generator) incrBy(ctx context.Context, key string, num int64) (cntPos int64, err error) {
	return g.client.IncrBy(ctx, key, num).Result()
}

func (g *Generator) expire(ctx context.Context, key string) {
	_, _ = g.client.Expire(ctx, key, counterKeyExpiration).Result()
}

func (g *Generator) timeMS() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func (g *Generator) counterKey(space string, serverID int64, ms int64) string {
	return fmt.Sprintf("id_generator:%v:%v:%v", space, serverID, ms)
}

func (g *Generator) pickServerID() (int64, error) {
	r, err := rand.Int(rand.Reader, big.NewInt(int64(len(g.serverIDs))))
	if err != nil {
		return 0, err
	}

	return g.serverIDs[r.Int64()], nil
}
