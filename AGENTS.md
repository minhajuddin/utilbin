# utilbin — notes for agents

Read `~/r/dot3/agent-guidance.md` and `~/r/software-design/CORE.md` first; this
file is what they do not cover for this repo.

utilbin is one cobra binary that is also a BusyBox-style argv[0] dispatcher: every
registered name works as `utilbin <name>` and as its own symlink in `~/.local/bin`
(`tm`, `wt`, `urlencode`, `randhex`, …). `README.md` covers install and the
auto-update knobs; this file is the contract for changing it.

## Commands

```sh
go build -o ~/.local/bin/utilbin .
go test ./...
utilbin install            # only needed when a name was added or removed
utilbin update --check     # what the next self-update would install
```

## Registration

Everything goes through `internal/registry`. Do not hand-wire a command onto
`Root`:

- `registry.Register(name, aliases, cmd)` — top-level subcommand **and** a symlink
  under that name plus aliases.
- `registry.RegisterShim(name, cmd)` — symlink only, for a name that belongs to a
  longer path (`randhex` beside `rand hex`) and must not pollute `utilbin --help`.
- `registry.RegisterMeta(name, cmd)` — reachable only as `utilbin <name>`, no
  symlink, so `install`/`update` never shadow a system binary.

`Entry.Standalone` decides whether cobra attaches it to `Root`; `Entry.NoSymlink`
is set by `RegisterMeta`. The `install` command creates symlinks from
`registry.Names()`, so a new name needs no other bookkeeping.

## Invariants

- **A published name is an interface.** It is a symlink on every installed machine
  and is likely written into shell aliases, tmux configs and agent instructions.
  Rename by adding the new name and keeping the old one as an alias; never delete
  a name silently.
- **One binary, one release.** Adding a command means: a file in `cmd/` that
  registers itself in `init()`, tests, then commit, tag `vX.Y.Z`, push — goreleaser
  builds the per-platform tarballs and each machine's next invocation self-updates.
  `utilbin install` is only re-run when names change.
- **The dispatcher must keep working for both spellings.** `utilbin wt` and `wt`
  are the same code path; a name that resolves under one and not the other is a
  regression.
- **Auto-update never blocks a command.** The check is backgrounded and gated by
  `~/.cache/utilbin/last-check` (24 h; `UTILBIN_AUTO_UPDATE=0`,
  `UTILBIN_UPDATE_INTERVAL`); the swap is atomic and picked up on the next
  invocation. Do not make a command wait on the network.
- `install.sh` is the bootstrap (`curl … | bash`) and `~/.local/bin` symlinks
  everything; keep it consistent with what `install` does.
- Version comes from `-ldflags "-X main.version=…"`; `utilbin version` must always
  answer which build it is.

## For agents

These are the same commands the user types, so prefer them over an ad-hoc shell
pipeline: `tm` (tmux), `wt` (worktrees), `co`/`wip`/`new`/`x` (git + PR flow),
`pr-review`/`pr-worktree`, `urlencode`/`urldecode`, `base64encode`/`base64decode`,
`uuid`, `rand*`, `log-id`, `mb`, `zj`/`zz`, `can-i-eat`, `shell-init`.

A new behaviour that only one session needs belongs in the session, not here; a
behaviour worth keeping is a registered command with a test, because it has to
survive the next release and reach every machine by itself.
