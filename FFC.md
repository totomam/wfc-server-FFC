# wfc-server-FFC

Fork of [WiiLink24/wfc-server](https://github.com/WiiLink24/wfc-server) (AGPL-3.0, see LICENSE) to host
**Fossil Fighters: Champions** (VDEE, GameSpy `dental2ds`) online: regular PvP and Team Plaza.

Branch `ffc` = upstream `main` + two FFC patches:
1. `nas`: fix the `/pr` name check for DS games (any DS game).
2. `sake`: Team Plaza support (Sake ops, `dental2ds/tbl_main_na` table, game ID 3026).

Tests: `go test ./...`; game-side conformance test and docs live in the FFC decomp repo (`docs/online/`).
Plan, hosting needs and open items (DS HTTPS certificate): FFC decomp `docs/online/BACKUP_WIILINK.md`.
Never commit keys or certificates to this repo.
