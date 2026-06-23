# What's New release notes

English-only release notes shown in the admin **What's New** modal (sourced from GitHub Release `body`).

One file per version, committed to git before tagging:

```
docs/whats-new/vX.Y.Z.md
```

## Template

```markdown
# HopStat vX.Y.Z

Brief one-line summary of the release.

## What's New

- Feature or UX change described clearly for admins.
- Another user-visible improvement.

## Fixes

- Bug fix with enough context to understand impact.

## Notes

- Optional: breaking changes, manual steps, config changes.
```

## Agent / release checklist

When the user says **build al** or **deploy et**:

1. Draft `docs/whats-new/vX.Y.Z.md` from the actual diff (English, formatted as above).
2. Commit code + whats-new file.
3. Tag `vX.Y.Z` and push.
4. Ensure GitHub Release uses this markdown as the release body.

See also: `.cursor/rules/release-whats-new.mdc`
