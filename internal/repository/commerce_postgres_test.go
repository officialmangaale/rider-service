package repository

import (
	"context"
	"testing"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
)

func TestCommercePayoutSnapshotAndWalletNamespaces(t *testing.T) {
	db := testpg.Open(t)
	if err := testpg.ApplyLifecycle(db); err != nil {
		t.Fatal(err)
	}
	const rider = "c6b46748-0000-4000-8000-000000000021"
	_, err := db.Exec(`INSERT INTO users(id) VALUES($1);
	`, rider)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE grocery_pricing_snapshots(grocery_order_id BIGINT PRIMARY KEY,rider_cost_cents BIGINT);
	CREATE TABLE order_pricing_snapshots(order_id BIGINT PRIMARY KEY,rider_cost_cents BIGINT);
	INSERT INTO grocery_pricing_snapshots VALUES(7,4550);
	INSERT INTO order_pricing_snapshots VALUES(7,3500);`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := NewDeliveryRepository(db)
	for _, kind := range []string{"food", "grocery"} {
		order := &models.DeliveryOrder{OrderID: 7, OrderType: kind}
		payout, err := repo.QuotedRiderPayout(ctx, order)
		if err != nil {
			t.Fatal(err)
		}
		want := 35.0
		if kind == "grocery" {
			want = 45.5
		}
		if payout != want {
			t.Fatalf("%s quote=%v want %v", kind, payout, want)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		id := 7
		if _, err = postWalletTransaction(ctx, tx, rider, models.WalletTxnDeliveryEarning, payout, &id, nil, nil, nil, nil, kind); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if _, err = postWalletTransaction(ctx, tx, rider, models.WalletTxnCashCollected, -100, &id, nil, nil, nil, nil, kind); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	var rows int
	var balance float64
	if err = db.QueryRow(`SELECT count(*) FROM rider_wallet_transactions`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT balance FROM rider_wallet_balances WHERE rider_id=$1`, rider).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if rows != 4 || balance != -119.5 {
		t.Fatalf("wallet rows=%d balance=%v", rows, balance)
	}
	if _, err = db.Exec(`INSERT INTO rider_wallet_transactions(rider_id,transaction_type,amount,balance_before,balance_after,order_id,order_type) VALUES($1,'delivery_earning',45.5,0,45.5,7,'grocery')`, rider); err == nil {
		t.Fatal("duplicate grocery payout accepted")
	}
	if _, err = db.Exec(`UPDATE grocery_pricing_snapshots SET rider_cost_cents=0 WHERE grocery_order_id=7`); err != nil {
		t.Fatal(err)
	}
	zero, err := repo.QuotedRiderPayout(ctx, &models.DeliveryOrder{OrderID: 7, OrderType: "grocery"})
	if err != nil || zero != 0 {
		t.Fatalf("zero payout lost: %v %v", zero, err)
	}
}
