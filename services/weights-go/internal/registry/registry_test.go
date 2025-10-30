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

func TestMemoryRegistryRaceCancelDuringAsyncSend(t *testing.T) {
	// This test verifies that canceling a subscription while an async send
	// goroutine is pending does not panic the service. The race occurs when:
	// 1. Upsert spawns an async send goroutine (because channel buffer is full)
	// 2. Subscriber calls cancel(), which closes the channel
	// 3. The async goroutine tries to send to the closed channel
	// Without recovery, this would panic and crash the service.

	reg := NewMemoryRegistry(1)
	ctx := context.Background()

	// Subscribe - channel has buffer size 1
	ch, cancel := reg.Subscribe("run-1", service.WatchOptions{ReplayLatest: false})

	// Send first update - fills the channel buffer
	_, err := reg.Upsert(ctx, service.PublishInput{
		RunID:       "run-1",
		Step:        1,
		Checksum:    "first",
		ArtifactURI: "uri://first",
		PublishedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	// Verify channel is full
	select {
	case snap := <-ch:
		if snap.Step != 1 {
			t.Fatalf("expected step 1, got %d", snap.Step)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel should have received first update")
	}

	// Send second update - fills buffer again
	_, err = reg.Upsert(ctx, service.PublishInput{
		RunID:       "run-1",
		Step:        2,
		Checksum:    "second",
		ArtifactURI: "uri://second",
		PublishedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	// Now the channel buffer is full. The next upsert will trigger async send.
	// Cancel immediately to close the channel before async goroutine runs.
	cancel()

	// Third upsert - channel is full, so it spawns async goroutine
	// The async goroutine will try to send to the now-closed channel
	_, err = reg.Upsert(ctx, service.PublishInput{
		RunID:       "run-1",
		Step:        3,
		Checksum:    "third",
		ArtifactURI: "uri://third",
		PublishedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("third upsert failed: %v", err)
	}

	// Give async goroutine time to attempt send to closed channel
	time.Sleep(100 * time.Millisecond)

	// If we reach here without panicking, the fix works correctly
}
