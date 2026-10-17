/*
===========================================================================

gmzoe_test.go - /ZOE

Spawns the requested monsters at the GM and kills each one with the ordinary
reward settlement; a refusal spawns nothing and kills nothing.

===========================================================================
*/
package action

import (
	"fmt"
	"testing"

	"opensro.online/server/internal/game/enterworld"
	"opensro.online/server/internal/game/item/wire"
)

const zoeTestRegion = uint16(0x62A8)

/*
================
deadMonsters
================
*/
func deadMonsters(rt *Runtime) int {
	dead := 0
	for _, row := range rt.Monsters.InstancesInRegions(testDivision, []uint16{zoeTestRegion}) {
		if row.CurrentHP == 0 {
			dead++
		}
	}
	return dead
}

/*
================
TestGMZoeSpawnsThenKillsForRewards
================
*/
func TestGMZoeSpawnsThenKillsForRewards(t *testing.T) {
	f := newMobKillFixture(t)
	if deadMonsters(f.rt) != 0 {
		t.Fatal("the fixture starts with a dead monster")
	}
	paid := 0
	f.rt.UpdateExperience = func(*enterworld.Character, int64, int64, uint32) ([]wire.Frame, bool) {
		paid++
		return []wire.Frame{{Opcode: wire.OpExpUpdate, Payload: []byte{0xA5}}}, true
	}
	if !f.rt.KillGMZoe(testDivision, f.gm.Name, 1933, 3) {
		t.Fatal("the zoe command was refused")
	}
	if dead := deadMonsters(f.rt); dead != 3 {
		t.Fatalf("%d dead monsters, want 3", dead)
	}
	if paid != 3 {
		t.Fatalf("the GM was paid %d times, want 3", paid)
	}
	public, private := f.opcodes(t)
	if !hasOp(public, wire.OpObjectStateRefresh) || !hasOp(private, wire.OpExpUpdate) {
		t.Fatalf("frames public=%v private=%v", public, private)
	}
}

/*
================
TestGMZoeRefusals
================
*/
func TestGMZoeRefusals(t *testing.T) {
	f := newMobKillFixture(t)
	if f.rt.KillGMZoe(testDivision, f.gm.Name, 0xFFFFFF, 2) {
		t.Fatal("an unknown monster reference was spawned")
	}
	if f.rt.KillGMZoe(testDivision, f.gm.Name, 1933, 0) {
		t.Fatal("a zero count was honored")
	}
	f.gm.GMPrivilege = false
	if f.rt.KillGMZoe(testDivision, f.gm.Name, 1933, 1) {
		t.Fatal("a non-GM ran the command")
	}
	if dead := deadMonsters(f.rt); dead != 0 {
		t.Fatalf("refusals killed %d monsters", dead)
	}
}

/*
================
TestGMZoeHonorsTheFullByteCount

The /LOADMONSTER cap of 250 must not truncate /ZOE's last five victims.
================
*/
func TestGMZoeHonorsTheFullByteCount(t *testing.T) {
	for _, count := range []uint8{250, 251, 255} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			f := newMobKillFixture(t)
			if !f.rt.KillGMZoe(testDivision, f.gm.Name, 1933, count) {
				t.Fatal("zoe refused")
			}
			if deadMonsters(f.rt) != int(count) || f.calls != int(count) {
				t.Fatalf("dead=%d paid=%d, want %d of each", deadMonsters(f.rt), f.calls, count)
			}
			original, _ := f.rt.Monsters.Get(testDivision, f.gid)
			if original.CurrentHP == 0 {
				t.Fatal("zoe killed a preexisting monster")
			}
		})
	}
}
