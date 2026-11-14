# rembox

Set up and manage remote development machines, from your Mac.

```sh
curl -fsSL https://rembox.aakashns.com/install.sh | sh
```

See [rembox.aakashns.com](https://rembox.aakashns.com) for the docs. Until the site is live, install with
`curl -fsSL https://raw.githubusercontent.com/aakashns/rembox/main/site/public/install.sh | sh`.

## Layout

- [`cli/`](cli): the `rembox` command, in Go. `make help` lists the build, test and release targets.
- [`site/`](site): rembox.aakashns.com, as plain HTML served by Caddy, and
  [`site/public/install.sh`](site/public/install.sh), the installer.

## Server scripts

`rembox setup` sends these to the server over ssh, as one script. In the order they run:

- [`lib.sh`](cli/internal/remote/lib.sh): helpers every script starts with, to show and run each command.
- [`setup.sh`](cli/internal/remote/setup.sh): runs the steps below, in three ssh sessions: `start`, `home` and `finish`.
- [`check.sh`](cli/internal/remote/check.sh): checks the server runs Ubuntu or Debian, on arm64 or amd64.
- [`hostname.sh`](cli/internal/remote/hostname.sh): makes the server's name in rembox its hostname.
- [`update.sh`](cli/internal/remote/update.sh): upgrades packages, turns on daily security updates, and installs dev tools (fzf and zoxide from their latest releases).
- [`user.sh`](cli/internal/remote/user.sh): asks for your user's name, and adds it, with a sudo password and your SSH keys.
- [`sshd.sh`](cli/internal/remote/sshd.sh): moves ssh to a new port, with keys only and no root login.
- [`firewall.sh`](cli/internal/remote/firewall.sh): turns on ufw, blocking every incoming connection but ssh.
- [`zsh.sh`](cli/internal/remote/zsh.sh): sets up zsh for your user, with plugins, fzf, zoxide and a git prompt.
- [`fail2ban.sh`](cli/internal/remote/fail2ban.sh): bans IP addresses that keep failing to log in.
- [`swap.sh`](cli/internal/remote/swap.sh): adds a swap file as big as memory, up to 4 GiB.
- [`locale.sh`](cli/internal/remote/locale.sh): adds your Mac's locale, so programs on the server don't warn.
- [`restart.sh`](cli/internal/remote/restart.sh): restarts the server, if an update needs it and you agree.

## Releasing

Every pull request bumps [`cli/VERSION`](cli/VERSION) (CI checks it). Merging to main runs
[`release.yml`](.github/workflows/release.yml), which builds `rembox-<os>-<arch>` for macOS and
Linux (arm64 and amd64) with a `SHA256SUMS` and build attestations, publishes them as release
`v<VERSION>`, then installs it on each platform to check it works.
