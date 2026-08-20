# autohaven

Automatically set wallpapers from [wallhaven](https://wallhaven.cc).

## Features

- fetches wallpapers from wallhaven
- saves history of downloaded wallpapers
- sets the wallpaper to the desktop
- sends notifications (via `notify-send`)

## Configuration

By default, `autohaven` looks for a configuration file at `~/.config/autohaven/config.yaml`.

Here is an example configuration:

```yaml
api_key: "YOUR_API_KEY"
sorting: toplist
top_range: 1M
categories: "100"
allow_sketchy: false
allow_nsfw: false
atleast: "1920x1080"
tags:
  - nature
  - landscape
output_dir: "~/.cache/autohaven"
history_file: "~/.cache/autohaven/history.json"
daemon: "cosmic-bg"
```

## CLI Options

You can customize the execution of `autohaven` at runtime using the following command-line flags:

- **`-config <path>`**: Path to a custom YAML configuration file (defaults to `~/.config/autohaven/config.yaml`).
- **`-daemon <name>`**: Override the desktop wallpaper daemon (e.g., `cosmic-bg` or `none` to disable setting it).
- **`-dry-run`**: Simulate fetching and picking a wallpaper without downloading it, changing the wallpaper, or updating history.
- **`-tag <tags>`**: Override config tags with a comma-separated list of tags (e.g., `-tag "anime, scenery"`).
- **`-no-notify`**: Disable desktop notifications.
- **`-clean`**: Delete the history file and clean the downloaded cache directory (`output_dir`), then exit.

### Examples

**Run with dry-run mode:**

```bash
go run main.go -dry-run
```

**Override tags and disable notifications:**

```bash
go run main.go -tag "space, astrophotography" -no-notify
```

**Clear history and cache directory:**

```bash
go run main.go -clean
```
