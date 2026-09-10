package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// The public rider pool must never contain an order a rider already holds.
//
// Two assignment paths write different columns: platform acceptance sets
// delivery_partner_id, while restaurant-owner assignment sets only
// assigned_rider_user_id and delivery_status. Filtering on delivery_partner_id
// alone left every owner-assigned order visible to every other rider, who
// would then be refused with a conflict on accept.

func TestAvailableOrdersExcludeEveryRiderColumn(t *testing.T) {
	for _, clause := range []string{
		"o.delivery_partner_id IS NULL",
		"o.rider_id IS NULL",
		"COALESCE(o.assigned_rider_user_id, '') = ''",
	} {
		if !strings.Contains(availableOrderPredicate, clause) {
			t.Errorf("predicate missing %q — orders assigned through that column would stay in the public pool", clause)
		}
	}
	for _, status := range []string{"rider_assigned", "picked_up", "out_for_delivery", "delivered"} {
		if !strings.Contains(availableOrderPredicate, "'"+status+"'") {
			t.Errorf("predicate does not exclude delivery_status %q", status)
		}
	}
}

func TestAvailableOrdersOnlyOffersReadyDeliveryOrders(t *testing.T) {
	if !strings.Contains(availableOrderPredicate, "o.order_status = 'ready'") {
		t.Error("an order must be ready before it is offered to riders")
	}
	if !strings.Contains(availableOrderPredicate, "o.order_type = 'DELIVERY'") {
		t.Error("only delivery orders belong in the rider pool")
	}
}

// captureMatcher records every statement sqlmock is asked to match.
type captureMatcher struct{ seen *[]string }

func (m captureMatcher) Match(expected, actual string) error {
	*m.seen = append(*m.seen, actual)
	return nil
}

// The count and the page must apply the same filter, or the pager reports a
// total that does not match what riders can actually see.
func TestAvailableOrdersCountAndPageShareOnePredicate(t *testing.T) {
	var statements []string
	db, mock, err := sqlmock.New(
		sqlmock.QueryMatcherOption(captureMatcher{seen: &statements}),
	)
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("page").WillReturnRows(sqlmock.NewRows([]string{"order_id"}))

	repo := NewOrderRepository(db)
	// The scan will fail on the stub row set; only the SQL text matters here.
	_, _, _ = repo.GetAvailableOrders(context.Background(), 20, 0)

	if len(statements) < 2 {
		t.Fatalf("expected a count query and a page query, saw %d", len(statements))
	}
	for i, sql := range statements[:2] {
		label := []string{"count", "page"}[i]
		if !strings.Contains(sql, "assigned_rider_user_id") {
			t.Errorf("%s query does not exclude owner-assigned orders:\n%s", label, sql)
		}
		if !strings.Contains(sql, "rider_assigned") {
			t.Errorf("%s query does not exclude delivery_status rider_assigned:\n%s", label, sql)
		}
	}
}
