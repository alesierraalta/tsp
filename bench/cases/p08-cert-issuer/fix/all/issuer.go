package issuer

import "time"

type Certificate struct {
	Subject         string
	Serial          int64
	Issued, Expires time.Time
}
type Issuer struct {
	Now          func() time.Time
	Allocate     func() int64
	Certificates map[int64]Certificate
}

func (s *Issuer) Issue(subject string) Certificate {
	now := s.Now().UTC()
	expires := now.AddDate(1, 0, 0)
	if expires.Month() != now.Month() {
		expires = expires.AddDate(0, 0, -1)
	}
	serial := s.Allocate()
	cert := Certificate{subject, serial, now, expires}
	if s.Certificates == nil {
		s.Certificates = map[int64]Certificate{}
	}
	s.Certificates[serial] = cert
	return cert
}
func (s *Issuer) Lookup(serial int64) (Certificate, bool) {
	cert, ok := s.Certificates[serial]
	return cert, ok
}
