// ghprer creates a PROD (-> main) and STAGE (-> develop) pull request
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

	reviewers := pickReviewers(stdin)

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
		out, errOut, err := runCapture("gh", args...)
		if err != nil {
			if strings.Contains(errOut, "already exists") {
				fmt.Printf("%s PR already exists, looking up its URL...\n", s.label)
				url, lookupErr := existingPRURL(s.base, branch)
				if lookupErr != nil {
					fmt.Fprintf(os.Stderr, "could not look up existing %s PR: %v\n", s.label, lookupErr)
					failures++
					continue
				}
				if len(reviewers) > 0 {
					if err := run("gh", "pr", "edit", url, "--add-reviewer", strings.Join(reviewers, ",")); err != nil {
						fmt.Fprintf(os.Stderr, "could not add reviewers to existing %s PR: %v\n", s.label, err)
					}
				}
				urls[s.label] = url
				continue
			}
			if strings.Contains(strings.ToLower(errOut), "no commits between") {
				fmt.Printf("%s PR skipped: no commits between %s and %s\n", s.label, s.base, branch)
				continue
			}
			fmt.Fprintf(os.Stderr, "failed to create %s PR: %v\n", s.label, err)
			failures++
			continue
		}
		urls[s.label] = lastLine(out)
	}

	if len(urls) > 0 {
		fmt.Printf("\nMohon review PR %s\n", strings.ReplaceAll(branch, "/", " "))
		if u, ok := urls["STAGE"]; ok {
			fmt.Println(u)
		}
		if u, ok := urls["PROD"]; ok {
			fmt.Println(u)
		}
	}

	if failures > 0 {
		os.Exit(1)
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

// runCapture behaves like run but also returns everything written to
// stdout and stderr, while still echoing both to the terminal live.
func runCapture(name string, args ...string) (stdout string, stderr string, err error) {
	cmd := exec.Command(name, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &outBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	cmd.Stdin = os.Stdin
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// existingPRURL looks up the URL of an already-open pull request from
// branch into base, used when "gh pr create" reports one already exists.
func existingPRURL(base, branch string) (string, error) {
	out, err := exec.Command("gh", "pr", "list",
		"--head", branch, "--base", base, "--state", "open",
		"--json", "url", "--jq", ".[0].url").Output()
	if err != nil {
		return "", err
	}
	url := strings.TrimSpace(string(out))
	if url == "" {
		return "", fmt.Errorf("no existing PR found for %s -> %s", branch, base)
	}
	return url, nil
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

// pickReviewers lists the repo's collaborators (the same people selectable
// as reviewers in GitHub's own PR UI) and lets the user interactively pick
// from them. Returns nil if the repo/collaborators can't be determined or
// the user picks none.
func pickReviewers(reader *bufio.Reader) []string {
	candidates, err := reviewCandidates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not list reviewers: %v\n", err)
	}

	fmt.Println("\nReviewers:")
	fmt.Println("  0 @copilot")
	for i, c := range candidates {
		fmt.Printf("  %d %s\n", i+1, c)
	}
	fmt.Print("Choose reviewers (comma separated numbers, blank for default 0,3,5,7,10): ")
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		line = "0,3,5,7,10"
	}

	var chosen []string
	for _, part := range strings.Split(line, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > len(candidates) {
			fmt.Fprintf(os.Stderr, "skipping invalid selection %q\n", part)
			continue
		}
		if n == 0 {
			chosen = append(chosen, "@copilot")
			continue
		}
		chosen = append(chosen, candidates[n-1])
	}
	return chosen
}

// reviewCandidates returns the current repo's collaborators, excluding the
// authenticated user (GitHub doesn't allow requesting your own review).
func reviewCandidates() ([]string, error) {
	repo, err := exec.Command("gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner").Output()
	if err != nil {
		return nil, fmt.Errorf("could not determine current repo: %w", err)
	}

	me, err := exec.Command("gh", "api", "user", "--jq", ".login").Output()
	if err != nil {
		return nil, fmt.Errorf("could not determine authenticated user: %w", err)
	}
	myLogin := strings.TrimSpace(string(me))

	out, err := exec.Command("gh", "api", fmt.Sprintf("repos/%s/collaborators", strings.TrimSpace(string(repo))),
		"--paginate", "--jq", ".[].login").Output()
	if err != nil {
		return nil, fmt.Errorf("could not list collaborators: %w", err)
	}

	var users []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l == myLogin {
			continue
		}
		users = append(users, l)
	}
	return users, nil
}
