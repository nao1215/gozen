package diag

// GZN14xx: a trace of a play (trace).
var (
	TraceBroken = register(1401, "TraceBroken", Entry{
		Summary: "a line of the trace is broken",
		Detail:  "trace reads the JSON Lines the game writes with --trace. A line is not JSON, has no kind t, or has a tick that is not a time.",
		Fix:     "Record the trace again with the game's --trace; do not edit it by hand.",
	})
	PictureFault = register(1402, "PictureFault", Entry{
		Summary: "a standing picture went wrong in the play",
		Detail:  "trace --check found a standing picture that moved while speaking, was left over after its line, hid an event picture, or was missing for a speaker.",
		Fix:     "Look at the event at the time the line names, and fix its show and hide commands or the speaker's pictures.",
	})
	MusicFault = register(1403, "MusicFault", Entry{
		Summary: "a song came later than asked for",
		Detail:  "trace --check found a song heard later than --music-late seconds after the game asked for it.",
		Fix:     "Make the song shorter to load (a smaller file) or ask for it earlier.",
	})
	SkipFault = register(1404, "SkipFault", Entry{
		Summary: "skipping passed a line that was not read",
		Detail:  "trace --check found a line marked not read while skip mode or scene skip was on. Skipping stops before every line not read.",
		Fix:     "Record the trace again with this engine; if it happens again, report it with the trace.",
	})
	VoiceFault = register(1406, "VoiceFault", Entry{
		Summary: "a voice could not be read in the play",
		Detail:  "trace --check found a voice line marked missing: the file of a line's voice was not there or was not a sound the engine reads.",
		Fix:     "Add the file under assets/ (ogg or wav), or fix the say.voice that names it; gozen check lists missing voices too.",
	})
	RollbackFault = register(1405, "RollbackFault", Entry{
		Summary: "rolling back did not go back",
		Detail:  "trace --check found a rollback line whose event stayed and whose line did not go back. Rolling back replays the event from its start to the line before.",
		Fix:     "Record the trace again with this engine; if it happens again, report it with the trace.",
	})
)

// GZN15xx: an output path.
var (
	OutputExists = register(1501, "OutputExists", Entry{
		Summary: "the output already exists",
		Detail:  "Commands that write a directory or a list never overwrite: the output must not exist yet, even as an empty directory.",
		Fix:     "Choose a new path, or remove the old output yourself.",
	})
	OutputOverlaps = register(1502, "OutputOverlaps", Entry{
		Summary: "the output is inside an input, or holds one",
		Detail:  "Writing into the game directory, the overlay or the list would change what is being read.",
		Fix:     "Write the output outside the inputs.",
	})
)

// GZN17xx: building a release or a trial tree (pack).
var (
	Unapproved = register(1701, "Unapproved", Entry{
		Summary: "an asset the game refers to is not approved",
		Detail:  "release copies only approved assets. An asset of the overlay that the game refers to has no approved inspection record, or it changed since the record.",
		Fix:     "Inspect the asset and add an approved record, or stop referring to it.",
	})
	ApprovalRecords = register(1702, "ApprovalRecords", Entry{
		Summary: "the approval settings or records cannot be read",
		Detail:  "The game's asset-policy.yaml, the overlay's records, or the --inspect file is missing or malformed, or no overlay is given.",
		Fix:     "Give --overlay and --inspect, and check the files the message names.",
	})
	ReferencedAssetMissing = register(1703, "ReferencedAssetMissing", Entry{
		Summary: "an asset the game refers to is not there",
		Detail:  "The tree being built refers to an asset that is neither in the game nor in the overlay.",
		Fix:     "Add the asset, or remove the reference; gozen check lists missing pictures too.",
	})
	TrialConfig = register(1704, "TrialConfig", Entry{
		Summary: "trial.yaml cannot be read",
		Detail:  "The range settings of trial are not valid: YAML, an assume that is not var.<name> or flag.<name>, a dive_max without blocked, or a bad pattern.",
		Fix:     "Fix trial.yaml as the message says.",
	})
)
