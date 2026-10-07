# Security Policy

We provide security updates in patch releases for supported minor releases.

## Reporting a Vulnerability
Please use GitHub's vulnerability reporting mechanism - https://github.com/obot-platform/obot/security - to report any vulnerabilities.

- Do **not** open public issues for security reports.
- Read our [threat model](THREAT_MODEL.md) first. It explains which roles we trust and which behavior is expected. Reports about behavior it describes as out of scope will be closed.

Each report should include:
- **The release you tested.** Test against the latest release. Issues that are already fixed in the latest release are not new vulnerabilities.
- **The attacker's starting point.** For example, unauthenticated, a standard user, or a Power User.
- **Steps that reproduce the issue against a running Obot deployment,** with the real requests and responses. A finding based only on reading the code, or on a unit test, is a lead, not a demonstrated vulnerability.
- **The impact,** meaning what the attacker can actually do or see that they could not before.

Please keep reports short and send one issue per report. If you used AI tools to find or write up the issue, say so, and confirm that a person reproduced it.

We’ll acknowledge your report within **2 business days**, provide a status update in **7 days**, and aim to issue a fix or mitigation within **30 days** (complex issues may take longer).

## How We Handle Reports
- **Verification and severity.** We determine severity based on confirmed impact. Reports without a reproduction against a running deployment are initially treated as unverified. We may verify the issue ourselves or request additional evidence before assigning severity or publishing an advisory.
- **Duplicates.** If more than one person reports the same issue, the first report gets the credit.
- **Low-severity issues and hardening.** We fix these in public pull requests, without a security advisory or CVE. We'll tell you when we do.
- **CVE IDs.** We request CVE IDs through GitHub when we publish an advisory. Please do **not** reserve a CVE ID for an Obot issue with another CVE Numbering Authority.

## Disclosure
We follow coordinated disclosure:
- We work with you to validate and remediate.
- After a fix/mitigation is available, we’ll publish release notes and credit reporters who wish to be acknowledged.

## Scope
Issues that impact the confidentiality, integrity, or availability of this project or its official packages/services are in scope, as described in our [threat model](THREAT_MODEL.md).

**Out of scope (non-exhaustive):**
- Issues where the attacker must already hold a trusted role and the impact stays within that role’s trust boundary, as defined in the threat model
- Deprecated or end-of-life versions
- Vulnerabilities in third-party dependencies not owned by us (please report upstream)

## Safe Harbor
We will not pursue legal action for good-faith security research aligned with this policy.  
Avoid privacy violations, service degradation, or data destruction. Only test against your own accounts and data.

## Receiving Fixes
Security fixes are shipped in patch releases. Upgrade to the latest patch of supported versions.  
We may issue public advisories (GHSA/CVE) when appropriate.

## Credits
With permission, we credit reporters in release notes.
