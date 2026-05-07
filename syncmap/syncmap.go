package syncmap

import "sync"

type SyncMap[K comparable, V any] struct {
	m sync.Map
}

// Load returns the value stored in the map for a key, or the zero value if no
// value is present. The ok result indicates whether value was found.
//
// Note: If the stored value is not of type V, this method will panic.
func (m *SyncMap[K, V]) Load(k K) (V, bool) {
	var zero V
	v, ok := m.m.Load(k)
	if !ok {
		return zero, false
	}
	return v.(V), true
}

// TryLoad is a safer version of Load.
// It reports whether the key exists (loaded) and whether the value has type V (okType).
// If loaded is false, okType is false.
func (m *SyncMap[K, V]) TryLoad(k K) (value V, loaded bool, okType bool) {
	v, loaded := m.m.Load(k)
	if !loaded {
		var zero V
		return zero, false, false
	}
	tv, okType := v.(V)
	if !okType {
		var zero V
		return zero, true, false
	}
	return tv, true, true
}

// Store sets the value for a key.
func (m *SyncMap[K, V]) Store(k K, v V) {
	m.m.Store(k, v)
}

// Delete deletes the value for a key.
func (m *SyncMap[K, V]) Delete(k K) {
	m.m.Delete(k)
}

// LoadAndDelete deletes the value for a key, returning the previous value if any.
//
// Note: If the stored value is not of type V, this method will panic.
func (m *SyncMap[K, V]) LoadAndDelete(k K) (V, bool) {
	var zero V
	v, loaded := m.m.LoadAndDelete(k)
	if !loaded {
		return zero, false
	}
	return v.(V), true
}

// TryLoadAndDelete is a safer version of LoadAndDelete.
// It reports whether the key existed (loaded) and whether the value had type V (okType).
// If loaded is false, okType is false.
func (m *SyncMap[K, V]) TryLoadAndDelete(k K) (value V, loaded bool, okType bool) {
	v, loaded := m.m.LoadAndDelete(k)
	if !loaded {
		var zero V
		return zero, false, false
	}
	tv, okType := v.(V)
	if !okType {
		var zero V
		return zero, true, false
	}
	return tv, true, true
}

// Swap swaps the value for a key and returns the previous value if any.
//
// Note: If the stored value is not of type V, this method will panic.
func (m *SyncMap[K, V]) Swap(k K, v V) (V, bool) {
	var zero V
	prev, loaded := m.m.Swap(k, v)
	if !loaded {
		return zero, false
	}
	return prev.(V), true
}

// TrySwap is a safer version of Swap.
// It reports whether a previous value existed (loaded) and whether that value had type V (okType).
// If loaded is false, okType is false.
func (m *SyncMap[K, V]) TrySwap(k K, v V) (prev V, loaded bool, okType bool) {
	p, loaded := m.m.Swap(k, v)
	if !loaded {
		var zero V
		return zero, false, false
	}
	tp, okType := p.(V)
	if !okType {
		var zero V
		return zero, true, false
	}
	return tp, true, true
}

// CompareAndSwap swaps the old and new values for key if the value stored in the map is equal to old.
func (m *SyncMap[K, V]) CompareAndSwap(k K, old, new V) bool {
	return m.m.CompareAndSwap(k, old, new)
}

// CompareAndDelete deletes the entry for key if the value stored in the map is equal to old.
func (m *SyncMap[K, V]) CompareAndDelete(k K, old V) bool {
	return m.m.CompareAndDelete(k, old)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
//
// Note: If any key is not of type K or any value is not of type V, this method will panic.
func (m *SyncMap[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(k, v any) bool {
		return f(k.(K), v.(V))
	})
}

// TryRange is a safer version of Range.
// It will stop and return false if it encounters a key/value of unexpected type.
func (m *SyncMap[K, V]) TryRange(f func(key K, value V) bool) (okType bool) {
	ok := true
	m.m.Range(func(k, v any) bool {
		tk, okK := k.(K)
		if !okK {
			ok = false
			return false
		}
		tv, okV := v.(V)
		if !okV {
			ok = false
			return false
		}
		return f(tk, tv)
	})
	return ok
}

func (m *SyncMap[K, V]) LoadOrStore(k K, v V) (V, bool) {
	actual, loaded := m.m.LoadOrStore(k, v)
	return actual.(V), loaded
}

// TryLoadOrStore is a safer version of LoadOrStore.
// It reports whether the value was loaded (loaded) and whether the existing value had type V (okType).
// If loaded is false, okType is true because the returned value is exactly v.
func (m *SyncMap[K, V]) TryLoadOrStore(k K, v V) (actual V, loaded bool, okType bool) {
	a, loaded := m.m.LoadOrStore(k, v)
	if !loaded {
		return v, false, true
	}
	ta, okType := a.(V)
	if !okType {
		var zero V
		return zero, true, false
	}
	return ta, true, true
}

// Len returns a best-effort count of entries by ranging over the map.
// Note: sync.Map does not provide a constant-time length operation.
func (m *SyncMap[K, V]) Len() int {
	n := 0
	m.m.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

// Keys returns a snapshot slice of keys.
func (m *SyncMap[K, V]) Keys() []K {
	keys := make([]K, 0)
	m.m.Range(func(k, _ any) bool {
		keys = append(keys, k.(K))
		return true
	})
	return keys
}

// Values returns a snapshot slice of values.
func (m *SyncMap[K, V]) Values() []V {
	values := make([]V, 0)
	m.m.Range(func(_, v any) bool {
		values = append(values, v.(V))
		return true
	})
	return values
}

// ToMap returns a snapshot copy of current entries into a built-in map.
func (m *SyncMap[K, V]) ToMap() map[K]V {
	out := make(map[K]V)
	m.m.Range(func(k, v any) bool {
		out[k.(K)] = v.(V)
		return true
	})
	return out
}

// Clear removes all entries.
func (m *SyncMap[K, V]) Clear() {
	m.m.Clear()
}

