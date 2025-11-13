# Versioning Instructions

### Snapshotting a new version

To snapshot a new version of the docs, run `make gen-docs-release version=<new version> prev_version=<previous version>`.
This will accomplish everything needed. Then, just commit the changes and create a pull request.

### Removing an old version

To remove an old version of the docs, run `make remove-docs-version version=<version to remove>`.
Then commit your changes and create a pull request.
