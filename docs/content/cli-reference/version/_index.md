---
title: "Version"
description: "Print the application version, including commit SHA and build details."
type: docs
---

Print the application version, including commit SHA and build details.

## Usage

```bash
outtake version
```

## Examples

### Print version

```bash
outtake version
```

### Print detailed version info

```bash
outtake version --verbose
```

### Print version as JSON

```bash
outtake version --json
```

## Options

| Flag | Short | Default | Type | Description |
| --- | --- | --- | --- | --- |
| `--json` |  | `false` | bool | Output version information in JSON format |
| `--verbose` |  | `false` | bool | Output detailed version information |
