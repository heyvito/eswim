package proto

import "github.com/heyvito/eswim/internal/fsm"

type SyncMode uint8

// stringer can be installed from golang.org/x/tools/cmd/stringer@latest
//go:generate stringer -type=SyncMode

const (
	SyncModeFirstPhase   SyncMode = 0b00
	SyncModeSecondPhase  SyncMode = 0b01
	SyncModeBootstrap    SyncMode = 0b10
	SyncModeBootstrapAck SyncMode = 0b11
)

// Sync represents a Sync request for another node. It contains all Node objects
// known by this (or the other) member.
type Sync struct {
	Nodes []Node
	Mode  SyncMode
}

func (s Sync) OpCode() OpCode { return SYNC }

func (s Sync) RequiredSize() int {
	return 2 + reduce(s.Nodes, func(node Node, acc int) int {
		return acc + node.RequiredSize()
	})
}

func (s Sync) Encode(into []byte) {
	w := newWriter(into).
		u8(uint8(s.Mode)).
		u8(uint8(len(s.Nodes)))
	forEachCast(s.Nodes, w.encoderNoChain)
}

// fsmStates: syncDecoder mode, size, nodes

type syncDecoderState uint8

const (
	syncDecoderStateMode syncDecoderState = iota
	syncDecoderStateSize
	syncDecoderStateNodes
)

// fsmStatesEnd

type syncDecoderContext struct {
	nodeDecoder *nodeDecoderT
}

func (s *syncDecoderContext) Init()  { s.nodeDecoder = nodeDecoder.New() }
func (s *syncDecoderContext) Reset() { s.nodeDecoder.Reset() }

var syncDecoder = fsm.Def[Sync, syncDecoderState, syncDecoderContext]{
	Feed: func(f *fsm.FSM[Sync, syncDecoderState, syncDecoderContext], state syncDecoderState, ctx *syncDecoderContext, b byte) error {
		switch state {
		case syncDecoderStateMode:
			f.Value.Mode = SyncMode(b)
			f.Transition(syncDecoderStateSize)

		case syncDecoderStateSize:
			f.Value.Nodes = make([]Node, 0, b)
			if b == 0 {
				return fsm.Done
			}
			f.TransitionSize(syncDecoderStateNodes, cap(f.Value.Nodes))

		case syncDecoderStateNodes:
			n, err := ctx.nodeDecoder.Feed(b)
			if err != nil {
				return err
			}

			if n != nil {
				f.Value.Nodes = append(f.Value.Nodes, *n)
				f.Size--
			}
			if f.Size == 0 {
				return fsm.Done
			}
		}
		return nil
	},
}
