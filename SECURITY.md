# Security

## Reporting a problem

If you find a security problem in Kishtin, please report it privately, not in a public issue:

1. Open the repository on GitHub: https://github.com/cathalbyrnegit/kishtin
2. Go to **Security**, then **Report a vulnerability**.
3. Say what the problem is, which version (`kishtind --version`), and how to reproduce it. Use made-up data: never send real staff, customer or HACCP records.

Kishtin is a small open-source project with no security team. Reports are read by the maintainer, who will reply on the report, agree a fix and a date to make it public, and credit you if you want.

Please do not test against a restaurant's live system. Run your own copy: see [Installing Kishtin](docs/install.md) or [build from source](README.md#build-from-source).

## Supported versions

Only the latest release gets security fixes. Updating is the same install line again; see [Update](docs/install.md#update).

## What is in scope

- `kishtind` and `kishtin`, their HTTP API, the screens they serve, and the install scripts.
- Ways around the append-only HACCP records, the hash chain, or `kishtin haccp verify`.
- Ways around device keys, staff PINs, manager passwords, permissions, tablet pairing, or the password recovery on the Kishtin computer.
- Kishtin's own certificate authority (`internal/localtls`) issuing a certificate it should not.
- Kishtin sending anything off the computer that it should not.

Out of scope: someone who already controls the Kishtin computer's user account (they can read its data folder, as the owner can), denial of service on the restaurant's own network, and the security of Square, SumUp or the computer's operating system.

## How Kishtin is built to be safe

- Everything stays on one computer in the restaurant. Nothing needs a cloud account, and nothing is sent anywhere unless a manager connects a till.
- The API needs a key for every request and listens only on the computer itself by default. Tablets connect over HTTPS with a certificate from Kishtin's own certificate authority, which can only sign local names and private addresses. Pairing a tablet needs a one-time code from a manager.
- HACCP records cannot be edited or deleted, and each one is chained to the one before it with SHA-256, so changes to the database file are found by `kishtin haccp verify`.
- Till tokens are kept in a file only the server can read, never in the database, backups or exports. Kishtin never sees card data.
- Customers' names and order notes are cleared 30 days after an order closes; see [What Kishtin records](docs/recording.md).

## Checking a download

Each release has `SHA256SUMS`, which the install scripts check before installing anything, and a software bill of materials (SPDX JSON) for each download (`kishtin_<system>_<processor>.tar.gz.sbom.json` or `.zip.sbom.json`) and for the source (`kishtin_source.sbom.json`). The bills of materials list every package built into Kishtin, so you can check them against published vulnerabilities with a scanner such as Grype or OSV-Scanner.

## Keeping the Kishtin computer safe

See [Keeping the Kishtin computer safe](docs/security.md): disk encryption, a separate account, updates, the firewall and backups, in plain steps.
