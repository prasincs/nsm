package request

import (
	"fmt"
	"reflect"
	"testing"
)

// TestRequestEncoding tests all request types implement proper encoding
func TestRequestEncoding(t *testing.T) {
	t.Run("DescribeNSM returns string", func(t *testing.T) {
		req := &DescribeNSM{}
		encoded := req.Encoded()
		expected := "DescribeNSM"
		if encoded != expected {
			t.Errorf("got %v, want %v", encoded, expected)
		}
	})

	t.Run("GetRandom returns string", func(t *testing.T) {
		req := &GetRandom{}
		encoded := req.Encoded()
		expected := "GetRandom"
		if encoded != expected {
			t.Errorf("got %v, want %v", encoded, expected)
		}
	})

	t.Run("DescribePCR returns map", func(t *testing.T) {
		req := &DescribePCR{Index: 5}
		encoded := req.Encoded()
		expectedMap, ok := encoded.(map[string]*DescribePCR)
		if !ok {
			t.Errorf("expected map[string]*DescribePCR, got %T", encoded)
			return
		}
		if expectedMap["DescribePCR"].Index != 5 {
			t.Errorf("got Index %d, want 5", expectedMap["DescribePCR"].Index)
		}
	})

	t.Run("Attestation returns map", func(t *testing.T) {
		req := &Attestation{
			Nonce:     []byte{1, 2},
			UserData:  []byte{3, 4},
			PublicKey: []byte{5, 6},
		}
		encoded := req.Encoded()
		expectedMap, ok := encoded.(map[string]*Attestation)
		if !ok {
			t.Errorf("expected map[string]*Attestation, got %T", encoded)
			return
		}
		att := expectedMap["Attestation"]
		if !reflect.DeepEqual(att.Nonce, []byte{1, 2}) {
			t.Errorf("got Nonce %v, want [1 2]", att.Nonce)
		}
	})

	// Test that all request types can be encoded without panicking
	requests := []Request{
		&DescribeNSM{},
		&DescribePCR{Index: 0},
		&ExtendPCR{Index: 0, Data: []byte{}},
		&LockPCR{Index: 0},
		&LockPCRs{Range: 0},
		&GetRandom{},
		&Attestation{},
	}

	for i, req := range requests {
		t.Run(fmt.Sprintf("request_%d_encodes", i), func(t *testing.T) {
			encoded := req.Encoded()
			if encoded == nil {
				t.Errorf("%T.Encoded() returned nil", req)
			}
		})
	}
}
