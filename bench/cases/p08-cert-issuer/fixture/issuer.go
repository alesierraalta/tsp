package issuer

import "time"

type Certificate struct {
	Subject         string
	Serial          int64
	Issued, Expires time.Time
}
type issuanceRecord struct {
	Serial int32
}

type Issuer struct {
	Now          func() time.Time
	Allocate     func() int64
	Certificates map[int64]Certificate
}

func (s *Issuer) Issue(subject string) Certificate {
	now := s.Now().UTC()
	expires := now.AddDate(1, 0, 0)
	record := issuanceRecord{Serial: int32(s.Allocate())}
	serial := int64(record.Serial)
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
