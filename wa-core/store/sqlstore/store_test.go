package sqlstore

import (
	"fmt"
	"testing"

	"whatsrook/wa-core/types"
)

func TestNewSQLStoreDefersCaches(t *testing.T) {
	store := NewSQLStore(nil, types.JID{User: "123", Server: types.DefaultUserServer})
	if store.contactCache != nil || store.identityCache != nil || store.migratedPNSessionsCache.items != nil {
		t.Fatal("SQL store allocated caches before use")
	}
}

func TestBuildSharedMassInsertQuery(t *testing.T) {
	query := buildSharedMassInsertQuery("INSERT VALUES ", " ON CONFLICT", 2, 2)
	want := "INSERT VALUES ($1,$2,$3),($1,$4,$5) ON CONFLICT"
	if query != want {
		t.Fatalf("unexpected query:\n%s\nwant:\n%s", query, want)
	}
}

func TestIdentityCacheIsBounded(t *testing.T) {
	store := &SQLStore{identityCache: make(map[string]identityCacheEntry, maxIdentityCacheEntries)}
	for i := 0; i < maxIdentityCacheEntries; i++ {
		store.setCachedIdentityLocked(fmt.Sprintf("%d:1", i), identityCacheEntry{Present: true})
	}
	store.setCachedIdentityLocked("new:1", identityCacheEntry{Present: true})

	if len(store.identityCache) != maxIdentityCacheEntries {
		t.Fatalf("cache grew to %d entries", len(store.identityCache))
	}
	if _, ok := store.identityCache["new:1"]; !ok {
		t.Fatal("new identity was not cached")
	}
}

func TestContactCacheIsBounded(t *testing.T) {
	store := &SQLStore{contactCache: make(map[types.JID]*types.ContactInfo, maxContactCacheEntries)}
	for i := 0; i < maxContactCacheEntries; i++ {
		jid := types.NewJID(fmt.Sprintf("%d", i), types.DefaultUserServer)
		store.setCachedContactLocked(jid, &types.ContactInfo{Found: true})
	}
	newJID := types.NewJID("new", types.DefaultUserServer)
	store.setCachedContactLocked(newJID, &types.ContactInfo{Found: true})

	if len(store.contactCache) != maxContactCacheEntries {
		t.Fatalf("cache grew to %d entries", len(store.contactCache))
	}
	if _, ok := store.contactCache[newJID]; !ok {
		t.Fatal("new contact was not cached")
	}
}

func TestMigratedPNSessionsCacheIsBounded(t *testing.T) {
	store := &SQLStore{}
	for i := 0; i < maxMigratedPNEntries; i++ {
		if !store.migratedPNSessionsCache.Add(fmt.Sprintf("%d:1", i)) {
			t.Fatalf("Add returned false for a new key %d", i)
		}
	}
	if !store.migratedPNSessionsCache.Add("new:1") {
		t.Fatal("new entry was not added")
	}

	if len(store.migratedPNSessionsCache.items) != maxMigratedPNEntries {
		t.Fatalf("cache grew to %d entries", len(store.migratedPNSessionsCache.items))
	}
	if _, ok := store.migratedPNSessionsCache.items["new:1"]; !ok {
		t.Fatal("new entry was not cached")
	}
}

func TestMigratedPNSessionsCacheAddIsIdempotent(t *testing.T) {
	store := &SQLStore{}
	if !store.migratedPNSessionsCache.Add("a:1") {
		t.Fatal("first Add for a new key should return true")
	}
	if store.migratedPNSessionsCache.Add("a:1") {
		t.Fatal("second Add for the same key should return false")
	}

	store.migratedPNSessionsCache.Remove("a:1")
	if !store.migratedPNSessionsCache.Add("a:1") {
		t.Fatal("Add after Remove should return true again")
	}
}
