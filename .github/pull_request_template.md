## Related issue

Fixes #<!-- issue number -->

## Description

<!-- What does this change, and why? Call out any design decisions or trade-offs. -->

## Checklist

- [ ] Acceptance tests added or updated (`TestAcc...`), and `make testacc` passes against a local DefectDojo (`make up`)
- [ ] Tested against every supported DefectDojo version (`make up DD_VERSION=...`, see `.github/defectdojo-versions.json`) where behavior differs
- [ ] `make generate` run, and the regenerated `docs/` committed
- [ ] `make lint` passes
- [ ] `CHANGELOG.md` updated under `## Unreleased`
