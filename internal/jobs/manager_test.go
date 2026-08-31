package jobs

import (
	"context"
	"testing"
)

func TestManagerCancellationAndSnapshots(t *testing.T) {
	manager := NewManager()
	handle, ctx, err := manager.Start(context.Background(), PhaseScanning, 10)
	if err != nil {
		t.Fatal(err)
	}
	handle.Update(3, "照片.jpg")
	handle.Cancel()
	if ctx.Err() == nil {
		t.Fatal("expected cancelled context")
	}
	snapshot, ok := manager.Snapshot(handle.Snapshot().ID)
	if !ok || snapshot.Status != StatusCancelled || snapshot.Processed != 3 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if _, _, err := manager.Start(context.Background(), PhasePlanning, 1); err != nil {
		t.Fatalf("finished job should not block a new job: %v", err)
	}
}
