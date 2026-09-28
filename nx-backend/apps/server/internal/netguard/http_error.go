package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// NormalizeHTTPError preserves transport detail across Go versions whose client timeout errors lack Is.
func NormalizeHTTPError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return fmt.Errorf("%w: %w", context.DeadlineExceeded, err)
	}
	return err
}
