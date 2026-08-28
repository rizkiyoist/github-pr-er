// github-pr-er creates a PROD (-> main) and STAGE (-> develop) pull request
// from the current git branch, titled from the branch name.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	prodBase := flag.String("prod-base", "main", "base branch for the PROD pull request")
	stageBase := flag.String("stage-base", "develop", "base branch for the STAGE pull request")
	body := flag.String("body", "", "body text for both pull requests")
	draft := flag.Bool("draft", false, "create pull requests as drafts")
	push := flag.Bool("push", false, "push the current branch to origin before creating PRs")
	only := flag.String("only", "", "only create one PR: \"prod\" or \"stage\"")
	yes := flag.Bool("y", true, "skip confirmation prompt")
	flag.Parse()

	if err := checkGH(); err != nil {
		fatal(err)
	}

	branch, err := currentBranch()
	if err != nil {
		fatal(fmt.Errorf("could not determine current branch: %w", err))
	}
	if branch == *prodBase || branch == *stageBase {
		fatal(fmt.Errorf("current branch %q is a base branch; checkout your feature branch first", branch))
	}

	title := titleFromBranch(branch)

	type prSpec struct {
		label string
		base  string
		title string
	}
	var specs []prSpec
	if *only == "" || *only == "prod" {
		specs = append(specs, prSpec{"PROD", *prodBase, fmt.Sprintf("[PROD] %s", title)})
	}
	if *only == "" || *only == "stage" {
		specs = append(specs, prSpec{"STAGE", *stageBase, fmt.Sprintf("[STAGE] %s", title)})
	}
	if len(specs) == 0 {
		fatal(fmt.Errorf("--only must be \"prod\" or \"stage\", got %q", *only))
	}

	fmt.Printf("Branch: %s\n", branch)
	for _, s := range specs {
		fmt.Printf("  %-5s %s  <-  %s   base=%s\n", s.label, s.title, branch, s.base)
	}

	stdin := bufio.NewReader(os.Stdin)

	if !*yes && !confirm(stdin, "Create these pull request(s)?") {
		fmt.Println("Aborted.")
		return
	}

	reviewers := pickReviewers(stdin, *prodBase, branch)

	if *push {
		if err := run("git", "push", "-u", "origin", branch); err != nil {
			fatal(fmt.Errorf("git push failed: %w", err))
		}
	}

	urls := map[string]string{}
	failures := 0
	for _, s := range specs {
		args := []string{"pr", "create", "--base", s.base, "--head", branch, "--title", s.title}
		if *body != "" {
			args = append(args, "--body", *body)
		} else {
			args = append(args, "--body", "")
		}
		if *draft {
			args = append(args, "--draft")
		}
		if len(reviewers) > 0 {
			args = append(args, "--reviewer", strings.Join(reviewers, ","))
		}
		fmt.Printf("\n==> creating %s PR (base: %s)\n", s.label, s.base)
		out, err := runCaptureStdout("gh", args...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create %s PR: %v\n", s.label, err)
			failures++
			continue
		}
		urls[s.label] = lastLine(out)
	}

	if failures > 0 {
		os.Exit(1)
	}

	fmt.Printf("\nMohon review PR %s\n", strings.ReplaceAll(branch, "/", " "))
	if u, ok := urls["STAGE"]; ok {
		fmt.Println(u)
	}
	if u, ok := urls["PROD"]; ok {
		fmt.Println(u)
	}
}

func checkGH() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh (GitHub CLI) not found in PATH; install it from https://cli.github.com/")
	}
	return nil
}

func currentBranch() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return "", fmt.Errorf("not on a named branch (detached HEAD?)")
	}
	return branch, nil
}

// titleFromBranch turns "feature/something-cool_here" into "Something Cool Here".
func titleFromBranch(branch string) string {
	name := branch
	if idx := strings.LastIndex(name, "/"); idx != -1 {
		name = name[idx+1:]
	}
	name = strings.NewReplacer("-", " ", "_", " ").Replace(name)
	words := strings.Fields(name)
	for i, w := range words {
		r := []rune(w)
		if len(r) > 0 {
			r[0] = toUpper(r[0])
		}
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - ('a' - 'A')
	}
	return r
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// runCaptureStdout behaves like run but also returns everything written to
// stdout, while still echoing it to the terminal live.
func runCaptureStdout(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	err := cmd.Run()
	return buf.String(), err
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func confirm(reader *bufio.Reader, prompt string) bool {
	fmt.Printf("%s [Y/n]: ", prompt)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes" || line == ""
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

type codeownersRule struct {
	pattern string
	re      *regexp.Regexp
	owners  []string
}

var codeownersLocations = []string{
	"CODEOWNERS",
	".github/CODEOWNERS",
	"docs/CODEOWNERS",
}

// pickReviewers looks for a CODEOWNERS file, matches it against the files
// changed relative to base, and lets the user interactively pick reviewers
// from the resulting owner list. Returns nil if there's no CODEOWNERS file,
// no matching owners, or the user picks none.
func pickReviewers(reader *bufio.Reader, base, branch string) []string {
	path := findCodeowners()
	if path == "" {
		return nil
	}

	rules, err := parseCodeowners(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not parse %s: %v\n", path, err)
		return nil
	}

	files, err := changedFiles(base, branch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not diff changed files: %v\n", err)
		return nil
	}

	owners := ownersForFiles(rules, files)
	if len(owners) == 0 {
		return nil
	}

	fmt.Println("\nPossible reviewers (from " + path + "):")
	for i, o := range owners {
		fmt.Printf("  %d %s\n", i+1, o)
	}
	fmt.Print("Choose reviewers (comma separated numbers, blank for none): ")
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	var chosen []string
	for _, part := range strings.Split(line, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 || n > len(owners) {
			fmt.Fprintf(os.Stderr, "skipping invalid selection %q\n", part)
			continue
		}
		chosen = append(chosen, owners[n-1])
	}
	return chosen
}

func findCodeowners() string {
	for _, p := range codeownersLocations {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// parseCodeowners reads a CODEOWNERS file into ordered pattern/owner rules.
func parseCodeowners(path string) ([]codeownersRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var rules []codeownersRule
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		re, err := codeownersPatternRegexp(fields[0])
		if err != nil {
			continue
		}
		rules = append(rules, codeownersRule{pattern: fields[0], re: re, owners: fields[1:]})
	}
	return rules, scanner.Err()
}

// codeownersPatternRegexp translates a CODEOWNERS (gitignore-style) pattern
// into a regexp that matches repo-relative, forward-slash file paths.
func codeownersPatternRegexp(pattern string) (*regexp.Regexp, error) {
	p := pattern
	anchored := strings.HasPrefix(p, "/")
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	hasSlash := strings.Contains(p, "/")

	var sb strings.Builder
	sb.WriteString("^")
	if !anchored && !hasSlash {
		sb.WriteString("(?:.*/)?")
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '*' && i+1 < len(p) && p[i+1] == '*':
			sb.WriteString(".*")
			i++
		case c == '*':
			sb.WriteString("[^/]*")
		case c == '?':
			sb.WriteString("[^/]")
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString("(?:/.*)?$")
	return regexp.Compile(sb.String())
}

// changedFiles lists files that differ between base and branch.
func changedFiles(base, branch string) ([]string, error) {
	out, err := exec.Command("git", "diff", "--name-only", base+"..."+branch).Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			files = append(files, l)
		}
	}
	return files, nil
}

// ownersForFiles applies CODEOWNERS rules to each file (last matching rule
// wins, per GitHub's semantics) and returns the union of owners, in the
// order they were first encountered, with any leading "@" stripped.
func ownersForFiles(rules []codeownersRule, files []string) []string {
	seen := map[string]bool{}
	var owners []string
	for _, f := range files {
		var matched []string
		for _, r := range rules {
			if r.re.MatchString(f) {
				matched = r.owners
			}
		}
		for _, o := range matched {
			o = strings.TrimPrefix(o, "@")
			if o == "" || seen[o] {
				continue
			}
			seen[o] = true
			owners = append(owners, o)
		}
	}
	return owners
}
