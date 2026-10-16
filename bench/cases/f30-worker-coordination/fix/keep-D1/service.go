package workqueue

import (
	"errors"
	"fmt"
)

type Loader func(string) (string, error)

type Service struct {
	cache   *Cache
	flights *FlightGroup
	load    Loader
}

func NewService(load Loader) *Service {
	return &Service{cache: NewCache(), flights: NewFlightGroup(), load: load}
}

func (s *Service) Fetch(key string) *Future {
	if !validKey(key) {
		f := newFuture()
		f.complete(Result{Err: errors.New("invalid key")})
		return f
	}
	value, generation, ok := s.cache.Lookup(key)
	if ok {
		return resolved(value)
	}
	flightKey := fmt.Sprintf("%d:%s", generation, key)
	return s.flights.Do(flightKey, func() (string, error) {
		value, err := s.load(key)
		if err == nil {
			s.cache.Store(key, value, generation)
		}
		return value, err
	})
}

func (s *Service) Invalidate(key string) uint64 {
	return s.cache.Invalidate(key)
}

func (s *Service) Stats() FlightStats {
	return s.flights.Stats()
}

func (s *Service) Cached() int {
	return s.cache.Size()
}
