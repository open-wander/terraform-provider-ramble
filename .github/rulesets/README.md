# Repository rulesets

GitHub does not apply these files automatically. They are the source of truth
for the repository's rulesets; import or update them by hand.

Import: Settings > Rules > Rulesets > New ruleset > Import a ruleset, then
choose the JSON file. To change a ruleset, edit the file in a pull request and
re-import it (or edit the ruleset in the UI and export it over the file).

| File | Applies to | Rules |
|------|------------|-------|
| `main.json` | default branch | no deletion, no force push, pull request required (one approval, from someone other than the last pusher; squash merge only), `DCO`, `build`, `generate`, `unit` and `acceptance` checks required; repository admins may bypass |
| `tags.json` | `v*` tags | no creation, deletion or update; repository admins may bypass. After importing, add the release GitHub App to the bypass list in the UI (its id is not in this file), or tag-release.yml cannot create tags |
| `regen.json` | `regen/**` branches | no creation, update, deletion or force push; repository admins may bypass. Add the release GitHub App to the bypass list in the UI, or regenerate.yml cannot push or close regeneration branches. The commit check in tag-release.yml relies on this: commit emails can be spoofed |

`DCO` comes from the DCO app (integration 1861); the other four are the jobs
in `.github/workflows/test.yml` (GitHub Actions, integration 15368). `acceptance`
is the aggregate of the Terraform and OpenTofu matrix, so bumping a tested
version does not change a required check name. Rename a job only together with
this file.

## Environments

Settings > Environments. These are not exported as files.

| Environment | Used by | Protection | Secrets |
|-------------|---------|------------|---------|
| `release` | `release.yml` | required reviewers (admins); deployment tags: a **tag** rule `v*` (a branch rule named `v*` does not match tags) | `GPG_PRIVATE_KEY`, `GPG_PASSPHRASE` |
| `automation` | `regenerate.yml`, `tag-release.yml` | deployment branches: `main` only; no reviewers, so regeneration stays automatic | `RELEASE_APP_PRIVATE_KEY` |

Move these secrets out of repository secrets: a repository secret is readable
from any branch a write collaborator pushes.

The release App's bypass on `tags.json` and `regen.json` is added by hand, so
re-importing either file removes it: add it again afterwards. An approval on a
regeneration pull request is not the review gate (the App is the last pusher,
so any write user can approve); review the diff before approving the `release`
deployment, whose approval screen does not show it.
