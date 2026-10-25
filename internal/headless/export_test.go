package headless

// entry points used by the external tests (headless_test); made unexported in ADR 0021 stage 1.

func (d *Driver) Fight(troop string) int { return d.fight(troop, true) }
