package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cartridge/web/internal/orchestrator"
)

func TestListRunsHandler(t *testing.T) {
	run := orchestrator.Run{ID: "run-123", Status: "running", Owner: "qa", CreatedAt: time.Now()}
	cases := []struct {
		name   string
		client orchestrator.Client
		want   int
		check  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "ok",
			client: mockClient{list: func(context.Context) ([]orchestrator.Run, error) {
				return []orchestrator.Run{run}, nil
			}},
			want: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var payload struct {
					Runs []orchestrator.Run `json:"runs"`
				}
				if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if len(payload.Runs) != 1 || payload.Runs[0].ID != run.ID {
					t.Fatalf("unexpected runs payload: %#v", payload.Runs)
				}
			},
		},
		{
			name: "backend error",
			client: mockClient{list: func(context.Context) ([]orchestrator.Run, error) {
				return nil, errors.New("boom")
			}},
			want:  http.StatusBadGateway,
			check: func(*testing.T, *httptest.ResponseRecorder) {},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHandler(t, tc.client)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
			rec := callHandler(t, h.instrument("runs.list", h.listRuns), req)
			if rec.Code != tc.want {
				t.Fatalf("expected status %d, got %d", tc.want, rec.Code)
			}
			tc.check(t, rec)
		})
	}
}
