package fsm

import "encoding/binary"

var DefaultByteOrder binary.ByteOrder = binary.BigEndian

type ExportedFSMDef[T any] interface {
	New() ExportedFSM[T]
}

type exportedDefinitionAdapter[T any, S ~uint8, C any] struct {
	def Def[T, S, C]
}

func (e exportedDefinitionAdapter[T, S, C]) New() ExportedFSM[T] {
	return e.def.New()
}

// Def represents a definition of a decoder that emits a result T based
// on a set of states S, and an optional custom state C. In case C is not
// required, use struct{} as its value. C may implement Resetter or Initializer
// as it deems fit. When implementing Initializer, Init will be called once
// as part of the FSM initialization. When implementing Resetter, Reset will be
// called whenever it is required.
type Def[T any, S ~uint8, C any] struct {
	// InitialSize represents the amount of bytes the decoder must consume
	// before invoking the delegated feed function with the first S state as the
	// current state. Optional.
	InitialSize int

	// InitialState represents the initial state the FSM must be after a reset
	// or initialization. Optional. By default, it will use S's zero value.
	InitialState S

	// Feed represents a function responsible for either handling each of the
	// incoming bytes, or a single call (depending on usage of
	// transition/initialSize). The function may return either Done, nil, or an
	// arbitrary error. Returning Done indicates that the FSM is done consuming
	// data, and will cause the upstream mechanism to return the associated
	// value T to the caller, with a nil error. Returning nil returns a nil
	// value T and a nil error to the caller, indicating that more data is
	// expected. Otherwise, returning an arbitrary error resets the FSM state
	// and returns the same error to the caller. Required.
	Feed func(f *FSM[T, S, C], state S, ctx *C, b byte) error

	// Prepare represents an optional function to be called once after
	// initialization that allows the parser to prepare its initial state out
	// of the main initialization handler. This function is not called by the
	// FSM itself, and any component interested in using it must explicitly call
	// it through FSM.Prepare.
	Prepare func(f *FSM[T, S, C], state S, ctx *C)

	// ByteOrder defines the byte order for this FSM. If unset, uses
	// fsm.DefaultByteOrder.
	ByteOrder binary.ByteOrder
}

// New returns a new FSM instance based on this definition. ByteOrder, if
// not provided to Def, will assume DefaultByteOrder as its value.
func (d Def[T, S, C]) New() *FSM[T, S, C] {
	var fsm = &FSM[T, S, C]{
		feedFn:          d.Feed,
		prepareFn:       d.Prepare,
		byteOrder:       d.ByteOrder,
		initialSize:     d.InitialSize,
		initialState:    d.InitialState,
		mustSatisfySize: false,
		context:         new(C),
	}

	if fsm.byteOrder == nil {
		fsm.byteOrder = DefaultByteOrder
	}

	if i, ok := any(fsm.context).(Initializer); ok {
		i.Init()
	}
	fsm.Init()
	return fsm
}

// NewGeneric returns a new FSM instance of this Def as an AnyDecoder.
func (d Def[T, S, C]) NewGeneric() AnyDecoder {
	return d.New()
}

// IntoExported returns the current fsm Def as an exported counterpart, allowing
// it to be used across packages without exposing internal types.
func (d Def[T, S, C]) IntoExported() ExportedFSMDef[T] {
	return exportedDefinitionAdapter[T, S, C]{d}
}
