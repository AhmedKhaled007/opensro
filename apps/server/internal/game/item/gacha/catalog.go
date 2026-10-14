// Package gacha owns the v1.150 Magic Pop reference-data and wire contracts.
// Gameplay mutation lives on action because that runtime already owns the
// authoritative inventory, selected-NPC, world and per-division lock planes.
package gacha

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"opensro.online/server/internal/game/enterworld"
	"opensro.online/server/internal/game/item/wire"
)

const (
	CardCodename     = "ITEM_MALL_GACHA_CARD"
	WinCardCodename  = "ITEM_MALL_GACHA_CARD_WIN"
	LoseCardCodename = "ITEM_MALL_GACHA_CARD_LOSE"
)

// Prize is one enabled gachaitemset.txt row.
type Prize struct {
	SetID          uint8
	EntryID        uint32
	RewardRefObjID uint32
	ChancePer10000 uint32
	Quantity       uint32
}

// ItemTemplate is the exact itemdata identity used when the ticket is
// replaced by a win/loss result card.
type ItemTemplate struct {
	RefObjID  uint32
	Codename  string
	TypeFlags uint16
}

// Catalog joins gachaitemset.txt and gachanpcmap.txt with the three card
// itemdata records. Entry IDs are globally unique in the shipped v1.150 file.
type Catalog struct {
	prizesByEntry map[uint32]Prize
	// uiSetIDByNpc is gachanpcmap column 3, the one set byte consumed by
	// CGlobalDataManager's v1.150 +0x454 producer and displayed by CIFGhaCha.
	// Column 4 is present in the media row but is not part of this UI lane;
	// authorizing it would let a forged 0x7053 select an undisplayed prize.
	uiSetIDByNpc        map[uint32]uint8
	alternateSetIDByNpc map[uint32]uint8
	rewardCodenames     []string
	wastePools          map[uint8][]wasteBand
	Card                ItemTemplate
	WinCard             ItemTemplate
	LoseCard            ItemTemplate
}

// HasNpc reports whether the exact NPC RefObjID is present in the shipped
// gachanpcmap.txt. It intentionally does not use an arbitrary prize entry as
// an existence witness: an NPC may expose a different set than entry 1.
func (c *Catalog) HasNpc(npcRefObjID uint32) bool {
	if c == nil {
		return false
	}
	return c.uiSetIDByNpc[npcRefObjID] != 0
}

// LoadCatalog reads the shipped v1.150 data. A malformed or internally
// inconsistent row fails construction: admitting 0x7053 without its complete
// authority is unsafe and would consume tickets incorrectly.
func LoadCatalog(textdataDir string, items enterworld.ItemRefSource) (*Catalog, error) {
	if textdataDir == "" {
		return nil, fmt.Errorf("gacha: textdata directory is required")
	}
	catalog := &Catalog{
		prizesByEntry:       make(map[uint32]Prize),
		uiSetIDByNpc:        make(map[uint32]uint8),
		alternateSetIDByNpc: make(map[uint32]uint8),
		wastePools:          make(map[uint8][]wasteBand),
	}
	if err := catalog.loadPrizes(filepath.Join(textdataDir, "gachaitemset.txt")); err != nil {
		return nil, err
	}
	if err := catalog.loadNpcMap(filepath.Join(textdataDir, "gachanpcmap.txt")); err != nil {
		return nil, err
	}
	if len(catalog.prizesByEntry) == 0 || len(catalog.uiSetIDByNpc) == 0 {
		return nil, fmt.Errorf("gacha: enabled prize and NPC tables must not be empty")
	}
	// Reward IDs are the values persisted into result cards. Resolve every
	// enabled row, including the alternate set, before accepting any ticket.
	source, ok := items.(interface {
		ItemRefByID(uint32) (*enterworld.ItemRef, bool)
	})
	if !ok {
		return nil, fmt.Errorf("gacha: itemdata source requires ID lookup")
	}
	sets := make(map[uint8]bool)
	names := make(map[string]bool)
	for _, prize := range catalog.prizesByEntry {
		sets[prize.SetID] = true
		ref, found := source.ItemRefByID(prize.RewardRefObjID)
		if !found || ref == nil || ref.RefObjID != prize.RewardRefObjID || ref.Codename == "" {
			return nil, fmt.Errorf("gacha: entry %d reward %d is missing or inconsistent", prize.EntryID, prize.RewardRefObjID)
		}
		byName, found := items.ItemRefByCodename(ref.Codename)
		if !found || byName == nil || byName.RefObjID != ref.RefObjID || byName.Codename != ref.Codename || byName.TypeFlags() != ref.TypeFlags() {
			return nil, fmt.Errorf("gacha: entry %d reward identity indexes disagree", prize.EntryID)
		}
		names[ref.Codename] = true
	}
	for npc, set := range catalog.uiSetIDByNpc {
		for _, referenced := range []uint8{set, catalog.alternateSetIDByNpc[npc]} {
			if !sets[referenced] {
				return nil, fmt.Errorf("gacha: NPC %d references missing enabled set %d", npc, referenced)
			}
		}
	}
	for name := range names {
		catalog.rewardCodenames = append(catalog.rewardCodenames, name)
	}
	sort.Strings(catalog.rewardCodenames)
	for _, set := range catalog.alternateSetIDByNpc {
		if _, prepared := catalog.wastePools[set]; prepared {
			continue
		}
		pool := []Prize{}
		for _, prize := range catalog.prizesByEntry {
			if prize.SetID == set {
				pool = append(pool, prize)
			}
		}
		sort.Slice(pool, func(i, j int) bool { return pool[i].EntryID < pool[j].EntryID })
		bands, err := prepareWastePool(pool)
		if err != nil {
			return nil, fmt.Errorf("gacha: waste set %d: %w", set, err)
		}
		catalog.wastePools[set] = bands
	}
	var err error
	if catalog.Card, err = resolveItem(items, CardCodename); err != nil {
		return nil, err
	}
	// 4DD770 indexes the ticket's authored Desc1 (lose) / Desc2 (win).
	ticket, _ := items.ItemRefByCodename(CardCodename)
	if catalog.WinCard, err = resolveItem(items, ticket.ParamDescriptions[1]); err != nil {
		return nil, err
	}
	if catalog.LoseCard, err = resolveItem(items, ticket.ParamDescriptions[0]); err != nil {
		return nil, err
	}
	if catalog.Card.RefObjID == catalog.WinCard.RefObjID || catalog.Card.RefObjID == catalog.LoseCard.RefObjID || catalog.WinCard.RefObjID == catalog.LoseCard.RefObjID {
		return nil, fmt.Errorf("gacha: ticket and result cards must have distinct identities")
	}
	if catalog.Card.TypeFlags != wire.PackTypeFlags(3, 3, 14, 1) {
		return nil, fmt.Errorf(
			"gacha: %s type word 0x%04X, want TID 3.3.14.1",
			CardCodename,
			catalog.Card.TypeFlags,
		)
	}
	resultFlags := wire.PackTypeFlags(3, 3, 14, 2)
	if catalog.WinCard.TypeFlags != resultFlags || catalog.LoseCard.TypeFlags != resultFlags {
		return nil, fmt.Errorf(
			"gacha: result cards must both be TID 3.3.14.2 (win=0x%04X lose=0x%04X)",
			catalog.WinCard.TypeFlags,
			catalog.LoseCard.TypeFlags,
		)
	}
	return catalog, nil
}

func resolveItem(items enterworld.ItemRefSource, codename string) (ItemTemplate, error) {
	if items == nil {
		return ItemTemplate{}, fmt.Errorf("gacha: itemdata source is absent")
	}
	ref, ok := items.ItemRefByCodename(codename)
	if !ok || ref == nil {
		return ItemTemplate{}, fmt.Errorf("gacha: itemdata row %s is absent", codename)
	}
	if ref.RefObjID == 0 || ref.Codename != codename {
		return ItemTemplate{}, fmt.Errorf("gacha: itemdata row %s has inconsistent identity", codename)
	}
	return ItemTemplate{
		RefObjID:  ref.RefObjID,
		Codename:  ref.Codename,
		TypeFlags: ref.TypeFlags(),
	}, nil
}

func (c *Catalog) loadPrizes(path string) error {
	return scanRows(path, func(line int, fields []string) error {
		if len(fields) < 6 {
			return fmt.Errorf("gacha: %s:%d has %d columns, want at least 6", path, line, len(fields))
		}
		enabled, err := fieldUint(path, line, fields, 0, 8)
		if err != nil || enabled == 0 {
			return err
		}
		setID, err := fieldUint(path, line, fields, 1, 8)
		if err != nil {
			return err
		}
		reward, err := fieldUint(path, line, fields, 2, 32)
		if err != nil {
			return err
		}
		chance, err := fieldUint(path, line, fields, 3, 32)
		if err != nil {
			return err
		}
		quantity, err := fieldUint(path, line, fields, 4, 16)
		if err != nil {
			return err
		}
		entryID, err := fieldUint(path, line, fields, 5, 32)
		if err != nil {
			return err
		}
		if setID == 0 || reward == 0 || quantity == 0 || entryID == 0 || chance > 10000 {
			return fmt.Errorf(
				"gacha: %s:%d invalid enabled row set=%d reward=%d chance=%d quantity=%d entry=%d",
				path, line, setID, reward, chance, quantity, entryID,
			)
		}
		if _, exists := c.prizesByEntry[uint32(entryID)]; exists {
			return fmt.Errorf("gacha: %s:%d duplicate entry id %d", path, line, entryID)
		}
		c.prizesByEntry[uint32(entryID)] = Prize{
			SetID:          uint8(setID),
			EntryID:        uint32(entryID),
			RewardRefObjID: uint32(reward),
			ChancePer10000: uint32(chance),
			Quantity:       uint32(quantity),
		}
		return nil
	})
}

func (c *Catalog) loadNpcMap(path string) error {
	return scanRows(path, func(line int, fields []string) error {
		if len(fields) < 4 {
			return fmt.Errorf("gacha: %s:%d has %d columns, want at least 4", path, line, len(fields))
		}
		enabled, err := fieldUint(path, line, fields, 0, 8)
		if err != nil || enabled == 0 {
			return err
		}
		npcRef, err := fieldUint(path, line, fields, 1, 32)
		if err != nil {
			return err
		}
		if npcRef == 0 {
			return fmt.Errorf("gacha: %s:%d has zero NPC RefObjID", path, line)
		}
		uiSetID, err := fieldUint(path, line, fields, 2, 8)
		if err != nil {
			return err
		}
		// Retain and validate the alternate set without authorizing it as a
		// player-selected entry. Its server lottery role is a separate contract.
		alternateSetID, err := fieldUint(path, line, fields, 3, 8)
		if err != nil {
			return err
		}
		if uiSetID == 0 || alternateSetID == 0 {
			return fmt.Errorf("gacha: %s:%d maps NPC %d to a zero set", path, line, npcRef)
		}
		if previous := c.uiSetIDByNpc[uint32(npcRef)]; previous != 0 {
			return fmt.Errorf("gacha: %s:%d duplicates NPC %d (UI set %d)", path, line, npcRef, previous)
		}
		c.uiSetIDByNpc[uint32(npcRef)] = uint8(uiSetID)
		c.alternateSetIDByNpc[uint32(npcRef)] = uint8(alternateSetID)
		return nil
	})
}

// PrizeForNpc validates that entryID belongs to the exact set displayed by
// the v1.150 client for the interacted NPC RefObjID.
func (c *Catalog) PrizeForNpc(npcRefObjID, entryID uint32) (Prize, bool) {
	if c == nil {
		return Prize{}, false
	}
	prize, ok := c.prizesByEntry[entryID]
	if !ok {
		return Prize{}, false
	}
	uiSetID := c.uiSetIDByNpc[npcRefObjID]
	if uiSetID == 0 {
		return Prize{}, false
	}
	return prize, prize.SetID == uiSetID
}

func scanRows(path string, visit func(line int, fields []string) error) error {
	rows := enterworld.ReadTextdataFile(path)
	if rows == nil {
		return fmt.Errorf("gacha: read %s: file missing or unreadable", path)
	}
	for idx, row := range rows {
		line := idx + 1
		text := strings.TrimSpace(strings.Join(row, "\t"))
		if text == "" || strings.HasPrefix(text, "//") || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if err := visit(line, fields); err != nil {
			return err
		}
	}
	return nil
}

func fieldUint(path string, line int, fields []string, column, bits int) (uint64, error) {
	value, err := strconv.ParseUint(fields[column], 10, bits)
	if err != nil {
		return 0, fmt.Errorf(
			"gacha: %s:%d column %d value %q: %w",
			path, line, column+1, fields[column], err,
		)
	}
	return value, nil
}

// RewardCodenames admits the complete reward reference closure to world data.
// Return detached storage so callers cannot mutate catalog ownership.
func (c *Catalog) RewardCodenames() []string {
	if c == nil {
		return nil
	}
	return append([]string(nil), c.rewardCodenames...)
}

// WastePrizeForNpc uses the native inclusive cumulative-key lookup
// (4CC620..4CC623). It is deliberately distinct from the strict win test.
func (c *Catalog) WastePrizeForNpc(npc uint32, sample uint32) (Prize, bool) {
	if c == nil || sample >= 10000 {
		return Prize{}, false
	}
	for _, band := range c.wastePools[c.alternateSetIDByNpc[npc]] {
		if sample <= band.upper {
			return band.prize, true
		}
	}
	return Prize{}, false
}

// IsResultCard reports whether the reference is a win or loss voucher, the two
// rows the roll writes and the operator exchanges.
func (c *Catalog) IsResultCard(refObjID uint32) bool {
	return refObjID == c.WinCard.RefObjID || refObjID == c.LoseCard.RefObjID
}

// IsPrizePair reports whether a reward reference and quantity are an enabled
// prize row. The roll writes only such pairs onto a voucher, so the exchange
// refuses any other pair rather than mint whatever a row happens to carry.
func (c *Catalog) IsPrizePair(rewardRefObjID uint32, quantity uint64) bool {
	if c == nil {
		return false
	}
	for _, prize := range c.prizesByEntry {
		if prize.RewardRefObjID == rewardRefObjID && uint64(prize.Quantity) == quantity {
			return true
		}
	}
	return false
}
