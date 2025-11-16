# Website

This website is built using [Docusaurus](https://docusaurus.io/), a modern static website generator.

### Installation

```
$ yarn
```

### Local Development

```
$ yarn start
```

This command starts a local development server and opens up a browser window. Most changes are reflected live without having to restart the server.

### Build

```
$ yarn build
```

This command generates static content into the `build` directory and can be served using any static contents hosting service.

## Versioning

This documentation uses Docusaurus versioning to maintain docs for multiple releases.

### Understanding Versions

- **Next (docs/ folder)**: Unreleased documentation for features in development
- **Versioned releases (versioned_docs/)**: Documentation snapshots for each stable release

### Creating a New Version

When releasing a new version of Obot (e.g., v0.9.0):

1. Ensure all documentation updates are merged to `docs/` folder
2. Run the versioning command:
   ```bash
   npm run docusaurus docs:version 0.9.0
   ```
3. This creates:
   - `versioned_docs/version-0.9.0/` - Snapshot of docs
   - `versioned_sidebars/version-0.9.0-sidebars.json` - Snapshot of sidebar
   - Updates `versions.json` to include "0.9.0"

4. Update `docusaurus.config.ts`:
   ```typescript
   docs: {
     lastVersion: '0.9.0', // Change from 'current' to first stable version
     // ... rest of config
   }
   ```

5. Commit the changes:
   ```bash
   git add versions.json versioned_docs/ versioned_sidebars/ docusaurus.config.ts
   git commit -m "docs: create version 0.9.0"
   ```

### Updating Documentation

**For unreleased features**: Edit files in `docs/` - these appear under "Next"

**For released versions**: Edit files in `versioned_docs/version-X.X.X/` - these appear under that specific version

### Version URLs

- Next: `https://docs.obot.ai/next/`
- Stable (default): `https://docs.obot.ai/` (shows latest stable release)
- Specific version: `https://docs.obot.ai/0.9.0/`

### Best Practices

1. **Write docs in docs/ first**: Always develop documentation in the main `docs/` folder
2. **Version on release**: Only create a version snapshot when tagging a release
3. **Patch updates**: For patch releases (e.g., 0.9.1), update the 0.9.0 version docs if needed - don't create new versions for patches
4. **Breaking changes**: Create new versions for major/minor releases that change APIs or features significantly
5. **Backport sparingly**: Only backport critical fixes to old versions - prefer forward-looking documentation