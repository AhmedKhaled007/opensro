/*
===========================================================================

trainingcamps.go - the training-camp door: opening, admission and notice

The authority store's camp aggregate. OpenCamp is the only command that
creates a camp (the NPC's 0x72EC); AdmitStudent appends to one, and
UpdateNotice edits its text. Each checks and commits under the store lock.

===========================================================================
*/
package store

// The training-camp plane's door (domain.TrainingCampStore over the
// training_camps and training_camp_members tables): the guilds-door
// pattern with the same two-part dirty unit - one campKey marks the camp
// row AND its whole member set, and the commit rewrites both with a
// whole-set replace (replaceCampTx).

import (
	"fmt"

	"opensro.online/server/internal/domain"
	"opensro.online/server/internal/domain/charactervitals"
)

// TrainingCamps returns the training-camp door (domain.TrainingCampStore
// over the training_camps / training_camp_members tables). Same lifetime
// contract as Characters().
func (s *Store) TrainingCamps() domain.TrainingCampStore {
	return storeCampDoor{s: s}
}

type storeCampDoor struct{ s *Store }

// Camp returns copies of the camp row and its member list in wire order;
// ok=false when no such camp is stored.
func (door storeCampDoor) Camp(divisionID string, campID int64) (domain.TrainingCampRecord, []domain.TrainingCampMemberRecord, bool) {
	door.s.mu.RLock()
	defer door.s.mu.RUnlock()
	camp, ok := door.s.camps[divisionID][campID]
	if !ok {
		return domain.TrainingCampRecord{}, nil, false
	}
	live := door.s.campMembers[divisionID][campID]
	members := make([]domain.TrainingCampMemberRecord, len(live))
	copy(members, live)
	return camp, members, true
}

// CampOfCharacter answers the camp a character is a MEMBER of (the master
// is a kind-0 member row, so masters resolve here too), resolved from the
// stored member sets (the GuildOfCharacter shape - no Character FK).
func (door storeCampDoor) CampOfCharacter(divisionID string, characterID int64) (int64, bool) {
	door.s.mu.RLock()
	defer door.s.mu.RUnlock()
	for campID, members := range door.s.campMembers[divisionID] {
		for _, member := range members {
			if member.CharID == characterID {
				return campID, true
			}
		}
	}
	return 0, false
}

// AdmitStudent is the camp aggregate's only topology command. The gameplay
// layer supplies version-specific level/capacity policy, and this door applies
// it together with relationship and lifecycle invariants under one lock.
func (door storeCampDoor) AdmitStudent(
	divisionID string,
	admission domain.TrainingCampAdmission,
) (domain.TrainingCampAdmissionResult, bool) {
	var refused domain.TrainingCampAdmissionResult
	s := door.s
	s.mu.Lock()
	defer s.mu.Unlock()

	if admission.MasterCharID < 1 ||
		admission.StudentCharID < 1 ||
		admission.MasterCharID == admission.StudentCharID ||
		admission.MasterMinLevel < 1 ||
		admission.StudentMaxLevel < 1 ||
		admission.StudentLimit < 1 ||
		admission.StudentLimit > 7 {
		return refused, false
	}
	master := s.characterByIDLocked(divisionID, admission.MasterCharID)
	student := s.characterByIDLocked(divisionID, admission.StudentCharID)
	if master == nil || student == nil || master.DeletePending || student.DeletePending {
		return refused, false
	}
	if resolvedCharacterLevel(master) < admission.MasterMinLevel ||
		resolvedCharacterLevel(student) > admission.StudentMaxLevel {
		return refused, false
	}
	if _, joined := door.campOfCharacterLocked(divisionID, student.ID); joined {
		return refused, false
	}

	campID, masterJoined := door.campOfCharacterLocked(divisionID, master.ID)
	if !masterJoined || admission.ExpectedCampID < 1 || campID != admission.ExpectedCampID {
		return refused, false
	}
	camp, exists := s.camps[divisionID][campID]
	if !exists || camp.MasterCharID != master.ID {
		return refused, false
	}
	members := s.campMembers[divisionID][campID]

	studentCount := 0
	for _, member := range members {
		if member.Kind == 2 {
			studentCount++
		}
	}
	if studentCount >= admission.StudentLimit || len(members) >= 8 {
		return refused, false
	}

	next := make([]domain.TrainingCampMemberRecord, len(members), len(members)+1)
	copy(next, members)
	next = append(next, domain.TrainingCampMemberRecord{CharID: student.ID, Kind: 2})
	s.campMembers[divisionID][campID] = next
	s.changes.camps[campKey{division: divisionID, campID: campID}] = true
	s.commitLocked(fmt.Sprintf("training-camp-admit-student %s/%d", divisionID, campID))

	resultMembers := make([]domain.TrainingCampMemberRecord, len(next))
	copy(resultMembers, next)
	return domain.TrainingCampAdmissionResult{
		Camp:    camp,
		Members: resultMembers,
	}, true
}

/*
================
OpenCamp

The NPC's camp opening (v1.150 0x72EC; v1.188 4E4680 checks, then 5DFCC0
burns the fee through 4E4650 and answers). 4E4680's order: alive (0x6807),
level (0x6809), gold (0x680A), already in a camp (0x6812). Its first test
(0x6808, a state flag on the requester) has no v1.150 counterpart: the
category-0x1D table names no code 8, so it is left out.

The camp id is the master's character id (TrainingCampRecord.ID), so an
existing row under that id - a camp the master left as a sub-mentor - still
refuses as in a camp. v1.188's seven-day reopen wait (0x681C) reads the
disband time a disband writes; the port has no disband yet, so no such
time exists to read.
================
*/
func (door storeCampDoor) OpenCamp(divisionID string, opening domain.TrainingCampOpening) (domain.TrainingCampOpenResult, domain.TrainingCampOpenRefusal) {
	var refused domain.TrainingCampOpenResult
	s := door.s
	s.mu.Lock()
	defer s.mu.Unlock()

	if opening.MasterCharID < 1 || opening.MinLevel < 1 || opening.Fee < 0 {
		return refused, domain.TrainingCampOpenUnavailable
	}
	master := s.characterByIDLocked(divisionID, opening.MasterCharID)
	if master == nil || master.DeletePending {
		return refused, domain.TrainingCampOpenUnavailable
	}
	if charactervitals.CurrentHP(master) == 0 {
		return refused, domain.TrainingCampOpenDead
	}
	if resolvedCharacterLevel(master) < opening.MinLevel {
		return refused, domain.TrainingCampOpenLevel
	}
	if ledgerGold(master) < opening.Fee {
		return refused, domain.TrainingCampOpenGold
	}
	if _, joined := door.campOfCharacterLocked(divisionID, master.ID); joined {
		return refused, domain.TrainingCampOpenInCamp
	}
	if _, exists := s.camps[divisionID][master.ID]; exists {
		return refused, domain.TrainingCampOpenInCamp
	}

	gold := ledgerGold(master) - opening.Fee
	master.Gold = &gold
	camp := domain.TrainingCampRecord{ID: master.ID, MasterCharID: master.ID}
	members := []domain.TrainingCampMemberRecord{{CharID: master.ID, Kind: 0}}
	if s.camps[divisionID] == nil {
		s.camps[divisionID] = map[int64]domain.TrainingCampRecord{}
	}
	if s.campMembers[divisionID] == nil {
		s.campMembers[divisionID] = map[int64][]domain.TrainingCampMemberRecord{}
	}
	s.camps[divisionID][camp.ID] = camp
	s.campMembers[divisionID][camp.ID] = members
	s.changes.camps[campKey{division: divisionID, campID: camp.ID}] = true
	s.changes.characters[master] = true
	s.commitLocked(fmt.Sprintf("training-camp-open %s/%d", divisionID, camp.ID))
	return domain.TrainingCampOpenResult{
		Camp:    camp,
		Members: append([]domain.TrainingCampMemberRecord(nil), members...),
		Gold:    gold,
	}, domain.TrainingCampOpenAccepted
}

func resolvedCharacterLevel(character *domain.Character) int64 {
	if character == nil || character.Level == nil || *character.Level < 1 {
		return 1
	}
	return *character.Level
}

// Native 5E43B0 -> 5E36A0 authorizes the kind-0 camp member. Keep the
// permission and expected aggregate checks in the same critical section as
// the update so a stale gameplay precheck cannot edit another camp.
func (door storeCampDoor) UpdateNotice(divisionID string, authorID, expectedCampID int64, subject, contents string) (domain.TrainingCampRecord, []domain.TrainingCampMemberRecord, bool) {
	s := door.s
	s.mu.Lock()
	defer s.mu.Unlock()
	camp, exists := s.camps[divisionID][expectedCampID]
	author := s.characterByIDLocked(divisionID, authorID)
	if !exists || author == nil || author.DeletePending || camp.MasterCharID != authorID || charactervitals.CurrentHP(author) == 0 {
		return domain.TrainingCampRecord{}, nil, false
	}
	members := s.campMembers[divisionID][expectedCampID]
	authorized := false
	for _, member := range members {
		if member.CharID == authorID && member.Kind == 0 {
			authorized = true
			break
		}
	}
	if !authorized {
		return domain.TrainingCampRecord{}, nil, false
	}
	// Wire-encoding and version-specific byte limits belong to the caller;
	// this command stores logical text and owns the authorization transaction.
	camp.Subject, camp.Contents = subject, contents
	s.camps[divisionID][expectedCampID] = camp
	s.changes.camps[campKey{division: divisionID, campID: expectedCampID}] = true
	s.commitLocked(fmt.Sprintf("training-camp-update-notice %s/%d", divisionID, expectedCampID))
	return camp, append([]domain.TrainingCampMemberRecord(nil), members...), true
}

// campOfCharacterLocked is CampOfCharacter's body for callers already
// holding s.mu.
func (door storeCampDoor) campOfCharacterLocked(divisionID string, characterID int64) (int64, bool) {
	for campID, members := range door.s.campMembers[divisionID] {
		for _, member := range members {
			if member.CharID == characterID {
				return campID, true
			}
		}
	}
	return 0, false
}
