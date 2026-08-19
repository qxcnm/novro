package user

import (
	"testing"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
)

func TestAdminRecordFromEntIncludesWalletBalance(t *testing.T) {
	userID := uuid.New()
	record := adminRecordFromEnt(&ent.User{
		ID:       userID,
		Username: "member.one",
		Edges: ent.UserEdges{
			Wallet: &ent.Wallet{UserID: userID, BalanceMicros: -1_234_567},
		},
	})
	if record.ID != userID || record.Username != "member.one" || record.BalanceMicros != -1_234_567 {
		t.Fatalf("unexpected administrator record: %+v", record)
	}
}

func TestAdminRecordFromEntTreatsMissingWalletAsZero(t *testing.T) {
	record := adminRecordFromEnt(&ent.User{ID: uuid.New(), Username: "legacy.member"})
	if record.BalanceMicros != 0 {
		t.Fatalf("unexpected missing-wallet balance: %+v", record)
	}
}
