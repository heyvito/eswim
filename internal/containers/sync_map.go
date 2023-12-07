package containers

import "sync"

type SyncMap[K comparable, V any] struct {
	inner sync.Map
}

func (s *SyncMap[K, V]) Load(key K) (value V, ok bool) {
	var v any
	if v, ok = s.inner.Load(key); !ok {
		return
	}
	return v.(V), ok
}

func (s *SyncMap[K, V]) Store(key K, value V) {
	s.inner.Store(key, value)
}

func (s *SyncMap[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	var v any
	v, loaded = s.inner.LoadOrStore(key, value)
	if loaded {
		actual = v.(V)
	}
	return
}

func (s *SyncMap[K, V]) LoadAndDelete(key K) (value V, loaded bool) {
	var v any
	v, loaded = s.inner.LoadAndDelete(key)
	if loaded {
		value = v.(V)
	}
	return
}

func (s *SyncMap[K, V]) Delete(key K) {
	s.inner.Delete(key)
}

func (s *SyncMap[K, V]) Swap(key K, value V) (previous V, loaded bool) {
	var prev any
	prev, loaded = s.inner.Swap(key, value)
	if loaded {
		previous = prev.(V)
	}
	return
}

func (s *SyncMap[K, V]) CompareAndSwap(key K, old, new V) bool {
	return s.inner.CompareAndSwap(key, old, new)
}

func (s *SyncMap[K, V]) CompareAndDelete(key K, old V) (deleted bool) {
	return s.inner.CompareAndDelete(key, old)
}

func (s *SyncMap[K, V]) Range(f func(key K, value V) bool) {
	s.inner.Range(func(key, value any) bool {
		return f(key.(K), value.(V))
	})
}
