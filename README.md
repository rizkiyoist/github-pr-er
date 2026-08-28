# github-pr-er

Create matching PROD (`main`) and STAGE (`develop`) pull requests from your
current git branch in one command.

Given branch `feature/something-cool`, it opens:

- `[PROD] Something Cool` → base `main`
- `[STAGE] Something Cool` → base `develop`

## Requirements

- [GitHub CLI](https://cli.github.com/) (`gh`), authenticated via `gh auth login`
- Go 1.21+ to build

## Install

```sh
go build -o github-pr-er.exe .
```

Put the resulting binary somewhere on your `PATH`.

## Usage

Run it from inside your repo on the feature branch you want to open PRs for:

```sh
github-pr-er
```

It shows a summary and asks for confirmation before creating anything.

### Flags

| Flag           | Default   | Description                                  |
|----------------|-----------|-----------------------------------------------|
| `--prod-base`  | `main`    | Base branch for the PROD pull request         |
| `--stage-base` | `develop` | Base branch for the STAGE pull request        |
| `--body`       | `""`      | Body text for both pull requests              |
| `--draft`      |           | Create pull requests as drafts                |
| `--push`       |           | Push the current branch to origin first       |
| `--only`       |           | Create only one PR: `prod` or `stage`         |
| `-y`           |           | Skip the confirmation prompt                  |
