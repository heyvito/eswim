package proto

// NOTICE: Do not attempt to simplify type-switches below using `default`.
//         Panicking is intended as a development utility to detect whether
//         All types are being correctly handled.

// { Alive Ml, inc = i } overrides
//   -> { Suspect Mi, inc = j } if i >  j
//   -> { Alive Mi, inc = j }   if i >  j
//   -> { Faulty Mi, inc = j }  if i >  j
//   -> { Join Mi, inc = j }    if i >  j
//   -> { Healthy Mi, inc = j } if i >  j
//   -> { Left Mi, inc = j }    if i >  j
//   -> { Wrap Mi, inc = j }    if i >  j

func (a *Alive) Invalidates(current *Event) bool {
	switch current.EventKind() {
	case EventSuspect, EventAlive, EventJoin, EventHealthy, EventLeft, EventWrap, EventFaulty:
		return a.Incarnation > current.Payload.GetIncarnation()
	}

	panic("Assertion failed: unreachable condition")
}

//{ Suspect Ml, inc = i } overrides
//   -> { Suspect Mi, inc = j } if i >  j
//   -> { Alive Mi, inc = j }   if i >= j
//   -> { Faulty Mi, inc = j }  if i >  j
//   -> { Join Mi, inc = j }    if i >= j
//   -> { Healthy Mi, inc = j } if i >= j
//   -> { Left Mi, inc = j }    if i >  j
//   -> { Wrap Mi, inc = j }    if i >= j

func (s *Suspect) Invalidates(current *Event) bool {
	switch current.EventKind() {
	case EventSuspect, EventFaulty, EventLeft:
		return s.Incarnation > current.Payload.GetIncarnation()
	case EventAlive, EventJoin, EventHealthy, EventWrap:
		return s.Incarnation >= current.Payload.GetIncarnation()
	}

	panic("Assertion failed: unreachable condition")
}

// { Faulty Ml, inc = i } overrides
//   -> { Alive Mi, inc = j }   any j
//   -> { Suspect Mi, inc = j } any j
//   -> { Faulty Mi, inc = j }  any j
//   -> { Join Mi, inc = j }    any j
//   -> { Healthy Mi, inc = j } any j
//   -> { Left Mi, inc = j }    any j
//   -> { Wrap Mi, inc = j }    any j

func (f *Faulty) Invalidates(current *Event) bool {
	switch current.EventKind() {
	case EventSuspect, EventAlive, EventJoin, EventHealthy, EventLeft, EventWrap, EventFaulty:
		return true
	}

	panic("Assertion failed: unreachable condition")
}

// { Join Ml, inc = i } overrides
//   -> { Alive Mi, inc = j }   i >  j
//   -> { Suspect Mi, inc = j } i >  j
//   -> { Faulty Mi, inc = j }  i != i
//   -> { Join Mi, inc = j }    i >  j
//   -> { Healthy Mi, inc = j } i >  j
//   -> { Left Mi, inc = j }    i >  j
//   -> { Wrap Mi, inc = j }    i >  j

func (j *Join) Invalidates(current *Event) bool {
	switch current.EventKind() {
	case EventAlive, EventSuspect, EventJoin, EventHealthy, EventLeft, EventWrap:
		return j.Incarnation > current.Payload.GetIncarnation()
	case EventFaulty:
		return false
	}

	panic("Assertion failed: unreachable condition")
}

// { Healthy Ml, inc = i } overrides
//   -> { Alive Mi, inc = j }   if i >  j
//   -> { Suspect Mi, inc = j } if i >= j
//   -> { Faulty Mi, inc = j }  if i != i
//   -> { Join Mi, inc = j }    if i >= j
//   -> { Healthy Mi, inc = j } if i >  j
//   -> { Left Mi, inc = j }    if i >  j
//   -> { Wrap Mi, inc = j }    if i >  j

func (h *Healthy) Invalidates(current *Event) bool {
	switch current.EventKind() {
	case EventSuspect, EventJoin:
		return h.Incarnation >= current.Payload.GetIncarnation()
	case EventAlive, EventHealthy, EventLeft, EventWrap:
		return h.Incarnation > current.Payload.GetIncarnation()
	case EventFaulty:
		return false
	}

	panic("Assertion failed: unreachable condition")
}

// { Left Ml, inc = i } overrides
//   -> { Alive Mi, inc = j }   if i >= j
//   -> { Suspect Mi, inc = j } if i >= j
//   -> { Faulty Mi, inc = j }  if i != i
//   -> { Join Mi, inc = j }    if i >= j
//   -> { Healthy Mi, inc = j } if i >= j
//   -> { Left Mi, inc = j }    if i >  j
//   -> { Wrap Mi, inc = j }    if i >= j

func (l *Left) Invalidates(current *Event) bool {
	switch current.EventKind() {
	case EventLeft:
		return l.Incarnation > current.Payload.GetIncarnation()
	case EventSuspect, EventJoin, EventAlive, EventHealthy, EventWrap:
		return l.Incarnation >= current.Payload.GetIncarnation()
	case EventFaulty:
		return false
	}

	panic("Assertion failed: unreachable condition")
}

// { Wrap Ml, inc = i } overrides
//   -> { Alive Mi, inc = j }   if i >= j
//   -> { Suspect Mi, inc = j } if i >= j
//   -> { Faulty Mi, inc = j }  if i != i
//   -> { Join Mi, inc = j }    if i >= j
//   -> { Healthy Mi, inc = j } if i >= j
//   -> { Left Mi, inc = j }    if i >  j
//   -> { Wrap Mi, inc = j }    if i >  j

func (w *Wrap) Invalidates(current *Event) bool {
	switch current.EventKind() {
	case EventLeft, EventWrap:
		return w.Incarnation > current.Payload.GetIncarnation()
	case EventAlive, EventSuspect, EventJoin, EventHealthy:
		return w.Incarnation >= current.Payload.GetIncarnation()
	case EventFaulty:
		return false
	}

	panic("Assertion failed: unreachable condition")
}
