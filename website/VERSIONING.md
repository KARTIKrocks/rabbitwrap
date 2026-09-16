# Documentation versioning

## The rules

### 1. Snapshot when a release changes documented behaviour — not on every release

A snapshot exists to answer one question: *what was true before this release
broke it?* If nothing broke, the snapshot is a byte-identical copy of a dozen
files that has to be maintained forever.

**The test:** does this release make an existing page **wrong for someone on
the previous version**? A changed default, changed semantics, a rename, a
removal, a deprecation — those get a snapshot. Purely additive releases do
not.

Unlike a post-1.0 project, **expect this to fire often here.** rabbitwrap is
pre-1.0, and its release history has been mostly behaviour-changing minors —
`v0.6.0` flipped the `RequeueOnError` default, `v0.10.0` split
`OnReconnectAborted` out of `OnDisconnect`, `v0.15.0` changed what `Stop`
does to broker-side registration. A sparse `versions.json` would be the
exception here, not the rule. Cut a snapshot whenever rule 1's test is true;
don't hold back waiting for a "big enough" release, because pre-1.0 semver
gives you no such signal — a `0.x` bump carries no promise about the size of
the change.

### 2. Additive changes get a version marker, not a snapshot

The one real problem a reader on an older version has is the inverse of a
breaking change: they read about something that does not exist in their
version yet, and it does not compile. Snapshots are an expensive fix for
that. A marker is a cheap one, and it is a better answer — a `0.15` snapshot
tells you what existed, a marker tells you what upgrading buys you.

The convention is plain Markdown, so it needs no components and survives
being copied into a snapshot:

| Situation | Write |
| --- | --- |
| New row in an API table | append `_0.17+_` to the description cell |
| New option or behaviour in prose | open the paragraph with `_Added in 0.17._` |
| New member inside a code block | trailing `// 0.17+` comment |
| Behaviour that changed | `_Changed in 0.17._` plus one line on what it was before |

Markers use `MAJOR.MINOR` — `0.17`, not `v0.17.0` — so they match snapshot
names and stay greppable. Drop a marker once it names a version older than
the oldest live snapshot; by then everyone reading has it.

### 3. Snapshots are `MAJOR.MINOR`, never patch

Versions are `0.16`, `0.17`, `1.0`. Never `0.16.0`, never `v0.16`.

A patch release (`0.16.1`) that changes documented behaviour is **edited into
the existing `versioned_docs/version-0.16/` in place**. It does not get its
own snapshot. A patch by definition does not change the contract; if it did,
the docs were already wrong, and the fix belongs in the snapshot that is
wrong.

`npm run cut-version` enforces the format; it rejects anything that isn't
`MAJOR.MINOR`, already exists, or is older than the current newest.

### 4. Only the newest 4 versions are built

`MAX_LIVE_VERSIONS` in `docusaurus.config.ts` caps how many snapshots get
built and indexed. Older ones stay in git — readable at their tag, restorable
by bumping the constant — but they don't cost build time or search index
size.

This keeps build time flat as releases accumulate instead of growing
linearly, which matters more here than it would on a slower-moving project
given rule 1 fires on most releases.

### 5. `docs/` is the future, not the present

| Directory | Serves | URL |
| --- | --- | --- |
| `docs/` | **Next** — unreleased, tracks `main` | `/docs/next/` |
| `versioned_docs/version-0.16/` | the current release | `/docs/` |

A reader who lands on `/docs/` sees released behaviour. Someone who wants
what's queued up next opens `/docs/next/`, which carries an "unreleased"
banner.

This means **a change that documents unreleased behaviour edits `docs/`**,
not the snapshot. The snapshot is frozen history.

## Release runbook

Every release starts the same way, and then rule 1 decides whether it ends
there.

### Every release

Make sure `docs/` describes the release accurately, and that new APIs carry
their `_0.17+_` markers.

### If the release only adds

Nothing else to do. `docs/` becomes the new truth on the next deploy, the
markers tell readers on older versions what they need to upgrade to, and the
existing snapshot keeps serving `/docs/`.

Wait — that last part is the catch. `/docs/` serves the newest **snapshot**,
so an additive release does not reach the default URL until the next
snapshot is cut. That is the deliberate trade: readers see the last release
whose behaviour is fully described, and `/docs/next/` carries everything
newer.

### If the release changes documented behaviour

```bash
npm run cut-version -- 0.17
npm run check
```

`versions.json`, `versioned_docs/version-0.17/`, and
`versioned_sidebars/version-0.17-sidebars.json` are created for you, `/docs/`
starts serving 0.17, and 0.16 moves into the version dropdown.

If the cut pushes a version out of the 4-version window, the script tells you
which one and prints the `git rm` to drop it for good.

### Patch releases

Never a snapshot. Edit `versioned_docs/version-<minor>/` directly, and mirror
the change into `docs/` if it still applies to the latest release.

## Day-to-day

```bash
npm start          # dev server, builds Next + newest version only (fast)
npm run start:all  # dev server with every live version
npm run build      # production build, all live versions
npm run build:fast # build check, Next + newest only — what CI runs on PRs
npm run check      # lint + typecheck + fast build
```

`DOCS_FAST_BUILD=true` is what makes the first and fourth fast: it narrows
the build to the docs you're actually editing. Never set it for a production
deploy — the deploy workflow doesn't.

## Adding a page

1. Create `docs/<id>.md` with `id`, `title`, and `description` frontmatter.
2. Add the id to `sidebars.ts` under the right category.

The sidebar is explicit rather than autogenerated so ordering is a deliberate
choice and a new file can't silently rearrange the nav.

## What does not belong here

Type signatures, method sets, and struct fields. Those are generated from
source on [pkg.go.dev](https://pkg.go.dev/github.com/KARTIKrocks/rabbitwrap)
and are always correct; hand-copying them here creates a second source of
truth that drifts.

These guides cover concepts, grouped overviews, configuration, and worked
examples — and link out for the exact signatures.
