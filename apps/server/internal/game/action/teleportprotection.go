/*
===========================================================================

teleportprotection.go - the untouchable grace after a teleport arrival

A successful return or reverse return scroll, gate or server relocation
arrives untouchable: body mode 2, the revival's state, for the revival's
grace (6.0 s, 11.0 in a siege world; spawnprotection.go). Monsters skip the
player as a target (540DE0) and the player cannot attack (5291D0, 0x3020).

Inference, stated here because SR_GameServer's teleport arm is not read:
the original has an untouchable grace after a teleport (owner, from play on
the original, 2026-10-24). The v1.150 client carries the state's notice for
refusal code 32 (68A0A0, UIIT_MSG_SKILL_USE_FAIL_CANT_ATTACK_TEMPORARILY,
"Cannot attack or be attacked for 5 seconds during untouchable status") and
never grants the state itself, so the server granted it. No binary read
here passes the notice's 5 seconds; the grace reuses the revival's.

The grace starts at the arrival's game-ready, not at the travel commit. A
loading session is outside the tick, so nothing can reach it there, and a
browser client's load can outlast the whole grace. The arrival only marks
the character; ReconcileScene grants the body mode once the scene is live.

===========================================================================
*/

package action

import (
	"opensro.online/server/internal/game/enterworld"
	"opensro.online/server/internal/game/item/wire"
	"opensro.online/server/internal/game/world/simulation"
)

/*
================
noteTeleportArrival

The caller holds the division action door, after the travel's last rollback
point: a failed teleport never marks. A later arrival before game-ready
leaves one mark, so one grace follows the scene that finally goes live.
================
*/
func (rt *Runtime) noteTeleportArrival(division string, c *enterworld.Character) {
	rt.teleportArrivals.Store(simulation.WorldKey(division, c.Name), struct{}{})
}

/*
================
startTeleportGrace

The game-ready half: a marked arrival becomes body mode 2 from now, through
the revival's grant and restore (grantReviveUntouchable), and is published
to the player and its peers. Any other scene going live, a login included,
finds no mark.
================
*/
func (rt *Runtime) startTeleportGrace(division, name string) {
	if _, marked := rt.teleportArrivals.LoadAndDelete(simulation.WorldKey(division, name)); !marked {
		return
	}
	unlock := rt.lockDivision(division)
	defer unlock()
	character := rt.findCharacter(division, name)
	if character == nil {
		return
	}
	var frames []wire.Frame
	rt.deps.UpdateDeferred(character, "teleport-untouchable", func() bool {
		if character.DeletePending || !enterworld.CharacterAlive(character) {
			return false
		}
		frames = rt.grantReviveUntouchable(division, character, rt.Now().UnixMilli())
		return len(frames) > 0
	})
	rt.publishBodyStatus(division, name, frames)
}
