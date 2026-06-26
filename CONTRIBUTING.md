# Contributing

Thank you for improving HopStat. This project lists **human maintainers only** as contributors on GitHub.

## Attribution (required)

AI tools (including Cursor) may help write code, but **must not appear as contributors**.

When you commit:

1. **Do not** add `Co-authored-by: Cursor <cursoragent@cursor.com>` or any other AI/bot co-author trailer.
2. **Do not** set Git `user.name` / `user.email` to Cursor or other automated agents.
3. Use your own identity as author; you are responsible for reviewing every change before push.

Before pushing, check the latest commit:

```bash
git log -1 --format=full
```

If an unwanted `Co-authored-by` line is present, amend the commit and remove it before pushing to shared branches.

### Cursor IDE

- Keep `.cursor/` **local only** — it is gitignored and must never be committed.
- Turn off automatic co-author attribution on commits in Cursor settings if it is enabled.
- Prefer committing from the terminal with a plain message when the IDE adds co-author trailers you do not want.

## Do not commit

| Path / pattern | Reason |
|----------------|--------|
| `.cursor/` | Local IDE/agent config |
| `config.yaml`, `.env` | Secrets |
| `*.db`, `*.pem`, `*.key` | Runtime / credentials |
| `md/` | Private planning notes (gitignored) |
| `node_modules/` | Dependencies |

Tracked exceptions (intentional): `web/dist/` (embedded SPA), `vendor/`, test GeoIP `.mmdb` under `internal/geo/testdata/`.

## Releases

When cutting a release (`build al`, `deploy et`, or tag `vX.Y.Z`):

1. Add English release notes: `docs/whats-new/vX.Y.Z.md`
2. Commit code + notes together
3. Tag and push — GitHub Release uses that markdown file

See [docs/whats-new/README.md](docs/whats-new/README.md).

## Development

```bash
make test
make lint
cd web/frontend && npm test
```

Frontend changes that ship in the binary also require `cd web/frontend && npm run build` and committing updated `web/dist/` unless your build pipeline embeds fresh assets another way.

## Questions

Open an issue or contact the maintainers listed in [README.md](README.md).
