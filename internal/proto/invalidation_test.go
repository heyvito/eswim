package proto

import (
	"fmt"
	"github.com/heyvito/eswim/internal/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

var T = containers.Tup[EventKind, EqualityMode]

var eventMap = map[EventKind]reflect.Type{
	EventJoin:    reflect.TypeOf(Join{}),
	EventHealthy: reflect.TypeOf(Healthy{}),
	EventSuspect: reflect.TypeOf(Suspect{}),
	EventFaulty:  reflect.TypeOf(Faulty{}),
	EventLeft:    reflect.TypeOf(Left{}),
	EventAlive:   reflect.TypeOf(Alive{}),
	EventWrap:    reflect.TypeOf(Wrap{}),
}

func makeIncEvent(kind EventKind, incarnation uint16) *Event {
	evT := eventMap[kind]
	rv := reflect.New(evT)
	rv.Elem().FieldByName("Incarnation").Set(reflect.ValueOf(incarnation))
	return &Event{Payload: rv.Interface().(EventEncoder)}
}

func TestMakeIncEvent(t *testing.T) {
	ev := makeIncEvent(EventJoin, 16)
	assert.Equal(t, uint16(16), ev.Payload.(*Join).Incarnation)
}

type EqualityMode int

const (
	EqualityEqual EqualityMode = iota
	EqualityGreaterThan
	EqualityLowerThan
	EqualityGreaterThanEqual
	EqualityLowerThanEqual
	EqualityNotEqual
	EqualityNever
	EqualityAlways
)

var equalityString = map[EqualityMode]string{
	EqualityEqual:            "==",
	EqualityGreaterThan:      ">",
	EqualityLowerThan:        "<",
	EqualityGreaterThanEqual: ">=",
	EqualityLowerThanEqual:   "<=",
	EqualityNotEqual:         "!=",
}

func wouldInvalidate(current, next EventKind, mode EqualityMode) bool {
	val := uint16(100)
	switch mode {
	case EqualityEqual:
		// Noop
	case EqualityGreaterThan:
		val++
	case EqualityLowerThan:
		val--
	case EqualityNotEqual:
		val = 200
	default:
		panic("this shouldn't happen")
	}

	currentEv := makeIncEvent(current, 100)
	nextEv := makeIncEvent(next, val)
	return nextEv.Payload.Invalidates(currentEv)
}

func AssertInvalidates(t *testing.T, next, current EventKind, mode EqualityMode) {
	t.Helper()

	var testMatrix map[EqualityMode]bool
	switch mode {
	case EqualityEqual:
		testMatrix = map[EqualityMode]bool{
			EqualityEqual:       true,
			EqualityGreaterThan: false,
			EqualityLowerThan:   false,
		}
	case EqualityGreaterThanEqual:
		testMatrix = map[EqualityMode]bool{
			EqualityEqual:       true,
			EqualityGreaterThan: true,
			EqualityLowerThan:   false,
		}
	case EqualityLowerThanEqual:
		testMatrix = map[EqualityMode]bool{
			EqualityEqual:       true,
			EqualityGreaterThan: false,
			EqualityLowerThan:   true,
		}
	case EqualityGreaterThan:
		testMatrix = map[EqualityMode]bool{
			EqualityEqual:       false,
			EqualityGreaterThan: true,
			EqualityLowerThan:   false,
		}
	case EqualityLowerThan:
		testMatrix = map[EqualityMode]bool{
			EqualityEqual:       false,
			EqualityGreaterThan: false,
			EqualityLowerThan:   true,
		}
	case EqualityNever:
		testMatrix = map[EqualityMode]bool{
			EqualityEqual:       false,
			EqualityGreaterThan: false,
			EqualityLowerThan:   false,
		}
	case EqualityAlways:
		testMatrix = map[EqualityMode]bool{
			EqualityEqual:       true,
			EqualityGreaterThan: true,
			EqualityLowerThan:   true,
		}
	}

	for mode, result := range testMatrix {
		should := ""
		if result == false {
			should = "not "
		}
		name := fmt.Sprintf("[%s] %s[i] %s %s[j] should %sinvalidate %s[j]",
			next.String(),
			current.String(),
			equalityString[mode],
			next.String(),
			should,
			current.String())

		t.Run(name, func(t *testing.T) {
			require.Equalf(t, result, wouldInvalidate(current, next, mode), "Assertion failed")
		})
	}
}

func AssertInvalidationRules(t *testing.T, kind EventKind, rules ...containers.Tuple[EventKind, EqualityMode]) {
	t.Helper()
	for _, v := range rules {
		otherKind, mode := v.Decompose()
		AssertInvalidates(t, kind, otherKind, mode)
	}
}

func TestAlive_Invalidates(t *testing.T) {
	AssertInvalidationRules(t, EventAlive,
		T(EventSuspect, EqualityGreaterThan),
		T(EventAlive, EqualityGreaterThan),
		T(EventFaulty, EqualityGreaterThan),
		T(EventJoin, EqualityGreaterThan),
		T(EventHealthy, EqualityGreaterThan),
		T(EventLeft, EqualityGreaterThan),
		T(EventWrap, EqualityGreaterThan),
	)
}

func TestSuspect_Invalidates(t *testing.T) {
	AssertInvalidationRules(t, EventSuspect,
		T(EventSuspect, EqualityGreaterThan),
		T(EventAlive, EqualityGreaterThanEqual),
		T(EventFaulty, EqualityGreaterThan),
		T(EventJoin, EqualityGreaterThanEqual),
		T(EventHealthy, EqualityGreaterThanEqual),
		T(EventLeft, EqualityGreaterThan),
		T(EventWrap, EqualityGreaterThanEqual),
	)
}

func TestFaulty_Invalidates(t *testing.T) {
	AssertInvalidationRules(t, EventFaulty,
		T(EventAlive, EqualityAlways),
		T(EventSuspect, EqualityAlways),
		T(EventFaulty, EqualityAlways),
		T(EventJoin, EqualityAlways),
		T(EventHealthy, EqualityAlways),
		T(EventLeft, EqualityAlways),
		T(EventWrap, EqualityAlways),
	)
}

func TestJoin_Invalidates(t *testing.T) {
	AssertInvalidationRules(t, EventJoin,
		T(EventAlive, EqualityGreaterThan),
		T(EventSuspect, EqualityGreaterThan),
		T(EventFaulty, EqualityNever),
		T(EventJoin, EqualityGreaterThan),
		T(EventHealthy, EqualityGreaterThan),
		T(EventLeft, EqualityGreaterThan),
		T(EventWrap, EqualityGreaterThan),
	)
}

func TestHealthy_Invalidates(t *testing.T) {
	AssertInvalidationRules(t, EventHealthy,
		T(EventAlive, EqualityGreaterThan),
		T(EventSuspect, EqualityGreaterThanEqual),
		T(EventFaulty, EqualityNever),
		T(EventJoin, EqualityGreaterThanEqual),
		T(EventHealthy, EqualityGreaterThan),
		T(EventLeft, EqualityGreaterThan),
		T(EventWrap, EqualityGreaterThan),
	)
}

func TestLeft_Invalidates(t *testing.T) {
	AssertInvalidationRules(t, EventLeft,
		T(EventAlive, EqualityGreaterThanEqual),
		T(EventSuspect, EqualityGreaterThanEqual),
		T(EventFaulty, EqualityNever),
		T(EventJoin, EqualityGreaterThanEqual),
		T(EventHealthy, EqualityGreaterThanEqual),
		T(EventLeft, EqualityGreaterThan),
		T(EventWrap, EqualityGreaterThanEqual),
	)
}

func TestWrap_Invalidates(t *testing.T) {
	AssertInvalidationRules(t, EventWrap,
		T(EventAlive, EqualityGreaterThanEqual),
		T(EventSuspect, EqualityGreaterThanEqual),
		T(EventFaulty, EqualityNever),
		T(EventJoin, EqualityGreaterThanEqual),
		T(EventHealthy, EqualityGreaterThanEqual),
		T(EventLeft, EqualityGreaterThan),
		T(EventWrap, EqualityGreaterThan),
	)
}
