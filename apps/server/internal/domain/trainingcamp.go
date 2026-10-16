/*
===========================================================================

trainingcamp.go - training-camp records and the store door contract

The camp row, its member rows, and the policy inputs and results of the
store door's opening and admission commands.

===========================================================================
*/
package domain

// Training-camp (mentor/academy) records + the store door contract.
//
// A training camp is a cross-character entity like a guild, so it persists
// in the authority store's training_camps / training_camp_members tables
// (the guilds/guild_members pattern), reached through the TrainingCampStore
// door below. EVIDENCE(v1.188, logic): the retail GameServer persists camps
// in the _TRAININGCAMP / _TRAININGCAMPMEMBER DB tables (SR_GameServer dump
// @0x00848bd1 / @0x008495f1 CDBRecord table binds) with honor-rank and
// buff-status satellites (@0x0084a201 / @0x0084a761) - the satellites are
// deliberately NOT modeled here (honor/buff mechanics are outside the
// invite lane).
//
// Deliberately NO Character FK (unlike GuildID): the camp seed resolves
// through CampOfCharacter at enter-world, so a second character-schema key
// buys nothing - a DECISION documented in internal/game/social/mentor.
type TrainingCampRecord struct {
	// ID is the camp id. DECISION: allocated as the master's character
	// id (one camp per master; no wire consumer ever sees a camp id -
	// the v1.150 client's 0x3AC5 frames carry member ids only - so a
	// separate id space would be invention).
	ID int64 `json:"id"`
	// MasterCharID is the founding master's character id (the kind-0
	// member row's CharID; kept on the row too so the camp resolves
	// masterless-ness without scanning members).
	MasterCharID int64  `json:"masterCharId"`
	Subject      string `json:"subject,omitempty"`
	Contents     string `json:"contents,omitempty"`
}

// TrainingCampMemberRecord is one persisted camp member. The wire member
// id is NOT stored - it derives from ObjectIDForCharacter at encode time
// (the party MemberID posture), and level/name project from the live
// character record so they can never rot.
type TrainingCampMemberRecord struct {
	CharID int64 `json:"charId"`
	// Kind is the client record's +0x44 byte: 0 master, 1 sub-mentor
	// (assistant guardian), 2 student/apprentice - the sub_81e9f0 grade
	// classifier's input (@0x0081ea08..0x0081ea71).
	Kind uint8 `json:"kind"`
}

// TrainingCampAdmission is the complete policy input for admitting one
// student. ExpectedCampID names the camp the master opened at the NPC
// (OpenCamp); the master must still own that exact camp. An admission never
// creates a camp: v1.150 opens one only through the NPC's 0x72EC.
//
// The level and capacity values are supplied by the gameplay package because
// they are protocol/version policy. The authority store applies them while it
// owns the character and camp records, eliminating a check-then-commit race.
type TrainingCampAdmission struct {
	MasterCharID    int64
	StudentCharID   int64
	ExpectedCampID  int64
	MasterMinLevel  int64
	StudentMaxLevel int64
	StudentLimit    int
}

// TrainingCampAdmissionResult is the committed aggregate snapshot returned
// by AdmitStudent. Members are in persisted wire order.
type TrainingCampAdmissionResult struct {
	Camp    TrainingCampRecord
	Members []TrainingCampMemberRecord
}

// TrainingCampOpening is the policy input for opening a camp at the NPC
// (v1.188 4E4680 -> 5DFCC0): the master's level floor and the gold fee the
// opening burns. The gameplay package supplies both; the store applies them
// with the debit and the new rows under one lock.
type TrainingCampOpening struct {
	MasterCharID int64
	MinLevel     int64
	Fee          int64
}

// TrainingCampOpenRefusal is why OpenCamp refused. The gameplay package maps
// each to the v1.150 category-0x1D code the 0xB2EC answer carries.
type TrainingCampOpenRefusal uint8

const (
	TrainingCampOpenAccepted TrainingCampOpenRefusal = iota
	// TrainingCampOpenUnavailable: no such live character, or one pending
	// deletion. No answer names it.
	TrainingCampOpenUnavailable
	// TrainingCampOpenDead: 4E4680's IsAlive test (0x6807).
	TrainingCampOpenDead
	// TrainingCampOpenLevel: below the level floor (0x6809).
	TrainingCampOpenLevel
	// TrainingCampOpenGold: less gold than the fee (0x680A).
	TrainingCampOpenGold
	// TrainingCampOpenInCamp: already a member of a camp (0x6812).
	TrainingCampOpenInCamp
)

// TrainingCampOpenResult is the committed opening: the new camp, its one
// master row, and the master's gold after the fee.
type TrainingCampOpenResult struct {
	Camp    TrainingCampRecord
	Members []TrainingCampMemberRecord
	Gold    int64
}

// TrainingCampStore is the authority store's training-camp door (the
// GuildStore twin over training_camps / training_camp_members). The dirty
// unit is the WHOLE camp - the row plus its member set - committed as one
// whole-set replace, and the member SLICE ORDER is the wire order the
// 0x3AC5 status-10 sub-1 roster loop emits.
type TrainingCampStore interface {
	// Camp returns copies of the camp row and its member list in wire
	// order; ok=false when no such camp is stored.
	Camp(divisionID string, campID int64) (TrainingCampRecord, []TrainingCampMemberRecord, bool)
	// CampOfCharacter answers the camp a character is a MEMBER of
	// (master included - the master is a kind-0 member row), resolved
	// from the stored member sets.
	CampOfCharacter(divisionID string, characterID int64) (int64, bool)
	// OpenCamp opens the master's camp at the NPC: it checks the live
	// character, level and gold and that no camp holds the master, then
	// burns the fee and installs the camp row and its kind-0 master row in
	// one transaction.
	OpenCamp(divisionID string, opening TrainingCampOpening) (TrainingCampOpenResult, TrainingCampOpenRefusal)
	// AdmitStudent appends one student to the master's existing camp. It
	// validates the expected camp, master ownership, both live character
	// records, lifecycle and level policy, student uniqueness and capacity
	// under one authority lock, then commits in one transaction.
	AdmitStudent(divisionID string, admission TrainingCampAdmission) (TrainingCampAdmissionResult, bool)
	// UpdateNotice rechecks the expected camp and kind-0 author under the
	// authority lock, persists the notice, and returns a detached snapshot.
	UpdateNotice(divisionID string, authorID, expectedCampID int64, subject, contents string) (TrainingCampRecord, []TrainingCampMemberRecord, bool)
}
