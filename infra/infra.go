package main

import (
	"context"
	"errors"
	"sync"

	"github.com/fino-io/finokit/db"
	"github.com/fino-io/finokit/idgen"
	idredis "github.com/fino-io/finokit/idgen/redis"
	"github.com/fino-io/finokit/messaging"
	"github.com/fino-io/finokit/messaging/nats"
	"github.com/fino-io/finokit/redis"
	"github.com/fino-io/finokit/storage"
	"github.com/fino-io/finokit/storage/minio"
	"github.com/fino-io/finokit/storage/s3"
)

var registerInfraDriversOnce sync.Once

type infra struct {
	OSS       storage.Storage
	IDGen     idgen.IDGenerator
	DB        db.Provider
	Redis     redis.Provider
	Messaging messaging.Provider
}

func registerInfraDrivers() {
	registerInfraDriversOnce.Do(func() {
		minio.Register()
		s3.Register()
		nats.Register()
	})
}

func buildInfra() (infra, error) {
	registerInfraDrivers()

	var deps infra
	fail := func(err error) (infra, error) {
		_ = closeInfra(context.Background(), deps)
		return infra{}, err
	}

	dbProvider, err := db.New()
	if err != nil {
		return fail(err)
	}
	deps.DB = dbProvider

	redisProvider, err := redis.New()
	if err != nil {
		return fail(err)
	}
	deps.Redis = redisProvider

	idGenerator, err := idredis.NewWithProvider(nil, redisProvider)
	if err != nil {
		return fail(err)
	}
	deps.IDGen = idGenerator

	oss, err := storage.New()
	if err != nil {
		return fail(err)
	}
	deps.OSS = oss

	messagingProvider, err := messaging.New()
	if err != nil {
		return fail(err)
	}
	deps.Messaging = messagingProvider

	return deps, nil
}

func (i infra) Close(ctx context.Context) error {
	return closeInfra(ctx, i)
}

func closeInfra(ctx context.Context, deps infra) error {
	var errs []error

	if deps.Messaging != nil {
		deps.Messaging.Shutdown()
	}

	if closer, ok := deps.IDGen.(interface{ Close() error }); ok {
		errs = append(errs, closer.Close())
	}

	if deps.Redis != nil {
		errs = append(errs, deps.Redis.Close(ctx))
	}

	return errors.Join(errs...)
}
