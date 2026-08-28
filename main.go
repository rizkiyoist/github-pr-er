// github-pr-er creates a PROD (-> main) and STAGE (-> develop) pull request
// from the current git branch, titled from the branch name.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	prodBase := flag.String("prod-base", "main", "base branch for the PROD pull request")
	stageBase := flag.String("stage-base", "develop", "base branch for the STAGE pull request")
	body := flag.String("body", "", "body text for both pull requests")
	draft := flag.Bool("draft", false, "create pull requests as drafts")
	push := flag.Bool("push", false, "push the current branch to origin before creating PRs")
	only := flag.String("only", "", "only create one PR: \"prod\" or \"stage\"")
	yes := flag.Bool("y", false, "skip confirmation prompt")
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

	if !*yes && !confirm("Create these pull request(s)?") {
		fmt.Println("Aborted.")
		return
	}

	if *push {
		if err := run("git", "push", "-u", "origin", branch); err != nil {
			fatal(fmt.Errorf("git push failed: %w", err))
		}
	}

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
		fmt.Printf("\n==> creating %s PR (base: %s)\n", s.label, s.base)
		if err := run("gh", args...); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create %s PR: %v\n", s.label, err)
			failures++
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

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
