package cache

import "sync/atomic"

type Stats struct {
	Hits    uint64
	Misses  uint64
	Sets    uint64
	Deletes uint64
}

func (s *Stats) hit()    { atomic.AddUint64(&s.Hits, 1) }
func (s *Stats) miss()   { atomic.AddUint64(&s.Misses, 1) }
func (s *Stats) set()    { atomic.AddUint64(&s.Sets, 1) }
func (s *Stats) delete() { atomic.AddUint64(&s.Deletes, 1) }
