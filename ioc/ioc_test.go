package ioc

import (
	"testing"
)

// TestCommand tests the IOCTL command calculation
func TestCommand(t *testing.T) {
	// Test that Command returns a non-zero value for valid inputs
	result := Command(READ|WRITE, 0x0A, 0, 16)
	if result == 0 {
		t.Error("Command should return non-zero value")
	}

	// Test that different inputs produce different results
	result1 := Command(READ, 0x0A, 0, 16)
	result2 := Command(WRITE, 0x0A, 0, 16)
	if result1 == result2 {
		t.Error("Different directions should produce different commands")
	}

	// Test with actual NSM values used in the main package
	nsmCommand := Command(READ|WRITE, 0x0A, 0, 16)
	if nsmCommand == 0 {
		t.Error("NSM command should be non-zero")
	}
}