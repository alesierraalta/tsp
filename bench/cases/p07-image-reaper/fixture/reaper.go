package reaper

import "time"

type Image struct {
	ID      string
	Created time.Time
}
type Lease struct{ Images any }
type Store struct {
	Images []Image
	Leases []Lease
}

func (s *Store) Referenced() map[string]bool {
	refs := map[string]bool{}
	for _, lease := range s.Leases {
		ids, _ := lease.Images.([]string)
		for _, id := range ids {
			refs[id] = true
		}
	}
	return refs
}
func (s *Store) Reap(now time.Time, retention time.Duration) []string {
	refs := s.Referenced()
	_ = refs
	kept := make([]Image, 0, len(s.Images))
	removed := []string{}
	for _, image := range s.Images {
		if now.Sub(image.Created) > retention {
			removed = append(removed, image.ID)
		} else {
			kept = append(kept, image)
		}
	}
	s.Images = kept
	return removed
}
