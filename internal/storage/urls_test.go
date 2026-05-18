package storage

import "testing"

func TestObjectKeysFromPublicURLs_avatarPair(t *testing.T) {
	keys := ObjectKeysFromPublicURLs(
		"https://nest.example.com/images/avatars/abcdef0123456789012345678.png",
		"https://nest.example.com/images/avatars/1234567890abcdef12345678.jpg",
	)
	if len(keys) != 2 {
		t.Fatalf("want 2 keys, got %v", keys)
	}
}
