package syncmap

import "sync"

type SyncMap[K comparable, V any] struct {
	m sync.Map
}

// Load 返回 key 对应的值；若不存在则返回零值，ok 表示是否命中。
//
// 注意：若实际存储值不是 V 类型，会触发 panic。
func (m *SyncMap[K, V]) Load(k K) (V, bool) {
	var zero V
	v, ok := m.m.Load(k)
	if !ok {
		return zero, false
	}
	return v.(V), true
}

// TryLoad 是 Load 的安全版本。
// loaded 表示是否存在该 key，okType 表示值是否为 V 类型。
// 若 loaded 为 false，则 okType 一定为 false。
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

// Store 设置 key 对应的值。
func (m *SyncMap[K, V]) Store(k K, v V) {
	m.m.Store(k, v)
}

// Delete 删除 key 对应的值。
func (m *SyncMap[K, V]) Delete(k K) {
	m.m.Delete(k)
}

// LoadAndDelete 删除 key 对应的值，并返回删除前的值。
//
// 注意：若实际存储值不是 V 类型，会触发 panic。
func (m *SyncMap[K, V]) LoadAndDelete(k K) (V, bool) {
	var zero V
	v, loaded := m.m.LoadAndDelete(k)
	if !loaded {
		return zero, false
	}
	return v.(V), true
}

// TryLoadAndDelete 是 LoadAndDelete 的安全版本。
// loaded 表示是否存在该 key，okType 表示值是否为 V 类型。
// 若 loaded 为 false，则 okType 一定为 false。
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

// Swap 交换 key 对应的值，并返回旧值（若存在）。
//
// 注意：若实际存储值不是 V 类型，会触发 panic。
func (m *SyncMap[K, V]) Swap(k K, v V) (V, bool) {
	var zero V
	prev, loaded := m.m.Swap(k, v)
	if !loaded {
		return zero, false
	}
	return prev.(V), true
}

// TrySwap 是 Swap 的安全版本。
// loaded 表示是否存在旧值，okType 表示旧值是否为 V 类型。
// 若 loaded 为 false，则 okType 一定为 false。
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

// CompareAndSwap 当 key 对应值等于 old 时，用 new 替换。
func (m *SyncMap[K, V]) CompareAndSwap(k K, old, new V) bool {
	return m.m.CompareAndSwap(k, old, new)
}

// CompareAndDelete 当 key 对应值等于 old 时删除该条目。
func (m *SyncMap[K, V]) CompareAndDelete(k K, old V) bool {
	return m.m.CompareAndDelete(k, old)
}

// Range 对 map 中的每个键值对依次调用 f。
// 若 f 返回 false，则提前停止遍历。
//
// 注意：若 key 不是 K 类型或 value 不是 V 类型，会触发 panic。
func (m *SyncMap[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(k, v any) bool {
		return f(k.(K), v.(V))
	})
}

// TryRange 是 Range 的安全版本。
// 遇到类型不匹配的键值会停止并返回 false。
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

// TryLoadOrStore 是 LoadOrStore 的安全版本。
// loaded 表示是否命中已有值，okType 表示已有值是否为 V 类型。
// 若 loaded 为 false，则 okType 为 true（返回值就是传入的 v）。
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

// Len 通过遍历估算当前条目数。
// 注意：sync.Map 不提供常数时间的长度操作。
func (m *SyncMap[K, V]) Len() int {
	n := 0
	m.m.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

// Keys 返回 key 的快照切片。
func (m *SyncMap[K, V]) Keys() []K {
	keys := make([]K, 0)
	m.m.Range(func(k, _ any) bool {
		keys = append(keys, k.(K))
		return true
	})
	return keys
}

// Values 返回 value 的快照切片。
func (m *SyncMap[K, V]) Values() []V {
	values := make([]V, 0)
	m.m.Range(func(_, v any) bool {
		values = append(values, v.(V))
		return true
	})
	return values
}

// ToMap 将当前条目快照拷贝到内置 map 中。
func (m *SyncMap[K, V]) ToMap() map[K]V {
	out := make(map[K]V)
	m.m.Range(func(k, v any) bool {
		out[k.(K)] = v.(V)
		return true
	})
	return out
}

// Clear 清空所有条目。
func (m *SyncMap[K, V]) Clear() {
	m.m.Clear()
}

