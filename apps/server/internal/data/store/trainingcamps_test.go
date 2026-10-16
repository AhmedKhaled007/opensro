/*
===========================================================================

trainingcamps_test.go - the training-camp door: opening, admission,
notice authority and persistence

===========================================================================
*/
package store

import (
	"reflect"
	"testing"
	"time"

	"opensro.online/server/internal/domain"
)

/*
================
openTestCamp

The master's camp opened at the NPC with no fee and the lowest level floor,
so a test reaches admission without staging gold or levels.
================
*/
func openTestCamp(t *testing.T, s *Store, master *domain.Character) domain.TrainingCampRecord {
	t.Helper()
	opened, refusal := s.TrainingCamps().OpenCamp(testDivision, domain.TrainingCampOpening{MasterCharID: master.ID, MinLevel: 1})
	if refusal != domain.TrainingCampOpenAccepted {
		t.Fatalf("open camp refused: %d", refusal)
	}
	return opened.Camp
}

/*
================
TestTrainingCampNoticeAuthorityAndPersistence
================
*/
func TestTrainingCampNoticeAuthorityAndPersistence(t *testing.T) {
	t.Parallel()
	dir, clock := t.TempDir(), newTestClock()
	s := openTest(t, dir, clock)
	master, student, outsider := guildTestCharacter("noticemaster"), guildTestCharacter("notestudent"), guildTestCharacter("noteoutsider")
	for _, c := range []*domain.Character{master, student, outsider} {
		if err := s.CreateCharacter(testDivision, "test-account", c); err != nil {
			t.Fatal(err)
		}
	}
	opened := openTestCamp(t, s, master)
	a, ok := s.TrainingCamps().AdmitStudent(testDivision, domain.TrainingCampAdmission{MasterCharID: master.ID, StudentCharID: student.ID, ExpectedCampID: opened.ID, MasterMinLevel: 1, StudentMaxLevel: 140, StudentLimit: 5})
	if !ok {
		t.Fatal("admission failed")
	}
	for _, request := range []struct {
		division     string
		author, camp int64
	}{{testDivision, student.ID, a.Camp.ID}, {testDivision, outsider.ID, a.Camp.ID}, {testDivision, master.ID, a.Camp.ID + 1}, {"other", master.ID, a.Camp.ID}} {
		if _, _, ok := s.TrainingCamps().UpdateNotice(request.division, request.author, request.camp, "unauthorized", "text"); ok {
			t.Fatalf("accepted %+v", request)
		}
	}
	unchanged, _, _ := s.TrainingCamps().Camp(testDivision, a.Camp.ID)
	if unchanged != a.Camp {
		t.Fatal("refusals changed camp")
	}
	camp, members, ok := s.TrainingCamps().UpdateNotice(testDivision, master.ID, a.Camp.ID, "Welcome", "Meet at the gate.\nRésumé")
	if !ok || camp.Subject != "Welcome" || camp.Contents != "Meet at the gate.\nRésumé" || !reflect.DeepEqual(members, a.Members) {
		t.Fatalf("update: %+v %+v %v", camp, members, ok)
	}
	members[0].Kind = 2
	_, live, _ := s.TrainingCamps().Camp(testDivision, a.Camp.ID)
	if !reflect.DeepEqual(live, a.Members) {
		t.Fatal("returned members alias authority")
	}
	s.MutateCharacter(master, "test-delete-pending", func() { master.DeletePending = true })
	if _, _, ok := s.TrainingCamps().UpdateNotice(testDivision, master.ID, a.Camp.ID, "bad", "bad"); ok {
		t.Fatal("delete-pending author accepted")
	}
	s.MutateCharacter(master, "test-restore", func() { master.DeletePending = false })
	s.MutateCharacter(master, "test-notice-death-race", func() { hp := int64(0); master.CurrentHP = &hp })
	if _, _, ok := s.TrainingCamps().UpdateNotice(testDivision, master.ID, a.Camp.ID, "bad", "bad"); ok {
		t.Fatal("dead author committed a notice")
	}
	s.MutateCharacter(master, "test-notice-revive", func() { master.CurrentHP = nil })
	s.Close()
	reopened := openTest(t, dir, clock)
	stored, storedMembers, ok := reopened.TrainingCamps().Camp(testDivision, a.Camp.ID)
	if !ok || stored != camp || !reflect.DeepEqual(storedMembers, a.Members) {
		t.Fatalf("reopened: %+v %+v %v", stored, storedMembers, ok)
	}
}

/*
================
TestTrainingCampDeletionInvariant
================
*/
func TestTrainingCampDeletionInvariant(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	clock := newTestClock()
	s := openTest(t, dir, clock)

	master := guildTestCharacter("campmaster")
	student := guildTestCharacter("campstudent")
	outsider := guildTestCharacter("campoutside")
	for _, c := range []*domain.Character{master, student, outsider} {
		if err := s.CreateCharacter(testDivision, "test-account", c); err != nil {
			t.Fatal(err)
		}
	}
	opened := openTestCamp(t, s, master)
	admission, ok := s.TrainingCamps().AdmitStudent(testDivision, domain.TrainingCampAdmission{
		MasterCharID:    master.ID,
		StudentCharID:   student.ID,
		ExpectedCampID:  opened.ID,
		MasterMinLevel:  1,
		StudentMaxLevel: 140,
		StudentLimit:    5,
	})
	if !ok {
		t.Fatal("initial student admission refused")
	}
	camp := admission.Camp

	stamp := clock.Now().UTC().Format(time.RFC3339)
	if s.ReserveCharacterDeletion(student, stamp) {
		t.Fatal("training-camp member entered delete-pending state")
	}
	if student.DeletePending {
		t.Fatal("refused reservation mutated the student")
	}
	if !s.ReserveCharacterDeletion(outsider, stamp) {
		t.Fatal("guildless/campless character reservation refused")
	}
	if _, admitted := s.TrainingCamps().AdmitStudent(testDivision, domain.TrainingCampAdmission{
		MasterCharID:    master.ID,
		StudentCharID:   outsider.ID,
		ExpectedCampID:  camp.ID,
		MasterMinLevel:  1,
		StudentMaxLevel: 140,
		StudentLimit:    5,
	}); admitted {
		t.Fatal("delete-pending character joined a training camp")
	}
	s.MutateCharacter(outsider, "delete-restore", func() {
		outsider.DeletePending = false
		outsider.DeleteReservedAt = ""
	})

	// Simulate an impossible pre-invariant record and prove the reaper does
	// not turn it into a dangling MasterCharID.
	s.MutateCharacter(master, "invalid-delete-reserve", func() {
		master.DeletePending = true
		master.DeleteReservedAt = stamp
	})
	clock.Advance(DeleteReservationWindow + time.Hour)
	if reaped := s.ReapMaturedDeletions(); len(reaped) != 0 {
		t.Fatalf("reaped training-camp master(s) %v, want fail-closed none", reaped)
	}
	if gotCamp, gotMembers, ok := s.TrainingCamps().Camp(testDivision, camp.ID); !ok ||
		gotCamp.MasterCharID != master.ID || len(gotMembers) != 2 {
		t.Fatalf("camp after refused reap = %+v members=%+v ok=%v", gotCamp, gotMembers, ok)
	}
	s.MutateCharacter(master, "restore-impossible-reservation", func() {
		master.DeletePending = false
		master.DeleteReservedAt = ""
	})
	s.Close()

	s2 := openTest(t, dir, clock)
	if gotCamp, gotMembers, ok := s2.TrainingCamps().Camp(testDivision, camp.ID); !ok ||
		gotCamp.MasterCharID != master.ID || len(gotMembers) != 2 {
		t.Fatalf("reopened camp = %+v members=%+v ok=%v", gotCamp, gotMembers, ok)
	}
	if got := len(s2.Characters().CharactersForDivision(testDivision)); got != 3 {
		t.Fatalf("reopened live characters = %d, want 3", got)
	}
}

/*
================
TestTrainingCampAdmissionRefusesInvalidAggregateCommands
================
*/
func TestTrainingCampAdmissionRefusesInvalidAggregateCommands(t *testing.T) {
	t.Parallel()
	s := openTest(t, t.TempDir(), newTestClock())
	master := guildTestCharacter("campvalid")
	student := guildTestCharacter("studentvalid")
	for _, character := range []*domain.Character{master, student} {
		if err := s.CreateCharacter(testDivision, "test-account", character); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name      string
		admission domain.TrainingCampAdmission
	}{
		{"unknown student", domain.TrainingCampAdmission{MasterCharID: master.ID, StudentCharID: 999, MasterMinLevel: 1, StudentMaxLevel: 140, StudentLimit: 5}},
		{"self admission", domain.TrainingCampAdmission{MasterCharID: master.ID, StudentCharID: master.ID, MasterMinLevel: 1, StudentMaxLevel: 140, StudentLimit: 5}},
		{"invalid master floor", domain.TrainingCampAdmission{MasterCharID: master.ID, StudentCharID: student.ID, MasterMinLevel: 0, StudentMaxLevel: 140, StudentLimit: 5}},
		{"invalid student ceiling", domain.TrainingCampAdmission{MasterCharID: master.ID, StudentCharID: student.ID, MasterMinLevel: 1, StudentMaxLevel: 0, StudentLimit: 5}},
		{"invalid student limit", domain.TrainingCampAdmission{MasterCharID: master.ID, StudentCharID: student.ID, MasterMinLevel: 1, StudentMaxLevel: 140, StudentLimit: 8}},
		{"stale existing camp", domain.TrainingCampAdmission{MasterCharID: master.ID, StudentCharID: student.ID, ExpectedCampID: master.ID + 99, MasterMinLevel: 1, StudentMaxLevel: 140, StudentLimit: 5}},
		// An admission never creates a camp: only OpenCamp does (0x72EC).
		{"campless master", domain.TrainingCampAdmission{MasterCharID: master.ID, StudentCharID: student.ID, MasterMinLevel: 1, StudentMaxLevel: 140, StudentLimit: 5}},
		{"campless master naming its own id", domain.TrainingCampAdmission{MasterCharID: master.ID, StudentCharID: student.ID, ExpectedCampID: master.ID, MasterMinLevel: 1, StudentMaxLevel: 140, StudentLimit: 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := s.TrainingCamps().AdmitStudent(testDivision, tc.admission); ok {
				t.Fatal("invalid training-camp admission accepted")
			}
		})
	}
}

/*
================
TestTrainingCampOpenChecksThenBurnsTheFee

4E4680's refusals in its order (dead, level, gold, already in a camp) change
nothing; an accepted opening burns the fee, installs the camp and its one
master row, books an academy_fee ledger row, and survives a reopen.
================
*/
func TestTrainingCampOpenChecksThenBurnsTheFee(t *testing.T) {
	t.Parallel()
	dir, clock := t.TempDir(), newTestClock()
	s := openTest(t, dir, clock)
	master, other := guildTestCharacter("openmaster"), guildTestCharacter("openother")
	for _, c := range []*domain.Character{master, other} {
		if err := s.CreateCharacter(testDivision, "test-account", c); err != nil {
			t.Fatal(err)
		}
	}
	const fee, floor = 100000, 60
	opening := domain.TrainingCampOpening{MasterCharID: master.ID, MinLevel: floor, Fee: fee}
	set := func(level, gold, hp int64) {
		s.MutateCharacter(master, "test-open-stage", func() {
			master.Level, master.Gold = &level, &gold
			if hp < 0 {
				master.CurrentHP = nil
			} else {
				master.CurrentHP = &hp
			}
		})
	}
	for _, tc := range []struct {
		name            string
		level, gold, hp int64
		want            domain.TrainingCampOpenRefusal
	}{
		// The dead test comes first, ahead of a level and gold that also fail.
		{"dead", 1, 0, 0, domain.TrainingCampOpenDead},
		{"level", floor - 1, fee, -1, domain.TrainingCampOpenLevel},
		{"gold", floor, fee - 1, -1, domain.TrainingCampOpenGold},
	} {
		set(tc.level, tc.gold, tc.hp)
		if _, refusal := s.TrainingCamps().OpenCamp(testDivision, opening); refusal != tc.want {
			t.Fatalf("%s: refusal %d, want %d", tc.name, refusal, tc.want)
		}
		if _, joined := s.TrainingCamps().CampOfCharacter(testDivision, master.ID); joined {
			t.Fatalf("%s: a refused opening made a camp", tc.name)
		}
		if *master.Gold != tc.gold {
			t.Fatalf("%s: a refused opening moved gold", tc.name)
		}
	}

	set(floor, fee+5, -1)
	opened, refusal := s.TrainingCamps().OpenCamp(testDivision, opening)
	if refusal != domain.TrainingCampOpenAccepted || opened.Gold != 5 || *master.Gold != 5 {
		t.Fatalf("open: %+v refusal %d gold %d", opened, refusal, *master.Gold)
	}
	if opened.Camp.ID != master.ID || opened.Camp.MasterCharID != master.ID ||
		!reflect.DeepEqual(opened.Members, []domain.TrainingCampMemberRecord{{CharID: master.ID, Kind: 0}}) {
		t.Fatalf("opened camp %+v", opened)
	}
	rows := ledgerRows(t, s)
	last := rows[len(rows)-1]
	if last.charID != master.ID || last.reason != "academy_fee" || last.gold != -fee || last.balance != 5 {
		t.Fatalf("ledger row %+v", last)
	}

	set(floor, 10*fee, -1)
	if _, refusal := s.TrainingCamps().OpenCamp(testDivision, opening); refusal != domain.TrainingCampOpenInCamp {
		t.Fatalf("second opening: refusal %d", refusal)
	}
	if *master.Gold != 10*fee {
		t.Fatal("a refused second opening moved gold")
	}

	s.Close()
	reopened := openTest(t, dir, clock)
	camp, members, ok := reopened.TrainingCamps().Camp(testDivision, opened.Camp.ID)
	if !ok || camp != opened.Camp || !reflect.DeepEqual(members, opened.Members) {
		t.Fatalf("reopened camp %+v %+v %v", camp, members, ok)
	}
}
