package response

import (
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// TestUnmarshalCBORStringResponses tests string-based responses
func TestUnmarshalCBORStringResponses(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(*Response) bool
	}{
		{
			name:    "LockPCR response",
			input:   "LockPCR",
			wantErr: false,
			check: func(r *Response) bool {
				return r.LockPCR != nil && r.LockPCRs == nil
			},
		},
		{
			name:    "LockPCRs response",
			input:   "LockPCRs",
			wantErr: false,
			check: func(r *Response) bool {
				return r.LockPCRs != nil && r.LockPCR == nil
			},
		},
		{
			name:    "unknown string response",
			input:   "UnknownResponse",
			wantErr: true,
			check:   func(r *Response) bool { return true },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := cbor.Marshal(tt.input)
			if err != nil {
				t.Fatalf("failed to marshal test input: %v", err)
			}

			var res Response
			err = res.UnmarshalCBOR(data)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalCBOR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !tt.check(&res) {
				t.Errorf("UnmarshalCBOR() result check failed")
			}
		})
	}
}

// TestUnmarshalCBORMapResponses tests map-based responses
func TestUnmarshalCBORMapResponses(t *testing.T) {
	t.Run("DescribeNSM response", func(t *testing.T) {
		input := mapResponse{
			DescribeNSM: &DescribeNSM{
				VersionMajor: 1,
				VersionMinor: 0,
				ModuleID:     "test-module",
				MaxPCRs:      16,
				Digest:       Digest("SHA256"),
			},
		}

		data, err := cbor.Marshal(input)
		if err != nil {
			t.Fatalf("failed to marshal test input: %v", err)
		}

		var res Response
		err = res.UnmarshalCBOR(data)
		if err != nil {
			t.Errorf("UnmarshalCBOR() error = %v", err)
			return
		}

		if res.DescribeNSM == nil {
			t.Error("expected DescribeNSM to be set")
			return
		}

		if res.DescribeNSM.ModuleID != "test-module" {
			t.Errorf("got ModuleID %q, want %q", res.DescribeNSM.ModuleID, "test-module")
		}
	})

	t.Run("Error response", func(t *testing.T) {
		input := mapResponse{
			Error: "InvalidRequest",
		}

		data, err := cbor.Marshal(input)
		if err != nil {
			t.Fatalf("failed to marshal test input: %v", err)
		}

		var res Response
		err = res.UnmarshalCBOR(data)
		if err != nil {
			t.Errorf("UnmarshalCBOR() error = %v", err)
			return
		}

		if res.Error != "InvalidRequest" {
			t.Errorf("got Error %q, want %q", res.Error, "InvalidRequest")
		}
	})
}

// TestUnmarshalCBORInvalid tests handling of invalid CBOR data
func TestUnmarshalCBORInvalid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "invalid CBOR",
			data: []byte{0xFF, 0xFF, 0xFF},
		},
		{
			name: "empty data",
			data: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var res Response
			err := res.UnmarshalCBOR(tt.data)
			if err == nil {
				t.Error("expected error for invalid CBOR data")
			}
		})
	}
}