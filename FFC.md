# wfc-server-FFC

Fork of [WiiLink24/wfc-server](https://github.com/WiiLink24/wfc-server) (AGPL-3.0, see LICENSE) to host
**Fossil Fighters: Champions** (VDEE, GameSpy `dental2ds`) online: regular PvP and Team Plaza.

Branch `ffc` = upstream `main` + two FFC patches:
1. `nas`: fix the `/pr` name check for DS games (any DS game).
2. `sake`: Team Plaza support (Sake ops, `dental2ds/tbl_main_na` table, game ID 3026).

Tests: `go test ./...`; game-side conformance test and docs live in the FFC decomp repo (`docs/online/`).
Plan, hosting needs and open items (DS HTTPS certificate): FFC decomp `docs/online/BACKUP_WIILINK.md`.
Never commit keys or certificates to this repo.

## Public stats API (leaderboard, ffc-decomp ROADMAP #21-#27, #32, #67)
Read-only JSON on the NAS HTTP port, no auth, CORS `*`; computed on read from `match_results` (package `ffcstats`):
- `/api/ffc/leaderboard?min=N` -- W/L/D + Elo (start 1000, K 32, draw 0.5, chronological replay), rank.
- `/api/ffc/vivosaurs` -- pick-rate + win-rate per creature id. `/api/ffc/h2h?a=PID&b=PID` -- head-to-head.
- `/api/ffc/matches?pid=PID&limit=N` -- match log, newest first. `/api/ffc/seasons` -- seasons.tsv.
- `/api/ffc/teams?min=N&limit=N` -- team compositions (creature-id set) + creature pairs: picks, W/L/D, rates (#24).
- `/api/ffc/balance?top=N` -- matches grouped by rules version (`match_ver`: 90 vanilla, else table+package crc), first/last match + vivosaur rates per version: the effect of each published table (#29).
- Common: `season=<id>|current` or `from`/`to=YYYY-MM-DD` (UTC, `to` exclusive); `queue=modded|vanilla`;
  `status=all` also counts `single`/`mismatch` rows (default: only `confirmed` = both consoles agreed).
- Seasons / resets: `seasons.tsv` in the server's working directory, lines `<id>\t<name>\t<YYYY-MM-DD start>`
  (example `seasons.example.tsv`); a season ends where the next starts. No file = no seasons (all-time only).

## Match alerts + webhook feed (ffc-decomp ROADMAP #28, #66)
- Every stored report logs `Match result <id> <status>`; a contradicting pair also logs a WARN `MISMATCH on <field>`.
- Events (config.xml `<eventReporting><discord><webhook><eventTypes>`, or `all`): `match_result` once per match when
  the 2nd console confirms it (players, result, queue, teams); `match_mismatch` when the reports disagree (+ field,
  second reporter). A lone (`single`) report sends nothing.
- Admin list: `GET /api/ffc/admin/flagged?secret=<apiSecret>&status=mismatch|single|both&days=30&limit=100` ->
  flagged rows (newest first, which side reported) + mismatch count per profile. Act with `/api/ban` (reason required).

## Live counter + admin dashboard (ffc-decomp ROADMAP #30, #33, #72)
- `GET /api/ffc/online` (public, counts only): `online` (logged in), `matchmaking` (QR2 sessions), `in_battle`,
  `modded` / `vanilla` (QR2 `dwc_mver` != / == 90). For a web/Discord widget (#39).
- `GET /api/ffc/admin/dashboard?secret=<apiSecret>&days=14`: online players (pid, name, GP status), match activity
  (per UTC hour + per day, queue split, status counts, peak hour), matches in 24 h, mismatch count per profile.

## Web presence widget (ffc-decomp ROADMAP #39)
- `GET /ffc/widget[?top=N]` (N 1..50, default 10): static HTML (inline script/style, no external JS/CSS/fonts) that reads
  `/api/ffc/online` + `/api/ffc/leaderboard` from the same host every 60 s; names via textContent only. CSP
  `connect-src 'self'`, `frame-ancestors *`. Embed: `<iframe src="https://SERVER/ffc/widget?top=5" width="320" height="360" style="border:0"></iframe>`.

## Bonus Data messages (ffc-decomp ROADMAP #96, #38)
- `GET /api/ffc/admin/bonus?secret=…` lists dlc/VDEE messages; `POST {"secret","id","text"}` writes one (id 1..64,
  printable ASCII + newlines, <= 1023 chars, atomic), `POST {"secret","id","delete":true}` removes it. Served by the
  existing Nd count/list/contents. Each id reaches a save ONCE (the game flags it), so every announcement needs a new
  id; ids 1..7 also give fossil rocks 600..606 (empty in the USA ROM), ids 8..64 rock id 0 (verify in game first).

## Ranked vs casual (ffc-decomp ROADMAP #35, server-only)
- Each result row gets `queue` from the QR2 group holding both players at report time: Battle with Anyone (dwc_mtype
  0/1, "anybody") = `ranked`, friend battle (2/3, "private") = `casual`, no shared group = `` (counted as ranked).
- Every public `/api/ffc/*` stats read takes `mode=ranked` (default: all but friend battles) | `casual` | `all`
  (independent of `queue=modded|vanilla`). Rows from before the column count as ranked.

## Event / boss Plaza teams (ffc-decomp ROADMAP #40)
- Table `ffc_plaza_events` (record_id, name, start_at, end_at UTC; created on start + schema.sql). A dental2ds
  `tbl_main_na` record with any window is served to the game (search / random / specific, all via GetSakeRecords)
  ONLY while one window is open; records without a window are unchanged. Delete its last window -> always-on again.
- Make the team with decomp `tools/online_ghost.py` (owner 999000001; "boss" = whatever stats/moves you set), then
  `POST /api/ffc/admin/events {"secret","record_id","name","start","end"}` (RFC 3339, end > start; 404 unknown record),
  `POST {"secret","id","delete":true}`, `GET ?secret=…[&all=1]` (windows not over / all, `active`, `team_name`).
  Decomp `tools/online_ghost.py event ...` wraps the same rows over psql.

## Plaza team / ghost ratings (ffc-decomp ROADMAP #57)
- `GET /api/ffc/plaza[?sort=rating|ratings|wins&min=N&ghosts=1&limit=N]` (public, rate-limited): teams visible now,
  `ratings` count + `avg_rating` (game RateRecord, one per rater), self-reported `battle_win/lose/draw`, `ghost`.
- `GET /api/ffc/admin/ratings?secret=…&record_id=N`: every rater pid + rating, histogram, average.

## Persistent player identity (ffc-decomp ROADMAP #53)
- A player is a profile id, bound to (WFC user id of the console firmware, gsbrcd = game code + a random code the
  save's Wi-Fi user data gets on its first connection). Same console + same save = same profile; a new save, a save
  moved to another DS (the game asks to "Update Wi-Fi User Information") or a new emulator firmware = a new profile
  (melonDS-verified, decomp docs/sessions/s100-online/identity.md).
- `GET /api/ffc/admin/links?secret=…`; `POST /api/ffc/admin/links {"secret","alias","canonical","note"}` links an
  extra profile to the main one (one level: linking to an alias uses its canonical; old links follow);
  `{"secret","alias","delete":true}` unlinks. Table `ffc_player_links`.
- Every public `/api/ffc/*` stat (leaderboard, Elo, h2h, matches `pid=`) counts an alias as its canonical; a
  match between two linked profiles is dropped from the stats. `match_results` rows are never rewritten.

## Tournaments + rewards feed (ffc-decomp ROADMAP #45, #43, server part)
- `tournaments.json` in the server working dir (example `tournaments.example.json`): single-elimination brackets,
  `players` in seed order (pairs 1-2, 3-4, ... in round 1; padded with byes), `start`/`end` UTC, `mode` casual
  (default: friend battles) | ranked | all. Edit the file to add a tournament; no restart needed.
- `/api/ffc/tournaments` (`?id=X`): each bracket resolved from **confirmed** match_results: a pairing is won by the
  first confirmed non-draw match between its two players inside the window and after both players' previous bracket
  win. Rounds, winners, the deciding match id/time, `champion`, `finished`. Profiles are canonical (#53 links).
- `/api/ffc/rewards`: champions of finished tournaments + top 3 by Elo (>= 5 confirmed ranked games) of every finished
  season. For a Discord bot to assign roles (#43); the bot itself (and its token) lives outside this server.

## Abuse limits (ffc-decomp ROADMAP #69, API part)
- Public `/api/ffc/*` reads: per client IP 20 requests burst, 60/min refill (HTTP 429 + Retry-After); loaded match
  rows cached 15 s per time window. Admin endpoints (secret) unlimited. Game protocols (GPCM/QR2/NATNEG) unchanged.
