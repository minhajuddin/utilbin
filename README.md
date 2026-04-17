# utilbin

A single Go binary that's a kitchen-sink CLI: URL/base64 codecs, random generators, UUIDs, tmux/git-worktree helpers — plus a self-update mechanism so adding a new command propagates to every machine you've installed it on.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/minhajuddin/utilbin/main/install.sh | bash
```

This drops `utilbin` into `~/.local/bin/` and creates symlinks for every sub-command (`tm`, `urlencode`, `wt`, …) pointing at it. Make sure `~/.local/bin` is on your `PATH`.

## Usage

```sh
utilbin urlencode 'hello world'
utilbin base64 encode < some-file
utilbin rand hex 32
utilbin uuid
utilbin update                    # force update check
```

Every sub-command also works as its own binary name thanks to the symlink farm:

```sh
urlencode 'hello world'
randhex 32
uuid
```

## Auto-update

Each invocation checks for a new release in the background, gated by a 24h timestamp at `~/.cache/utilbin/last-check`. New version → downloaded, atomically swapped over the running binary, picked up on the next invocation.

Knobs:

- `UTILBIN_AUTO_UPDATE=0` — disable background updates entirely
- `UTILBIN_UPDATE_INTERVAL=12h` — change the check interval
- `utilbin update --check` — show what's available without installing
- `utilbin update --force` — install latest regardless of cache

## Adding a new command

1. Drop a new file in `cmd/` that registers a `*cobra.Command` via `registry.Register(...)`.
2. Commit, push, tag `vX.Y.Z` — goreleaser builds and publishes per-platform tarballs to the GitHub Release.
3. On every machine, the next time you run anything, `utilbin` notices the new release and self-updates. Run `utilbin install` once to symlink the new command name (only needed when adding/removing names).
