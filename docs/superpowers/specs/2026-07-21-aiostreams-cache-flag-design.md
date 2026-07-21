# Surface a cache/ready flag through AIOStreams + WebStreamr

**Date:** 2026-07-21
**Status:** Approved (design)

## Problem

seedstrem marks already-downloaded torrents as ready in the Stremio stream it
builds — `seedstrem ⚡` in the name and `✅ ready` / `⬇ 45%` in the description.
When the addon is consumed through **AIOStreams** (which the user runs alongside
WebStreamr), that readiness is invisible: the user cannot tell which seedstrem
streams are instant vs which still need downloading.

The cause is how AIOStreams processes streams. It re-parses and **reformats**
every incoming stream, regenerating the `name`/`description` from parsed
variables. seedstrem's custom readiness text lives in the stream `name`, which
AIOStreams discards, so the ready state never reaches the UI.

## Why the "native cache badge" is out of scope

AIOStreams renders its native `⚡ Ready` badge only for streams it recognizes as
a known debrid/usenet/cloud **service**. In `packages/core/src/parser/streams.ts`,
`parseServiceData(stream.name)` matches the name against a fixed `SERVICE_DETAILS`
list (Real-Debrid, AllDebrid, TorBox, Premiumize, EasyNews, pikpak, …) and only
*then* looks for a cached symbol (`⚡ 🚀 cached 🌩️ 📫` or a `+`). The `cached`
flag is a property **of** a detected service — no service, no badge. There is no
`behaviorHints.cached` path and no generic "local/torrent/direct" service entry.

seedstrem is a personal seedbox, not any of those services. The only way to fire
the native badge is to write a **false** service name into the stream name (e.g.
`TorBox ⚡`), which misrepresents the stream and collides with AIOStreams' own
service dedup/filtering. Rejected as dishonest.

## Approach: ride readiness on the indexer-tag channel

AIOStreams' base parser (used for addons without a dedicated preset, i.e.
seedstrem) extracts an **indexer** tag from the stream **description** — the text
following one of the emojis `🌐 ⚙️ 🔗 🔎 🔍 ☁️`. This is the channel that renders
the `🔍 Peerflix` / `🔍 Private torrent` tags seen in other addons' output, and it
**survives AIOStreams' reformatting**.

seedstrem currently emits its indexer in the *name*, not the description, so
AIOStreams shows nothing for it today. We fix that and carry readiness in the
same tag.

### Regex constraint (verified against source)

The capture is:

```
(?:🌐|⚙️|🔗|🔎|🔍|☁️)\s*([^\p{Emoji_Presentation}\n]*?)(?=\p{Emoji_Presentation}|$|\n)
```

The capture group **excludes emoji**. So `⚙️ seedstrem ⚡` is captured as just
`seedstrem` — a trailing glyph cannot carry the signal. Readiness must therefore
be a **plain-text word** inside the tag.

### Tag scheme

Add exactly one description line, prefixed with `⚙️` (U+2699 U+FE0F — the exact
codepoint in AIOStreams' emoji set), placed **after** the raw release title so
AIOStreams still selects the title line for filename/quality parsing.

| Stream state | Builder | New description line | AIOStreams tag |
|---|---|---|---|
| Fresh Prowlarr candidate | `toStreamItem` | `⚙️ seedstrem · <indexer>` (or `⚙️ seedstrem` if indexer empty) | `seedstrem · Peerflix` |
| Downloaded / ready (`progress ≥ 1`) | `toOwnedStreamItem` | `⚙️ seedstrem · cached` | `seedstrem · cached` |
| Downloading (`0 < progress < 1`) | `toOwnedStreamItem` | `⚙️ seedstrem · <NN>%` | `seedstrem · 45%` |
| Owned, queued (`progress == 0`) | `toOwnedStreamItem` | `⚙️ seedstrem · queued` | `seedstrem · queued` |

- Separator is `·` (U+00B7 middle dot — not an emoji, captured cleanly).
- The qualifier is decided per builder: `toStreamItem` (fresh) uses the indexer
  name; `toOwnedStreamItem` (owned/cached) uses the progress-derived word. This
  matches how the stream handler already routes items — owned/cached torrents go
  through `toOwnedStreamItem`, fresh candidates through `toStreamItem`.
- `store.Torrent` has no indexer field, so ready/owned streams are labeled
  `seedstrem` (no indexer name available); fresh candidates keep their real
  indexer name.

### What stays unchanged

- The existing `seedstrem ⚡` / `seedstrem ⚙ <indexer>` **name** — untouched, so
  direct Stremio users and other aggregators keep today's display.
- The existing `✅ ready` / `⬇ NN%` / `✅ downloaded` **stat line** in the
  description — untouched.

This change is purely **additive**: one new parseable description line.

## Affected code

`internal/stremio/stream.go`:
- `toStreamItem` — append the `⚙️ seedstrem · <indexer>` line to `detail`.
- `toOwnedStreamItem` — append the `⚙️ seedstrem · <cached|NN%|queued>` line to
  `detail`.
- A small shared helper to build the `⚙️ seedstrem · <qualifier>` line keeps the
  two call sites DRY and testable.

## Testing

Table-driven unit tests in `internal/stremio` (Go stdlib `testing`):

1. `toStreamItem` with an indexer → description contains `⚙️ seedstrem · <indexer>`.
2. `toStreamItem` with empty indexer → description contains `⚙️ seedstrem` and no
   trailing `· `.
3. `toOwnedStreamItem` with `progress == 1` → `⚙️ seedstrem · cached`.
4. `toOwnedStreamItem` with `progress == 0.45` → `⚙️ seedstrem · 45%`.
5. `toOwnedStreamItem` with `progress == 0` → `⚙️ seedstrem · queued`.
6. Regression: the raw release title remains description **line 1** (AIOStreams
   filename parsing) and the existing stat line is still present.
7. (Optional but valuable) A parser-parity test that applies the AIOStreams
   indexer regex to the generated description and asserts the captured group
   equals the expected tag text (`seedstrem · cached`, `seedstrem · Peerflix`,
   etc.) — guards against emoji/spacing regressions.

## Risks / notes

- Rendering of the tag ultimately depends on the user's AIOStreams formatter
  template; some templates may style the indexer differently. The text is always
  present in seedstrem's own description regardless, so direct users are never
  worse off.
- Uses the exact `⚙️` codepoint (with U+FE0F) from AIOStreams' set — the `⚙`
  already used in the *name* omits the variation selector, so do not copy that
  one for the description line.
