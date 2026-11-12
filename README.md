# rembox

Set up and manage remote development machines, from your Mac.

```sh
curl -fsSL https://rembox.org/install.sh | sh
```

See [rembox.org](https://rembox.org) for the docs. Until the site is live, install with
`curl -fsSL https://raw.githubusercontent.com/aakashns/rembox/main/site/public/install.sh | sh`.

## Layout

- [`cli/`](cli): the `rembox` command, in Go. `make help` lists the build, test and release targets.
- [`site/`](site): rembox.org, and [`site/public/install.sh`](site/public/install.sh), the installer.

## Releasing

Every pull request bumps [`cli/VERSION`](cli/VERSION) (CI checks it). Merging to main runs
[`release.yml`](.github/workflows/release.yml), which builds `rembox-<os>-<arch>` for macOS and
Linux (arm64 and amd64) with a `SHA256SUMS` and build attestations, publishes them as release
`v<VERSION>`, then installs it on each platform to check it works.
