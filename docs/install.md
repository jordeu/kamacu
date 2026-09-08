# Installing the Kamacu app

This document is written to be executed by a coding agent, not read as prose. If
you are an agent and were pointed here, follow the steps in order, run the
verifications yourself, and report the results. Substitute real values wherever
you see `<ANGLE-BRACKET-PLACEHOLDERS>`. The same steps cover a fresh install
and an update of an existing install.

## 0. The instruction this document serves

A user installs or updates Kamacu by pasting this line to you, verbatim:

```
read https://raw.githubusercontent.com/jordeu/kamacu/master/docs/install.md and install kamacu
```

You are the intended executor of that line: carry out steps 1–7 below.

## 1. What you are installing

Kamacu is a single self-contained binary (Go server with the web frontend
embedded) downloaded from the latest GitHub release at
https://github.com/jordeu/kamacu/releases. Run as `kamacu serve`, it serves the
app at http://127.0.0.1:7333 (localhost-only by design).

To install you need only `curl`, `tar`, and a sha256 tool — no Go, no Node.
`git` and `tmux` are runtime prerequisites of the app itself (worktrees and
durable bash tabs); warn about them if missing, but do not block the install.

## 2. Preflight — run these first

### 2.1 OS and architecture

```sh
uname -s   # Darwin (macOS) or Linux
uname -m   # arm64 | aarch64 | x86_64
```

Map to the release asset suffix (`<OS>_<ARCH>`):

| `uname -s` | `uname -m` | asset suffix |
| --- | --- | --- |
| Darwin | arm64 | `darwin_arm64` |
| Darwin | x86_64 | `darwin_amd64` |
| Linux | arm64 or aarch64 | `linux_arm64` |
| Linux | x86_64 | `linux_amd64` |

Anything else: stop and report — no release asset exists for this platform
(building from source is described in the README).

### 2.2 Is kamacu already installed? (decides fresh install vs update)

```sh
command -v kamacu || echo NOT_ON_PATH
kamacu --version 2>/dev/null || true
```

Record three things when a path prints:

1. The absolute path of the binary (resolve symlinks to their target; if it is
   relative, run it through `readlink -f`). This is `<DEST>` — an update
   replaces the binary **in place** at the same path.
2. The current version (`kamacu --version` prints it without the `v`, e.g.
   `1.14.0`).
3. Whether a boot service from step 5 already exists
   (`~/.config/systemd/user/kamacu.service` on Linux,
   `~/Library/LaunchAgents/com.jordeu.kamacu.plist` on macOS) and whether the
   app is running:

   ```sh
   curl -fsS http://127.0.0.1:7333/api/projects -o /dev/null -w '%{http_code}\n'
   # 200 = app running; connection refused = not running
   ```

If `NOT_ON_PATH`, this is a fresh install: `<DEST>` will be
`$HOME/.local/bin/kamacu` (created in step 3).

### 2.3 Tools this guide needs

```sh
command -v curl tar && (command -v sha256sum || command -v shasum)
```

All must resolve (`shasum` is the macOS spelling). If something is missing,
stop and report.

## 3. Download, verify, install

### 3.1 Resolve the latest release

```sh
TAG=$(basename "$(curl -fsSIL -o /dev/null -w '%{url_effective}' https://github.com/jordeu/kamacu/releases/latest)")
VER=${TAG#v}
```

`$TAG` must look like `v1.14.0` — if it does not, stop and report. If step 2.2
found an installed version equal to `$VER`, skip to step 5 (the binary is
already current; only the boot question may still be pending).

### 3.2 Download and verify the checksum

Using `<OS>_<ARCH>` from step 2.1:

```sh
TMP=$(mktemp -d)
ASSET="kamacu_${VER}_${OS}_${ARCH}.tar.gz"
curl -fsSLo "$TMP/$ASSET"      "https://github.com/jordeu/kamacu/releases/download/$TAG/$ASSET"
curl -fsSLo "$TMP/checksums.txt" "https://github.com/jordeu/kamacu/releases/download/$TAG/checksums.txt"
EXPECTED=$(awk -v f="$ASSET" '$2 == f {print $1}' "$TMP/checksums.txt")
ACTUAL=$(cd "$TMP" && { sha256sum "$ASSET" 2>/dev/null || shasum -a 256 "$ASSET"; } | cut -d' ' -f1)
[ -n "$EXPECTED" ] && [ "$EXPECTED" = "$ACTUAL" ] || echo "CHECKSUM MISMATCH"
```

`CHECKSUM MISMATCH` (or an empty `$EXPECTED`) means a corrupted download —
**do not install**. Delete `$TMP`, retry the two downloads once, and stop with
a report if it fails again.

### 3.3 Install the binary

The tarball holds `kamacu` at its root (plus `LICENSE`, `README.md`):

```sh
tar -xzf "$TMP/$ASSET" -C "$TMP"
mkdir -p "$(dirname "<DEST>")"
install -m 0755 "$TMP/kamacu" "<DEST>.new" && mv -f "<DEST>.new" "<DEST>"
rm -rf "$TMP"
```

The `.new` + `mv` sequence is a rename(2): atomic, and safe even if the app is
currently running (the running process keeps the old inode until restarted —
see step 4). Never `cp` over a running binary (fails with `Text file busy`).

For a fresh install `<DEST>` is `$HOME/.local/bin/kamacu`. For an update it is
the path recorded in step 2.2.

### 3.4 PATH (fresh install only)

```sh
case ":$PATH:" in *":$HOME/.local/bin:"*) echo ON_PATH ;; *) echo OFF_PATH ;; esac
```

If `OFF_PATH`, tell the user to add `export PATH="$HOME/.local/bin:$PATH"` to
their shell rc (`~/.bashrc`, `~/.zshrc`, …) — do not edit the rc file yourself
unless asked. In this session, call the binary by its absolute path.

### 3.5 Verify

```sh
"<DEST>" --version
```

It must print `$VER` exactly. If this was an update, report old → new version
to the user.

## 4. Restart after an update (updates only)

- **Boot service exists** (step 2.2) — restart it with the step 5 commands so
  the service picks up the new binary, then re-verify with step 6.
- **App running manually** — the running instance still serves the old
  version. Tell the user to restart it (`kamacu serve`) when convenient; do
  not kill their process yourself.
- **App not running** — nothing to do.

## 5. Run at boot? — ask the user, do not guess

If a boot service already exists (step 2.2), do not re-ask: just make sure it
is enabled and running after an update. Otherwise **stop and ask the user
now**:

> Should Kamacu run as a background service, started automatically at boot?
> Or will you start it manually with `kamacu serve` when you need it?

Then follow exactly one of the sections below. `<KAMACU_BIN>` below is
`<DEST>` fully resolved (no `~`, no `$HOME` — unit files do not expand them;
on Linux you may keep the `%h` specifier exactly as written).

### 5A. Background service — Linux (systemd user unit)

```sh
mkdir -p "$HOME/.config/systemd/user"
cat > "$HOME/.config/systemd/user/kamacu.service" <<EOF
[Unit]
Description=Kamacu (local-only multi-agent kanban)
After=network.target

[Service]
ExecStart=%h/.local/bin/kamacu serve
Restart=on-failure

[Install]
WantedBy=default.target
EOF
systemctl --user daemon-reload
systemctl --user enable --now kamacu.service
```

If `<DEST>` is not `$HOME/.local/bin/kamacu`, write the resolved absolute path
in `ExecStart` instead of `%h/.local/bin/kamacu`.

By default a user unit starts at **login**, not boot. For it to run from
power-on with nobody logged in, also run `loginctl enable-linger "$USER"` —
only with the user's consent.

Logs: `journalctl --user -u kamacu -f`. Control: `systemctl --user
{status,restart,stop} kamacu`.

### 5B. Background service — macOS (LaunchAgent)

```sh
KAMACU_BIN=<KAMACU_BIN>   # resolved absolute path of the installed binary
mkdir -p "$HOME/Library/LaunchAgents" "$HOME/.kamacu/log"
cat > "$HOME/Library/LaunchAgents/com.jordeu.kamacu.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.jordeu.kamacu</string>
  <key>ProgramArguments</key>
  <array>
    <string>$KAMACU_BIN</string>
    <string>serve</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$HOME/.kamacu/log/kamacu.out.log</string>
  <key>StandardErrorPath</key><string>$HOME/.kamacu/log/kamacu.err.log</string>
</dict>
</plist>
EOF
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.jordeu.kamacu.plist"
```

`<KAMACU_BIN>` is `<DEST>` fully expanded (e.g.
`/Users/you/.local/bin/kamacu`) — launchd does not expand `~` or `$HOME`. The
LaunchAgent loads at login/GUI boot; `KeepAlive` restarts it if it dies.
If `bootstrap` fails because the label is already loaded
(`Bootstrap failed: 5`), run `launchctl bootout "gui/$(id -u)/com.jordeu.kamacu"`
first and retry.

Restart after an update: `launchctl kickstart -k "gui/$(id -u)/com.jordeu.kamacu"`.
Logs: `tail -f "$HOME/.kamacu/log/kamacu.out.log"`.

### 5C. Manual — no service

Install nothing. Tell the user: start the app with `kamacu serve` (or
`<KAMACU_BIN> serve` if `$HOME/.local/bin` is not on PATH yet) and open
http://127.0.0.1:7333.

## 6. Verify the app answers (if it is supposed to be running)

```sh
curl -fsS http://127.0.0.1:7333/api/projects -o /dev/null -w '%{http_code}\n'
```

- `200` — the app is up. Report success: version, `<DEST>`, PATH state, and
  the boot choice from step 5.
- `connection refused` — for a service install, check its status and logs
  (`systemctl --user status kamacu` / the `tail -f` above), fix, retry. For a
  manual choice this is expected until the user first runs `kamacu serve` —
  say so in your report.

## 7. Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| `uname` pair not in the step 2.1 table | No release asset for this platform | Report; building from source is described in the README |
| `CHECKSUM MISMATCH` twice | Corrupted or tampered download | Stop; do not install; report both hash values to the user |
| `Text file busy` while installing | Binary was copied over instead of renamed | Use the exact `install … "<DEST>.new" && mv -f` sequence from step 3.3 |
| `command -v kamacu` still fails after a fresh install | `$HOME/.local/bin` not on PATH in this shell | Add the export line to the shell rc (user does it, or asks you to), use the absolute path meanwhile |
| `systemctl --user` fails with `Failed to connect to bus` | No user session bus (common over plain SSH) | Run from a logged-in session or GUI terminal; or `loginctl enable-linger "$USER"` and retry |
| Service enabled but step 6 shows `connection refused` | Another instance holds port 7333, or the unit failed | `systemctl --user status kamacu` + `journalctl --user -u kamacu -n 50`; if a manual instance holds the port, stop it and restart the service |
| `launchctl bootstrap` fails with `Bootstrap failed: 5` | Label already loaded | `launchctl bootout "gui/$(id -u)/com.jordeu.kamacu"` then retry the bootstrap |
| Updated binary but app still reports the old version | The running process kept the old inode by design | Restart: service → step 4 commands; manual → the user reruns `kamacu serve` |
