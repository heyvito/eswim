package containers

import "fmt"

func InsertAt[S interface{ ~[]E }, E any](base S, item E, pos int) S {
	return append(base[:pos], append(S{item}, base[pos:]...)...)
}

func AllMatch[S interface{ ~[]E }, E any](base S, matcher func(item E) bool) bool {
	for _, v := range base {
		if !matcher(v) {
			return false
		}
	}

	return true
}

func DrainChan[T any](ch chan T) {
drain:
	for {
		select {
		case <-ch:
		default:
			break drain
		}
	}
}

func MapFn[S interface{ ~[]E }, E any, T any](set S, fn func(E) T) []T {
	res := make([]T, 0, len(set))
	for _, item := range set {
		res = append(res, fn(item))
	}
	return res
}

func StringerStr[T fmt.Stringer](i T) string { return i.String() }

func StrMapper[S interface{ ~[]E }, E fmt.Stringer](set S) []string {
	return MapFn(set, StringerStr[E])
}
