# Contributing

## Issues

Issues are very valuable to this project.

* Ideas are a valuable source of contributions others can make
* Problems show where this project is lacking
* With a question you show where contributors can improve the user experience

Thank you for creating them.

## Pull Requests

Pull requests are, a great way to get your ideas into this repository.

When deciding if I merge in a pull request I look at the following things:

### Does it state intent

You should be clear which problem you're trying to solve with your contribution.

For example:

> Add link to code of conduct in README.md

Doesn't tell me anything about why you're doing that

> Add link to code of conduct in README.md because users don't always look in the CONTRIBUTING.md

Tells me the problem that you have found, and the pull request shows me the action you have taken to solve it.

### Is it of good quality

* There are no spelling mistakes
* It reads well
* For english language contributions: Has a good score on [Grammarly](https://www.grammarly.com) or [Hemingway App](http://www.hemingwayapp.com/)

### Does it move this repository closer to my vision for the repository

The aim of this repository is:

* To provide a README.md and assorted documents anyone can copy and paste, into their project
* The content is usable by someone who hasn't written something like this before
* Foster a culture of respect and gratitude in the open source community.

### Does it follow the contributor covenant

This repository has a [code of conduct](CODE_OF_CONDUCT.md), This repository has a code of conduct, I will remove things that do not respect it.

## Development

### Prerequisites

Install [`mise`](https://mise.jdx.dev/) and run `mise install` to pick up the Go version and `hk` Git hooks from `mise.toml`.

### Linting

We use `golangci-lint` in order to maintain a good code quality and a consistent style.

Get the `golangci-lint` by following their installation instructions on [golangci-lint.run](https://golangci-lint.run/) or simply run:

```shell
bin install github.com/golangci/golangci-lint
```

Lint the source code by executing:

```shell
golangci-lint run ./...
```

Or via the `mise` task:

```shell
mise run lint
```

Use `mise run verify` for the full non-mutating check suite (`go mod download`, formatting, `go mod tidy -diff`, and `golangci-lint run`), and `mise run fix` to apply formatting and module tidy fixes.

### Git hooks

Install the repository hooks once per clone:

```shell
mise run hooks
```

This runs `hk install --mise`, wiring `hk.pkl` into `.git/hooks` with the `mise` environment active. Hooks enforce `gofmt` on staged files and [Conventional Commits](https://www.conventionalcommits.org/) on commit messages.
