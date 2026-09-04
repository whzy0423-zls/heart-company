package realtime

import "testing"

func TestDirectGatewayRequiresProtocolTicket(t *testing.T) {
	if NewDirectGateway(nil, NewDirectHub(), nil) == nil {
		t.Fatal("gateway should be constructible")
	}
}
