package pagination

const (
	DefaultPageSize = 50
	MaxPageSize     = 1000
)

func normalizePageSize(pageSize int) (int, error) {
	if pageSize < 0 {
		return 0, ErrInvalidPageSize
	}

	if pageSize == 0 {
		return DefaultPageSize, nil
	}

	if pageSize > MaxPageSize {
		return MaxPageSize, nil
	}
	return pageSize, nil
}

// Resolve validates a list request and returns its cursor offset, normalized
// page size, and binding for creating a subsequent page token.
func Resolve(codec *CursorCodec, token string, pageSize int, namespace string, scope any) (int, int, string, error) {
	binding, err := binding(namespace, scope)
	if err != nil {
		return 0, 0, "", err
	}

	offset, err := codec.DecodeOffset(token, binding)
	if err != nil {
		return 0, 0, "", err
	}

	size, err := normalizePageSize(pageSize)
	if err != nil {
		return 0, 0, "", err
	}

	return offset, size, binding, nil
}

// ResolveKeyset validates a list request and returns the last-seen key, normalized
// page size, and binding for creating a subsequent page token.
func ResolveKeyset(codec *CursorCodec, token string, pageSize int, namespace string, scope any) (string, int, string, error) {
	binding, err := binding(namespace, scope)
	if err != nil {
		return "", 0, "", err
	}

	position, err := codec.DecodeKeyset(token, binding)
	if err != nil {
		return "", 0, "", err
	}

	size, err := normalizePageSize(pageSize)
	if err != nil {
		return "", 0, "", err
	}

	return position, size, binding, nil
}

// Page is a standard list result. It satisfies Paginator so transports can
// consistently expose pagination metadata without knowing the concrete item type.
type Page[T any] struct {
	Items         []T
	TotalCount    int32
	NextPageToken string
}

func (p Page[T]) GetTotalCount() int32     { return p.TotalCount }
func (p Page[T]) GetNextPageToken() string { return p.NextPageToken }
