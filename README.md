# rembox - Set up and manage remote dev machines

Docs: https://rembox.aakashns.com

Turn any Ubuntu or Debian server into a locked-down box with secure SSH, developer tools, AI coding agents, and remote access from your phone. Run this command to install `rembox` on Mac/Linux/WSL:

```sh
curl -fsSL https://rembox.aakashns.com/install.sh | sh
```

Run `rembox setup` to begin setting up a remote server.

## Layout

- [`cli/`](cli): the `rembox` command, in Go. `make help` lists the build, test and release targets.
- [`site/`](site): rembox.aakashns.com, as plain HTML served by Caddy, and
  [`site/public/install.sh`](site/public/install.sh), the installer.

## Server scripts

`rembox setup` sends these to the server over ssh, as one script. In the order they run:

- [`lib.sh`](cli/internal/remote/lib.sh): helpers every script starts with, to show and run each command.
- [`setup.sh`](cli/internal/remote/setup.sh): runs the steps below, in two ssh sessions: `start`, then `home`, which carries on as root with `finish`.
- [`check.sh`](cli/internal/remote/check.sh): checks the server runs Ubuntu or Debian, on arm64 or amd64.
- [`hostname.sh`](cli/internal/remote/hostname.sh): makes the server's name in rembox its hostname.
- [`swap.sh`](cli/internal/remote/swap.sh): adds a swap file as big as memory, up to 4 GiB.
- [`git.sh`](cli/internal/remote/git.sh): has apt install gh from GitHub's apt repo; later, as your user, sets your name and email for git and logs gh in to GitHub.
- [`postgres.sh`](cli/internal/remote/postgres.sh): has apt install PostgreSQL from PostgreSQL's apt repo; later, adds a PostgreSQL superuser for your user, so it can run psql as itself.
- [`update.sh`](cli/internal/remote/update.sh): upgrades packages, turns on daily security updates, and installs dev tools (fzf, zoxide and Zellij from their latest releases).
- [`toolchains.sh`](cli/internal/remote/toolchains.sh): installs the latest Go; later, as your user, the latest bun, Node (with Vite+), Python (with uv), Rust (with rustup) and Playwright (with Chromium). Asks before upgrading them.
- [`user.sh`](cli/internal/remote/user.sh): asks for your user's name, and adds it, with a sudo password, your SSH keys and zsh.
- [`sshd.sh`](cli/internal/remote/sshd.sh): moves ssh to the port you give, with keys only and no root login.
- [`firewall.sh`](cli/internal/remote/firewall.sh): turns on ufw, blocking every incoming connection but ssh.
- [`zsh.sh`](cli/internal/remote/zsh.sh): sets up zsh for your user, with plugins, fzf, zoxide, a git prompt, and Zellij on ssh logins, starting locked, with Ctrl+g to unlock it, and no startup tips.
- [`agents.sh`](cli/internal/remote/agents.sh): asks whether to install Claude Code, Codex, OpenCode and Pi, installs each with its own installer, and logs it in, to a subscription or with API billing.
- [`opentunnel.sh`](cli/internal/remote/opentunnel.sh): installs opentunnel, with its own installer, for public links to dev servers, to test them from your phone, and connects its tunnel. Asks before upgrading it.
- [`instructions.sh`](cli/internal/remote/instructions.sh): writes `~/.config/rembox/AGENTS.md`, about the server, and links each agent's global instructions to it.
- [`t3.sh`](cli/internal/remote/t3.sh): makes ~/work for your projects, installs T3 Code, connects it to your devices with T3 Connect, and shows a QR code for its phone app.
- [`fail2ban.sh`](cli/internal/remote/fail2ban.sh): bans IP addresses that keep failing to log in.
- [`locale.sh`](cli/internal/remote/locale.sh): adds your Mac's locale, so programs on the server don't warn.
- [`restart.sh`](cli/internal/remote/restart.sh): restarts the server, if an update needs it and you agree.

## Development

In `cli/`, `make build` builds `bin/rembox`, and `make lint test` checks the code and the server
scripts. To try a change on a real server, use an [OrbStack](https://orbstack.dev) machine:

- `scripts/orb-server.sh NAME [IMAGE]` makes a machine that looks like a fresh cloud server
  (`ubuntu:noble` by default, or `debian:bookworm`), and prints how to log in to it. Then run
  `cli/bin/rembox setup` with those answers, and again with `rembox setup NAME`, to check the
  second run changes nothing. `orb delete -f NAME` removes it.
- `scripts/e2e.sh [IMAGE]` does all that, answering setup's questions itself, in a temp HOME.
- To run one step by itself, send lib.sh, its file and a call to it:
  `orb -m NAME -u root bash -c "$(cat cli/internal/remote/{lib,swap}.sh; echo setup_swap)"`.
- If `NAME.orb.local` says "No route to host", give your terminal Local Network access in
  macOS's settings, or use `127.0.0.1`, which OrbStack forwards to the machine.
- OrbStack machines are containers, so setup skips swap.

## Releasing

Every pull request bumps [`cli/VERSION`](cli/VERSION) (CI checks it). Merging to main runs
[`release.yml`](.github/workflows/release.yml), which builds `rembox-<os>-<arch>` for macOS and
Linux (arm64 and amd64) with a `SHA256SUMS` and build attestations, publishes them as release
`v<VERSION>`, then installs it on each platform to check it works.
