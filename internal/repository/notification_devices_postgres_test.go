package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
)

// The shape of the table shared with restaurant-service
// (restaurant-service/migrations/016_owner_intelligence.sql). The primary key
// is a generated UUID; the only uniqueness on a token is the (tenant, user,
// token) index.
const notificationDevicesSchema = `
CREATE TABLE notification_devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    outlet_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    platform TEXT NOT NULL,
    push_token TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX uq_notification_devices_token ON notification_devices (tenant_id, user_id, push_token);`

func deviceRepo(t *testing.T) (*NotificationRepository, *sql.DB) {
	t.Helper()
	db := testpg.Open(t)
	if _, err := db.Exec(notificationDevicesSchema); err != nil {
		t.Fatal(err)
	}
	return NewNotificationRepository(db), db
}

func tokensOf(t *testing.T, repo *NotificationRepository, rider string) []string {
	t.Helper()
	tokens, err := repo.DeviceTokens(context.Background(), rider)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, token := range tokens {
		out = append(out, token.Token)
	}
	return out
}

func TestThePreviousTokenUpsertFailedOnEveryRepeatedRegistration(t *testing.T) {
	// The app registers its token at every start. The statement this replaced
	// named the primary key as its conflict target; the key is a generated
	// UUID, so it could never conflict, and the second registration of the same
	// token hit the (tenant, user, token) unique index and failed instead.
	_, db := deviceRepo(t)
	const previous = `INSERT INTO notification_devices (tenant_id, outlet_id, user_id, platform, push_token, created_at)
		VALUES ('rider', 'rider', $1, $2, $3, NOW())
		ON CONFLICT ON CONSTRAINT notification_devices_pkey DO UPDATE SET push_token = $3`
	if _, err := db.Exec(previous, "rider-1", "android", "tok"); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if _, err := db.Exec(previous, "rider-1", "android", "tok"); err == nil {
		t.Fatal("the previous statement was expected to fail on a repeat")
	}
}

func TestRegisteringTheSameTokenAgainIsHarmless(t *testing.T) {
	repo, db := deviceRepo(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := repo.RegisterDeviceToken(ctx, "rider-1", "android", "tok-a"); err != nil {
			t.Fatalf("registration %d: %v", i+1, err)
		}
	}
	var rows int
	_ = db.QueryRow(`SELECT count(*) FROM notification_devices`).Scan(&rows)
	if rows != 1 {
		t.Fatalf("rows=%d, want one registration", rows)
	}

	// The platform is refreshed and the registration counts as recent.
	if _, err := db.Exec(`UPDATE notification_devices SET created_at = created_at - interval '1 day'`); err != nil {
		t.Fatal(err)
	}
	if err := repo.RegisterDeviceToken(ctx, "rider-1", "ios", "tok-a"); err != nil {
		t.Fatal(err)
	}
	var platform string
	var age time.Duration
	var ageSeconds float64
	_ = db.QueryRow(`SELECT platform, extract(epoch FROM now() - created_at) FROM notification_devices`).Scan(&platform, &ageSeconds)
	age = time.Duration(ageSeconds * float64(time.Second))
	if platform != "ios" || age > time.Minute {
		t.Fatalf("platform=%s age=%v", platform, age)
	}
}

func TestATokenBelongsToWhoeverSignedInLast(t *testing.T) {
	repo, _ := deviceRepo(t)
	ctx := context.Background()

	if err := repo.RegisterDeviceToken(ctx, "rider-1", "android", "shared-phone"); err != nil {
		t.Fatal(err)
	}
	// The phone is handed to another rider who signs in.
	if err := repo.RegisterDeviceToken(ctx, "rider-2", "android", "shared-phone"); err != nil {
		t.Fatal(err)
	}

	if got := tokensOf(t, repo, "rider-1"); len(got) != 0 {
		t.Fatalf("the previous rider still receives this phone's offers: %v", got)
	}
	if got := tokensOf(t, repo, "rider-2"); len(got) != 1 || got[0] != "shared-phone" {
		t.Fatalf("new rider tokens: %v", got)
	}
}

func TestOneRidersOtherDevicesAreUntouchedByANewRegistration(t *testing.T) {
	repo, _ := deviceRepo(t)
	ctx := context.Background()
	for _, tok := range []string{"phone", "tablet"} {
		if err := repo.RegisterDeviceToken(ctx, "rider-1", "android", tok); err != nil {
			t.Fatal(err)
		}
	}
	got := tokensOf(t, repo, "rider-1")
	if len(got) != 2 {
		t.Fatalf("both devices should stay registered: %v", got)
	}
}

func TestAtMostFiveNewestTokensAreKept(t *testing.T) {
	repo, db := deviceRepo(t)
	ctx := context.Background()
	for i := 1; i <= 7; i++ {
		tok := fmt.Sprintf("tok-%d", i)
		if err := repo.RegisterDeviceToken(ctx, "rider-1", "android", tok); err != nil {
			t.Fatal(err)
		}
		// Spread the timestamps so "newest" is unambiguous.
		if _, err := db.Exec(`UPDATE notification_devices SET created_at = now() - make_interval(secs => $1) WHERE push_token = $2`, float64(100-i), tok); err != nil {
			t.Fatal(err)
		}
	}
	// One more registration triggers the trim after the timestamps are set.
	if err := repo.RegisterDeviceToken(ctx, "rider-1", "android", "tok-8"); err != nil {
		t.Fatal(err)
	}
	got := tokensOf(t, repo, "rider-1")
	if len(got) != 5 {
		t.Fatalf("kept %d tokens: %v", len(got), got)
	}
	if got[0] != "tok-8" {
		t.Fatalf("newest first: %v", got)
	}
	for _, stale := range []string{"tok-1", "tok-2", "tok-3"} {
		for _, kept := range got {
			if kept == stale {
				t.Fatalf("%s should have been trimmed: %v", stale, got)
			}
		}
	}
}

func TestRemovingATokenAtSignOutOnlyRemovesThatRidersToken(t *testing.T) {
	repo, _ := deviceRepo(t)
	ctx := context.Background()
	_ = repo.RegisterDeviceToken(ctx, "rider-1", "android", "tok")
	_ = repo.RegisterDeviceToken(ctx, "rider-2", "android", "other")

	// Someone else cannot unregister a rider's token.
	if err := repo.RemoveDeviceToken(ctx, "rider-2", "tok"); err != nil {
		t.Fatal(err)
	}
	if got := tokensOf(t, repo, "rider-1"); len(got) != 1 {
		t.Fatalf("rider-1 lost a token they own: %v", got)
	}

	if err := repo.RemoveDeviceToken(ctx, "rider-1", "tok"); err != nil {
		t.Fatal(err)
	}
	if got := tokensOf(t, repo, "rider-1"); len(got) != 0 {
		t.Fatalf("token survived sign-out: %v", got)
	}
	// Removing what is already gone is fine (sign-out is retried).
	if err := repo.RemoveDeviceToken(ctx, "rider-1", "tok"); err != nil {
		t.Fatal(err)
	}
	if got := tokensOf(t, repo, "rider-2"); len(got) != 1 {
		t.Fatalf("rider-2 affected: %v", got)
	}
}

func TestRestaurantOwnerTokensInTheSameTableAreNeverTouched(t *testing.T) {
	repo, db := deviceRepo(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO notification_devices (tenant_id, outlet_id, user_id, platform, push_token, created_at)
		VALUES ('owner-tenant', 'outlet', 'owner-1', 'android', 'tok', now())`); err != nil {
		t.Fatal(err)
	}
	// A rider registering the same string, then removing it, must not affect
	// another tenant's row.
	_ = repo.RegisterDeviceToken(ctx, "rider-1", "android", "tok")
	_ = repo.RemoveDeviceToken(ctx, "rider-1", "tok")
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM notification_devices WHERE tenant_id='owner-tenant'`).Scan(&n)
	if n != 1 {
		t.Fatalf("an owner's registration was disturbed: %d", n)
	}
}
