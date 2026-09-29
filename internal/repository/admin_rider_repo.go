package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// ListRidersFilter is the admin rider-list query (platform upgrade Module 10).
type ListRidersFilter struct {
	Search             string // matches name/phone, case-insensitive substring
	OnlineOnly         bool
	NegativeWalletOnly bool
	// IncludeLocation gates current lat/lng in the result — a separate flag
	// (not just "is admin") per the module's own "where permissions allow"
	// caveat. rider-service has no granular admin-permission tiers yet
	// (that is Module 18's job), so today this is caller-declared rather
	// than permission-checked; callers must not set it for a caller who
	// shouldn't see live rider location.
	IncludeLocation bool
	Limit           int
	Offset          int
}

// riderListJoins is shared between the count and the row query so the two
// stay consistent (a filter added to one WHERE clause must see the same
// joined columns in the other).
const riderListJoins = `
	FROM users u
	LEFT JOIN rider_availability ra ON ra.rider_id = u.id::text
	LEFT JOIN rider_wallet_balances wb ON wb.rider_id = u.id
	LEFT JOIN (
		SELECT delivery_partner_id AS rider_id, COUNT(*) AS cnt
		FROM orders WHERE order_status = 'delivered' AND delivery_partner_id IS NOT NULL
		GROUP BY delivery_partner_id
	) comp ON comp.rider_id = u.id::text
	LEFT JOIN (
		SELECT delivery_partner_id AS rider_id, COUNT(*) AS cnt
		FROM orders WHERE order_status = 'cancelled' AND delivery_partner_id IS NOT NULL
		GROUP BY delivery_partner_id
	) canc ON canc.rider_id = u.id::text
	LEFT JOIN (
		SELECT rider_id,
			SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END) AS total_earnings,
			SUM(CASE WHEN type = 'incentive' AND amount > 0 THEN amount ELSE 0 END) AS total_incentives,
			SUM(CASE WHEN type = 'penalty' THEN ABS(amount) ELSE 0 END) AS total_penalties
		FROM rider_earnings GROUP BY rider_id
	) earn ON earn.rider_id = u.id
	LEFT JOIN LATERAL (
		SELECT s.created_at AS last_settlement_at, s.is_full_and_final
		FROM rider_settlements s
		WHERE s.rider_id = u.id
		ORDER BY s.created_at DESC
		LIMIT 1
	) ls ON true
`

func (f ListRidersFilter) whereClause(startArg int) (string, []interface{}) {
	clauses := []string{"u.primary_role IN ('delivery_driver', 'rider')", "COALESCE(u.is_deleted, false) = false"}
	var args []interface{}
	n := startArg
	if f.Search != "" {
		clauses = append(clauses, fmt.Sprintf(
			"(u.first_name ILIKE $%d OR u.last_name ILIKE $%d OR u.display_name ILIKE $%d OR u.phone ILIKE $%d)", n, n, n, n))
		args = append(args, "%"+f.Search+"%")
		n++
	}
	if f.OnlineOnly {
		clauses = append(clauses, "COALESCE(ra.is_online, false) = true")
	}
	if f.NegativeWalletOnly {
		clauses = append(clauses, "COALESCE(wb.balance, 0) < 0")
	}
	return strings.Join(clauses, " AND "), args
}

// ListRiders returns the Module 10 admin rider list: identity, live status,
// completed/cancelled order counts, earnings/incentives/penalties, and
// wallet/settlement state built on the Module 11 ledger this migration adds.
func ListRiders(ctx context.Context, db *sql.DB, f ListRidersFilter) ([]models.AdminRiderSummary, int64, error) {
	if f.Limit <= 0 {
		f.Limit = 20
	}
	if f.Limit > 200 {
		f.Limit = 200
	}

	where, whereArgs := f.whereClause(1)

	var total int64
	countQuery := "SELECT COUNT(*) " + riderListJoins + " WHERE " + where
	if err := db.QueryRowContext(ctx, countQuery, whereArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count riders: %w", err)
	}

	locationSelect := "NULL::double precision, NULL::double precision, NULL::timestamptz"
	locationJoin := ""
	if f.IncludeLocation {
		locationSelect = "rl.latitude, rl.longitude, rl.last_updated_at"
		locationJoin = "LEFT JOIN rider_locations rl ON rl.rider_id = u.id::text"
	}

	args := append([]interface{}{}, whereArgs...)
	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	args = append(args, f.Limit, f.Offset)

	query := fmt.Sprintf(`
		SELECT
			u.id, u.user_id, COALESCE(u.phone, ''), COALESCE(u.first_name, ''), COALESCE(u.last_name, ''), COALESCE(u.display_name, ''),
			COALESCE(ra.is_online, false), COALESCE(ra.is_available, false), ra.current_order_id,
			COALESCE(comp.cnt, 0), COALESCE(canc.cnt, 0),
			COALESCE(earn.total_earnings, 0), COALESCE(earn.total_incentives, 0), COALESCE(earn.total_penalties, 0),
			COALESCE(wb.balance, 0),
			ls.last_settlement_at, ls.is_full_and_final,
			%s
		%s
		%s
		WHERE %s
		ORDER BY u.created_at DESC
		LIMIT $%d OFFSET $%d
	`, locationSelect, riderListJoins, locationJoin, where, limitArg, offsetArg)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list riders: %w", err)
	}
	defer rows.Close()

	out := []models.AdminRiderSummary{}
	for rows.Next() {
		var s models.AdminRiderSummary
		var userID, firstName, lastName, displayName string
		var lastSettlementAt sql.NullTime
		var lastSettlementFinal sql.NullBool
		if err := rows.Scan(
			&s.RiderID, &userID, &s.Phone, &firstName, &lastName, &displayName,
			&s.IsOnline, &s.IsAvailable, &s.CurrentOrderID,
			&s.CompletedOrders, &s.CancelledDeliveries,
			&s.TotalEarnings, &s.TotalIncentives, &s.TotalPenalties,
			&s.WalletBalance,
			&lastSettlementAt, &lastSettlementFinal,
			&s.CurrentLatitude, &s.CurrentLongitude, &s.LocationUpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan rider row: %w", err)
		}

		s.Name = resolveRiderName(firstName, lastName, displayName)
		s.IsNegativeWallet = s.WalletBalance < 0
		if s.WalletBalance < 0 {
			s.PlatformReceivable = -s.WalletBalance
			// The ledger's only realistic sources of a negative balance today
			// are uncollected COD cash (cash_collected) and penalties; there is
			// no separate cash-in-hand sub-ledger, so COD pending is modeled as
			// the same figure as platform receivable. See wallet_models.go.
			s.CODPending = -s.WalletBalance
		} else {
			s.RiderPayable = s.WalletBalance
		}
		if lastSettlementAt.Valid {
			t := lastSettlementAt.Time
			s.LastSettlementAt = &t
			if lastSettlementFinal.Bool {
				s.LastSettlementStatus = "full_and_final"
			} else {
				s.LastSettlementStatus = "partial"
			}
		} else {
			s.LastSettlementStatus = "never_settled"
		}

		out = append(out, s)
	}
	return out, total, rows.Err()
}

func resolveRiderName(first, last, display string) string {
	full := strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
	if full != "" {
		return full
	}
	return display
}
