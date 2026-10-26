# Installing Kishtin

Kishtin runs on one computer in the restaurant, which every screen in the kitchen connects to. It is two programs: `kishtind` (the server) and `kishtin` (the command line). This page installs both from a release.

## What you need

- A computer that stays on in the restaurant: a small PC, a Mac, or an office computer. Linux, macOS or 64-bit Windows, on an Intel/AMD (amd64) or ARM (arm64) processor.
- An internet connection to download Kishtin. After that, Kishtin works without the internet.
- Nothing else. There is no account to make and nothing else to install.

## Install

### Windows

1. Open the Start menu, type **PowerShell**, and open **Windows PowerShell**.
2. Paste this line and press Enter:

   ```powershell
   irm https://github.com/cathalbyrnegit/kishtin/releases/latest/download/install.ps1 | iex
   ```

It puts Kishtin in `%LOCALAPPDATA%\Programs\kishtin\bin` and adds that folder to your PATH, so new PowerShell windows can run `kishtind` by name.

### macOS and Linux

1. Open **Terminal** (on a Mac: press ⌘ Space, type Terminal, press Enter).
2. Paste this line and press Enter:

   ```sh
   curl -fsSL https://github.com/cathalbyrnegit/kishtin/releases/latest/download/install.sh | sh
   ```

It puts Kishtin in `~/.local/bin`. If that folder is not on your PATH, the installer says which line to add to `~/.profile`.

### What the installer checks

Before installing anything, the installer:

- downloads the release for your system and its `SHA256SUMS` list;
- stops unless the list has exactly one entry for that download and the download matches it;
- stops unless the download holds `kishtind` and `kishtin` and nothing else.

If any check fails, it says what went wrong and nothing is installed.

## Start Kishtin and set it up

1. Start the server: type `kishtind` and press Enter. Leave the window open: closing it stops Kishtin.
2. On the same computer, open a browser at **http://127.0.0.1:8080**.
3. Fill in **Set up Kishtin**: the restaurant's name, your name and a password. See [Setting Kishtin up](screens.md#setting-kishtin-up).

`kishtind` keeps its data in `~/.kishtin` (on Windows, `C:\Users\<you>\.kishtin`): one database file, attached photos and backups.

## Tablets and phones

Kishtin lets tablets and phones on the restaurant's network use it from the start. Connect each one with a QR code from **Setup › Staff and access › Device keys › Connect a tablet**; see [Connecting a tablet or phone](screens.md#connecting-a-tablet-or-phone).

- Kishtin answers tablets over HTTPS on port 8443, with its own certificate, and on port 8080 with only the page a tablet opens first, which helps it install the certificate. Everything else on port 8080 is for the Kishtin computer itself. Every request still needs a device key, and pairing needs a one-time code from a manager.
- The first time `kishtind` starts, Windows asks whether to allow it on networks, and a Mac whether to accept incoming connections. Allow it on **private** networks only, not public ones.
- Keep tablets on the staff Wi-Fi, not a guest network: guests' phones should not reach the kitchen system at all.
- On a computer with several networks or a VPN, name the address tablets should use in `kishtin.toml`: `tablet_addresses = ["192.168.1.20"]` under `[server]`.
- To keep Kishtin on this computer only, set `tablets = false` under `[server]`.

## Update

Run the same install line again. It replaces the two programs and leaves the data alone. Stop `kishtind` first (press Ctrl C in its window), and take a backup in **Setup › Backups** before a big update. Start `kishtind` again afterwards; it brings the database up to date when it starts.

`kishtind --version` shows which version is installed.

## Choices

Set these before the install line:

| Setting | Does |
|---|---|
| `KISHTIN_VERSION=v0.2.0` | installs that release instead of the latest |
| `KISHTIN_INSTALL_DIR=/some/folder` | installs there instead |
| `KISHTIN_RELEASE_URL=https://…` | downloads from a mirror: a folder holding the archives and `SHA256SUMS` |
| `KISHTIN_NO_MODIFY_PATH=1` | Windows only: leaves your PATH as it is |

For example, on macOS or Linux:

```sh
curl -fsSL https://github.com/cathalbyrnegit/kishtin/releases/latest/download/install.sh | KISHTIN_VERSION=v0.2.0 sh
```

On Windows: `$env:KISHTIN_VERSION = 'v0.2.0'` on its own line first.

## Download by hand

Each release on [GitHub](https://github.com/cathalbyrnegit/kishtin/releases) has one archive per system, and `SHA256SUMS`:

| System | Archive |
|---|---|
| Linux, Intel/AMD | `kishtin_linux_amd64.tar.gz` |
| Linux, ARM (Raspberry Pi 4 and 5, ARM mini PCs) | `kishtin_linux_arm64.tar.gz` |
| Mac with Apple silicon (M1 and later) | `kishtin_darwin_arm64.tar.gz` |
| Mac with Intel | `kishtin_darwin_amd64.tar.gz` |
| Windows, Intel/AMD | `kishtin_windows_amd64.zip` |
| Windows, ARM | `kishtin_windows_arm64.zip` |

Check the archive against its line in `SHA256SUMS` (`sha256sum`, `shasum -a 256`, or `Get-FileHash` on Windows), unpack it, and put `kishtind` and `kishtin` somewhere on your PATH. On a Mac, a file downloaded in a browser is blocked the first time ("cannot be opened"); the install line above avoids that. To allow a hand-downloaded copy, run `xattr -d com.apple.quarantine kishtind kishtin` in its folder.

## Licence

Kishtin is free to use, copy and change under the Apache License, Version 2.0. `kishtind licenses` prints its licence and the licences of the open-source software built into it.

## Uninstall

Delete `kishtind` and `kishtin` from where they were installed. Kishtin's data stays in `~/.kishtin` until you delete that folder too. Keep a backup: HACCP records are evidence for inspections.

## Not done yet

Said plainly, so nobody is caught out:

- No release has been published yet, so the install lines above do not work until the first `v*` tag is pushed (see Making a release). Until then, build from source as in the [README](../README.md#build-from-source).
- Kishtin does not start by itself when the computer starts. Someone has to run `kishtind` after a restart.
- Each tablet installs Kishtin's certificate once, by hand, following the steps on its screen. On an iPad that is three screens of Settings.
- If the router gives the Kishtin computer a new address, tablets must be pointed at it again (Kishtin renews its own certificate by itself). Reserve the computer's address in the router to avoid this.
- The programs are not signed by Apple or Microsoft. The install lines above work without warnings; a copy downloaded in a browser may show one.
- There are no click-to-install packages (`.msi`, `.pkg`, `.deb`) yet.

## Making a release

For developers: push a tag such as `v0.2.0`. The Release workflow runs the tests, then GoReleaser builds every archive with the screens embedded, writes `SHA256SUMS`, and publishes them with the two install scripts. Try it locally without publishing with `make release-snapshot` (needs [GoReleaser](https://goreleaser.com) v2); the archives land in `dist/`.
