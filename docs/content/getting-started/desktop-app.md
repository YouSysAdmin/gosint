---
title: Desktop app
description: "gosint on your own machine in a native window, for macOS, Linux and Windows, with no server to run."
weight: 40
---

The desktop app is gosint for one person on one machine. It starts a gosint
node that only your computer can reach, and shows the console in a native
window drawn by the system's web view (WebKit on macOS and Linux, WebView2 on
Windows). Closing the window stops the node.

Everything works as on a server: investigations, transforms, flows, files, the
assistant. The data is encrypted the same way and lives in one directory of
your user account.

## Download and install

Each [release](https://github.com/YouSysAdmin/gosint/releases) carries the
desktop app beside the server archives:

| Platform | File |
| --- | --- |
| macOS Apple Silicon | `gosint-desktop_v<version>_darwin_arm64.zip` |
| Linux amd64, arm64 | `gosint-desktop_v<version>_linux_<arch>.deb` or `.tar.gz` |
| Windows amd64 | `gosint-desktop_v<version>_windows_amd64.zip` |

Each file has a `.sha256` beside it.

### macOS

Unzip the archive and move `gosint.app` to Applications. It needs macOS 12 or
newer.

The app is not notarized by Apple, so the first launch is blocked by
Gatekeeper. Clear the quarantine flag once:

```bash
xattr -d com.apple.quarantine /Applications/gosint.app
```

### Linux

On Debian, Ubuntu and their relatives install the package, which pulls in
WebKitGTK:

```bash
sudo apt install ./gosint-desktop_v1.2.0_linux_amd64.deb
```

It adds `gosint` to the applications menu. The `.tar.gz` holds the binary, a
`gosint.desktop` entry and an icon for other distributions. There you need
`libwebkit2gtk-4.1` and GTK 3 installed yourself. The file dialogs use
`zenity` and links open through `xdg-open`, so have both installed.

### Windows

Unzip the archive anywhere and run `gosint-desktop.exe`. The window needs the
Microsoft Edge WebView2 Runtime, which Windows 10 and 11 normally ship. When it
is missing the app says so and offers Microsoft's download page.

Windows SmartScreen may warn about an unsigned program on the first run.

## First start

The app opens on the same steps as a server:

1. **Protect this server**: choose the unseal key that encrypts everything.
   It is asked at every start of the app. Without it the data cannot be read,
   and it cannot be recovered.
2. **Set up gosint**: create your account, which is the administrator.

See [First run](/getting-started/first-run/) for both.

After that a restart of the app asks only the unseal key. You stay signed in:
a session lasts 7 days without use and 30 days at most, then you sign in again.

## Where the data lives

| System | Directory |
| --- | --- |
| macOS | `~/Library/Application Support/gosint` |
| Linux | `$XDG_DATA_HOME/gosint`, by default `~/.local/share/gosint` |
| Windows | `%LOCALAPPDATA%\gosint` |

The directory holds the database `gosint.db`, the attached files, the cache,
your [transform files](/extending/transform-files/) in `transforms`, and the
log in `gosint.log`. Back it up as a whole.

It also holds a `gosint.yaml`, created empty on the first start. It takes the
same keys as a server's [configuration](/reference/configuration/), so you can
for example keep sessions longer:

```yaml
auth:
  session_ttl: 720h      # 30 days without use
  session_max_life: 0    # no limit from the sign-in
```

The app ignores any `gosint.yaml` in the current directory, only this one is
read. The address the node listens on and TLS are fixed by the app.

## What differs from the browser

- **Saving a file** (an export, a CSV, a downloaded attachment, the graph as
  PNG) opens the system's save dialog instead of the browser's downloads.
- **Links to other sites** open in your default browser, not in the window.
- **Passkeys** are not offered: the web view has no authenticator. Use a
  password, with an authenticator app code if you like. See
  [Your account](/guide/account/).
- **One instance at a time**: a second launch says gosint is already running.
- **Update notice**: when a newer stable release is out, an **Update** badge
  beside your avatar links to its release notes. Turn it off with
  `console.update_check: false` in `gosint.yaml`.

## Build it yourself

The window links the system web view, so the app is built on the platform it
runs on, with a C compiler (cgo). On Linux install `libwebkit2gtk-4.1-dev`
first, on Windows a MinGW gcc.

```bash
task desktop           # the console and bin/gosint-desktop
task desktop-package   # the same, packed into dist/ for this machine
```

`task desktop-package` makes an ad hoc signed `gosint.app` in a zip on macOS
(it needs `rsvg-convert` for the icon), a `.tar.gz` and a `.deb` on Linux (it needs `nfpm`), and a zip on
Windows. Set `VERSION=v1.2.0` to name the files.
