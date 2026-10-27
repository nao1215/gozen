package diag

// GZN01xx: warnings of check. The game loads and the command exits 0, but
// something is likely not what the author meant.
var (
	TextOverflow = register(101, "TextOverflow", Entry{
		Summary: "a text does not fit the message window",
		Detail:  "The layout check lays out every line of dialogue and every choice with the game's font and display rules. This one needs more lines than the window shows, so the player would see it cut or paged where the author did not plan it.",
		Fix:     "Shorten the text, split it into two lines of dialogue, or change the window in game.yaml.",
	})
	FaceBox = register(102, "FaceBox", Entry{
		Summary: "a balloon face has no face box, or the box is stale",
		Detail:  "A painted scene cuts each person's balloon face around the face that gozen faces found and wrote to assets/faces.json. The file is missing, has no box for a picture, has none found, or the picture changed since, so the face is guessed.",
		Fix:     "Run gozen faces <game-dir> again, or give the doll face_at.",
	})
	Unreachable = register(103, "Unreachable", Entry{
		Summary: "a place of a map or a painted scene cannot be reached",
		Detail:  "A point no road leads to, a point without a way back, an exit or a thing that does not touch the walkable area, or a spot without do (which is no entry) cannot be used by the player.",
		Fix:     "Connect it (a road, an exit back, walk that touches it) or remove it; give a spot a do to make it an entry.",
	})
	DrawnDifferently = register(104, "DrawnDifferently", Entry{
		Summary: "a picture or a shape will be drawn otherwise than written",
		Detail:  "A polygon that crosses itself, a person without an image (its doll takes plain colors), or an icon that is not square (it is cut round from its middle) is drawn, but probably not as meant.",
		Fix:     "Fix the polygon's points, give the person an image, or make the icon square.",
	})
	LanguageWords = register(105, "LanguageWords", Entry{
		Summary: "the game's language and its words or font do not match",
		Detail:  "language is not set while the game is written in Japanese, so the engine's own words are English; or the regular font lacks letters of the chosen words, which would show as blanks.",
		Fix:     "Set language in game.yaml, or choose a font that has the letters.",
	})
	RuleTargetIgnored = register(106, "RuleTargetIgnored", Entry{
		Summary: "a part of an AI rule's target is not used",
		Detail:  "For some actions the target of a rule only decides whether the rule fires: a random enemy is drawn anew, a self skill always hits its user, and by is read only with pick min or max.",
		Fix:     "Remove the part the message names, or use a skill whose target the rule chooses.",
	})
	SetOrder = register(107, "SetOrder", Entry{
		Summary: "a set reads another target of the same set",
		Detail:  "The targets of one set are assigned one after another in key order, so a right-hand side that reads another target of the same set depends on that order.",
		Fix:     "Split it into two set commands in the order you mean.",
	})
	Unvoiced = register(109, "Unvoiced", Entry{
		Summary: "lines of a voiced actor have no voice",
		Detail:  "An actor with a voices folder has lines without say.voice. They are shown without a voice; the warning counts them so the recording can be finished.",
		Fix:     "Record the lines it names and add voice to each, or leave them silent on purpose.",
	})
	UnknownSource = register(110, "UnknownSource", Entry{
		Summary: "a translation is of a text the game does not have",
		Detail:  "A key of lang/<code>.yaml is neither a text of the game (a line, a choice, a name) nor a word of the engine. The text was changed or removed after it was translated.",
		Fix:     "Change the key to the text as it is written now, or remove the entry.",
	})
	Untranslated = register(111, "Untranslated", Entry{
		Summary: "a text of the game is not translated yet",
		Detail:  "A language of game.yaml's languages has no translation of this text in lang/<code>.yaml, so the text is shown as written.",
		Fix:     "Add the text as a key of lang/<code>.yaml with its translation; gozen check --format json lists every one with its path.",
	})
	UnusedFlag = register(112, "UnusedFlag", Entry{
		Summary: "a flag is set but nothing reads it",
		Detail:  "A set writes flag.<name>, and no condition, expression or line of the game reads it. It may be a typo of another flag, or left over.",
		Fix:     "Read the flag where it should matter, fix its name, or remove the set.",
	})
	UnsetFlag = register(113, "UnsetFlag", Entry{
		Summary: "a flag is read but nothing sets it",
		Detail:  "A condition or expression reads flag.<name>, and no set, gallery scene or state.names declaration gives it a value, so it stays false (unless a test starts with --flags or the game's Go code sets it).",
		Fix:     "Set the flag where it should become true, fix its name, or declare it in state.names.",
	})
	FunctionValue = register(114, "FunctionValue", Entry{
		Summary: "an expression uses a function's name as a value",
		Detail:  "A built-in function (floor, max, len and the others) or a function of the game (rand, chance, pages) is written alone, not called, as in floor + 1. The expression fails when it is evaluated. floor is the floor's number only in generate's size, the count of generate.place and a roamer's count, appear, wait and every; elsewhere it is the function and the floor's number is var.floor.",
		Fix:     "Call the function (floor(x)), or use the value you meant (var.floor).",
	})
	DevOnlySet = register(108, "DevOnlySet", Entry{
		Summary: "a set changes an actor's stats or level outside the development mode",
		Detail:  "Setting a party member's stats or level is for the development mode (--dev) and tests. The game skips it when played normally.",
		Fix:     "Use a command the game keeps (an item, a class change, experience), or keep it in a development-only event.",
	})
	MissingBestiaryLore = register(115, "MissingBestiaryLore", Entry{
		Summary: "an encountered enemy has no bestiary lore",
		Detail:  "The monster book is enabled and a troop can include this enemy, but its lore field is empty.",
		Fix:     "Add a short lore description to the enemy, or turn off ui.bestiary.",
	})
	UnobtainableMonster = register(116, "UnobtainableMonster", Entry{
		Summary: "a monster cannot be captured or bred from capturable monsters",
		Detail:  "The breeding graph has no path from a monster with a positive recruit chance to this species, including the male and female sexes needed at each step. A species whose only recipes loop back through species that cannot be obtained has no lineage and is reported too.",
		Fix:     "Make it recruitable or add a breeding recipe reachable from capturable parents.",
	})
	NoCaptureEncounter = register(117, "NoCaptureEncounter", Entry{
		Summary: "a recruitable monster has no encounter in a location",
		Detail:  "A positive recruit chance only matters after winning a battle with that species. No location encounter names a troop containing it. A scripted battle may still offer it, so this is a warning.",
		Fix:     "Add the species to a troop in a reachable location encounter, or confirm that a scripted battle is intentional.",
	})
	ShortEpisode = register(118, "ShortEpisode", Entry{
		Summary: "a stream episode lasts less than 20 minutes",
		Detail:  "The beats of the episode, timed by the timing of the streams file, add up to less than 20 minutes, so a viewer sees it end soon after opening the game.",
		Fix:     "Add beats to the episode, or lengthen the timing.",
	})
)

// GZN11xx: the definitions of the game. The game does not load, and check
// exits 1. Each one points at a file, a line and the path of the definition.
var (
	UnknownReference = register(1101, "UnknownReference", Entry{
		Summary: "a definition names something that is not defined",
		Detail:  "A troop, item, actor, skill, location, event, song, picture set, state, stat, node or another named thing is referred to but not defined, or the thing has no part of that name (a point of a map, a mood of a song, a face of a puppet).",
		Fix:     "Fix the spelling or define the thing. gozen book lists every ID of the game.",
	})
	Duplicate = register(1102, "Duplicate", Entry{
		Summary: "something is defined twice",
		Detail:  "The same ID or key appears twice, in one file or across the files of data/. The message names the previous definition.",
		Fix:     "Rename or remove one of them.",
	})
	Expression = register(1103, "Expression", Entry{
		Summary: "an expression cannot be read or uses a name unknown there",
		Detail:  "An expression (when, a formula, a condition) has a syntax error, uses a name that does not exist where it is written (turn exists only in battle, a and b only in an effect), or mixes types.",
		Fix:     "Fix the expression; the message lists the names known at that place.",
	})
	Shape = register(1104, "Shape", Entry{
		Summary: "a definition is missing a part, or has parts that do not go together",
		Detail:  "A key that is required is missing or empty, two keys exclude each other, a key is used where it means nothing, or a value has the wrong YAML type (a list where a map belongs, text where a number belongs).",
		Fix:     "Write the definition in the shape the Schema page shows for it.",
	})
	MissingAsset = register(1105, "MissingAsset", Entry{
		Summary: "an asset file does not exist",
		Detail:  "A picture, a sound, a face, a font or another file named by the definitions is not under assets/. Names are relative to assets/ and case-sensitive.",
		Fix:     "Add the file, or fix the name.",
	})
	OutOfRange = register(1106, "OutOfRange", Entry{
		Summary: "a number is out of its range",
		Detail:  "The value is a number, but outside what the key accepts: a rate between 0 and 1, a count of 1 or more, a size above 0, a level within the class's maximum.",
		Fix:     "Use a number in the range the message states.",
	})
	NotAWord = register(1107, "NotAWord", Entry{
		Summary: "a value is not one of the words the key accepts",
		Detail:  "The key takes one word from a fixed list, such as a direction, a mode or a screen name. The message lists the words.",
		Fix:     "Use one of the listed words.",
	})
	SchemaVersion = register(1108, "SchemaVersion", Entry{
		Summary: "a word needs a newer schema_version",
		Detail:  "The word was added in a later version of the YAML schema, and game.yaml declares an earlier schema_version, so an older engine would not read the game the same way.",
		Fix:     "Raise schema_version in game.yaml to the version the message names.",
	})
	YAMLSyntax = register(1109, "YAMLSyntax", Entry{
		Summary: "a YAML file cannot be parsed",
		Detail:  "The file is not valid YAML: an indentation, a quote, a colon or a bracket is wrong. A value of a description containing a colon and a space must be quoted.",
		Fix:     "Fix the YAML at the line and column the message names.",
	})
	GameUnreadable = register(1110, "GameUnreadable", Entry{
		Summary: "the game directory or game.yaml cannot be read",
		Detail:  "The directory given is not a game directory: it does not exist, cannot be read, or has no game.yaml.",
		Fix:     "Pass the directory that holds game.yaml, data/ and assets/.",
	})
	ThemeMissing = register(1111, "ThemeMissing", Entry{
		Summary: "a dungeon theme is named but its pictures are not in the game",
		Detail:  "A dungeon or one of its looks says theme: <name>, but assets/dungeon/themes/<name>/ lacks wall.jpg, floor.jpg, ceiling.jpg, door.jpg or theme.yaml. The theme's pictures are not part of the game until they are copied in.",
		Fix:     "Run gozen theme add <game-dir> --name <name> (gozen theme list shows the names), or put your own folder of that name there.",
	})
	MonsterRules = register(1112, "MonsterRules", Entry{
		Summary: "a monster collection or breeding rule is invalid",
		Detail:  "A recruit setting, monster family, rank, sex, sex ratio, pair or breeding requirement is invalid or ambiguous. A pair or family rule also fails when its species cannot supply one male and one female parent.",
		Fix:     "Define known classes, families and species; use positive ranks and valid probabilities; give each unordered pair one result and ensure its parents can have opposite sexes.",
	})
	MonsterGrowth = register(1113, "MonsterGrowth", Entry{
		Summary: "a monster cannot use its configured growth",
		Detail:  "The recruit level or a learned skill exceeds the monster class maximum, or its experience requirement fails to rise with the level. Such a monster cannot progress as authored.",
		Fix:     "Keep recruit and skill levels within the class maximum and make exp_for increase from level two through that maximum.",
	})
)

// GZN12xx: fonts and text layout.
var FontUnreadable = register(1201, "FontUnreadable", Entry{
	Summary: "a font of the game cannot be read",
	Detail:  "The layout check reads the game's fonts to lay out the dialogue, and one of them is missing or is not a TrueType or OpenType font.",
	Fix:     "Fix the font files of game.yaml, or check the structure alone with gozen check --only structure.",
})
