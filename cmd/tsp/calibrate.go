package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/alesierraalta/tsp/internal/calibrate"
)

// runCalibrate seeds sealed defects into a throwaway worktree for testing-skill calibration: the answer key leaves
// the tree, so the agent that runs the testing never reads what it is supposed to find. Exit codes: 0 measured or
// listed, 1 the seeder refused (not a throwaway worktree, key inside it, too few sites, a line that moved),
// 2 the invocation is incomplete and nothing was read.
func runCalibrate(args []string) int {
	fs := flag.NewFlagSet("calibrate", flag.ContinueOnError)
	root := fs.String("root", "", "throwaway worktree whose branch starts with calib- or skilltest-")
	filesFlag := fs.String("files", "", "comma-separated source files relative to root")
	count := fs.Int("count", 5, "mutations to apply")
	keyOut := fs.String("key-out", "", "answer key path outside the worktree")
	seed := fs.Int64("seed", 0, "deterministic pick; another seed picks other sites")
	list := fs.Bool("list", false, "print candidate site counts per file and write nothing")
	replay := fs.String("replay", "", "re-apply the mutations of an existing answer key")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*root) == "" {
		fmt.Fprintln(os.Stderr, "calibrate: --root is required: the throwaway worktree to seed")
		return 2
	}
	if strings.TrimSpace(*replay) != "" {
		res, err := calibrate.Replay(*root, *replay)
		if err != nil {
			fmt.Fprintln(os.Stderr, "calibrate:", err)
			return 1
		}
		printCalibrateResult("replayed", res)
		return 0
	}
	files, err := calibrateFiles(*filesFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "calibrate:", err)
		return 2
	}
	if *list {
		counts, total, err := calibrate.List(*root, files)
		if err != nil {
			fmt.Fprintln(os.Stderr, "calibrate:", err)
			return 1
		}
		for _, file := range files {
			fmt.Printf("%5d  %s\n", counts[file], file)
		}
		fmt.Printf("total candidate sites: %d\n", total)
		return 0
	}
	if strings.TrimSpace(*keyOut) == "" {
		fmt.Fprintln(os.Stderr, "calibrate: --key-out is required: the key lives outside the worktree")
		return 2
	}
	res, err := calibrate.Seed(*root, files, *count, *seed, *keyOut)
	if err != nil {
		fmt.Fprintln(os.Stderr, "calibrate:", err)
		return 1
	}
	printCalibrateResult("applied", res)
	return 0
}

// printCalibrateResult states what landed and where the key is, with the digest a later report cites.
func printCalibrateResult(verb string, res calibrate.KeyResult) {
	fmt.Printf("%s: %d\n", verb, res.Applied)
	fmt.Printf("files touched: %s\n", strings.Join(res.Files, ", "))
	fmt.Printf("key: %s\n", res.KeyPath)
	fmt.Printf("key sha256: %s\n", res.SHA256)
}

// calibrateFiles splits --files. An entry that is empty after trimming is the operator's typo, not a file, and a
// list that holds no file is the invocation missing a value it needs.
func calibrateFiles(raw string) ([]string, error) {
	var files []string
	for _, part := range strings.Split(raw, ",") {
		if name := strings.TrimSpace(part); name != "" {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("--files is required: one or more source files relative to --root, comma-separated")
	}
	return files, nil
}
