package config

// The hard ceilings the service refuses to be configured above. They are constants rather than
// settings because a deployment that raises them turns one request into an out-of-memory event,
// and the number is not what a deployment needs to tune.
const (
	// HardPageSize is the largest page any list endpoint may return.
	HardPageSize = 500
	// HardMaxUpload is the largest object the service may be configured to accept.
	HardMaxUpload = 64 << 20
)

// ClampPage bounds a requested page size to the configured one. A request may ask for fewer rows
// than the configured page size; it may not ask for more.
func (l Limits) ClampPage(requested int) int {
	switch {
	case requested <= 0:
		return l.PageSize
	case requested > l.PageSize:
		return l.PageSize
	default:
		return requested
	}
}

// ClampUpload bounds a declared upload size to the configured one.
func (l Limits) ClampUpload(declared int64) int64 {
	switch {
	case declared <= 0:
		return l.MaxUpload
	case declared > l.MaxUpload:
		return l.MaxUpload
	default:
		return declared
	}
}

// PageCount reports how many pages a row count needs at the configured page size.
func (l Limits) PageCount(rows int) int {
	if rows <= 0 {
		return 0
	}
	pages := rows / l.PageSize
	if rows%l.PageSize != 0 {
		pages++
	}
	return pages
}

// Valid reports whether the limits can be applied at all.
func (l Limits) Valid() bool {
	return l.PageSize > 0 && l.MaxUpload > 0 && l.MaxBody >= l.MaxUpload
}
