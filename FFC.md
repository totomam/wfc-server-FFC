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

## Bonus Data messages (ffc-decomp ROADMAP #96, #38)
- `GET /api/ffc/admin/bonus?secret=…` lists dlc/VDEE messages; `POST {"secret","id","text"}` writes one (id 1..64,
  printable ASCII + newlines, <= 1023 chars, atomic), `POST {"secret","id","delete":true}` removes it. Served by the
  existing Nd count/list/contents. Each id reaches a save ONCE (the game flags it), so every announcement needs a new
  id; ids 1..7 also give fossil rocks 600..606 (empty in the USA ROM), ids 8..64 rock id 0 (verify in game first).

## Abuse limits (ffc-decomp ROADMAP #69, API part)
- Public `/api/ffc/*` reads: per client IP 20 requests burst, 60/min refill (HTTP 429 + Retry-After); loaded match
  rows cached 15 s per time window. Admin endpoints (secret) unlimited. Game protocols (GPCM/QR2/NATNEG) unchanged.
