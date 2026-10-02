package models

import "time"

// Wallet transaction types (platform upgrade Module 11). Positive amounts
// increase what the platform owes the rider; negative amounts increase what
// the rider owes the platform (e.g. uncollected COD cash).
const (
	WalletTxnDeliveryEarning       = "delivery_earning"
	WalletTxnIncentive             = "incentive"
	WalletTxnCashCollected         = "cash_collected"
	WalletTxnCODLiability          = "cod_liability"
	WalletTxnRiderPaymentToCompany = "rider_payment_to_company"
	WalletTxnCompanyPaymentToRider = "company_payment_to_rider"
	WalletTxnPenalty               = "penalty"
	WalletTxnManualAdjustment      = "manual_adjustment"
	WalletTxnSettlement            = "settlement"
	WalletTxnRefundReversal        = "refund_reversal"
)

// RiderWalletTransaction is one immutable row of a rider's net-payable
// ledger — never updated or deleted after insert.
type RiderWalletTransaction struct {
	ID               int64     `json:"id"`
	RiderID          string    `json:"rider_id"`
	TransactionType  string    `json:"transaction_type"`
	Amount           float64   `json:"amount"`
	BalanceBefore    float64   `json:"balance_before"`
	BalanceAfter     float64   `json:"balance_after"`
	OrderID          *int      `json:"order_id,omitempty"`
	ReferenceNumber  *string   `json:"reference_number,omitempty"`
	PaymentMode      *string   `json:"payment_mode,omitempty"`
	Notes            *string   `json:"notes,omitempty"`
	CreatedByAdminID *string   `json:"created_by_admin_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// RiderSettlement is one admin-recorded settlement event.
type RiderSettlement struct {
	ID                  int64     `json:"id"`
	RiderID             string    `json:"rider_id"`
	PreviousBalance     float64   `json:"previous_balance"`
	SettlementAmount    float64   `json:"settlement_amount"`
	AmountReceived      float64   `json:"amount_received"`
	AmountPaid          float64   `json:"amount_paid"`
	AdjustmentAmount    float64   `json:"adjustment_amount"`
	ClosingBalance      float64   `json:"closing_balance"`
	PaymentMode         *string   `json:"payment_mode,omitempty"`
	ReferenceNumber     *string   `json:"reference_number,omitempty"`
	IsFullAndFinal      bool      `json:"is_full_and_final"`
	Notes               *string   `json:"notes,omitempty"`
	AdminID             string    `json:"admin_id"`
	WalletTransactionID *int64    `json:"wallet_transaction_id,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

// AdminRiderSummary is one row of the Module 10 admin rider list.
type AdminRiderSummary struct {
	RiderID        string `json:"rider_id"`
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	IsOnline       bool   `json:"is_online"`
	IsAvailable    bool   `json:"is_available"`
	CurrentOrderID *int   `json:"current_order_id,omitempty"`

	CompletedOrders     int `json:"completed_orders"`
	CancelledDeliveries int `json:"cancelled_deliveries"`

	TotalEarnings   float64 `json:"total_earnings"`
	TotalIncentives float64 `json:"total_incentives"`
	TotalPenalties  float64 `json:"total_penalties"`

	WalletBalance      float64 `json:"wallet_balance"`
	IsNegativeWallet   bool    `json:"is_negative_wallet"`
	CODPending         float64 `json:"cod_pending"`
	PlatformReceivable float64 `json:"platform_receivable"` // = max(0, -wallet_balance)
	RiderPayable       float64 `json:"rider_payable"`       // = max(0, wallet_balance)

	LastSettlementAt     *time.Time `json:"last_settlement_at,omitempty"`
	LastSettlementStatus string     `json:"last_settlement_status,omitempty"` // "full_and_final" | "partial" | "never_settled"

	// Current operational location — only populated when the caller passed
	// includeLocation=true (an explicit permission/consent gate; see
	// repository.ListRiders), per the module's own "where permissions
	// allow" caveat.
	CurrentLatitude   *float64   `json:"current_latitude,omitempty"`
	CurrentLongitude  *float64   `json:"current_longitude,omitempty"`
	LocationUpdatedAt *time.Time `json:"location_updated_at,omitempty"`
}
