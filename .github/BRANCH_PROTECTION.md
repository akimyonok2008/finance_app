# Required GitHub repository settings

GitHub repository settings are not stored in this repository. A repository
administrator must configure the following rules for `main`.

## Branch protection

Create a branch ruleset targeting `main` and enable:

- Require a pull request before merging.
- Require at least one approval and dismiss stale approvals when new commits
  are pushed.
- Require conversation resolution before merging.
- Require branches to be up to date before merging.
- Block force pushes and branch deletion.
- Do not allow bypassing the ruleset, including for administrators.
- Require these status checks:
  - `Backend / format, test, race, vet`
  - `Frontend / lint, test, build`
  - `PostgreSQL / migrations and startup`
  - `Docker / production smoke`
- `Browser / release-critical journey` (isolated Chromium release scenarios plus Firefox, WebKit, mobile, accessibility, resilience, concurrency, idempotency, and scale contracts)

The checks above are emitted by `.github/workflows/ci.yml` on pull requests and
on pushes to `main`. After the first workflow run, select the checks by their
exact displayed names.

## Release approval

Create a GitHub Actions environment named `production`. Add at least one
required reviewer who is not the person initiating the release, prevent
self-review, and limit deployment branches/tags to `main`.

The manual `Release candidate` workflow intentionally does not deploy to any
cloud. It re-runs CI, verifies backup/restore, builds images tagged with the
exact commit SHA, uploads the images and metadata, and then stops at the
`production` environment approval boundary. Promotion is a separate,
explicitly authorized operation.
