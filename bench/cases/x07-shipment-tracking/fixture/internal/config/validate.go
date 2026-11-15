package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Validate checks a loaded configuration. The checks are the ones a listener must not start without:
// a served tenant, a secret long enough to be one, a lifetime inside its bound, a zone the platform
// carries, caps inside their ceilings, a tax rate inside 0..100%, an egress list that is a list of
// hosts, and a root that exists, is absolute and is not the filesystem root.
func (c *Config) Validate() error {
	var problems []string
	if len(c.Tenants) == 0 {
		problems = append(problems, "no tenant is served")
	}
	for _, tenant := range c.Tenants {
		if !canonical(tenant) {
			problems = append(problems, fmt.Sprintf("tenant %q is not a name segment", tenant))
		}
	}
	if len(c.TokenSecret) < 16 {
		problems = append(problems, fmt.Sprintf("token secret is %d bytes, need 16", len(c.TokenSecret)))
	}
	if c.TokenTTL < 0 {
		problems = append(problems, fmt.Sprintf("token lifetime %s", c.TokenTTL))
	}
	if c.Zone == nil {
		problems = append(problems, "zone is not resolved")
	}
	if err := c.Caps.Validate(); err != nil {
		problems = append(problems, err.Error())
	}
	if c.TaxRate < 0 || c.TaxRate > 10000 {
		problems = append(problems, fmt.Sprintf("tax rate %d basis points is outside 0..10000", c.TaxRate))
	}
	for _, host := range c.EgressHosts {
		if strings.TrimSpace(host) != host || strings.ContainsAny(host, ":/@?# ") {
			problems = append(problems, fmt.Sprintf("egress host %q is not a bare host", host))
		}
	}
	if c.Addr == "" {
		problems = append(problems, "listen address is empty")
	}
	if c.Renderer != "" && !filepath.IsAbs(c.Renderer) {
		problems = append(problems, fmt.Sprintf("renderer %q is not absolute", c.Renderer))
	}
	if err := CheckRoot(c.Root); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("config: %s", strings.Join(problems, "; "))
	}
	return nil
}

// CheckRoot checks that the workspace root exists, is a directory, is absolute, is not the filesystem
// root and is not a symbolic link. A root that is a link would make the containment check a check of the
// link's target, which is a directory the deployment did not choose.
func CheckRoot(root string) error {
	if root == "" || !filepath.IsAbs(root) {
		return fmt.Errorf("root %q is not absolute", root)
	}
	cleaned := filepath.Clean(root)
	if cleaned == string(filepath.Separator) {
		return fmt.Errorf("root is the filesystem root")
	}
	info, err := os.Lstat(cleaned)
	if err != nil {
		return fmt.Errorf("root %q: %v", root, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("root %q is a symbolic link", root)
	}
	if !info.IsDir() {
		return fmt.Errorf("root %q is not a directory", root)
	}
	return nil
}

// canonical is the tenant name rule: a lower-case name segment. It is duplicated from internal/identity
// on purpose: configuration is validated before the directory exists, and the two rules must agree.
func canonical(value string) bool {
	if len(value) < 2 || len(value) > 32 {
		return false
	}
	if value[0] < 'a' || value[0] > 'z' {
		return false
	}
	last := value[len(value)-1]
	if (last < 'a' || last > 'z') && (last < '0' || last > '9') {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}
