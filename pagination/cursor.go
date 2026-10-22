package pagination

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	jsoniter "github.com/json-iterator/go"
)

var ErrInvalidPageToken = errors.New("invalid page token")

var jsonCodec = jsoniter.ConfigCompatibleWithStandardLibrary

type cursor struct {
	Offset   int    `json:"offset"`
	Position string `json:"position,omitempty"`
	Binding  string `json:"binding"`
}

// CursorCodec creates opaque, URL-safe page tokens using AES-256-GCM.
type CursorCodec struct {
	gcm cipher.AEAD
}

func NewCursorCodec(key []byte) (*CursorCodec, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("pagination key must be 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create pagination cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create pagination cipher mode: %w", err)
	}
	return &CursorCodec{gcm: gcm}, nil
}

// NewCursorCodecFromBase64 accepts a standard Base64 key, with or without padding.
func NewCursorCodecFromBase64(encodedKey string) (*CursorCodec, error) {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(encodedKey)
		if err != nil {
			return nil, fmt.Errorf("decode pagination key: %w", err)
		}
	}
	return NewCursorCodec(key)
}

func (c *CursorCodec) encode(cursor cursor) (string, error) {
	if c == nil || c.gcm == nil || cursor.Offset < 0 {
		return "", ErrInvalidPageToken
	}

	payload, err := jsonCodec.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("marshal page token: %w", err)
	}

	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate page token nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(append(nonce, c.gcm.Seal(nil, nonce, payload, nil)...)), nil
}

func (c *CursorCodec) decode(token, binding string) (cursor, error) {
	if c == nil || c.gcm == nil {
		return cursor{}, ErrInvalidPageToken
	}

	ciphertext, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(ciphertext) < c.gcm.NonceSize() {
		return cursor{}, ErrInvalidPageToken
	}

	nonce, ciphertext := ciphertext[:c.gcm.NonceSize()], ciphertext[c.gcm.NonceSize():]
	payload, err := c.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return cursor{}, ErrInvalidPageToken
	}

	var decoded cursor
	if err := jsonCodec.Unmarshal(payload, &decoded); err != nil || decoded.Offset < 0 || decoded.Binding != binding {
		return cursor{}, ErrInvalidPageToken
	}
	return decoded, nil
}

func (c *CursorCodec) EncodeOffset(offset int, binding string) (string, error) {
	return c.encode(cursor{Offset: offset, Binding: binding})
}

func (c *CursorCodec) DecodeOffset(token, binding string) (int, error) {
	if token == "" {
		return 0, nil
	}

	decoded, err := c.decode(token, binding)
	if err != nil || decoded.Position != "" {
		return 0, ErrInvalidPageToken
	}
	return decoded.Offset, nil
}

func (c *CursorCodec) EncodeKeyset(position, binding string) (string, error) {
	if position == "" {
		return "", ErrInvalidPageToken
	}

	return c.encode(cursor{Position: position, Binding: binding})
}

func (c *CursorCodec) DecodeKeyset(token, binding string) (string, error) {
	if token == "" {
		return "", nil
	}

	decoded, err := c.decode(token, binding)
	if err != nil || decoded.Position == "" || decoded.Offset != 0 {
		return "", ErrInvalidPageToken
	}
	return decoded.Position, nil
}

func binding(namespace string, value any) (string, error) {
	payload, err := jsonCodec.Marshal([]any{namespace, value})
	if err != nil {
		return "", fmt.Errorf("marshal page token binding: %w", err)
	}

	sum := sha256.Sum256(payload)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
