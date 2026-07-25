package server

import (
	"fmt"
	"time"
)

type membershipPeriod struct {
	Start   time.Time
	Expires time.Time
}

func membershipDurationDays(plan string) (int, error) {
	switch plan {
	case "vip_month":
		return 30, nil
	case "vip_quarter":
		return 90, nil
	case "vip_year":
		return 365, nil
	default:
		return 0, fmt.Errorf("unsupported membership plan %q", plan)
	}
}

func calculateMembershipPeriod(plan string, activation time.Time, currentExpiry *time.Time) (membershipPeriod, error) {
	days, err := membershipDurationDays(plan)
	if err != nil {
		return membershipPeriod{}, err
	}
	base := activation
	if currentExpiry != nil && currentExpiry.After(activation) {
		base = *currentExpiry
	}
	return membershipPeriod{
		Start:   activation,
		Expires: base.AddDate(0, 0, days),
	}, nil
}
