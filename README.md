# CPQ

CPQ is a local Linux tray utility for showing coding-plan quota usage across
supported AI providers.

It currently supports Codex and MiniMax. Codex quota is read through the
installed Codex CLI, while MiniMax quota is read with per-account API keys stored
in the OS keyring. CPQ does not store Codex credentials.

## Features

- System tray usage percentage for the active coding-plan account
- Account manager for switching between Codex and MiniMax profiles
- 5-hour and weekly coding-plan usage windows
- Native dashboard with dark and light mode
- Localhost-only MiniMax API proxy flows for CLI/proxy use
- OS keyring storage for MiniMax API keys
- SQLite history storage for legacy usage snapshots
- Linux release artifacts for source archive, Debian package, RPM, and AppImage

## Requirements

- Linux desktop with system tray support
- Codex CLI installed and logged in for Codex quota
- MiniMax API key for MiniMax quota accounts
- Go 1.26.2 or newer for building from source
- OS keyring support for storing MiniMax API keys

Check Codex login:

```bash
codex login status
```

## Build

```bash
GOCACHE=/tmp/gocache go test ./...
GOCACHE=/tmp/gocache go vet ./...
GOCACHE=/tmp/gocache go build -o cpq .
```

Run:

```bash
./cpq
```

CLI mode for the older MiniMax flow is still available:

```bash
./cpq --cli
```

View recorded usage history:

```bash
./cpq history
```

Run the localhost MiniMax proxy:

```bash
./cpq proxy --port 11434
```

Open the tray menu or the dashboard's `Accounts` button to add, switch, logout,
or remove provider profiles. Codex profiles use the local Codex CLI session;
MiniMax profiles store API keys in the OS keyring.

## Accounts

Profiles are stored in the local CPQ config file:

```text
~/.config/coplanage/config.json
```

MiniMax API keys are stored separately in the OS keyring service:

```text
coplanage
```

Codex profiles do not store tokens. They use the active Codex CLI login, or an
optional `CODEX_HOME` path when you add a separate Codex profile.

## Releases

The application identity is `coplanage`, while the command name is `cpq`.

Tagged releases publish Linux artifacts on GitHub:

- `cpq-<version>-linux-amd64.tar.gz`
- `cpq_<version>_amd64.deb`
- `cpq-<version>-*.rpm`
- `cpq-<version>-x86_64.AppImage`
- `SHA256SUMS`

Create a release by pushing a version tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Build release artifacts locally:

```bash
scripts/package-linux.sh 0.1.0 amd64
```

Artifacts are written to `dist/`.

## Security Notes

- Codex credentials remain managed by the Codex CLI.
- MiniMax API keys, if used, are stored in the OS keyring service `coplanage`.
- Local login/proxy servers bind to `127.0.0.1` only.
- The Codex quota integration depends on Codex CLI's internal app-server API. If
  Codex changes that API, CPQ may need an update.

## License

MIT. See [LICENSE](LICENSE).
