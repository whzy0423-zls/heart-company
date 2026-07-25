package miniapp

import (
	"context"
	"errors"
	"testing"
)

func TestInsertBookingRequiresPositiveForeignKeysAndQueryTarget(t *testing.T) {
	store := &Store{}
	valid := BookingInput{ContactName: "张三", Phone: "13812345678"}
	if _, err := store.InsertBooking(context.Background(), nil, 0, valid, 1); !errors.Is(err, ErrInvalidBooking) {
		t.Fatalf("user id error = %v, want ErrInvalidBooking", err)
	}
	if _, err := store.InsertBooking(context.Background(), nil, 1, valid, 0); !errors.Is(err, ErrInvalidBooking) {
		t.Fatalf("signup id error = %v, want ErrInvalidBooking", err)
	}
	if _, err := store.InsertBooking(context.Background(), nil, 1, valid, 2); !errors.Is(err, ErrNilDBTX) {
		t.Fatalf("nil query target error = %v, want ErrNilDBTX", err)
	}
}

func TestNormalizeBookingInputTrimsAndNormalizesPhone(t *testing.T) {
	got, err := normalizeBookingInput(BookingInput{
		Kind:          " consult ",
		ContactName:   " 张三 ",
		Phone:         " 138-1234 5678 ",
		Intent:        " 了解咨询 ",
		PreferredTime: " 周六下午 ",
		Message:       " 请联系我 ",
	})
	if err != nil {
		t.Fatalf("normalizeBookingInput() error = %v", err)
	}
	want := BookingInput{Kind: "consult", ContactName: "张三", Phone: "13812345678", Intent: "了解咨询", PreferredTime: "周六下午", Message: "请联系我"}
	if got != want {
		t.Fatalf("normalized = %+v, want %+v", got, want)
	}

	defaultKind, err := normalizeBookingInput(BookingInput{ContactName: "张三", Phone: "13812345678"})
	if err != nil || defaultKind.Kind != "consult" {
		t.Fatalf("default kind = %+v, %v", defaultKind, err)
	}
}
