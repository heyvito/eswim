package fsm

import (
	"encoding/binary"
	"errors"
)

// Initializer allows FSMs to automatically initialize custom contexts (provided
// as type C to the FSM). Contexts implementing this interface will have Init
// called during the FSM initialization.
type Initializer interface {
	Init()
}

// Resetter allows FSMs to automatically reset custom contexts (provided
// as type C to the FSM). Contexts implementing this interface will have Reset
// called whenever the FSM is resetting its internal values to their initial
// states.
type Resetter interface {
	Reset()
}

// ContextAccessor implements a way to access an FSM's internal context (C)
// without requiring C to be known.
type ContextAccessor interface {
	Context() any
}

// AnyDecoder represents a generic instance of decoderImpl that only
// implements FeedAny.
type AnyDecoder interface {
	// FeedAny acts like FSM.Feed, but returns any. See FSM.Feed for more info.
	FeedAny(b byte) (any, error)
	Prepare()
}

// AnyDefinition represents a generic Def, useful for facilities that need a
// generic representation of a FSM definition, without requiring strict type
// definition.
type AnyDefinition interface {
	NewGeneric() AnyDecoder
}

// ExportedFSM provides an adapter to allow FSMs to be used across packages
// without exposing explicit types used internally.
type ExportedFSM[T any] interface {
	Init()
	Feed(b byte) (*T, error)
}

// FSM represents a finite state machine built from a Def.
type FSM[T any, S ~uint8, C any] struct {
	Value   *T
	Size    int
	Payload []byte

	feedFn          func(f *FSM[T, S, C], state S, ctx *C, b byte) error
	prepareFn       func(f *FSM[T, S, C], state S, ctx *C)
	byteOrder       binary.ByteOrder
	initialSize     int
	initialState    S
	state           S
	context         *C
	currentByte     byte
	mustSatisfySize bool
}

// Append appends the current byte being fed to the internal payload buffer,
// and decrements the size counter.
func (i *FSM[T, S, C]) Append() {
	i.Payload = append(i.Payload, i.currentByte)
	i.Size--
}

// ResetPayload clears the internal payload buffer, retaining its capacity.
func (i *FSM[T, S, C]) ResetPayload() {
	i.Payload = i.Payload[:0]
}

// Init is an internal method that's responsible for initializing the current
// decoder. It sets all required internal state, and automatically calls the
// initializer for any custom data associated, if any.
func (i *FSM[T, S, C]) Init() {
	i.Size = i.initialSize
	i.mustSatisfySize = false
	if i.initialSize != 0 {
		i.SatisfySize()
	}
	if r, ok := any(i.context).(Resetter); ok {
		r.Reset()
	}
	i.currentByte = 0
	i.state = i.initialState
	i.ResetPayload()
	i.Value = new(T)
}

// Feed feeds a given byte to the decoder. It automatically invokes all required
// delegates. feed either return nil, if no value was produced, or a T pointer,
// in case the feed operation yields a value.
func (i *FSM[T, S, C]) Feed(b byte) (*T, error) {
	i.currentByte = b
	if i.mustSatisfySize {
		if !i.SizeSatisfied() {
			return nil, nil
		}
		i.mustSatisfySize = false
	}
	err := i.feedFn(i, i.state, i.context, b)
	if errors.Is(err, Done) {
		defer i.Reset()
		return copyIndirect(i.Value), nil
	}

	return nil, nil
}

// SatisfySize indicates that the decoder must not call the delegate feed
// function until payload receives size bytes.
func (i *FSM[T, S, C]) SatisfySize() {
	i.mustSatisfySize = true
}

// FeedAny acts like feed, but returns `any` instead of *T. This is useful
// for non-specialized callers, which cannot handle the static typed form of
// feed. See feed for more information.
func (i *FSM[T, S, C]) FeedAny(b byte) (any, error) {
	pkt, err := i.Feed(b)
	var anyPkt any

	// This seems odd at first, but it helps us avoid returning
	// anyInterface(nil) to the caller (which is not equal to nil)
	if pkt == nil {
		anyPkt = nil
	} else {
		anyPkt = pkt
	}

	return anyPkt, err
}

// Reset resets the current decoder to its initial state.
func (i *FSM[T, S, C]) Reset() { i.Init() }

// Transition transitions the current decoder state to the provided value.
func (i *FSM[T, S, C]) Transition(next S) {
	i.TransitionSize(next, 0)
}

// U16 converts the two initial bytes present in payload into a 16-bit unsigned
// integer using the defined binary order. Panics in case len(payload) < 2
func (i *FSM[T, S, C]) U16() uint16 {
	return i.byteOrder.Uint16(i.Payload)
}

// U32 converts the four initial bytes present in payload into a big-endian
// 32-bit unsigned integer. Panics in case len(payload) < 4
func (i *FSM[T, S, C]) U32() uint32 {
	return i.byteOrder.Uint32(i.Payload)
}

// CopyPayload copies bytes present in payload to another provided buffer.
// It will copy at most min(len(payload), len(into)) bytes. The provided buffer
// must already a given length (not capacity).
func (i *FSM[T, S, C]) CopyPayload(into []byte) { copy(into, i.Payload) }

// TransitionSize transitions the decoder to a given state, and automatically
// sets its internal size to the provided value, also invoking resetPayload.
func (i *FSM[T, S, C]) TransitionSize(next S, size int) {
	i.Size = size
	i.state = next
	i.ResetPayload()
}

// TransitionSatisfySize acts as transitionSize, except that it also invokes
// SatisfySize, meaning that the next call to the delegate feed function will
// only occur for the provided state S after size bytes have been consumed.
func (i *FSM[T, S, C]) TransitionSatisfySize(next S, size int) {
	i.TransitionSize(next, size)
	i.SatisfySize()
}

// SizeSatisfied invokes append and returns whether the amount of bytes fed up
// to this point has satisfied the size condition set previously.
func (i *FSM[T, S, C]) SizeSatisfied() bool {
	i.Append()
	return i.Size == 0
}

// Context returns the current FSM context (C) without explicitly knowing C.
func (i *FSM[T, S, C]) Context() any { return i.context }

// Prepare can be called by users of this given FSM to have its prepare function
// called, if any was provided to its Def.
func (i *FSM[T, S, C]) Prepare() {
	if i.prepareFn != nil {
		i.prepareFn(i, i.state, i.context)
	}
}

func copyIndirect[T any](obj *T) *T {
	other := *obj
	return &other
}
