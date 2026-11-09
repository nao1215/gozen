// Package event executes game commands independently of any display.
// Hosts call New, Start, Next and Answer. Next returns a Request when user input
// or a blocking presentation is needed; Answer supplies a choice or battle result.
// Immediate effects are delivered synchronously through Host.Handle. A nil Host
// discards immediate effects. A moving host must update State.Location and Steps
// after receiving MoveEffect; dungeon placement has already been applied.
// Optional AsideBlocker suppresses automatic inserted dialogue.
//
// Runner borrows the validated DB, State and host; they must remain alive and be
// used serially. Log receives command/evaluation diagnostics; set it when driving
// Runner directly. The headless driver collects these diagnostics as errors.
package event
