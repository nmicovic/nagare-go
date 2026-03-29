package notifications

import (
	"path/filepath"
	"testing"
)

func TestStoreAddAndList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.json")
	store := NewStore(path)

	store.Add("my-session", "task completed")
	store.Add("other-session", "needs input")

	all := store.ListAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(all))
	}
	if all[0].SessionName != "other-session" {
		t.Errorf("newest first: got %q", all[0].SessionName)
	}
}

func TestStoreMarkRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.json")
	store := NewStore(path)

	store.Add("test", "message")
	all := store.ListAll()
	id := all[0].ID

	store.MarkRead(id)
	all = store.ListAll()
	if !all[0].Read {
		t.Error("notification should be marked read")
	}
}

func TestStoreDismiss(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.json")
	store := NewStore(path)

	store.Add("test", "message")
	all := store.ListAll()
	id := all[0].ID

	store.Dismiss(id)
	if len(store.ListAll()) != 0 {
		t.Error("notification should be dismissed")
	}
}

func TestStoreUnreadCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.json")
	store := NewStore(path)

	store.Add("a", "msg1")
	store.Add("b", "msg2")

	if store.UnreadCount() != 2 {
		t.Errorf("unread = %d, want 2", store.UnreadCount())
	}

	all := store.ListAll()
	store.MarkRead(all[0].ID)
	if store.UnreadCount() != 1 {
		t.Errorf("unread = %d, want 1", store.UnreadCount())
	}
}

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notifications.json")

	store1 := NewStore(path)
	store1.Add("test", "persisted")

	store2 := NewStore(path)
	if len(store2.ListAll()) != 1 {
		t.Error("notification should persist across loads")
	}
}
