# Penguin live acceptance — 2026-10-22

Worktree: `E:/CodeProjects/opensro-lab`, branch `feat/attack-pet-penguin-rework`.
These are authenticated gameplay-protocol tests against the running GameWorld,
not mocked combat. They do not certify rendered effects or sounds.

## Setup and scope

The user explicitly authorized temporary GM for **LabPetTest**, native giants,
testing and cleanup. The stopped local Nomad stack was restored with its same
state directory and existing account/authority DB. Its pinned Nomad executable
is in the hub `.tools/nomad/2.0.7`, not the worktree. Vite5180 was restored from
the worktree. No database fixture edits, monster HP overrides, skill-rank
changes or drop-rate changes were used.

The temporary allowlist appended `global-official:LabPetTest` to the existing
`global-official:asd2`. Native `/LOADMONSTER` type4 created level20 Chakji and
then level30 Hyeongcheon giants in the Mangyang field, region25258 x912/z643.
Level20 giants could die before the final pulse; level30 giants have native
38180HP and survived. Learned books/gear were retained, including the existing
effective-rank bonus from equipment.

## Results

| Check | Captured result |
| --- | --- |
| Blizzard III | Target453261: **38180 → 33681 → 28308 → 22935 → 18009 HP**, four hits at one-second intervals. |
| Final freeze | All three giants453261–453263 received native status mask**3** (freeze + frostbite) after the last pulse. Earlier pulses had mask2 only. |
| Shatter III | Penguin killed frozen453261 with **12576** damage. Neighbors453262/453263 received D6 signals and each lost **6288HP**, exactly50%:18174→11886 and17805→11517. |
| Resonance III echo | At five pet stacks, Lance III used separate tokens**37 and38**. Primary hit**17448**, echo**16610**. Fresh giant453274 ended at**4122HP**:38180−17448−16610. |
| Hourglass | Same charged Lance published**4000ms** row cooldown; an uncharged Lance published**6000ms**. One flat2000ms refund; no second cooldown for echo. |
| No recursive progress | After the charged original plus echo, pet/owner/Hourglass counters were**1/0/1**, starting one new action rather than counting the echo. |

The successful freeze/Shatter cast began at Unix-ms1792647221547; its first
release was1792647222253 and final freeze1792647225255. The frozen killing hit
was1792647227953. Charged Lance released at1792647268352; echo and4000ms timer
arrived at1792647268353. Uncharged6000ms timer arrived at1792647285453.

## Bug found and fixed

Storm pulses already changed authoritative HP, but `state.public` routes pet
frames to observers **except the owner**. The pulse code sent the owner only
private rewards, omitting HP/status updates. This made surviving targets appear
undamaged and hid the final freeze from the owner's client.

`advancePenguin` now returns HP/status/reward frames to the existing private
pet tick batch. Observer publication remains intact. Changes reach
`petpenguin.go`, its caller in `petai.go`, and `petpenguin_test.go`.
The expanded `TestPenguinStormPulsesAndFinalFreeze` failed before this fix and
passes after it, checking owner HP after every pulse and freeze delivery.
Temporary server trace logging was removed before final deployment.

The test console also needed correct handling of combined HP/status packets:
the status mask follows present HP/MP words, not always offset7. It now sends
follow only after the committed cooldown receipt, avoiding automatic basic
attacks killing a test target prematurely. Repeated monster-list330A tails
are not treated as fresh world entry. These are ignored test-tool corrections.

## Verification and evidence

- Full `go test ./...` passed after the production fix.
- All focused `TestPenguin` tests passed after strengthening HP/freeze assertions.
- `golangci-lint run ./...`: **0 issues**. `git diff --check`: passed.
- No client implementation changed in this follow-up; its earlier23 focused
  test results remain recorded in `PENGUIN-REWORK.md`.
- The cgo race gate remains unavailable; no visual/audio acceptance claim.

Local detailed evidence (ignored, preserve for continuation):

- `.state/penguin-targeted-live.log`: timestamped commands/native frames.
- `.state/penguin-gm-evidence.py` / `.json`: read-only assertions over the
  captured pulse, freeze, Shatter, echo, counters and cooldown evidence.
- `apps/server/.state/penguin-gm-regressions.log`,
  `penguin-gm-all-tests.log`, `penguin-gm-lint.log`.
- `.state/penguin-gm-{deploy,trace-deploy,fix-deploy,revoke-deploy}.log`.
- `.state/penguin-gm-baseline.json` and `penguin-gm-cleanup.json`: local state
  snapshots; do not commit these account snapshots.

## Cleanup and final deployment

All remaining spawned giants were killed with native `/MOBKILL` mode1
(no rewards); already defeated targets correctly refused that command.
Earlier test populations were discarded during ordinary deployment restart.
LabPetTest returned to Hotan via `/GOTOWN`, combat stopped, sockets closed.
Ordinary combat rewards, satiety and rental-clock passage were retained;
test loot uses ordinary expiration. No characters died.

The original allowlist **`global-official:asd2` is restored**. Read-only DB
verification found LabPetTest unprivileged. A fresh login's harmless WORLDSTATUS
request was refused; the server logged “GM command from a character without
GM privilege refused” at07:37:03.639 local time. No test GM remains.

Final healthy deployment: **Agentv0 / GameWorldv3** on the restarted development
agent. Release ID (not binary SHA):
`813830f56cb577572299f474149706d586c1aa52ca33739a135b3892ff5690bf`.
GameWorld allocation:`1848c69b-d8fb-4d45-8a4c-8da1cf0ce04e`.
Expansion remains enabled. Vite:`http://127.0.0.1:5180/`.

LabPetTest: **13226HP**, Hotan region23687 x1138/y250/z153. Penguin:16945HP.
Books, passive selections, skill points and equipped pet gear are preserved;
normal row cooldown timestamps changed through casting. Read-only comparison
confirmed **LabBlader, BondOwner and BondParty unchanged**. No commit/push made.

Next: visual/audio review when the permitted browser session is accessible,
then balance review. Do not repeat GM setup merely to reproduce these completed
checks, or claim the pending rendered/race checks passed.

## User review

The user subsequently tested the deployed game, reported “looks good”, and
approved committing/pushing the penguin rework. This records user acceptance;
it does not claim the unavailable race gate or every individual sound was tested.
