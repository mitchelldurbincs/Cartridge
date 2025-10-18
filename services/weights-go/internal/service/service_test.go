package service

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

type stubRegistry struct {
	upsertCalled bool
	upsertInput  PublishInput
	upsertErr    error
	snapshot     VersionSnapshot
	subscribeCh  chan VersionSnapshot
	cancelFn     func()
}

func (r *stubRegistry) Upsert(_ context.Context, input PublishInput) (VersionSnapshot, error) {
	r.upsertCalled = true
	r.upsertInput = input
	if r.upsertErr != nil {
		return VersionSnapshot{}, r.upsertErr
	}
	if r.snapshot.RunID == "" {
		r.snapshot = VersionSnapshot{
			RunID:       input.RunID,
			Step:        input.Step,
			Checksum:    input.Checksum,
			ArtifactURI: input.ArtifactURI,
			Metadata:    input.Metadata,
			PublishedAt: input.PublishedAt,
		}
	}
	return r.snapshot, nil
}

func (r *stubRegistry) Current(context.Context, string) (VersionSnapshot, error) {
	if r.snapshot.RunID == "" {
		return VersionSnapshot{}, ErrRunNotFound
	}
	return r.snapshot, nil
}

func (r *stubRegistry) Subscribe(string, WatchOptions) (<-chan VersionSnapshot, func()) {
	if r.subscribeCh == nil {
		r.subscribeCh = make(chan VersionSnapshot)
	}
	if r.cancelFn == nil {
		r.cancelFn = func() { close(r.subscribeCh) }
	}
	return r.subscribeCh, r.cancelFn
}

type stubPublisher struct {
	called    bool
	lastRunID string
}

func (p *stubPublisher) Publish(context.Context, VersionSnapshot) error {
	p.called = true
	return nil
}

type stubMetrics struct {
	publishCalls int
	lastErr      error
	subscribed   int
	cancelled    int
}

func (m *stubMetrics) PublishComplete(_ time.Duration, err error) {
	m.publishCalls++
	m.lastErr = err
}

func (m *stubMetrics) StreamSubscribed() {
	m.subscribed++
}

func (m *stubMetrics) StreamCancelled() {
	m.cancelled++
}

func TestPublishInvokesPublisherAndMetrics(t *testing.T) {
	reg := &stubRegistry{}
	pub := &stubPublisher{}
	metrics := &stubMetrics{}
	logger := zerolog.New(io.Discard)
	svc := New(reg, logger, WithPublisher(pub), WithMetrics(metrics))

	snapshot, err := svc.Publish(context.Background(), PublishInput{
		RunID:       "run-1",
		Step:        42,
		Checksum:    "abc",
		ArtifactURI: "s3://bucket/object",
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if !reg.upsertCalled {
		t.Fatal("expected registry.Upsert to be invoked")
	}
	if !pub.called {
		t.Fatal("expected compatibility publisher to be invoked")
	}
	if metrics.publishCalls != 1 || metrics.lastErr != nil {
		t.Fatalf("expected metrics to record successful publish, got calls=%d err=%v", metrics.publishCalls, metrics.lastErr)
	}
	if snapshot.RunID != "run-1" {
		t.Fatalf("unexpected snapshot returned: %+v", snapshot)
	}
}

func TestPublishRecordsFailureMetrics(t *testing.T) {
	reg := &stubRegistry{}
	metrics := &stubMetrics{}
	logger := zerolog.New(io.Discard)
	svc := New(reg, logger, WithMetrics(metrics))

	_, err := svc.Publish(context.Background(), PublishInput{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if reg.upsertCalled {
		t.Fatal("registry.Upsert should not be called on invalid input")
	}
	if metrics.publishCalls != 1 || metrics.lastErr == nil {
		t.Fatalf("expected metrics to record failure, got calls=%d err=%v", metrics.publishCalls, metrics.lastErr)
	}
}

func TestStreamMetricsLifecycle(t *testing.T) {
	ch := make(chan VersionSnapshot)
	cancel := func() { close(ch) }
	reg := &stubRegistry{subscribeCh: ch, cancelFn: cancel}
	metrics := &stubMetrics{}
	logger := zerolog.New(io.Discard)
	svc := New(reg, logger, WithMetrics(metrics))

	_, stop := svc.Stream("run-2", WatchOptions{})
	if metrics.subscribed != 1 {
		t.Fatalf("expected subscriber count to increment, got %d", metrics.subscribed)
	}
	stop()
	if metrics.cancelled != 1 {
		t.Fatalf("expected subscriber cancellation to be recorded, got %d", metrics.cancelled)
	}
}
