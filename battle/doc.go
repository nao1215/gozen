// Package battle implements deterministic, turn-based combat without graphics.
// Game hosts construct a Battle from validated definitions, a state and a known
// troop ID, call Begin, then StartTurn and Next until a result is reached.
// End applies rewards to the state; call it once per completed battle. Rise
// restores fallen participating actors when the host elects to continue.
// Action, Unit and Step let frontends choose actions and present their results.
// Decision and RuleEvaluation let tools observe AI decisions without re-evaluating
// random conditions. Callbacks must not mutate the battle during a decision.
// Battle borrows and mutates its state and random source; use it serially.
package battle
