package diag

// GZN14xx: a trace of a play (trace).
var (
	TraceBroken = register(1401, "TraceBroken", Entry{
		Summary: "a line of the trace is broken",
		Detail:  "trace reads the JSON Lines the game writes with --trace. A line is not JSON, has no kind t, or has a tick that is not a time.",
		Fix:     "Record the trace again with the game's --trace; do not edit it by hand.",
		NotYAML: "This reads malformed JSON Lines from a recorded trace, not game YAML.",
	})
	PictureFault = register(1402, "PictureFault", Entry{
		Summary: "a standing picture went wrong in the play",
		Detail:  "trace --check found a standing picture that moved while speaking, was left over after its line, hid an event picture, or was missing for a speaker.",
		Fix:     "Look at the event at the time the line names, and fix its show and hide commands or the speaker's pictures.",
		NotYAML: "This requires standing-picture events in a runtime trace; checking YAML does not execute the play.",
	})
	MusicFault = register(1403, "MusicFault", Entry{
		Summary: "a song came later than asked for",
		Detail:  "trace --check found a song heard later than --music-late seconds after the game asked for it.",
		Fix:     "Make the song shorter to load (a smaller file) or ask for it earlier.",
		NotYAML: "This measures music delivery times in a runtime trace; YAML alone has no timing evidence.",
	})
	SkipFault = register(1404, "SkipFault", Entry{
		Summary: "skipping passed a line that was not read",
		Detail:  "trace --check found a line marked not read while skip mode or scene skip was on. Skipping stops before every line not read, unless the player set Text to skip to all (the skip line then carries unread).",
		Fix:     "Record the trace again with this engine; if it happens again, report it with the trace.",
		NotYAML: "This requires skip and unread-line events in a runtime trace.",
	})
	VoiceFault = register(1406, "VoiceFault", Entry{
		Summary: "a voice could not be read in the play",
		Detail:  "trace --check found a voice line marked missing: the file of a line's voice was not there or was not a sound the engine reads.",
		Fix:     "Add the file under assets/ (ogg or wav), or fix the say.voice that names it; gozen check lists missing voices too.",
		NotYAML: "This requires a missing-voice event in a runtime trace.",
	})
	RollbackFault = register(1405, "RollbackFault", Entry{
		Summary: "rolling back did not go back",
		Detail:  "trace --check found a rollback line whose event stayed and whose line did not go back. Rolling back replays the event from its start to the line before.",
		Fix:     "Record the trace again with this engine; if it happens again, report it with the trace.",
		NotYAML: "This requires rollback events and positions in a runtime trace.",
	})
)

// GZN15xx: an output path.
var (
	OutputExists = register(1501, "OutputExists", Entry{
		Summary: "the output already exists",
		Detail:  "Commands that write a directory or a list never overwrite: the output must not exist yet, even as an empty directory.",
		Fix:     "Choose a new path, or remove the old output yourself.",
		NotYAML: "This depends on an output path already existing on the filesystem.",
	})
	OutputOverlaps = register(1502, "OutputOverlaps", Entry{
		Summary: "the output is inside an input, or holds one",
		Detail:  "Writing into the game directory, the overlay or the list would change what is being read.",
		Fix:     "Write the output outside the inputs.",
		NotYAML: "This depends on the relationship between command-line input and output paths.",
	})
)

// GZN17xx: building a release or a trial tree (pack).
var (
	Unapproved = register(1701, "Unapproved", Entry{
		Summary: "an asset the game refers to is not approved",
		Detail:  "release copies only approved assets. An asset of the overlay that the game refers to has no approved inspection record, or it changed since the record.",
		Fix:     "Inspect the asset and add an approved record, or stop referring to it.",
		NotYAML: "This requires release approval records and an asset overlay, not just checked game definitions.",
	})
	ApprovalRecords = register(1702, "ApprovalRecords", Entry{
		Summary: "the approval settings or records cannot be read",
		Detail:  "The game's asset-policy.yaml, the overlay's records, or the --inspect file is missing or malformed, or no overlay is given.",
		Fix:     "Give --overlay and --inspect, and check the files the message names.",
		NotYAML: "This reports release policy or inspection-record I/O and parsing, which gozen check does not perform.",
	})
	ReferencedAssetMissing = register(1703, "ReferencedAssetMissing", Entry{
		Summary: "an asset the game refers to is not there",
		Detail:  "The tree being built refers to an asset that is neither in the game nor in the overlay.",
		Fix:     "Add the asset, or remove the reference; gozen check lists missing pictures too.",
		NotYAML: "This is raised while composing a release tree from a game and an asset overlay.",
	})
	TrialConfig = register(1704, "TrialConfig", Entry{
		Summary: "trial.yaml cannot be read",
		Detail:  "The range settings of trial are not valid: YAML, an assume that is not var.<name> or flag.<name>, a dive_max without blocked, or a bad pattern.",
		Fix:     "Fix trial.yaml as the message says.",
		NotYAML: "This is raised by trial when reading its selection settings; gozen check does not read trial.yaml.",
	})
)
