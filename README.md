# Kishtin

Kishtin, from *cistin*, Irish for kitchen.

A kitchen system for one restaurant. Staff log food safety checks on a tablet or phone, the pass sees every open order, a cook scheduler tells each station what to start and when, and much of it works by voice. It runs on one computer in the restaurant, keeps working with the internet down, and keeps everything in one SQLite file the restaurant owns.

## Documentation

**[cathalbyrnegit.github.io/Kishtin](https://cathalbyrnegit.github.io/Kishtin/)**, built from [`docs/`](docs/).

- [Getting started](docs/getting-started.md): setting Kishtin up in a restaurant, step by step, for non-technical owners.
- [What Kishtin does](docs/features.md): everything built today, and what is not built yet.
- [Keeping the Kishtin computer safe](docs/security.md): disk encryption, updates, backups and restoring.
- [Building and developing Kishtin](docs/developers.md): building from source, the command line, configuration and the development commands.

## Install

On Windows, in PowerShell:

```powershell
irm https://github.com/cathalbyrnegit/kishtin/releases/latest/download/install.ps1 | iex
```

On macOS or Linux, in Terminal:

```sh
curl -fsSL https://github.com/cathalbyrnegit/kishtin/releases/latest/download/install.sh | sh
```

Then open `http://127.0.0.1:8080/` on the same computer to set it up. See [Installing Kishtin](docs/install.md) for updates, options and what it does not do yet. No release has been published yet: until one is, [build from source](docs/developers.md#build-from-source).

## Development

```sh
git submodule update --init   # fetches the kit-ui component library
make build                    # screens, then bin/kishtind and bin/kishtin
make test lint                # Go tests and golangci-lint
make docs-serve               # this documentation, with live reload on :8000
```

Go 1.27 or later and Bun 1.4.4. The rest of the commands are in [Building and developing Kishtin](docs/developers.md#development). [AGENTS.md](AGENTS.md) has the plan and the rules for contributors and coding agents. Tests, fixtures and docs use synthetic data only.

## Security

To report a security problem, see [SECURITY.md](SECURITY.md).

## Licence

Kishtin is licensed under the [Apache License, Version 2.0](LICENSE). See [NOTICE](NOTICE). It includes open-source software under its own licences (MIT, BSD and Apache 2.0); `kishtind licenses` prints them all, and they are kept in [internal/legal/licenses.txt](internal/legal/licenses.txt).
