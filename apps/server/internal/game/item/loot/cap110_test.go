/*
===========================================================================
cap110_test.go - optional drop compilation and startup refusal boundaries.
===========================================================================
*/
package loot

import (
	"encoding/json"
	"testing"
)

/*
================
TestCap110EquipmentUsesInstalledExpansionBucket
================
*/
func TestCap110EquipmentUsesInstalledExpansionBucket(t *testing.T) {
	var source cap110Catalog
	source.Version = 1
	if err := json.Unmarshal(equipmentJSON, &source.Equipment); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(propertiesJSON, &source.Properties); err != nil {
		t.Fatal(err)
	}
	const code = "ITEM_CH_SWORD_11_A"
	source.Equipment.Items = append(source.Equipment.Items, equipmentRef{Codename: code, Group: 30, Type: "3:1:6:2", Weight: 100, Absolute: 100, Level: 101})
	source.Properties.Items[code] = propertyItem{Codename: code, Type: [4]uint8{3, 1, 6, 2}, Class: 31, MaxMagic: 9}
	source.Equipment.Normal[109] = make([]float32, source.Equipment.Widths["normal"])
	source.Equipment.Normal[109][30] = 1
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	next, props, err := compileCap110(data)
	if err != nil {
		t.Fatal(err)
	}
	ref, ok := next.selectEquipment(0, 30, false, 110, func() (uint32, error) { return 0, nil })
	if !ok || ref.Codename != code || props.Items[code].Class != 31 {
		t.Fatal("expansion bucket or properties lost")
	}
	if err := InitCap110(t.TempDir(), false); err != nil {
		t.Fatalf("native off read optional file: %v", err)
	}
	if err := InitCap110(t.TempDir(), true); err == nil {
		t.Fatal("on admitted absent drop input")
	}
}

/*
================
TestCap110EquipmentRejectsMalformedData
================
*/
func TestCap110EquipmentRejectsMalformedData(t *testing.T) {
	for _, data := range []string{`{"Version":2}`, `{"Version":1,"Unknown":1}`, `{}`} {
		if _, _, err := compileCap110([]byte(data)); err == nil {
			t.Fatal("malformed optional loot admitted")
		}
	}
}

/*
================
TestCap110ExpansionBucketsStayAwayFromNativeLevels

Degree-10 items of required level 90 sit in a bucket the native catalog leaves
empty. A level-90 monster must still step down as natively; a level-91 monster
may pick the item.
================
*/
func TestCap110ExpansionBucketsStayAwayFromNativeLevels(t *testing.T) {
	var source cap110Catalog
	source.Version = 1
	if err := json.Unmarshal(equipmentJSON, &source.Equipment); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(propertiesJSON, &source.Properties); err != nil {
		t.Fatal(err)
	}
	const code = "ITEM_CH_SWORD_10_A"
	source.Equipment.Items = append(source.Equipment.Items, equipmentRef{Codename: code, Group: 30, Type: "3:1:6:2", Weight: 100, Absolute: 100, Level: 90})
	source.Properties.Items[code] = propertyItem{Codename: code, Type: [4]uint8{3, 1, 6, 2}, Class: 31, MaxMagic: 9}
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	next, _, err := compileCap110(data)
	if err != nil {
		t.Fatal(err)
	}
	markExpansionBuckets(next, equipment)
	roll := func() (uint32, error) { return 0, nil }
	if ref, ok := next.selectEquipment(0, 30, false, 91, roll); !ok || ref.Codename != code {
		t.Fatalf("a level-91 monster lost the expansion item: %+v %v", ref, ok)
	}
	// Level 90 steps down to the native degree-9 class, as the native catalog does.
	if ref, ok := next.selectEquipment(0, 30, false, 90, roll); !ok || ref.Codename == code || ref.Group >= 27 {
		t.Fatalf("a level-90 monster did not step down to a native item: %+v %v", ref, ok)
	}
	for key, bucket := range next.buckets {
		if equipment.buckets[key] != nil && bucket.expansionOnly {
			t.Fatalf("native-filled bucket %+v was marked expansion-only", key)
		}
	}
}
