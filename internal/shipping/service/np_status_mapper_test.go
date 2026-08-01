package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapNPStatus(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     string
		expectedStatus string
		expectDelivery bool
	}{
		{"Delivered (9)", "9", ShipmentStatusDelivered, true},
		{"Delivered (10)", "10", ShipmentStatusDelivered, true},
		{"Delivered (11)", "11", ShipmentStatusDelivered, true},
		{"Shipped (1)", "1", ShipmentStatusShipped, false},
		{"Shipped (7)", "7", ShipmentStatusShipped, false},
		{"Shipped (101)", "101", ShipmentStatusShipped, false},
		{"Returned (103)", "103", ShipmentStatusReturned, false},
		{"Returned (104)", "104", ShipmentStatusReturned, false},
		{"Unknown (999)", "999", ShipmentStatusUnknown, false},
		{"Empty string", "", ShipmentStatusUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, isDelivered := MapNPStatus(tt.statusCode)
			assert.Equal(t, tt.expectedStatus, status)
			assert.Equal(t, tt.expectDelivery, isDelivered)
		})
	}
}
