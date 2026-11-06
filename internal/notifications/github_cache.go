package notifications

import (
	"context"
	"net/http"
	"sync"
)

type githubReadCacheKey struct{}
type githubReadResult struct {
	data    []byte
	headers http.Header
}
type githubReadCache struct {
	sync.Mutex
	reads map[string]githubReadResult
	bytes int
}

// A cache belongs to one subject retrieval phase. Revalidation always starts a
// fresh phase; identity checks and failed reads are never cached.
func withGitHubReadCache(ctx context.Context) context.Context {
	if _, ok := ctx.Value(githubReadCacheKey{}).(*githubReadCache); ok {
		return ctx
	}
	return freshGitHubReadCache(ctx)
}

func freshGitHubReadCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, githubReadCacheKey{}, &githubReadCache{reads: map[string]githubReadResult{}})
}
