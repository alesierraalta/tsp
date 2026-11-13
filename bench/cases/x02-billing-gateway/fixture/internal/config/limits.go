package config

import "fmt"

// Limits are the caps a request is held to. They travel as one value so a handler cannot enforce
// one of them from a configuration and another from a default.
type Limits struct {
	// PageSize is the largest page a listing returns, whatever a caller asks for.
	PageSize int
	// BodyBytes is the number of bytes a request body may carry before it is refused.
	BodyBytes int64
	// InvoiceRows is the largest number of lines one invoice may carry.
	InvoiceRows int
	// EventCount is the number of usage events the store accepts before it refuses a write.
	EventCount int
	// TaxRateBasis is the denominator of a tax rate: a rate of 1900 at basis 10000 is 19%.
	TaxRateBasis int
}

// Validate checks the caps on their own, for a caller that builds them without a configuration.
func (l Limits) Validate() error {
	if l.PageSize < 1 {
		return fmt.Errorf("config: page size %d is not positive", l.PageSize)
	}
	if l.BodyBytes < 1 {
		return fmt.Errorf("config: body cap %d is not positive", l.BodyBytes)
	}
	if l.InvoiceRows < 1 {
		return fmt.Errorf("config: invoice row cap %d is not positive", l.InvoiceRows)
	}
	if l.EventCount < 1 {
		return fmt.Errorf("config: event cap %d is not positive", l.EventCount)
	}
	if l.TaxRateBasis < 1 {
		return fmt.Errorf("config: tax rate basis %d is not positive", l.TaxRateBasis)
	}
	return nil
}

// PageSize returns the number of rows a listing returns for a request that asked for want, which is
// never more than the deployment's page size and never less than one.
func (l Limits) PageSizeFor(want int) int {
	if want <= 0 {
		return l.PageSize
	}
	if want > l.PageSize {
		return l.PageSize
	}
	return want
}
