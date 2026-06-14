# Faker

A CLI tool that generates fake system artifacts to stress test machines and validate security solutions.

## What it does

- **Chrome artifacts** — fake cookies, cache, and browsing history
- **Trash/Recycle Bin** — Lorem Ipsum files moved to OS trash
- **Temp files** — random files written to the system temp directory
- **Firewall hits** — HTTP requests to configurable high-risk domains
- **Virus signatures** — EICAR test files (optionally packaged as an ISO and mounted)
- **Registry entries** — fake entries (Windows only)

## Usage

1. Edit `config.toml` to enable/disable modules and set counts.
2. Run the binary:

```bash
go run main.go
```

## Configuration

All settings live in `config.toml`. Each module has an `enabled` flag and a `count`. Chrome paths auto-detect but can be overridden.

## Disclaimer

This tool is intended **only** for stress testing and security solution validation. Using it for malicious or nefarious purposes is strictly prohibited.
