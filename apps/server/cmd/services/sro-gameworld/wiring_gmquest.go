/*
===========================================================================

wiring_gmquest.go - the GM quest tools' composition root

Connects the quest runtime's /INITQ, /REMOVEQ, /COMPQ and /RESETQ (quest/gm.go)
to the action owner's hooks (action/gmquest.go). The action package never
imports the quest package, so the results are copied across here.

===========================================================================
*/
package main

import (
	"opensro.online/server/internal/game/action"
	"opensro.online/server/internal/game/enterworld"
	"opensro.online/server/internal/game/quest"
)

/*
================
gmQuestHooks
================
*/
func gmQuestHooks(quests *quest.Runtime) action.GMQuestHooks {
	named := func(tool func(*enterworld.Character, string) (quest.OpResult, error)) func(*enterworld.Character, string) (action.GMQuestResult, error) {
		return func(character *enterworld.Character, codename string) (action.GMQuestResult, error) {
			result, err := tool(character, codename)
			return action.GMQuestResult{Frames: result.Frames, Broadcast: result.Broadcast}, err
		}
	}
	return action.GMQuestHooks{
		Init:     named(quests.GMInitQuest),
		Remove:   named(quests.GMRemoveQuest),
		Complete: named(quests.GMCompleteQuest),
		Reset: func(character *enterworld.Character) (action.GMQuestResult, error) {
			result, err := quests.GMResetQuests(character)
			return action.GMQuestResult{Frames: result.Frames, Broadcast: result.Broadcast}, err
		},
	}
}
