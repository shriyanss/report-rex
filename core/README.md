# report-rex-core

Python package providing the core build engine for [Report Rex](../README.md).

## Installation

```bash
pip install report-rex-core
```

## Usage

```bash
# Build a report
report-rex build --dir reporting/example --output report.pdf

# Scaffold a new report directory
report-rex new my-engagement

# Validate a report directory
report-rex validate --dir my-engagement
```

## Development

```bash
pip install -e ".[dev]"
pytest tests/ -v
```
