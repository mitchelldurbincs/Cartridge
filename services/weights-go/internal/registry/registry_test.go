package registry

import (
	"context"
	"testing"
	"time"

	"github.com/cartridge/weights/internal/service"
)

func TestMemoryRegistryUpsertAndCurrent(t *testing.T) {
	reg := NewMemoryRegistry(2)
	ctx := context.Background()

	_, err := reg.Current(ctx, "missing")
	if err == nil {
		t.Fatalf("expected error for missing run")
	}

	input := service.PublishInput{
		RunID:       "run-1",
		Step:        42,
		Checksum:    "abc123",
		ArtifactURI: "s3://bucket/model",
		Metadata:    map[string]string{"format": "torch"},
		PublishedAt: time.Unix(1700000000, 0),
	}

	snapshot, err := reg.Upsert(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := reg.Current(ctx, "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Step != snapshot.Step || got.Checksum != snapshot.Checksum {
		t.Fatalf("unexpected snapshot: %#v", got)
	}

	if len(got.Metadata) != 1 || got.Metadata["format"] != "torch" {
		t.Fatalf("metadata not preserved: %#v", got.Metadata)
	}
}

func TestMemoryRegistrySubscribe(t *testing.T) {
	reg := NewMemoryRegistry(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, stop := reg.Subscribe("run-1", service.WatchOptions{ReplayLatest: true})
	defer stop()

	select {
	case <-time.After(50 * time.Millisecond):
	case <-ch:
		t.Fatalf("did not expect replay without snapshot")
	}

	go func() {
		reg.Upsert(ctx, service.PublishInput{
			RunID:       "run-1",
			Step:        1,
			Checksum:    "one",
			ArtifactURI: "uri://one",
			PublishedAt: time.Now(),
		})
	}()

	select {
	case snap := <-ch:
		if snap.Step != 1 {
			t.Fatalf("expected step 1, got %d", snap.Step)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for update")
	}
}
