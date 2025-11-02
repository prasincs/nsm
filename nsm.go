// Package nsm implements the Nitro Security Module interface.
package nsm

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/fxamacker/cbor/v2"
	"github.com/hf/nsm/ioc"
	"github.com/hf/nsm/request"
	"github.com/hf/nsm/response"
)

const (
	maxRequestSize  = 0x1000
	maxResponseSize = 0x3000
	ioctlMagic      = 0x0A
)

// FileDescriptor is a generic file descriptor interface that can be closed.
// os.File conforms to this interface.
type FileDescriptor interface {
	// Provide the uintptr for the file descriptor.
	Fd() uintptr

	// Close the file descriptor.
	Close() error
}

// Options for the opening of the NSM session.
type Options struct {
	// A function that opens the NSM device file `/dev/nsm`.
	Open func() (FileDescriptor, error)

	// A function that implements the syscall.Syscall interface and is able to
	// work with the file descriptor returned from `Open` as the `a1` argument.
	Syscall func(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, err syscall.Errno)
}

// DefaultOptions can be used to open the default NSM session on `/dev/nsm`.
var DefaultOptions = Options{
	Open: func() (FileDescriptor, error) {
		return os.Open("/dev/nsm")
	},
	Syscall: syscall.Syscall,
}

// ErrorIoctlFailed is an error returned when the underlying ioctl syscall has
// failed.
type ErrorIoctlFailed struct {
	// Errno is the errno returned by the syscall.
	Errno syscall.Errno
}

// Error returns the formatted string.
func (err *ErrorIoctlFailed) Error() string {
	return fmt.Sprintf("ioctl failed on device with errno %v", err.Errno)
}

// ErrorGetRandomFailed is an error returned when the GetRandom request as part
// of a `Read` has failed with an error code, is invalid or did not return any
// random bytes.
type ErrorGetRandomFailed struct {
	ErrorCode response.ErrorCode
}

// Error returns the formatted string.
func (err *ErrorGetRandomFailed) Error() string {
	if err.ErrorCode != "" {
		return fmt.Sprintf("GetRandom failed with error code %v", err.ErrorCode)
	}

	return "GetRandom response did not include random bytes"
}

var (
	// ErrSessionClosed is returned when the session is in a closed state.
	ErrSessionClosed error = errors.New("Session is closed")
)

// A Session is used to interact with the NSM.
type Session struct {
	fd      FileDescriptor
	options Options
	reqpool *sync.Pool
	respool *sync.Pool
}

type ioctlMessage struct {
	Request  syscall.Iovec
	Response syscall.Iovec
}

func send(options Options, fd uintptr, req []byte, res []byte) ([]byte, error) {
	// Validate slices to prevent panic on empty slices
	if len(req) == 0 {
		return nil, errors.New("request buffer is empty")
	}
	if len(res) == 0 {
		return nil, errors.New("response buffer is empty")
	}

	iovecReq := syscall.Iovec{
		Base: &req[0],
	}
	iovecReq.SetLen(len(req))

	iovecRes := syscall.Iovec{
		Base: &res[0],
	}
	iovecRes.SetLen(len(res))

	msg := ioctlMessage{
		Request:  iovecReq,
		Response: iovecRes,
	}

	// IOCTL calls to /dev/nsm are synchronous and block until the NSM device
	// responds. Each call performs a context switch to the Nitro hypervisor.
	// Reference: https://github.com/aws/aws-nitro-enclaves-nsm-api
	_, _, err := options.Syscall(
		syscall.SYS_IOCTL,
		fd,
		uintptr(ioc.Command(ioc.READ|ioc.WRITE, ioctlMagic, 0, uint(unsafe.Sizeof(msg)))),
		uintptr(unsafe.Pointer(&msg)),
	)

	if err != 0 {
		return nil, &ErrorIoctlFailed{
			Errno: err,
		}
	}

	// Validate response length to prevent buffer overrun
	if msg.Response.Len > uint64(len(res)) {
		return nil, fmt.Errorf("response length %d exceeds buffer size %d", msg.Response.Len, len(res))
	}

	return res[:msg.Response.Len], nil
}

// OpenSession opens a new session with the provided options.
func OpenSession(opts Options) (*Session, error) {
	// Set defaults if not provided
	if opts.Open == nil {
		opts.Open = DefaultOptions.Open
	}
	if opts.Syscall == nil {
		opts.Syscall = DefaultOptions.Syscall
	}

	fd, err := opts.Open()
	if err != nil {
		return nil, err
	}

	session := &Session{
		options: opts,
		fd:      fd,
	}
	session.reqpool = &sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, maxRequestSize))
		},
	}
	session.respool = &sync.Pool{
		New: func() interface{} {
			return make([]byte, maxResponseSize)
		},
	}

	return session, nil
}

// OpenDefaultSession opens a new session with the default options.
func OpenDefaultSession() (*Session, error) {
	return OpenSession(DefaultOptions)
}

// Close this session. It is not thread safe to Close while other threads are
// Read-ing or Send-ing.
func (sess *Session) Close() error {
	if sess == nil || sess.fd == nil {
		return nil
	}

	var err error
	
	// Always clear the session state to prevent reuse, even on panic
	defer func() {
		sess.fd = nil
		sess.reqpool = nil
		sess.respool = nil
	}()
	
	// Close the file descriptor
	err = sess.fd.Close()
	return err
}

// Send an NSM request to the device and await its response. 
// IOCTL operations are synchronous and expensive - each call blocks and
// context-switches to the Nitro hypervisor. Use sparingly.
// Safe to call from multiple goroutines, but not while Close-ing.
// Each call reserves up to 16KB of memory.
func (sess *Session) Send(req request.Request) (response.Response, error) {
	if req == nil {
		return response.Response{}, fmt.Errorf("request cannot be nil")
	}

	reqbRaw := sess.reqpool.Get()
	reqb, ok := reqbRaw.(*bytes.Buffer)
	if !ok {
		sess.reqpool.Put(reqbRaw)
		return response.Response{}, fmt.Errorf("pool returned unexpected type %T", reqbRaw)
	}
	defer sess.reqpool.Put(reqb)

	reqb.Reset()
	encoder := cbor.NewEncoder(reqb)
	err := encoder.Encode(req.Encoded())
	if err != nil {
		return response.Response{}, fmt.Errorf("failed to encode request: %w", err)
	}

	// Validate encoded request size
	if reqb.Len() > maxRequestSize {
		return response.Response{}, fmt.Errorf("encoded request size %d exceeds maximum %d", reqb.Len(), maxRequestSize)
	}

	resbRaw := sess.respool.Get()
	resb, ok := resbRaw.([]byte)
	if !ok {
		sess.respool.Put(resbRaw)
		return response.Response{}, fmt.Errorf("pool returned unexpected type %T", resbRaw)
	}
	defer sess.respool.Put(resb)

	return sess.sendMarshaled(reqb, resb)
}

func (sess *Session) sendMarshaled(reqb *bytes.Buffer, resb []byte) (response.Response, error) {
	res := response.Response{}

	if sess == nil || sess.fd == nil {
		return res, ErrSessionClosed
	}

	resb, err := send(sess.options, sess.fd.Fd(), reqb.Bytes(), resb)
	if err != nil {
		return res, err
	}

	// Validate response data before unmarshaling
	if len(resb) == 0 {
		return res, fmt.Errorf("empty response from NSM device")
	}
	if len(resb) > maxResponseSize {
		return res, fmt.Errorf("response size %d exceeds maximum %d", len(resb), maxResponseSize)
	}

	err = cbor.Unmarshal(resb, &res)
	if err != nil {
		return res, fmt.Errorf("failed to unmarshal CBOR response: %w", err)
	}

	return res, nil
}

// Read entropy from the NSM device. This method blocks until the entire slice
// is filled with cryptographically secure random bytes from the NSM.
// Each GetRandom request is a synchronous IOCTL that context-switches to the
// Nitro hypervisor, making it expensive. Consider using returned entropy to
// seed a DRBG rather than calling repeatedly.
// Safe to call from multiple goroutines, but not while Close-ing.
func (sess *Session) Read(into []byte) (int, error) {
	reqb := sess.reqpool.Get().(*bytes.Buffer)
	defer sess.reqpool.Put(reqb)

	getRandom := request.GetRandom{}

	reqb.Reset()
	encoder := cbor.NewEncoder(reqb)
	err := encoder.Encode(getRandom.Encoded())
	if err != nil {
		return 0, err
	}

	resbRaw := sess.respool.Get()
	resb, ok := resbRaw.([]byte)
	if !ok {
		sess.respool.Put(resbRaw)
		return 0, fmt.Errorf("pool returned unexpected type %T", resbRaw)
	}
	defer sess.respool.Put(resb)

	for i := 0; i < len(into); {
		res, err := sess.sendMarshaled(reqb, resb)

		if err != nil {
			return i, err
		}

		if res.Error != "" || res.GetRandom == nil || res.GetRandom.Random == nil || len(res.GetRandom.Random) == 0 {
			return i, &ErrorGetRandomFailed{
				ErrorCode: res.Error,
			}
		}

		copied := copy(into[i:], res.GetRandom.Random)
		if copied == 0 {
			return i, &ErrorGetRandomFailed{
				ErrorCode: "no data received",
			}
		}
		i += copied
	}

	return len(into), nil
}
