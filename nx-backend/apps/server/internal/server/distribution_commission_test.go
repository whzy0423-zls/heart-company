package server

import (
	"os"
	"strings"
	"testing"
)

func TestPaidSettlementContainsDistributionCommissionHook(t *testing.T) {
	b, err := os.ReadFile("app_payment_settlement.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"generateDistributionCommissionsTx", "ON CONFLICT(order_id,agent_id) DO NOTHING", "distribution_commission_records"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q", want)
		}
	}
}
