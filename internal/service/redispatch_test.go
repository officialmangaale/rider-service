package service

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/ws"
)

// Order 13286: dispatched at 06:06:03 while the only nearby rider's last GPS
// point was 16 minutes old, so it matched nobody and was never offered again.
// RedispatchOrder is what now offers it once that rider is eligible.

var deliveryOrderColumns = []string{
	"delivery_order_id", "order_id", "restaurant_id", "customer_id",
	"pickup_latitude", "pickup_longitude", "pickup_address",
	"drop_latitude", "drop_longitude", "drop_address",
	"amount", "payment_mode", "delivery_status", "assigned_rider_id",
	"created_at", "updated_at", "assigned_at", "picked_up_at", "delivered_at",
	"rider_user_id", "assignment_type", "restaurant_owned", "restaurant_name", "restaurant_phone",
	"customer_name", "customer_phone", "items_summary", "rider_arrived_at", "order_type",
}

func deliveryOrderRow(status string, assignedRider interface{}, restaurantOwned bool) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(deliveryOrderColumns).AddRow(
		16, 13286, 27, 0,
		28.41, 77.04, "pickup",
		28.42, 77.05, "drop",
		250.0, "cash", status, assignedRider,
		now, now, nil, nil, nil,
		nil, "platform", restaurantOwned, "Fateh Cafe", "",
		"", "", "", nil, "food",
	)
}

func newRedispatchService(t *testing.T) (*DeliveryService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	svc := NewDeliveryService(
		repository.NewDeliveryRepository(db),
		repository.NewRiderRepository(db),
		ws.NewHub(),
		nil,
		5.0, // radius km
		5,   // max riders
		30,  // request expiry seconds
		nil, // no Redis
	)
	return svc, mock
}

func expectRedispatchable(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta("FROM delivery_orders WHERE delivery_order_id=$1")).
		WithArgs(16).
		WillReturnRows(deliveryOrderRow(models.DeliveryStatusNoRiderFound, nil, false))
	mock.ExpectQuery("SELECT EXISTS.*FROM orders o").WithArgs(13286).
		WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta("status='accepted'")).
		WithArgs(16).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM delivery_order_requests")).
		WithArgs(16).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
}

var nearbyColumns = []string{"rider_id", "latitude", "longitude", "distance_km"}

func TestRedispatchOffersTheOrderToARiderWhoIsNowEligible(t *testing.T) {
	svc, mock := newRedispatchService(t)
	expectRedispatchable(mock)
	mock.ExpectQuery(regexp.QuoteMeta("status = 'rejected'")).
		WithArgs(16).
		WillReturnRows(sqlmock.NewRows([]string{"rider_id"}))
	mock.ExpectQuery(regexp.QuoteMeta("FROM rider_locations rl")).
		WithArgs(28.41, 77.04, 5.0, 5).
		WillReturnRows(sqlmock.NewRows(nearbyColumns).AddRow("rider-ramesh", 28.41, 77.04, 0.0))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE delivery_orders SET delivery_status=$2")).
		WithArgs(16, models.DeliveryStatusRiderSearching).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO delivery_order_requests")).
		WithArgs(16, 13286, "rider-ramesh", 0.0, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"request_id", "delivery_order_id", "order_id", "rider_id", "status", "distance_km", "expires_at", "created_at", "updated_at"}).
			AddRow(901, 16, 13286, "rider-ramesh", "pending", 0.0, time.Now().Add(30*time.Second), time.Now(), time.Now()))

	offered, err := svc.RedispatchOrder(context.Background(), 16)
	if err != nil {
		t.Fatalf("RedispatchOrder: %v", err)
	}
	if offered != 1 {
		t.Fatalf("expected one offer, got %d", offered)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A sweep that finds nobody must not touch the row: it runs every 20 seconds.
func TestRedispatchWithNoEligibleRiderWritesNothing(t *testing.T) {
	svc, mock := newRedispatchService(t)
	expectRedispatchable(mock)
	mock.ExpectQuery(regexp.QuoteMeta("status = 'rejected'")).
		WithArgs(16).
		WillReturnRows(sqlmock.NewRows([]string{"rider_id"}))
	mock.ExpectQuery(regexp.QuoteMeta("FROM rider_locations rl")).
		WillReturnRows(sqlmock.NewRows(nearbyColumns))

	offered, err := svc.RedispatchOrder(context.Background(), 16)
	if err != nil || offered != 0 {
		t.Fatalf("expected 0 offers and no error, got %d, %v", offered, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A decline is that rider's answer for this order. The next rider still gets
// it, and the search asks for one extra rider to make up the difference.
func TestRedispatchSkipsARiderWhoDeclined(t *testing.T) {
	svc, mock := newRedispatchService(t)
	expectRedispatchable(mock)
	mock.ExpectQuery(regexp.QuoteMeta("status = 'rejected'")).
		WithArgs(16).
		WillReturnRows(sqlmock.NewRows([]string{"rider_id"}).AddRow("rider-declined"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM rider_locations rl")).
		WithArgs(28.41, 77.04, 5.0, 6).
		WillReturnRows(sqlmock.NewRows(nearbyColumns).
			AddRow("rider-declined", 28.41, 77.04, 0.2).
			AddRow("rider-next", 28.41, 77.04, 1.5))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE delivery_orders SET delivery_status=$2")).
		WithArgs(16, models.DeliveryStatusRiderSearching).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO delivery_order_requests")).
		WithArgs(16, 13286, "rider-next", 1.5, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"request_id", "delivery_order_id", "order_id", "rider_id", "status", "distance_km", "expires_at", "created_at", "updated_at"}).
			AddRow(902, 16, 13286, "rider-next", "pending", 1.5, time.Now().Add(30*time.Second), time.Now(), time.Now()))

	offered, err := svc.RedispatchOrder(context.Background(), 16)
	if err != nil {
		t.Fatalf("RedispatchOrder: %v", err)
	}
	if offered != 1 {
		t.Fatalf("expected exactly the non-declining rider to be offered, got %d offers", offered)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Between the sweep's SELECT and this call a rider may accept, or the
// restaurant may assign its own rider. The current row decides.
func TestRedispatchStopsWhenTheOrderWasAssignedMeanwhile(t *testing.T) {
	for name, row := range map[string]*sqlmock.Rows{
		"platform rider accepted":    deliveryOrderRow(models.DeliveryStatusRiderAssigned, "rider-x", false),
		"restaurant's own rider":     deliveryOrderRow(models.DeliveryStatusRiderAssigned, "own-rider", true),
		"cancelled by rider-service": deliveryOrderRow(models.DeliveryStatusCancelled, nil, false),
	} {
		t.Run(name, func(t *testing.T) {
			svc, mock := newRedispatchService(t)
			mock.ExpectQuery(regexp.QuoteMeta("FROM delivery_orders WHERE delivery_order_id=$1")).
				WithArgs(16).
				WillReturnRows(row)

			offered, err := svc.RedispatchOrder(context.Background(), 16)
			if err != nil || offered != 0 {
				t.Fatalf("expected no offer, got %d, %v", offered, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// A pending offer is still running; a second one must not be layered on it.
func TestRedispatchStopsWhileAnOfferIsPending(t *testing.T) {
	svc, mock := newRedispatchService(t)
	mock.ExpectQuery(regexp.QuoteMeta("FROM delivery_orders WHERE delivery_order_id=$1")).
		WithArgs(16).
		WillReturnRows(deliveryOrderRow(models.DeliveryStatusRiderSearching, nil, false))
	mock.ExpectQuery("SELECT EXISTS.*FROM orders o").WithArgs(13286).
		WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta("status='accepted'")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM delivery_order_requests")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	offered, err := svc.RedispatchOrder(context.Background(), 16)
	if err != nil || offered != 0 {
		t.Fatalf("expected no offer, got %d, %v", offered, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExcludeDeclinedRidersKeepsNearestFirstAndCaps(t *testing.T) {
	riders := []models.NearbyRider{{RiderID: "a"}, {RiderID: "b"}, {RiderID: "c"}, {RiderID: "d"}}

	got := excludeDeclinedRiders(riders, map[string]bool{"b": true}, 2)
	if len(got) != 2 || got[0].RiderID != "a" || got[1].RiderID != "c" {
		t.Fatalf("unexpected riders %+v", got)
	}
	// No declines: the input passes through, as on a first dispatch.
	if got := excludeDeclinedRiders(riders, nil, 5); len(got) != 4 {
		t.Fatalf("expected all riders without declines, got %+v", got)
	}
}
