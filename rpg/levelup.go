package rpg

import (
	"fmt"
	"strings"

	lang "github.com/nao1215/gozen/text"
)

// LevelUpLine formats a frontend level-up notice; three or more simultaneous
// increases use a collective name when all living members are included.
func LevelUpLine(names []string, alive int, term string, lv int) string {
	who := strings.Join(names, lang.T("、"))
	if len(names) >= 3 && len(names) == alive {
		who = lang.T("みんな")
	}
	return fmt.Sprintf(lang.T("%sは %s%dに あがった！"), who, term, lv)
}
