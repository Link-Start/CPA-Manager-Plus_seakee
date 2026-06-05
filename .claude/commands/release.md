---
argument-hint: [version] [--lang=zh,en] [--merge=squash|merge|rebase] [--wait-checks] [--dry-run] [--no-emoji]
description: Draft release notes, open PR, auto-merge, push tag
---

# Release

End-to-end CPAMP release: draft notes -> confirm -> branch -> commit -> PR -> merge -> tag.
CI (`.github/workflows/release.yml`) picks up `docs/release-notes/<tag>-<lang>.md` as the GitHub Release body; absent -> it falls back to `git log`.

This command is CPA Manager Plus specific. Do not install or reuse it as a global cross-project release command.

## Flags
| Flag | Default | Effect |
|------|---------|--------|
| `version` | inferred | Target tag, e.g. `v1.0.2`. Omit -> infer from commits |
| `--lang` | `zh,en` | Note langs to generate (`<tag>-<lang>.md`) |
| `--merge` | `squash` | PR merge method |
| `--wait-checks` | false | Poll PR Check green before merge |
| `--dry-run` | false | Preview notes + plan only; no files/branch/PR/tag |
| `--no-emoji` | false | No emoji in commit/notes |

## Conventions (shared with CI + `docs/release.md`)
- File: `docs/release-notes/<tag>-<lang>.md`; `<tag>` keeps `v` prefix (e.g. `v1.0.2-zh.md`).
- Release-body primary lang: `zh` -> `zh-CN` -> `en`; other langs linked at top.
- Tag: `v<major>.<minor>.<patch>` (+ optional `-alpha|-beta|-rc.<n>`).
- `zh` is the authored source; other langs are faithful translations (same structure, links, stats).
- `docs/release-notes/` is reserved for versioned release note files only; release process docs live in `docs/release.md`.

## Rules
1. Never tag before notes land on `main` — merge the PR first, then tag the latest `main`.
2. Confirmation gate before ANY state change ("确认"/"confirm"/"go").
3. No AI markers in commits/PR/notes ("Claude Code", "Co-Authored-By: Claude").
4. Branch from an up-to-date `main`; never commit notes directly to `main`.
5. The tag commit MUST contain the note files (so CI checkout can read them).

## Workflow

### 1. Preflight
```bash
gh auth status                         # must be logged in
current_branch="$(git branch --show-current)"
git status --porcelain                 # must be empty (clean tree)
git rev-parse --verify main
git rev-parse --verify origin/main
git rev-list --left-right --count main...origin/main
last_tag="$(git tag --list 'v*' --sort=-v:refname | head -1)"
```
Abort on dirty tree, missing auth, missing local/remote main refs, or a main/origin-main mismatch. Do not switch branches, pull, write files, or mutate refs before the confirmation gate.

### 2. Resolve version
- `version` given -> validate format; ensure absent: `git tag -l "<version>"` empty.
- Else infer from `${last_tag}..HEAD --no-merges`: any `feat`/`✨` -> bump minor; `BREAKING CHANGE` or `!` -> major; otherwise (`fix`/`perf`/...) -> patch.
- Print proposed version, await confirm.

### 3. Collect changes
```bash
repo="$(gh repo view --json nameWithOwner -q .nameWithOwner)"
range="${last_tag}..HEAD"
git rev-list --count "$range"                       # commit count
git diff --shortstat "$range"                       # files, +add/-del
git log "$range" --no-merges --pretty=format:'%h%x09%s'   # to group by type/emoji
# External contributors + their commit subjects (exclude owner) for a per-person summary;
# range commits are already on remote main.
owner="$(gh repo view --json owner -q .owner.login)"
gh api "repos/$repo/compare/${last_tag}...$(git rev-parse HEAD)" --paginate \
  --jq '.commits[] | select(.author.login) | [.author.login, (.commit.message|split("\n")[0])] | @tsv' 2>/dev/null \
  | awk -F'\t' -v owner="$owner" '$1 != owner'         # rows: <login>\t<subject>; empty => omit Acknowledgements
# Fallback if API unavailable: git shortlog -sne "$range", then git log --author=<name> for subjects
```
Compare link: `https://github.com/$repo/compare/${last_tag}...<version>`.

### 4. Draft notes (author `zh`, then translate)
Draft note content in memory per template in `docs/release.md`; do not write files before confirmation:
- Title `# CPA Manager Plus <version>`
- Scale line: `> 📊 <n> commits · <files> files changed · +<add> / -<del>`
- Other-lang links at top (e.g. `[English →](./<version>-en.md)`)
- Highlights grouped by type, dropping merges/noise: ✨ Features / 🐛 Fixes / ⚡️ Performance / ♻️ Refactor / 📝 Docs / 🔧 Chore / 👷 CI / 📦 Build
- Acknowledgements: one line per external contributor (step 3, owner excluded), format `@login — <one concise zh sentence on what they contributed, summarized from their commit subjects>`. OMIT the section if none.
- `**Full Changelog**: <compare link>`
Mirror into each other `--lang` preview, preserving structure/links/stats.

### 5. Plan + preview -> confirm
Show: resolved version, note files to be written, branch `release/<version>`, commit msg, PR base `main`, merge method, tag step, and whether `main` needs an update after confirmation. Render note previews. Await "确认".
`--dry-run` stops here without writing files, switching branches, opening PRs, merging, or tagging.

### 6. Execute (after confirm)
```bash
git switch main && git pull --ff-only
git switch -c "release/<version>"
# Now write docs/release-notes/<version>-*.md from the confirmed previews.
test -f "docs/release-notes/<version>-zh.md" || test -f "docs/release-notes/<version>-en.md"
git add docs/release-notes/<version>-*.md
git commit -m "📝 docs(release): add <version> release notes" \
  -m "Add versioned zh/en release notes for <version>." \
  -m "CI release.yml uses these as the GitHub Release body."
git push -u origin "release/<version>"
```
- Open PR (base `main`) via GitHub tooling (`gh pr create` or equivalent).
- `--wait-checks`: poll PR checks/status until success.
- Merge via GitHub tooling (`gh pr merge` or equivalent; method = `--merge`, default squash).
- Land the tag on merged `main`:
```bash
git switch main && git pull --ff-only
git tag "<version>" && git push origin "<version>"   # triggers release.yml
git push origin --delete "release/<version>"          # delete_branch_on_merge is false
```

### 7. Report
Output Release URL `https://github.com/$repo/releases/tag/<version>` + Actions run link. Note that CI now builds assets and publishes the Release using the curated notes.

## Notes
- CI matches `docs/release-notes/<tag>-{zh,zh-CN,en}.md` exactly; any filename mismatch -> silent fallback to `git log`. Keep names exact.
- Pre-release tags (`-beta`, `-rc.N`, ...) work too; curated notes optional (fallback covers them).
- This command performs irreversible, outward-facing actions (merge to `main`, push tag). Honor the confirmation gate every run.
