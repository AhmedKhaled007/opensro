/*
===========================================================================
cap110.go - optional Alexandria field nests from the verified projection.

Port-only, not native. SRO_CAP110_MONSTERS is off by default. The compiler
joins database identities by codename; runtime resolves the installed refs.
===========================================================================
*/
package monster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"opensro.online/server/internal/game/world/alexandria"
)

const EnvCap110Monsters = "SRO_CAP110_MONSTERS"
const cap110PopulationFile = "cap110-monsters.txt"
const cap110PopulationVersion = 1

/*
================
Cap110Enabled
================
*/
func Cap110Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvCap110Monsters))) {
	case "1", "on", "true":
		return true
	}
	return false
}

/*
================
cap110Nest
================
*/
type cap110Nest struct {
	Codename                                         string
	NestID                                           uint32
	RegionID                                         uint16
	X, Y, Z, Radius, GenerateRadius                  float64
	MaxCount, RespawnDelayMinSec, RespawnDelayMaxSec int
	Respawn                                          bool
	InitialDir                                       uint16
	ChampionGenPercentage                            int
	Controls                                         TacticsControls
	Champion                                         *TacticsControls
}

/*
================
AppendCap110

Off never reads the optional file. On fails before startup when its data is
absent or malformed; a partial field population must not reach gameplay.
The nests stand in Alexandria's regions, which the server hides unless
SRO_ALEXANDRIA is on (world/alexandria), so on also requires that switch.
================
*/
func AppendCap110(template Template, textdataDir string, enabled bool) (Template, error) {
	if !enabled {
		return template, nil
	}
	if city, err := alexandria.EnabledFromEnv(); err != nil || !city {
		return Template{}, fmt.Errorf("%s needs %s on: its nests stand in the Alexandria regions", EnvCap110Monsters, alexandria.EnvAlexandria)
	}
	data, err := os.ReadFile(filepath.Join(textdataDir, cap110PopulationFile))
	if err != nil {
		return Template{}, fmt.Errorf("cap110 population: %w", err)
	}
	return appendCap110Data(template, data)
}

/*
================
appendCap110Data
================
*/
func appendCap110Data(template Template, data []byte) (Template, error) {
	var source struct {
		Version int
		Rows    []cap110Nest
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&source); err != nil {
		return Template{}, fmt.Errorf("cap110 population: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Template{}, fmt.Errorf("cap110 population has trailing data")
	}
	if source.Version != cap110PopulationVersion || len(source.Rows) == 0 {
		return Template{}, fmt.Errorf("cap110 population has invalid version or no nests")
	}
	refs := make(map[string]MonsterRef, len(template.Refs))
	for _, ref := range template.Refs {
		refs[ref.Codename] = ref
	}
	nests := append([]NestRow(nil), template.Nests...)
	seen := make(map[uint32]bool, len(source.Rows))
	for _, row := range source.Rows {
		ref, ok := refs[row.Codename]
		// Port-only, not native: Apis retains source grade 8; other later
		// special monsters remain outside the optional outdoor population.
		const apisSourceGrade = 8
		apis := row.Codename == "MOB_SD_APIS" && ref.MonsterType == apisSourceGrade
		if !ok || !strings.HasPrefix(row.Codename, "MOB_SD_") || ref.Level < 100 || ref.Level > 110 || (ref.MonsterType != 0 && !apis) {
			return Template{}, fmt.Errorf("cap110 population has invalid field reference %q", row.Codename)
		}
		if row.NestID == 0 || seen[row.NestID] || row.RegionID&0x8000 != 0 || row.MaxCount < 0 || row.MaxCount > math.MaxUint16 ||
			row.RespawnDelayMinSec < 0 || row.RespawnDelayMaxSec < row.RespawnDelayMinSec ||
			row.ChampionGenPercentage < 0 || row.ChampionGenPercentage > 100 || row.Controls.ChangeTarget > 2 {
			return Template{}, fmt.Errorf("cap110 population has invalid nest %d", row.NestID)
		}
		for _, value := range []float64{row.X, row.Y, row.Z, row.Radius, row.GenerateRadius} {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return Template{}, fmt.Errorf("cap110 population has nonfinite nest %d", row.NestID)
			}
		}
		if row.Radius < 0 || row.GenerateRadius < 0 || row.Controls.SightRange < 0 ||
			(row.Champion != nil) != (row.Controls.ChampionID != 0) {
			return Template{}, fmt.Errorf("cap110 population has inconsistent tactics %d", row.NestID)
		}
		seen[row.NestID] = true
		nest := NestRow{
			WorldCode: "INS_DEFAULT", PolicyPinned: true,
			SpawnPoint: SpawnPoint{RefObjID: ref.RefObjID, RegionID: row.RegionID, X: row.X, Y: row.Y, Z: row.Z},
			Radius:     row.Radius, GenerateRadius: row.GenerateRadius, MaxCount: row.MaxCount,
			RespawnDelayMinSec: row.RespawnDelayMinSec, RespawnDelayMaxSec: row.RespawnDelayMaxSec,
			Respawn: row.Respawn, InitialDir: row.InitialDir, ChampionGenPercentage: row.ChampionGenPercentage,
			Controls: row.Controls, HasControls: true, Aggressive: row.Controls.AggressType == 0,
			SightRange: float64(row.Controls.SightRange), TargetPolicy: row.Controls.ChangeTarget,
			NativeTacticsFlags: row.Controls.Flags,
		}
		if row.Champion != nil {
			c := *row.Champion
			nest.HasChampionTactics = true
			nest.ChampionTactics = ChampionTactics{Controls: c, HasControls: true, Aggressive: c.AggressType == 0,
				SightRange: float64(c.SightRange), TargetPolicy: c.ChangeTarget, NativeTacticsFlags: c.Flags}
		}
		nests = append(nests, nest)
	}
	result := TemplateFromParts(template.Refs, nests)
	result.SummonRefs = append([]uint32(nil), template.SummonRefs...)
	result.EvidenceMatches = template.EvidenceMatches
	return result, nil
}
