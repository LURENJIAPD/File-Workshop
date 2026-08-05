package requestid

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const Header = "X-Request-ID"

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

var fallbackCounter atomic.Uint64

type contextKey struct{}

func Resolve(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if validRequestID.MatchString(candidate) {
		return candidate
	}
	return New()
}

func New() string {
	id, err := uuid.NewV7()
	if err == nil {
		return id.String()
	}
	return fmt.Sprintf("request-%d-%d", time.Now().UTC().UnixNano(), fallbackCounter.Add(1))
}

func WithContext(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, contextKey{}, requestID)
}

func FromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(contextKey{}).(string)
	return requestID
}
