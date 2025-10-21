package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/cartridge/web/internal/orchestrator"
)

type mockClient struct {
	list func(context.Context) ([]orchestrator.Run, error)
	get  func(context.Context, string) (orchestrator.Run, error)
}

func (m mockClient) ListRuns(ctx context.Context) ([]orchestrator.Run, error) {
	if m.list != nil {
		return m.list(ctx)
	}
	return nil, nil
}

func (m mockClient) GetRun(ctx context.Context, id string) (orchestrator.Run, error) {
	if m.get != nil {
		return m.get(ctx, id)
	}
	return orchestrator.Run{}, nil
}

func newHandler(t *testing.T, client orchestrator.Client) Handler {
	t.Helper()
	logger := zerolog.New(io.Discard)
	return Handler{client: client, logger: logger, startedAt: time.Now().Add(-time.Minute), level: zerolog.InfoLevel, service: "web-go"}
}

func callHandler(t *testing.T, handler http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}
