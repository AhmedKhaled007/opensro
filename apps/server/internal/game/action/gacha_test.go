package action

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"opensro.online/server/internal/game/enterworld"
	"opensro.online/server/internal/game/item/gacha"
	"opensro.online/server/internal/game/item/grounditem"
	"opensro.online/server/internal/game/item/wire"
	"opensro.online/server/internal/game/world/simulation"
)

func gachaFixture(t *testing.T) (*Runtime, *enterworld.Character, []byte) {
	t.Helper()
	c := testCharacter()
	items := testItems()
	for i, name := range []string{gacha.CardCodename, gacha.WinCardCodename, gacha.LoseCardCodename} {
		variant := int64(2)
		if i == 0 {
			variant = 1
		}
		items[name] = &enterworld.ItemRef{RefObjID: uint32(900 + i), Codename: name, TypeIDs: [4]int64{3, 3, 14, variant}}
	}
	items[gacha.CardCodename].ParamDescriptions = [20]string{gacha.LoseCardCodename, gacha.WinCardCodename}
	rt, _ := newTestRuntime(c, gmItemSource{items})
	dir := t.TempDir()
	for name, value := range map[string]string{"gachaitemset.txt": "1 1 11459 5000 1 1\n1 2 11460 10000 1 2\n", "gachanpcmap.txt": "1 9251 1 2\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := rt.ConfigureGacha(dir); err != nil {
		t.Fatal(err)
	}
	rt.NpcSpawn.Enabled = true
	rt.NpcRoster = []simulation.NpcDef{{ObjectID: 17, RefObjID: 9251, AuthoredSpawn: true, Spawn: simulation.SeedWorldState(c).Spawn}}
	rt.Selected.Set(testDivision, c.Name, 17)
	c.MissionInventory = []enterworld.InventoryRow{{Slot: 13, RefObjID: 900, Codename: gacha.CardCodename, TypeFlags: items[gacha.CardCodename].TypeFlags(), StackCount: 1}}
	return rt, c, wire.NewWriter(9).U32(17).U32(1).U8(13).Payload()
}

func TestGachaCommitResultOrderAndReplay(t *testing.T) {
	for _, roll := range []uint32{0, 4999, 5000, 9999} {
		t.Run(string(rune('A'+roll%26)), func(t *testing.T) {
			rt, c, p := gachaFixture(t)
			draws := 0
			rt.GachaRoll = func() (uint32, error) { draws++; return roll, nil }
			frames, reason := rt.HandleGachaRoll(testDivision, c, p)
			if reason != "" {
				t.Fatal(reason)
			}
			assertOpcodes(t, frames, 0x3645, 0xb053)
			win := roll < 5000
			want := uint32(902)
			code := byte(0)
			if win {
				want = 901
				code = 1
			}
			if c.MissionInventory[0].RefObjID != want || !reflect.DeepEqual(frames[1].Payload, []byte{1, code}) {
				t.Fatalf("wrong result: %+v %+v", c.MissionInventory, frames)
			}
			reward := uint64(11460)
			wantDraws := 2
			if win {
				reward = 11459
				wantDraws = 1
			}
			if draws != wantDraws {
				t.Fatalf("draws=%d want %d", draws, wantDraws)
			}
			if !reflect.DeepEqual(c.MissionInventory[0].MagicOptions, []uint64{reward, 1}) {
				t.Fatal("reward lost")
			}
			before := c.Snapshot()
			again, reason := rt.HandleGachaRoll(testDivision, c, p)
			if reason == "" || len(again) != 0 || draws != wantDraws || !reflect.DeepEqual(c.Snapshot(), before) {
				t.Fatal("replayed ticket mutated state")
			}
		})
	}
}

func TestGachaAdmissionFailuresPreserveTicket(t *testing.T) {
	for _, kind := range []string{"catalog", "random missing", "random failure", "random range", "selection", "outside scope", "disabled NPC", "entry", "slot", "ticket", "delete pending", "truncated", "trailing"} {
		t.Run(kind, func(t *testing.T) {
			rt, c, p := gachaFixture(t)
			rt.GachaRoll = func() (uint32, error) { return 0, nil }
			switch kind {
			case "catalog":
				rt.GachaCatalog = nil
			case "random missing":
				rt.GachaRoll = nil
			case "random failure":
				rt.GachaRoll = func() (uint32, error) { return 0, errors.New("entropy unavailable") }
			case "random range":
				rt.GachaRoll = func() (uint32, error) { return 10000, nil }
			case "selection":
				rt.Selected.Set(testDivision, c.Name, 18)
			case "outside scope":
				rt.NpcRoster[0].Spawn.RegionID = 0
			case "disabled NPC":
				rt.NpcSpawn.Enabled = false
			case "entry":
				p[4] = 99
			case "slot":
				p[8] = 1
			case "ticket":
				c.MissionInventory[0].RefObjID = 999
			case "unlisted pair":
				// Resolvable and in range, but no prize row names it: a pair no roll writes.
				c.MissionInventory[0].MagicOptions = []uint64{uint64(rt.GachaCatalog.Card.RefObjID), 1}
			case "stacked":
				c.MissionInventory[0].StackCount = 2
			case "delete pending":
				c.DeletePending = true
			case "truncated":
				p = p[:8]
			case "trailing":
				p = append(p, 0)
			}
			before := c.Snapshot()
			frames, reason := rt.HandleGachaRoll(testDivision, c, p)
			if reason == "" || len(frames) != 0 || !reflect.DeepEqual(c.Snapshot(), before) {
				t.Fatalf("refusal changed state: %s %+v", reason, frames)
			}
		})
	}
}

func TestGachaOpeningUsesWorldAdmission(t *testing.T) {
	rt, c, _ := gachaFixture(t)
	p := wire.NewWriter(8).U32(17).U32(0x10000).Payload()
	frames, reason := rt.HandleGachaNpcAction(testDivision, c, p)
	if reason != "" {
		t.Fatal(reason)
	}
	assertOpcodes(t, frames, 0xb338)
	rt.NpcRoster[0].Spawn.RegionID = 0
	if frames, reason = rt.HandleGachaNpcAction(testDivision, c, p); reason == "" || len(frames) != 0 {
		t.Fatal("out-of-scope machine admitted")
	}
}

func TestGachaRewardsAreAdmittedToWorldReferences(t *testing.T) {
	rt, _, _ := gachaFixture(t)
	counts := map[string]int{}
	for _, name := range rt.StaticRefItemCodenames() {
		counts[name]++
	}
	for _, name := range rt.GachaCatalog.RewardCodenames() {
		if counts[name] != 1 {
			t.Fatalf("reward %s admitted %d times", name, counts[name])
		}
	}
}

func TestPublishedRewardsSendNoGroundReferenceDelta(t *testing.T) {
	rt, _, _ := gachaFixture(t)
	rewards := rt.GachaCatalog.RewardCodenames()
	if len(rewards) == 0 {
		t.Fatal("fixture has no rewards")
	}
	published := grounditem.Item{RefObjID: 70001, Codename: rewards[0], TypeFlags: wire.PackTypeFlags(3, 3, 1, 1)}
	if frames := rt.groundReferences([]grounditem.Item{published}); len(frames) != 0 {
		t.Fatalf("published reward resent as %d reference frames", len(frames))
	}
	unpublished := grounditem.Item{RefObjID: 50080, Codename: "ITEM_CH_SWORD_09_B", TypeFlags: wire.PackTypeFlags(3, 1, 6, 2)}
	if frames := rt.groundReferences([]grounditem.Item{unpublished}); len(frames) != 1 {
		t.Fatalf("unpublished drop produced %d reference frames, want 1", len(frames))
	}
}

func TestGachaFailedWasteDrawPreservesTicket(t *testing.T) {
	for _, bad := range []bool{false, true} {
		rt, c, p := gachaFixture(t)
		calls := 0
		rt.GachaRoll = func() (uint32, error) {
			calls++
			if calls == 1 {
				return 5000, nil
			}
			if bad {
				return 10000, nil
			}
			return 0, errors.New("waste entropy unavailable")
		}
		before := c.Snapshot()
		frames, reason := rt.HandleGachaRoll(testDivision, c, p)
		if reason == "" || len(frames) != 0 || calls != 2 || !reflect.DeepEqual(before, c.Snapshot()) {
			t.Fatalf("failed waste draw committed: %v %s", frames, reason)
		}
	}
}

// operatorFixture is the roll fixture with Gori beside the machine and a voucher in the bag.
func operatorFixture(t *testing.T, roll uint32) (*Runtime, *enterworld.Character, []byte) {
	t.Helper()
	rt, c, p := gachaFixture(t)
	rt.GachaRoll = func() (uint32, error) { return roll, nil }
	if _, reason := rt.HandleGachaRoll(testDivision, c, p); reason != "" {
		t.Fatal(reason)
	}
	rt.NpcRoster = append(rt.NpcRoster, simulation.NpcDef{
		ObjectID: 18, RefObjID: 9252, Codename: gachaOperatorCodename, AuthoredSpawn: true,
		Spawn: simulation.SeedWorldState(c).Spawn,
	})
	rt.Selected.Set(testDivision, c.Name, 18)
	return rt, c, wire.NewWriter(4).U32(18).Payload()
}

func TestGachaExchangeSwapsTheVoucherForItsRecordedReward(t *testing.T) {
	for _, tc := range []struct {
		name   string
		roll   uint32
		reward uint32
	}{{"win", 0, 11459}, {"loss", 9999, 11460}} {
		t.Run(tc.name, func(t *testing.T) {
			rt, c, p := operatorFixture(t, tc.roll)
			frames, reason := rt.HandleGachaExchange(testDivision, c, p)
			if reason != "" {
				t.Fatal(reason)
			}
			rewardSlot := int64(-1)
			for _, row := range c.MissionInventory {
				if rt.GachaCatalog.IsResultCard(row.RefObjID) {
					t.Fatalf("voucher survived the exchange: %+v", row)
				}
				if row.RefObjID == tc.reward && row.StackCount == 1 {
					rewardSlot = row.Slot
				}
			}
			// The voucher's freed slot is the first free one, so the reward takes it.
			if rewardSlot != 13 {
				t.Fatalf("reward %d at slot %d, want the voucher's slot 13: %+v", tc.reward, rewardSlot, c.MissionInventory)
			}
			quantity := -1
			for index, frame := range frames {
				if frame.Opcode == 0x3645 {
					quantity = index
					if !reflect.DeepEqual(frame.Payload, []byte{13, 8, 0, 0}) {
						t.Fatalf("voucher update %v", frame.Payload)
					}
				}
				if frame.Opcode == wire.OpItemMoveResponse && reflect.DeepEqual(frame.Payload, []byte{1, 15, 13, 0}) {
					t.Fatal("voucher left by a row delete, which 7654B0 does not turn into the exchange notice")
				}
				// A frame writing slot 13 before the 0x3645 would let the client zero the reward.
				if frame.Opcode == wire.OpItemMoveResponse && len(frame.Payload) > 2 && frame.Payload[2] == 13 && quantity < 0 {
					t.Fatalf("frame %d writes slot 13 before the voucher's 0x3645: %+v", index, frames)
				}
			}
			if quantity != 0 {
				t.Fatalf("the voucher's 0x3645 is frame %d, want first: %+v", quantity, frames)
			}
		})
	}
}

func TestGachaExchangeTakesTheLowestSlotVoucherFirst(t *testing.T) {
	rt, c, p := operatorFixture(t, 0)
	extra := c.MissionInventory[0]
	extra.Slot = 20
	extra.MagicOptions = []uint64{11460, 1}
	extra.RefObjID, extra.Codename = rt.GachaCatalog.LoseCard.RefObjID, rt.GachaCatalog.LoseCard.Codename
	c.MissionInventory = append(c.MissionInventory, extra)
	if _, reason := rt.HandleGachaExchange(testDivision, c, p); reason != "" {
		t.Fatal(reason)
	}
	for _, row := range c.MissionInventory {
		if row.RefObjID == 11459 {
			return
		}
	}
	t.Fatalf("slot 13's win reward was not granted first: %+v", c.MissionInventory)
}

func TestGachaExchangeConsumesTheVoucherItRead(t *testing.T) {
	rt, c, p := operatorFixture(t, 0)
	// The paid voucher sits at slot 20; a same-codename card without a pair
	// (an operator grant, say) sits below it at slot 13.
	c.MissionInventory[0].Slot = 20
	blank := c.MissionInventory[0]
	blank.Slot = 13
	blank.MagicOptions = nil
	c.MissionInventory = append(c.MissionInventory, blank)
	if _, reason := rt.HandleGachaExchange(testDivision, c, p); reason != "" {
		t.Fatal(reason)
	}
	rewards := 0
	var blankKept, voucherKept bool
	for _, row := range c.MissionInventory {
		switch {
		case row.RefObjID == 11459:
			rewards += int(row.StackCount)
		case row.Slot == 13 && row.RefObjID == blank.RefObjID && len(row.MagicOptions) == 0:
			blankKept = true
		case row.Slot == 20 && rt.GachaCatalog.IsResultCard(row.RefObjID):
			voucherKept = true
		}
	}
	if rewards != 1 || !blankKept || voucherKept {
		t.Fatalf("rewards=%d blank kept=%v voucher kept=%v: %+v", rewards, blankKept, voucherKept, c.MissionInventory)
	}
	before := c.Snapshot()
	if frames, reason := rt.HandleGachaExchange(testDivision, c, p); reason == "" || len(frames) != 0 ||
		!reflect.DeepEqual(c.Snapshot(), before) {
		t.Fatal("the blank card paid out the voucher's reward a second time")
	}
}

func TestGachaExchangeRefusalsPreserveTheVoucher(t *testing.T) {
	for _, kind := range []string{"selection", "machine", "outside scope", "no voucher", "bad pair", "unlisted pair", "stacked", "delete pending", "truncated", "trailing", "catalog"} {
		t.Run(kind, func(t *testing.T) {
			rt, c, p := operatorFixture(t, 0)
			switch kind {
			case "selection":
				rt.Selected.Set(testDivision, c.Name, 17)
			case "machine":
				rt.Selected.Set(testDivision, c.Name, 17)
				p = wire.NewWriter(4).U32(17).Payload()
			case "outside scope":
				rt.NpcRoster[1].Spawn.RegionID = 0
			case "no voucher":
				c.MissionInventory = nil
			case "bad pair":
				c.MissionInventory[0].MagicOptions = []uint64{11459, 0}
			case "unlisted pair":
				// Resolvable and in range, but no prize row names it: a pair no roll writes.
				c.MissionInventory[0].MagicOptions = []uint64{uint64(rt.GachaCatalog.Card.RefObjID), 1}
			case "stacked":
				c.MissionInventory[0].StackCount = 2
			case "delete pending":
				c.DeletePending = true
			case "truncated":
				p = p[:3]
			case "trailing":
				p = append(p, 0)
			case "catalog":
				rt.GachaCatalog = nil
			}
			before := c.Snapshot()
			frames, reason := rt.HandleGachaExchange(testDivision, c, p)
			if reason == "" || !reflect.DeepEqual(c.Snapshot(), before) {
				t.Fatalf("refusal changed state: %q %+v", reason, frames)
			}
		})
	}
}

func TestGachaExchangeReplayFindsNoVoucher(t *testing.T) {
	rt, c, p := operatorFixture(t, 0)
	if _, reason := rt.HandleGachaExchange(testDivision, c, p); reason != "" {
		t.Fatal(reason)
	}
	before := c.Snapshot()
	if frames, reason := rt.HandleGachaExchange(testDivision, c, p); reason == "" || len(frames) != 0 ||
		!reflect.DeepEqual(c.Snapshot(), before) {
		t.Fatal("replay granted a second reward")
	}
}
