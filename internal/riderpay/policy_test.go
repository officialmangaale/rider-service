package riderpay

import "testing"

func TestSalaryIncentiveExcludesDistanceAndSalaryAndCancellationKeepsSalary(t *testing.T) {
	p := Policy{RiderID: "rider", Mode: "salary", SalaryPeriod: "monthly", SalaryCents: 2000000, IncentivePermille: 100, Cancellation: map[string]Cancellation{"picked_up": {IncentivePermille: 500}}}
	amount, incentive, err := p.Quote(42500, 12)
	if err != nil || amount != 4250 || incentive != 4250 {
		t.Fatal(amount, incentive, err)
	}
	if got := p.CancellationPay("picked_up", incentive); got != 2125 || p.SalaryCents != 2000000 {
		t.Fatal(got, p.SalaryCents)
	}
	p.BaseCents = 100
	if p.Validate() == nil {
		t.Fatal("salary plus base accepted")
	}
}
func TestPerDeliveryOnlyBaseAndDistance(t *testing.T) {
	p := Policy{RiderID: "rider", Mode: "per_delivery", SalaryPeriod: "monthly", BaseCents: 2000, PerKmCents: 500, Cancellation: map[string]Cancellation{"rider_assigned": {FixedCents: 1000}}}
	pay, _, err := p.Quote(500000, 3)
	if err != nil || pay != 3500 || p.CancellationPay("rider_assigned", 0) != 1000 {
		t.Fatal(pay, err)
	}
	p.IncentivePermille = 10
	if p.Validate() == nil {
		t.Fatal("per-delivery plus salary incentive accepted")
	}
}
