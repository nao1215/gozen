package testgame

// Tactics is an original decision table covering individual, group and item actions.
const Tactics = `
skills:
  group_heal: {name: Shared tonic, scope: all_allies, effects: [{heal: 10, variance: 0}]}
  raise: {name: Rally, scope: dead_ally, effects: [{revive: 0.5}]}
  group_raise: {name: Team rally, scope: all_dead_allies, effects: [{revive: 0.5}]}
  random_hit: {name: Scatter, scope: random_enemy, effects: [{damage: 1}]}
  all_hit: {name: Ripple, scope: everyone, effects: [{damage: 1}]}
  retreat: {name: Withdraw, scope: none, effects: [{escape: true}]}
  menu_heal: {name: Bandage, scope: one_ally, where: [menu], effects: [{heal: 1}]}
tactic_conditions:
  any: {name: いつでも, kind: always}
  low_hp: {name: Health below threshold, kind: hp_below, subject: allies}
  low_mp: {name: Energy threshold, kind: mp_at_most, subject: self}
  poisoned: {name: Poisoned target, kind: state, subject: target, state: poison}
  fallen: {name: Fallen ally, kind: dead, subject: allies}
  target_hp: {name: Target health, kind: hp_at_most, subject: target}
  complex:
    name: Conditional spark
    kind: template
    when: "self.mp >= 3 && foes.with_poison > 0 && (turn % 2 == 0 || flag.ready)"
    chance: "0.75 + rand() * 0.25"
    priority: 4
    weight: 2
    once: true
  group_complex: {name: Team recovery, kind: template, when: "allies.min_hp_rate < 0.6 && allies.alive - allies.sum_hp_rate >= 0.9 && self.mp >= 3"}
  guard_complex: {name: Brace while waiting, kind: template, when: "foes.with_poison + foes.with_sleep > 0"}
tactic_targets:
  enemy: {name: Opponent, scope: one_enemy}
  ally: {name: Injured ally, scope: one_ally, selector: {pick: min, by: t.hp_rate}}
  fallen: {name: Fallen ally, scope: dead_ally, selector: first}
  self: {name: Self, scope: self}
  allies: {name: Team, scope: all_allies}
  enemies: {name: Opponents, scope: all_enemies}
  fallen_all: {name: Fallen team, scope: all_dead_allies}
  random: {name: Random opponent, scope: random_enemy}
  everyone: {name: Everyone, scope: everyone}
  none: {name: No target, scope: none}
  complex: {name: Warmth-sensitive opponent, scope: one_enemy, selector: {where: "t.resist_fire > 1.2 && !t.states.sleep", pick: max, by: t.hp}}
tactic_actions:
  attack: {name: Strike, use: attack}
  guard: {name: Brace, use: guard}
  fire: {name: Spark, use: fire}
  heal: {name: Restore, use: heal}
  potion: {name: Tonic, item: potion}
  group_heal: {name: Shared tonic, use: group_heal}
  raise: {name: Rally, use: raise}
  group_raise: {name: Team rally, use: group_raise}
  big: {name: Sweep, use: big}
  random_hit: {name: Scatter, use: random_hit}
  all_hit: {name: Ripple, use: all_hit}
  retreat: {name: Withdraw, use: retreat}
ai:
  original_complex:
    - when: "self.mp >= 3 && foes.with_poison > 0 && (turn % 2 == 0 || flag.ready)"
      chance: "0.75 + rand() * 0.25"
      priority: 4
      weight: 2
      once: true
      use: fire
      target: {where: "t.resist_fire > 1.2 && !t.states.sleep", pick: max, by: t.hp}
  original_group:
    - {when: "allies.min_hp_rate < 0.6 && allies.alive - allies.sum_hp_rate >= 0.9 && self.mp >= 3", use: group_heal}
  original_guard:
    - {when: "foes.with_poison + foes.with_sleep > 0", use: guard}
`

// AutoTacticsGame selects the capability rules for independent availability tests.
const AutoTacticsGame = `
title: Training course
start: {location: camp, party: [scout], items: {potion: 2}}
battle:
  ai: gambit
  auto_tactics:
    capability:
      - {enabled: true, condition: any, target: enemies, action: big}
      - {enabled: true, condition: any, target: allies, action: group_heal}
      - {enabled: true, condition: any, target: ally, action: heal}
      - {enabled: true, condition: any, target: ally, action: potion}
      - {enabled: true, condition: any, target: enemy, action: fire}
      - {enabled: true, condition: any, target: self, action: guard}
      - {enabled: true, condition: any, target: enemy, action: attack}
`

// AutoTacticsData provides the equivalent direct rules and an equipment-granted skill.
const AutoTacticsData = `
ai:
  capability:
    - {use: big}
    - {use: group_heal}
    - {use: heal, target: {pick: min, by: t.hp_rate}}
    - {item: potion, target: {pick: min, by: t.hp_rate}}
    - {use: fire}
    - {use: guard}
    - {use: attack}
stone_sets:
  gift: {name: Teamwork, stones: [blade], skill: group_heal}
`
