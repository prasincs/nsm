package nsm

import (
	"bytes"
	"errors"
	"os"
	"sync"
	"syscall"
	"testing"

	"github.com/hf/nsm/request"
)

// mockFileDescriptor implements FileDescriptor for testing
type mockFileDescriptor struct {
	fd       uintptr
	closed   bool
	closeErr error
}

func (m *mockFileDescriptor) Fd() uintptr {
	return m.fd
}

func (m *mockFileDescriptor) Close() error {
	if m.closed {
		return os.ErrClosed
	}
	m.closed = true
	return m.closeErr
}

// TestSendEmptyBufferValidation tests the fix for potential panics on empty slices
func TestSendEmptyBufferValidation(t *testing.T) {
	opts := Options{
		Open: func() (FileDescriptor, error) {
			return &mockFileDescriptor{fd: 1}, nil
		},
		Syscall: func(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, err syscall.Errno) {
			return 0, 0, 0
		},
	}

	tests := []struct {
		name    string
		req     []byte
		res     []byte
		wantErr string
	}{
		{
			name:    "empty request buffer should error",
			req:     []byte{},
			res:     make([]byte, 100),
			wantErr: "request buffer is empty",
		},
		{
			name:    "empty response buffer should error",
			req:     make([]byte, 100),
			res:     []byte{},
			wantErr: "response buffer is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := send(opts, 1, tt.req, tt.res)
			if err == nil {
				t.Fatal("expected error but got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestOpenSessionErrorHandling tests that OpenSession returns nil on error
func TestOpenSessionErrorHandling(t *testing.T) {
	t.Run("open failure returns nil session", func(t *testing.T) {
		expectedErr := errors.New("failed to open device")
		opts := Options{
			Open: func() (FileDescriptor, error) {
				return nil, expectedErr
			},
		}

		sess, err := OpenSession(opts)
		if sess != nil {
			t.Error("expected nil session on error, got non-nil")
		}
		if err != expectedErr {
			t.Errorf("got error %v, want %v", err, expectedErr)
		}
	})

	t.Run("successful open returns session", func(t *testing.T) {
		opts := Options{
			Open: func() (FileDescriptor, error) {
				return &mockFileDescriptor{fd: 1}, nil
			},
		}

		sess, err := OpenSession(opts)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if sess == nil {
			t.Error("expected non-nil session")
		}
		if sess.fd == nil {
			t.Error("expected non-nil file descriptor")
		}
	})
}

// TestSessionCloseNilSafety tests Close with nil session and nil fd
func TestSessionCloseNilSafety(t *testing.T) {
	t.Run("close on nil session", func(t *testing.T) {
		var sess *Session
		err := sess.Close()
		if err != nil {
			t.Errorf("expected nil error for nil session, got %v", err)
		}
	})

	t.Run("close on session with nil fd", func(t *testing.T) {
		sess := &Session{}
		err := sess.Close()
		if err != nil {
			t.Errorf("expected nil error for nil fd, got %v", err)
		}
	})

	t.Run("close sets fields to nil", func(t *testing.T) {
		fd := &mockFileDescriptor{fd: 1}
		sess := &Session{
			fd: fd,
			reqpool: &sync.Pool{
				New: func() any {
					return bytes.NewBuffer(make([]byte, 0, maxRequestSize))
				},
			},
			respool: &sync.Pool{
				New: func() any {
					return make([]byte, maxResponseSize)
				},
			},
		}

		err := sess.Close()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify cleanup happened
		if sess.fd != nil {
			t.Error("fd should be nil after close")
		}
		if sess.reqpool != nil {
			t.Error("reqpool should be nil after close")
		}
		if sess.respool != nil {
			t.Error("respool should be nil after close")
		}
		if !fd.closed {
			t.Error("underlying fd should be closed")
		}
	})
}

// TestSendNilValidation tests nil request validation
func TestSendNilValidation(t *testing.T) {
	sess := &Session{
		fd: &mockFileDescriptor{fd: 1},
		reqpool: &sync.Pool{
			New: func() any {
				return bytes.NewBuffer(make([]byte, 0, maxRequestSize))
			},
		},
	}

	_, err := sess.Send(nil)
	if err == nil {
		t.Error("expected error for nil request")
	}
	expected := "request cannot be nil"
	if err.Error() != expected {
		t.Errorf("got error %q, want %q", err.Error(), expected)
	}
}

// TestSendClosedSession tests behavior on closed session
func TestSendClosedSession(t *testing.T) {
	t.Run("send on session with nil fd", func(t *testing.T) {
		sess := &Session{} // nil fd
		_, err := sess.Send(&request.DescribeNSM{})
		if err != ErrSessionClosed {
			t.Errorf("got error %v, want %v", err, ErrSessionClosed)
		}
	})

	t.Run("send on closed session", func(t *testing.T) {
		sess := &Session{
			fd: &mockFileDescriptor{fd: 1},
			// pools are nil - simulates closed session
		}
		_, err := sess.Send(&request.DescribeNSM{})
		if err != ErrSessionClosed {
			t.Errorf("got error %v, want %v", err, ErrSessionClosed)
		}
	})
}

// TestPoolTypeSafety tests safe type assertions for pools
func TestPoolTypeSafety(t *testing.T) {
	// Create a pool that returns wrong type
	badReqPool := &sync.Pool{
		New: func() any {
			return "wrong type" // Should be *bytes.Buffer
		},
	}

	sess := &Session{
		fd:      &mockFileDescriptor{fd: 1},
		reqpool: badReqPool,
		respool: &sync.Pool{
			New: func() any {
				return make([]byte, maxResponseSize)
			},
		},
	}

	_, err := sess.Send(&request.DescribeNSM{})
	if err == nil {
		t.Error("expected error for wrong pool type")
	}
	expected := "pool returned unexpected type string"
	if err.Error() != expected {
		t.Errorf("got error %q, want %q", err.Error(), expected)
	}

	// Test wrong response pool type
	badResPool := &sync.Pool{
		New: func() any {
			return make([]int, 10) // Should be []byte
		},
	}

	sess2 := &Session{
		fd: &mockFileDescriptor{fd: 1},
		reqpool: &sync.Pool{
			New: func() any {
				return bytes.NewBuffer(make([]byte, 0, maxRequestSize))
			},
		},
		respool: badResPool,
	}

	_, err = sess2.Send(&request.DescribeNSM{})
	if err == nil {
		t.Error("expected error for wrong response pool type")
	}
	expected = "pool returned unexpected type []int"
	if err.Error() != expected {
		t.Errorf("got error %q, want %q", err.Error(), expected)
	}
}

// TestErrorTypes tests custom error type formatting
func TestErrorTypes(t *testing.T) {
	t.Run("ErrorIoctlFailed", func(t *testing.T) {
		err := &ErrorIoctlFailed{Errno: syscall.EINVAL}
		expected := "ioctl failed on device with errno invalid argument"
		if err.Error() != expected {
			t.Errorf("got %q, want %q", err.Error(), expected)
		}
	})

	t.Run("ErrorGetRandomFailed with error code", func(t *testing.T) {
		err := &ErrorGetRandomFailed{ErrorCode: "InvalidRequest"}
		expected := "GetRandom failed with error code InvalidRequest"
		if err.Error() != expected {
			t.Errorf("got %q, want %q", err.Error(), expected)
		}
	})

	t.Run("ErrorGetRandomFailed without error code", func(t *testing.T) {
		err := &ErrorGetRandomFailed{}
		expected := "GetRandom response did not include random bytes"
		if err.Error() != expected {
			t.Errorf("got %q, want %q", err.Error(), expected)
		}
	})
}

// TestReadClosedSession tests Read method on closed session
func TestReadClosedSession(t *testing.T) {
	sess := &Session{
		reqpool: &sync.Pool{
			New: func() any {
				return bytes.NewBuffer(make([]byte, 0, maxRequestSize))
			},
		},
		respool: &sync.Pool{
			New: func() any {
				return make([]byte, maxResponseSize)
			},
		},
	}

	buf := make([]byte, 32)
	n, err := sess.Read(buf)
	if err != ErrSessionClosed {
		t.Errorf("got error %v, want %v", err, ErrSessionClosed)
	}
	if n != 0 {
		t.Errorf("expected 0 bytes read, got %d", n)
	}
}

// TestRequestSizeValidation tests request size limits
func TestRequestSizeValidation(t *testing.T) {
	// Create a mock request that encodes to a large size
	largeReq := &request.Attestation{
		UserData: make([]byte, maxRequestSize), // This will exceed limit when encoded
	}

	sess := &Session{
		fd: &mockFileDescriptor{fd: 1},
		reqpool: &sync.Pool{
			New: func() any {
				return bytes.NewBuffer(make([]byte, 0, maxRequestSize))
			},
		},
		respool: &sync.Pool{
			New: func() any {
				return make([]byte, maxResponseSize)
			},
		},
	}

	_, err := sess.Send(largeReq)
	if err == nil {
		t.Error("expected error for oversized request")
	}
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
	// The exact error depends on CBOR encoding, but should contain size information
	t.Logf("Got expected error for large request: %v", err)
}