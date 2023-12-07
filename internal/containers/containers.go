package containers

// Triple represents a generic container with three values of types T, U, and V.
type Triple[T, U, V any] struct {
	First  T
	Second U
	Third  V
}

// Decompose returns T, U, and V objects in order. Useful for obtaining all
// values in a single call.
func (t Triple[T, U, V]) Decompose() (T, U, V) {
	return t.First, t.Second, t.Third
}

// Tri returns a new Triple with provided arguments.
func Tri[T, U, V any](first T, second U, third V) Triple[T, U, V] {
	return Triple[T, U, V]{first, second, third}
}

type Tuple[T, U any] struct {
	First  T
	Second U
}

func (t Tuple[T, U]) Decompose() (T, U) {
	return t.First, t.Second
}

func Tup[T, U any](first T, second U) Tuple[T, U] {
	return Tuple[T, U]{first, second}
}
