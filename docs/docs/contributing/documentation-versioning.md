---
sidebar_position: 4
title: Documentation Versioning
description: How to create and manage versioned documentation snapshots
---

# Documentation Versioning

This guide explains how to create and manage versioned snapshots of the Obot documentation using Docusaurus versioning features.

## Overview

The Obot documentation uses Docusaurus versioning to maintain documentation for multiple releases. Each version is a snapshot of the documentation at a specific point in time, allowing users to view docs for the version they're running.

**Location**: Versioned docs are stored in `docs/versioned_docs/version-X.Y.Z/`

## Creating a New Version

When releasing a new version of Obot, snapshot the current documentation:

### Command

```bash
make gen-docs-release version=<new version> prev_version=<previous version>
```

### Example

```bash
# Release v0.16.0, with v0.15.0 as the previous version
make gen-docs-release version=0.16.0 prev_version=0.15.0
```

### What This Does

1. Creates a new directory: `docs/versioned_docs/version-0.16.0/`
2. Copies all current docs from `docs/docs/` into the versioned directory
3. Updates `docs/versions.json` to include the new version
4. Updates `docs/versioned_sidebars/` with the sidebar for this version

### Post-Snapshot Steps

1. Review the generated files in `versioned_docs/version-X.Y.Z/`
2. Commit all changes:
   ```bash
   git add docs/versioned_docs/ docs/versions.json docs/versioned_sidebars/
   git commit -m "docs: snapshot version X.Y.Z documentation"
   ```
3. Create a pull request with the documentation snapshot

## Removing an Old Version

When a version is no longer supported, remove its documentation snapshot:

### Command

```bash
make remove-docs-version version=<version to remove>
```

### Example

```bash
# Remove v0.13.0 documentation
make remove-docs-version version=0.13.0
```

### What This Does

1. Removes `docs/versioned_docs/version-0.13.0/` directory
2. Removes entry from `docs/versions.json`
3. Removes sidebar from `docs/versioned_sidebars/`

### Post-Removal Steps

1. Commit the changes:
   ```bash
   git add docs/
   git commit -m "docs: remove unsupported version 0.13.0"
   ```
2. Create a pull request

## Versioning Best Practices

### When to Create a Version

- ✅ Before each major release (e.g., 0.16.0)
- ✅ Before significant breaking changes
- ✅ When new features require updated documentation
- ❌ Not needed for patch releases (e.g., 0.15.1) unless docs change

### When to Remove a Version

- After a version reaches end-of-life
- When maintaining too many versions becomes burdensome
- Typically keep last 3-4 major versions

### Version Naming

- Use semantic versioning: `MAJOR.MINOR.PATCH`
- Match the version format in `versions.json`
- Examples: `0.15.0`, `0.16.0`, `1.0.0`

## Viewing Versioned Docs

Users can switch between versions using the version dropdown in the documentation site:

- **Latest (unreleased)**: Current `docs/docs/` content (from main branch)
- **Versioned releases**: Content from `versioned_docs/version-X.Y.Z/`

## Makefile Targets

The versioning commands are defined in the project Makefile:

```makefile
# Generate a new documentation version
make gen-docs-release version=X.Y.Z prev_version=A.B.C

# Remove an old documentation version
make remove-docs-version version=X.Y.Z
```

## Troubleshooting

### Issue: Version dropdown not showing new version

**Cause**: `versions.json` not updated correctly

**Solution**: Manually add version to `docs/versions.json`:
```json
[
  "0.16.0",
  "0.15.0",
  "0.14.0"
]
```

### Issue: Sidebar missing in versioned docs

**Cause**: Sidebar file not created in `versioned_sidebars/`

**Solution**: Copy current sidebar:
```bash
cp docs/sidebars.ts docs/versioned_sidebars/version-0.16.0-sidebars.json
```

### Issue: Build fails after versioning

**Cause**: Broken links in versioned docs

**Solution**: Run build and fix reported broken links:
```bash
cd docs && npm run build
```

## Related Documentation

- [Docusaurus Versioning Guide](https://docusaurus.io/docs/versioning)
- [Contributing Guide](./contributor-guide.md)
- [Upstream Merge Process](./upstream-merge-process.md)

---

*Last Updated: 2026-01-13*
