# Repository rulesets

GitHub does not apply these files automatically. They are the source of truth
for the repository's rulesets; import or update them by hand.

Import: Settings > Rules > Rulesets > New ruleset > Import a ruleset, then
choose the JSON file. To change a ruleset, edit the file in a pull request and
re-import it (or edit the ruleset in the UI and export it over the file).

| File | Applies to | Rules |
|------|------------|-------|
| `main.json` | default branch | no deletion, no force push, pull request required (no approval needed, squash merge only), `DCO`, `build`, `generate`, `unit` and `acceptance` checks required; repository admins may bypass |

`DCO` comes from the DCO app (integration 1861); the other four are the jobs
in `.github/workflows/test.yml` (GitHub Actions, integration 15368). `acceptance`
is the aggregate of the Terraform and OpenTofu matrix, so bumping a tested
version does not change a required check name. Rename a job only together with
this file.
