# tdl-delete

A [tdl](https://github.com/iyear/tdl) extension for deleting Telegram messages.

## Install

```bash
tdl extension install wazk9595/tdl-delete
```

## Usage

### Delete by message URL

```bash
tdl delete --url https://t.me/c/1234567890/42
tdl delete --url https://t.me/c/1234567890/42 --url https://t.me/c/1234567890/43
```

### Delete from tdl chat export JSON

```bash
tdl delete --from export.json
tdl delete --from export1.json --from export2.json
```

### Delete by chat + message ID

```bash
# Channel or supergroup (numeric ID with -100 prefix)
tdl delete --chat -1001234567890 --id 42,43,44

# By username
tdl delete --chat @mychannel --id 42

# Saved Messages
tdl delete --chat me --id 73767
```

### Mix inputs

```bash
tdl delete --from export.json --url https://t.me/c/1234567890/99
```

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--from` | tdl chat export JSON file (repeatable) | |
| `--url` | Message URL (repeatable) | |
| `--chat` | Chat username, numeric ID, or `me` | |
| `--id` | Message IDs, comma-separated (used with `--chat`) | |
| `--revoke` | Delete for all users | `true` |

## Build from source

```bash
git clone https://github.com/wazk9595/tdl-delete.git
cd tdl-delete
go mod tidy
go build -o tdl-delete
tdl extension install --force ./tdl-delete
```
