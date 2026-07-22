package commands

import (
	"context"
	"testing"

	"whatsrook/sender"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

type testLIDStore struct {
	lidToPN map[types.JID]types.JID
	pnToLID map[types.JID]types.JID
}

func (m *testLIDStore) PutManyLIDMappings(ctx context.Context, mappings []store.LIDMapping) error {
	return nil
}

func (m *testLIDStore) PutLIDMapping(ctx context.Context, lid, jid types.JID) error {
	return nil
}

func (m *testLIDStore) GetPNForLID(ctx context.Context, lid types.JID) (types.JID, error) {
	if pn, ok := m.lidToPN[lid.ToNonAD()]; ok {
		return pn, nil
	}
	return types.JID{}, nil
}

func (m *testLIDStore) GetLIDForPN(ctx context.Context, pn types.JID) (types.JID, error) {
	if lid, ok := m.pnToLID[pn.ToNonAD()]; ok {
		return lid, nil
	}
	return types.JID{}, nil
}

func (m *testLIDStore) GetManyLIDsForPNs(ctx context.Context, pns []types.JID) (map[types.JID]types.JID, error) {
	return nil, nil
}

func TestIsAdminRaw(t *testing.T) {
	// Setup JIDs
	adminPN := types.NewJID("12345", types.DefaultUserServer)
	adminLID := types.NewJID("54321", types.HiddenUserServer)

	memberPN := types.NewJID("67890", types.DefaultUserServer)
	memberLID := types.NewJID("09876", types.HiddenUserServer)

	// Create mock LID store
	lStore := &testLIDStore{
		lidToPN: map[types.JID]types.JID{
			adminLID:  adminPN,
			memberLID: memberPN,
		},
		pnToLID: map[types.JID]types.JID{
			adminPN:  adminLID,
			memberPN: memberLID,
		},
	}

	// Create a dummy store.Device
	deviceStore := &store.Device{
		LIDs: lStore,
	}

	client := &whatsmeow.Client{
		Store: deviceStore,
	}

	info := &types.GroupInfo{
		Participants: []types.GroupParticipant{
			{
				JID:          adminLID, // Admin JID in group info is their LID
				IsAdmin:      true,
				IsSuperAdmin: false,
			},
			{
				JID:          memberPN, // Member JID in group info is their PN
				IsAdmin:      false,
				IsSuperAdmin: false,
			},
		},
	}

	ctx := context.Background()

	// Test case 1: admin LID checks against group participant (which is LID) -> should match
	if !sender.IsAdminRaw(ctx, client, info, adminLID) {
		t.Error("expected adminLID to be admin")
	}

	// Test case 2: admin PN checks against group participant (LID in group) -> should match (mapping resolved)
	if !sender.IsAdminRaw(ctx, client, info, adminPN) {
		t.Error("expected adminPN to be admin via LID mapping")
	}

	// Test case 3: member PN checks against group participant (PN in group) -> should not match admin status
	if sender.IsAdminRaw(ctx, client, info, memberPN) {
		t.Error("expected memberPN to not be admin")
	}

	// Test case 4: member LID checks against group participant (PN in group) -> should not match admin status
	if sender.IsAdminRaw(ctx, client, info, memberLID) {
		t.Error("expected memberLID to not be admin")
	}

	// Test case 5: completely unknown JID
	unknownJID := types.NewJID("11111", types.DefaultUserServer)
	if sender.IsAdminRaw(ctx, client, info, unknownJID) {
		t.Error("expected unknown JID to not be admin")
	}
}
