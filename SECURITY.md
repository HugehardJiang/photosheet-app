# Security policy

Please do not report security vulnerabilities in a public issue.

Use GitHub Security Advisories for this repository when the feature is available: open the repository Security tab, choose Advisories, and create a private report. If private advisories are unavailable, contact the maintainers through the repository's private GitHub contact options and include the repository name and a safe way to reply. Do not invent or rely on an unverified email address.

Security reports are especially important for:

- path traversal, symlink escape or arbitrary file overwrite;
- malicious project.json handling;
- image, TIFF or RAW parser crashes and denial of service;
- updater signature, hash, rollback or download validation issues;
- dependency vulnerabilities or unsafe native-library loading.

Include the affected version or commit, operating system, reproduction steps, impact and a minimal non-private fixture when possible. Redact file paths, tokens and image metadata that you do not need to share.

The project is Beta. The latest release and the main branch are the supported lines until a longer support policy is published. There is no promise of a fixed response time, but reports will be acknowledged as soon as practical.
