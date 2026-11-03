package riderpay

import (
	"errors"
	"math"
)

type Cancellation struct {
	FixedCents        int64 `json:"fixed_cents"`
	IncentivePermille int64 `json:"incentive_permille"`
}
type Policy struct {
	RiderID           string                  `json:"rider_id"`
	Enabled           bool                    `json:"enabled"`
	Mode              string                  `json:"mode"`
	BaseCents         int64                   `json:"base_cents"`
	PerKmCents        int64                   `json:"per_km_cents"`
	SalaryCents       int64                   `json:"salary_cents"`
	SalaryPeriod      string                  `json:"salary_period"`
	IncentiveCents    int64                   `json:"incentive_cents"`
	IncentivePermille int64                   `json:"incentive_permille"`
	Cancellation      map[string]Cancellation `json:"cancellation"`
	Version           int64                   `json:"version"`
}

func (p Policy) Validate() error {
	if p.RiderID == "" || p.Version < 0 || (p.Mode != "salary" && p.Mode != "per_delivery") || (p.SalaryPeriod != "weekly" && p.SalaryPeriod != "monthly") {
		return errors.New("rider, earning model and salary period are required")
	}
	for _, v := range []int64{p.BaseCents, p.PerKmCents, p.SalaryCents, p.IncentiveCents} {
		if v < 0 || v > 100000000 {
			return errors.New("invalid earning amount")
		}
	}
	if p.IncentivePermille < 0 || p.IncentivePermille > 1000 || (p.IncentiveCents > 0 && p.IncentivePermille > 0) {
		return errors.New("choose a fixed incentive or a percentage, not both")
	}
	if p.Mode == "salary" && (p.BaseCents != 0 || p.PerKmCents != 0) {
		return errors.New("salary mode cannot include base or per-km pay")
	}
	if p.Mode == "per_delivery" && (p.SalaryCents != 0 || p.IncentiveCents != 0 || p.IncentivePermille != 0) {
		return errors.New("per-delivery mode uses base plus per-km only")
	}
	for stage, c := range p.Cancellation {
		switch stage {
		case "rider_assigned", "rider_arrived_restaurant", "picked_up", "on_the_way":
		default:
			return errors.New("unsupported cancellation stage")
		}
		if c.FixedCents < 0 || c.FixedCents > 100000000 || c.IncentivePermille < 0 || c.IncentivePermille > 1000 {
			return errors.New("invalid cancellation pay")
		}
		if (p.Mode == "salary" && c.FixedCents != 0) || (p.Mode == "per_delivery" && c.IncentivePermille != 0) {
			return errors.New("salary cancellation uses a percentage of incentive; per-delivery uses a fixed amount")
		}
	}
	return nil
}
func (p Policy) Quote(discountedItems int64, km float64) (amount, incentive int64, err error) {
	if err = p.Validate(); err != nil {
		return
	}
	if discountedItems < 0 || discountedItems > 100000000 || math.IsNaN(km) || math.IsInf(km, 0) || km < 0 || km > 1000 {
		return 0, 0, errors.New("invalid payout inputs")
	}
	if p.Mode == "salary" {
		incentive = p.IncentiveCents + (discountedItems*p.IncentivePermille+500)/1000
		amount = incentive
	} else {
		amount = p.BaseCents + (int64(math.Round(km*1000))*p.PerKmCents+500)/1000
	}
	if amount > 100000000 {
		return 0, 0, errors.New("payout exceeds limit")
	}
	return
}
func (p Policy) CancellationPay(stage string, incentive int64) int64 {
	c := p.Cancellation[stage]
	if p.Mode == "salary" {
		return (incentive*c.IncentivePermille + 500) / 1000
	}
	return c.FixedCents
}
