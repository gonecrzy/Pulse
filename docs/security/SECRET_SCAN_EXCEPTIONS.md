# Secret scan exceptions

## Reviewed Gitleaks false positive

Gitleaks v8.30.1 found the same 17-character value twice while scanning PR #1's delivered commit range and synthetic merge source. Both findings were in the roadmap's release-readiness prose and shared SHA-256 fingerprint `ea842f61d3ea6a91777de6f2447fefb501f50dccdf21cf926510fa3b80c63bc8`.

The original finding is at line 141 of commit `8bd6901eb986e712ec4c90a2fab87526f78d0c33`, rule `generic-api-key`. The second report was the same text in the synthetic PR merge at line 128; `git blame` traced both to the original commit. The text is an unquoted, slash-separated natural-language policy phrase. It contains no assignment, credential field, endpoint, code, or configuration value, and it occurs nowhere else in the delivered tree. This is a detector false positive, not a usable credential or public content digest.

The current roadmap sentence has been reworded to avoid reproducing the detector's ambiguous match in future synthetic merge commits. `.gitleaksignore` adds only the exact historical Gitleaks fingerprint below; all preexisting exact fingerprints are preserved:

`8bd6901eb986e712ec4c90a2fab87526f78d0c33:docs/plans/2026-10-23-sindri-integration-roadmap.md:generic-api-key:141`

No file, path, detector, or value pattern is broadly excluded. Full-history scanning, Gitleaks v8.30.1, redaction, and failure behavior remain enabled. Re-triage this exception if the finding context or fingerprint changes.
