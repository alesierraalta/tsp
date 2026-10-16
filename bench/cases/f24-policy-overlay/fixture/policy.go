package policy

type Config struct {
	Retries int
	Enabled bool
}

type Patch struct {
	Retries *int
	Enabled *bool
}

func Merge(base Config, patch Patch) Config {
	if patch.Retries != nil && *patch.Retries != 0 {
		base.Retries = *patch.Retries
	}
	if patch.Enabled != nil && *patch.Enabled {
		base.Enabled = *patch.Enabled
	}
	return base
}
