# Repository rulesets

GitHub does not apply these files automatically. They are the source of truth
for the repository's rulesets; import or update them by hand.

Import: Settings > Rules > Rulesets > New ruleset > Import a ruleset, then
choose the JSON file. To change a ruleset, edit the file in a pull request and
re-import it (or edit the ruleset in the UI and export it over the file).

| File | Applies to | Rules |
|------|------------|-------|
| `main.json` | default branch | no deletion, no force push, pull request required (no approval needed, squash merge only), `DCO` check required; repository admins may bypass |

When the provider's CI workflows exist, add their check names to
`required_status_checks` in `main.json`.
