# github-pr-er

Create matching PROD (`main`) and STAGE (`develop`) pull requests from your
current git branch, in one command.

Given branch `feature/something-cool`, it opens:

- `[PROD] Something Cool` → base `main`
- `[STAGE] Something Cool` → base `develop`

## Requirements

- [GitHub CLI](https://cli.github.com/) (`gh`), authenticated via `gh auth login`
- Go 1.21+ to build

## Install

```sh
go build -o ghprer .
```

Put the resulting binary somewhere on your `PATH`.

## Usage

Run it from your feature branch, inside the repo:

```sh
ghprer
```

It will:

1. Show a summary and ask for confirmation.
2. Let you pick reviewers by number from the repo's collaborators
   (`0` is always Copilot).
3. Create the PR(s) — or, if one already exists, reuse it and add any
   newly picked reviewers to it.
4. Print a short "Mohon review" message with the PR link(s).

### Flags

| Flag           | Default   | Description                             |
|----------------|-----------|------------------------------------------|
| `--prod-base`  | `main`    | Base branch for the PROD pull request    |
| `--stage-base` | `develop` | Base branch for the STAGE pull request   |
| `--body`       | `""`      | Body text for both pull requests         |
| `--draft`      |           | Create pull requests as drafts           |
| `--push`       |           | Push the current branch to origin first  |
| `--only`       |           | Create only one PR: `prod` or `stage`    |
| `-y`           | `true`    | Skip the confirmation prompt             |

## Example

```
$ ghprer
Branch: hotfix/jobdesc-auth
  PROD  [PROD] Jobdesc Auth  <-  hotfix/jobdesc-auth   base=main
  STAGE [STAGE] Jobdesc Auth  <-  hotfix/jobdesc-auth   base=develop

Reviewers:
  0 @copilot
  1 alice
  2 bob
  3 carol
  4 dave
  5 erin
Choose reviewers (comma separated numbers, blank for none): 2,4

==> creating PROD PR (base: main)
a pull request for branch "hotfix/jobdesc-auth" into branch "main" already exists:
https://github.com/your-org/your-repo/pull/657
PROD PR already exists, looking up its URL...
https://github.com/your-org/your-repo/pull/657

==> creating STAGE PR (base: develop)
a pull request for branch "hotfix/jobdesc-auth" into branch "develop" already exists:
https://github.com/your-org/your-repo/pull/658
STAGE PR already exists, looking up its URL...
https://github.com/your-org/your-repo/pull/658

Mohon review PR hotfix jobdesc-auth
https://github.com/your-org/your-repo/pull/658
https://github.com/your-org/your-repo/pull/657
```