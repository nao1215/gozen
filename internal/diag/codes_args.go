package diag

// GZN2xxx: the command line is invalid; the process exits 2 and nothing is
// read or written. The fix is always in the command line itself.

// GZN21xx: the shape of the command line.
var (
	UnknownCommand = register(2101, "UnknownCommand", Entry{
		Summary: "there is no such command",
		Detail:  "The word is not a command of gozen or of the group it follows. The commands are grouped (ADR 0024): check, sim, play, book, pack, art, version, and help. An old name such as curve or memos is answered with its new place, for example gozen sim dive.",
		Fix:     "Use the command the message names, or run gozen --help (or the group with --help) for the list.",
	})
	Operands = register(2102, "Operands", Entry{
		Summary: "the command takes another number of arguments",
		Detail:  "A command takes a fixed list of positional arguments, such as a game directory and a troop for sim battle. Too few or too many were given; an option that needs a value may have taken the next word.",
		Fix:     "Give the arguments the usage line of the command's --help shows, in that order. Put -- before an argument that starts with a dash.",
	})
	UnknownOption = register(2103, "UnknownOption", Entry{
		Summary: "the command has no such option",
		Detail:  "Options belong to a command: an option of sim battle is not an option of sim dive, and a group such as sim has only --help.",
		Fix:     "Check the option in the command's --help.",
	})
	SingleDashOption = register(2104, "SingleDashOption", Entry{
		Summary: "a long option was written with one dash",
		Detail:  "Options follow the GNU style (ADR 0018). A long name with one dash, such as -seed, would be read as the short options -s -e -e -d, so it is refused instead.",
		Fix:     "Write the long option with two dashes, such as --seed.",
	})
	OptionBeforeCommand = register(2105, "OptionBeforeCommand", Entry{
		Summary: "an option was given before the command",
		Detail:  "gozen itself takes only --help and --version. Options of a command come after the command's name, because the command decides what they mean.",
		Fix:     "Write gozen <command> [OPTIONS], for example gozen sim battle --seed 7 game lesson.",
	})
	CommandMissing = register(2106, "CommandMissing", Entry{
		Summary: "a command is required",
		Detail:  "gozen, and a group such as sim or art, does nothing by itself. The message lists the commands it holds.",
		Fix:     "Add one of the listed commands, or run the group with --help to read about them.",
	})
)

// GZN22xx: the value of an option.
var (
	BadValue = register(2201, "BadValue", Entry{
		Summary: "an option has a value of the wrong type or out of range",
		Detail:  "The value cannot be read as the option's type (a number, a duration), or it is outside the range the option accepts: a count that must be positive, a rate between 0 and 1, a time that cannot be negative, or a pair such as FLOOR:TROOP written another way.",
		Fix:     "Give a value of the type and range the option's description in --help states.",
	})
	NotAChoice = register(2202, "NotAChoice", Entry{
		Summary: "an option has a value that is not one of its choices",
		Detail:  "The option takes one word from a fixed list, such as --format text or json, or check --only structure or layout. A misspelled choice is refused instead of falling back to a default.",
		Fix:     "Use one of the values the message lists.",
	})
)

// GZN23xx: options that do not go together, or need each other.
var (
	OptionConflict = register(2301, "OptionConflict", Entry{
		Summary: "two options do not go together",
		Detail:  "The options ask for things that exclude each other, such as a saved state and a chosen party for sim battle, or --raw and --overlay for play shots.",
		Fix:     "Keep one of the options the message names.",
	})
	OptionNeedsAnother = register(2302, "OptionNeedsAnother", Entry{
		Summary: "an option needs another option",
		Detail:  "Some options mean something only in company: --as-is keeps the HP of a --state, --try compares the seeds of one --floor, and --overlay and --inspect go together.",
		Fix:     "Add the option the message names, or drop the one that needs it.",
	})
	OptionRequired = register(2303, "OptionRequired", Entry{
		Summary: "a required option is missing",
		Detail:  "The command cannot run without the option: play shots needs the game binary (--bin), pack release the inspection records (--inspect), and art prompt the catalog (--config).",
		Fix:     "Add the option the message names.",
	})
)

// GZN24xx: a path or a name the command line gives.
var (
	BadPath = register(2401, "BadPath", Entry{
		Summary: "a path must be relative and stay inside its directory",
		Detail:  "The option names a file or a directory inside the game, such as pack trial --config or pack release --prefix. It must be a relative path separated by / that does not climb out with .. or start at the root.",
		Fix:     "Write the path relative to the game directory, without .., such as trial.yaml.",
	})
	UnknownName = register(2402, "UnknownName", Entry{
		Summary: "the command line names something the game does not have",
		Detail:  "An ID given on the command line, such as the troop of sim battle, the location of sim dive or sim floors, a boss or a shop, a persona of sim design --only, a shot of play shots --only, or a model of art fetch, is not defined. A typo in an argument is an argument error, so it exits 2 in every command.",
		Fix:     "Check the spelling against the game's data (gozen book lists every ID), or the list the message names.",
	})
)

// GZN25xx: gozen help.
var UnknownCode = register(2501, "UnknownCode", Entry{
	Summary: "there is no such diagnostic code",
	Detail:  "gozen help explains a code of the form GZN and four digits, and this one is not registered. The message lists the codes of the same family when there are any.",
	Fix:     "Copy the code from the end of the diagnostic, or look it up on the error codes page.",
})
