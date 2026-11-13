// Package labels renders a shipment's label with the deployment's label binary. The binary is the
// configured one — an absolute path to an executable regular file — and it is started without a shell
// with a fixed argument vector and a fixed environment. The input and output paths are the ones the
// caller resolved through internal/workspace: a path whose last segment begins with "-" is refused, so
// a name cannot be read as an option, and the artifact the binary wrote is opened only after proving it
// is a regular file inside the tree and carries the PDF signature.
package labels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/identity"
	"example.com/shiptrack/internal/idgen"
	"example.com/shiptrack/internal/textutil"
)

// Errors a render reports.
var (
	ErrRunner = errors.New("labels: invalid renderer")
	ErrInput  = errors.New("labels: invalid input path")
	ErrRender = errors.New("labels: render failed")
	ErrOutput = errors.New("labels: bad artifact")
)

// pdfSignature is the first bytes a rendered label must carry.
const pdfSignature = "%PDF-"

// Spec is what a label says. Every field is validated before the spec is written: the shipment and the
// partner are name segments and identifiers, the carrier and the service are canonical ids, and the
// note is display text that the renderer receives as text. Nothing here is a path, and nothing here is
// markup: the renderer is a program the deployment configured, not a page.
type Spec struct {
	Tenant   string   `json:"tenant"`
	Partner  string   `json:"partner"`
	Shipment string   `json:"shipment"`
	Carrier  string   `json:"carrier"`
	Lane     string   `json:"lane"`
	Service  string   `json:"service"`
	Grams    int64    `json:"grams"`
	At       string   `json:"at"`
	Note     string   `json:"note"`
	Lines    []string `json:"lines"`
}

// NewSpec validates and builds a label spec.
func NewSpec(partner identity.Partner, shipment, carrier, lane, service string, grams int64, at time.Time, note string) (Spec, error) {
	if !identity.Canonical(partner.Tenant) || !identity.Canonical(partner.ID) {
		return Spec{}, fmt.Errorf("%w: partner %q", ErrInput, partner.ID)
	}
	if !idgen.Valid(shipment) {
		return Spec{}, fmt.Errorf("%w: shipment %q", ErrInput, shipment)
	}
	if !identity.Canonical(carrier) || !identity.Canonical(service) {
		return Spec{}, fmt.Errorf("%w: carrier %q service %q", ErrInput, carrier, service)
	}
	if strings.Count(lane, ">") != 1 {
		return Spec{}, fmt.Errorf("%w: lane %q", ErrInput, lane)
	}
	origin, destination, _ := strings.Cut(lane, ">")
	if !identity.Canonical(origin) || !identity.Canonical(destination) {
		return Spec{}, fmt.Errorf("%w: lane %q", ErrInput, lane)
	}
	if grams <= 0 {
		return Spec{}, fmt.Errorf("%w: grams %d", ErrInput, grams)
	}
	if len(note) > 500 {
		return Spec{}, fmt.Errorf("%w: note is %d bytes", ErrInput, len(note))
	}
	for _, r := range note {
		if r < 0x20 && r != '\t' || r == 0x7f {
			return Spec{}, fmt.Errorf("%w: note carries a control character", ErrInput)
		}
	}
	return Spec{
		Tenant:   partner.Tenant,
		Partner:  partner.ID,
		Shipment: shipment,
		Carrier:  carrier,
		Lane:     lane,
		Service:  service,
		Grams:    grams,
		At:       clock.FormatInstant(at),
		Note:     note,
		Lines: []string{
			"Shipment " + shipment,
			"Carrier " + carrier + " · " + service,
			"Lane " + origin + " to " + destination,
			"Weight " + textutil.Grams(grams) + " kg",
		},
	}, nil
}

// Marshal renders the spec as the JSON the renderer reads.
func (s Spec) Marshal() ([]byte, error) {
	// encoding/json escapes the characters that could end a JSON string, so a note is a JSON string and
	// never a second document inside the input file.
	encoded, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInput, err)
	}
	return encoded, nil
}

// Runner is the configured label binary, its argument template and its bounds.
type Runner struct {
	bin       string
	maxInput  int64
	maxOutput int64
	timeout   time.Duration
}

// New examines the configured binary. It must be an absolute path to an executable regular file: a
// relative path would let the working directory decide which program runs, and a directory or a device
// is not a renderer. The input and output bounds must be inside the deployment's caps.
func New(bin string, maxInput, maxOutput int64) (*Runner, error) {
	if bin == "" {
		return nil, fmt.Errorf("%w: no renderer configured", ErrRunner)
	}
	if !filepath.IsAbs(bin) {
		return nil, fmt.Errorf("%w: %q is not absolute", ErrRunner, bin)
	}
	if strings.ContainsAny(bin, "\x00\n\r") {
		return nil, fmt.Errorf("%w: %q carries a control character", ErrRunner, bin)
	}
	info, err := os.Stat(bin)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRunner, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("%w: %q is not an executable regular file", ErrRunner, bin)
	}
	if maxInput < 1 || maxInput > 4<<20 {
		return nil, fmt.Errorf("%w: input cap %d", ErrRunner, maxInput)
	}
	if maxOutput < 1 || maxOutput > 32<<20 {
		return nil, fmt.Errorf("%w: output cap %d", ErrRunner, maxOutput)
	}
	return &Runner{bin: bin, maxInput: maxInput, maxOutput: maxOutput, timeout: 30 * time.Second}, nil
}

// Bin is the configured binary's path.
func (r *Runner) Bin() string { return r.bin }

// Render runs the binary over an input file and an output file. Both paths are absolute and both are
// checked for an option-shaped last segment; the process is started with the argument vector
// `--input <path> --output <path>` and the fixed environment `PATH` and `LC_ALL`, so the deployment's
// secrets are not inherited by a program it renders with.
func (r *Runner) Render(ctx context.Context, input, output string) error {
	if err := checkArgPath(input); err != nil {
		return fmt.Errorf("%w: input: %v", ErrInput, err)
	}
	if err := checkArgPath(output); err != nil {
		return fmt.Errorf("%w: output: %v", ErrInput, err)
	}
	info, err := os.Lstat(input)
	if err != nil || !info.Mode().IsRegular() || info.Size() > r.maxInput {
		return fmt.Errorf("%w: input %q", ErrInput, input)
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	command := exec.CommandContext(ctx, r.bin, "--input", input, "--output", output)
	command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	stdout := &bounded{limit: 8 << 10}
	stderr := &bounded{limit: 8 << 10}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%w: %v (stderr: %s)", ErrRender, err, strings.TrimSpace(stderr.String()))
	}
	return r.verify(output)
}

// verify proves the artifact the binary wrote is a regular file inside the tree, inside the size bound
// and a PDF. The file is examined with Lstat, so a binary that wrote a symbolic link is refused instead
// of followed: the link's target is not the artifact the service asked for.
func (r *Runner) verify(output string) error {
	info, err := os.Lstat(output)
	if err != nil {
		return fmt.Errorf("%w: %q: %v", ErrOutput, output, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %q is not a regular file", ErrOutput, output)
	}
	if info.Size() < int64(len(pdfSignature)) || info.Size() > r.maxOutput {
		return fmt.Errorf("%w: %q is %d bytes", ErrOutput, output, info.Size())
	}
	file, err := os.Open(output)
	if err != nil {
		return fmt.Errorf("%w: %q: %v", ErrOutput, output, err)
	}
	defer file.Close()
	head := make([]byte, len(pdfSignature))
	if _, err := io.ReadFull(file, head); err != nil {
		return fmt.Errorf("%w: %q: %v", ErrOutput, output, err)
	}
	if string(head) != pdfSignature {
		return fmt.Errorf("%w: %q does not start with %s", ErrOutput, output, pdfSignature)
	}
	return nil
}

// checkArgPath refuses a path that is not absolute or whose last segment begins with "-": an argument
// vector is not a shell, but an argument that looks like an option is still read as one by the program
// that receives it.
func checkArgPath(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("%q is not absolute", path)
	}
	if strings.ContainsAny(path, "\x00\n\r") {
		return fmt.Errorf("%q carries a control character", path)
	}
	base := filepath.Base(path)
	if strings.HasPrefix(base, "-") {
		return fmt.Errorf("%q is shaped like an option", base)
	}
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return fmt.Errorf("%q names no file", path)
	}
	return nil
}

// bounded collects a process's output up to a limit and discards the rest, so a renderer that prints a
// gigabyte does not become the service's memory.
type bounded struct {
	limit int
	bytes []byte
}

func (b *bounded) Write(p []byte) (int, error) {
	room := b.limit - len(b.bytes)
	if room > 0 {
		if len(p) < room {
			room = len(p)
		}
		b.bytes = append(b.bytes, p[:room]...)
	}
	return len(p), nil
}

func (b *bounded) String() string { return string(b.bytes) }
