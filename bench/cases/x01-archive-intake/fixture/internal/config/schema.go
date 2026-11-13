package config

import "fmt"

// Variable is one environment variable a deployment sets: its name, what it means, and the default the
// service uses when it is absent. The table is what the admin console prints, and the startup usage
// block is written from the same list.
type Variable struct {
	Name     string
	Meaning  string
	Default  string
	Required bool
}

// Variables lists the environment the service reads, in the order the configuration loads it.
func Variables() []Variable {
	return []Variable{
		{"ARCHIVE_ROOT", "absolute directory the archive owns", "", true},
		{"ARCHIVE_TOKEN_SECRET", "token signing secret, at least 16 bytes", "", true},
		{"ARCHIVE_ADDR", "listen address of the HTTP surface", "127.0.0.1:8080", false},
		{"ARCHIVE_TENANTS", "comma-separated tenant ids, compared byte for byte", "acme", false},
		{"ARCHIVE_PAGE_SIZE", "default listing size", "25", false},
		{"ARCHIVE_MAX_UPLOAD", "largest object in bytes, with a k/M/G suffix", "32Mi", false},
		{"ARCHIVE_MAX_ENTRIES", "largest manifest entry count", "500", false},
		{"ARCHIVE_MAX_PAGES", "largest declared page count", "2000", false},
		{"ARCHIVE_MAX_EXPORT_ROWS", "largest number of exported rows", "10000", false},
		{"ARCHIVE_SESSION_TTL", "token lifetime as a Go duration", "1h", false},
		{"ARCHIVE_RETENTION_DAYS", "retention window in whole UTC days", "30", false},
		{"ARCHIVE_ZONE", "IANA zone the operator pages print, stored instants stay UTC", "UTC", false},
	}
}

// LookupVariable finds one variable by name.
func LookupVariable(name string) (Variable, bool) {
	for _, variable := range Variables() {
		if variable.Name == name {
			return variable, true
		}
	}
	return Variable{}, false
}

// Required lists the variables a deployment must set, for the startup failure message.
func Required() []string {
	var out []string
	for _, variable := range Variables() {
		if variable.Required {
			out = append(out, variable.Name)
		}
	}
	return out
}

// Describe renders the table as the block the console prints: name, default and meaning.
func Describe() []string {
	lines := make([]string, 0, len(Variables()))
	for _, variable := range Variables() {
		def := variable.Default
		if variable.Required {
			def = "required"
		}
		lines = append(lines, fmt.Sprintf("%s (%s): %s", variable.Name, def, variable.Meaning))
	}
	return lines
}
