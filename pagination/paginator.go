package pagination

import "errors"

var ErrInvalidPageSize = errors.New("page size must not be negative")

type Paginator interface {
	GetTotalCount() int32
	GetNextPageToken() string
}
