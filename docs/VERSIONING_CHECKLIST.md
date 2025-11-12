# Documentation Versioning Checklist

Use this checklist when releasing a new version of Obot.

## Pre-Release

- [ ] All documentation PRs for this release are merged to `docs/` folder
- [ ] Documentation is reviewed and tested locally
- [ ] Breaking changes are clearly documented
- [ ] New features have corresponding documentation
- [ ] Installation/configuration docs are up to date

## Creating the Version

- [ ] Determine version number (e.g., 0.9.0)
- [ ] Run: `npm run docusaurus docs:version X.X.X`
- [ ] Verify files created:
  - `versioned_docs/version-X.X.X/`
  - `versioned_sidebars/version-X.X.X-sidebars.json`
  - Updated `versions.json`

## Configuration Update (First Version Only)

If this is the first stable version:
- [ ] Update `docusaurus.config.ts`:
  - Change `lastVersion: 'current'` to `lastVersion: 'X.X.X'`
- [ ] Test that the new version is now the default

## Verification

- [ ] Build succeeds: `npm run build`
- [ ] Version dropdown shows all versions including "Next"
- [ ] Default path `/` shows the new stable version
- [ ] `/next/` shows unreleased docs
- [ ] Specific version path `/X.X.X/` works
- [ ] All navigation and links work in each version
- [ ] Edit links point to correct GitHub paths

## Commit and Deploy

- [ ] Commit versioning changes:
  ```bash
  git add versions.json versioned_docs/ versioned_sidebars/ docusaurus.config.ts
  git commit -m "docs: create version X.X.X"
  ```
- [ ] Push to repository
- [ ] Verify deployment to https://docs.obot.ai

## Post-Release

- [ ] Announce new documentation version (if applicable)
- [ ] Update any external links to documentation
- [ ] Consider archiving very old versions (if more than 10 versions exist)
