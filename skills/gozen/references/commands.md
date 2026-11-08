# Event commands

Each event step has one verb. The table lists built-in verbs; fields such as
`then`, `fighters` and `prompt` accompany a verb and are not separate steps.
See [Command](https://nao1215.github.io/gozen/schema/#command) for argument shapes
and defaults. Aliases declared in `game.yaml` under `verbs` call events; they
are local to that game. Prefer typed `call.args` for reusable behavior.

| Event verb | Purpose |
| --- | --- |
| `act` | Queues one move for the next auto battle: who acts, the skill or item, and the target. |
| `affect` | Puts a state on party members or removes one outside battle. |
| `battle` | Starts a battle against a troop, an ID of troops. |
| `breed` | Breeds two owned monster instances and chooses inherited skills. Needs schema version 6. |
| `buy` | Buys items from a shop, the same as the Buy screen: the shop, the item and the count. |
| `call` | Run another event, optionally with typed arguments or a bounded collection of calls, then continue. |
| `call_view` | Change a video call camera or shared picture without mutating game state. |
| `capture` | Recruit a permitted monster species and record the admission result. |
| `carry` | Keep the values game.yaml carry names for the next new game (schema 10); written beside the saves, never in a save. |
| `checkpoint` | Replace the retry point with current state, random stream and a restore-entry event. |
| `choice` | Asks the player to pick one of several answers. Each answer has its own commands. `muted: {seconds, do}` hides them behind Unmute; left muted, `do` runs. |
| `clear` | If true, ends the game as cleared. |
| `dive` | Starts a dive into a generated dungeon, an ID of locations whose dungeon has `generate`. `floor` chooses the floor. |
| `effect` | Play a screen effect or a moving portrait reaction. |
| `end` | If true, stops the event here. |
| `end_turn` | Ends the campaign's turn: the other factions follow their rules and the next turn begins. Needs schema version 5. |
| `equip` | Equips an item from the bag on a party member. Leaving out `item` empties the slot, and leaving out both equips the best gear. |
| `equip_stones` | Puts the stones of a stone set on an actor's stone slots, taking them from the bag or from members not using them. |
| `exp` | An expression for the experience each living party member gains. |
| `game_over` | Ends the game lost, as a lost battle does. Needs schema version 5. |
| `give` | Adds an item to the bag: the item ID and a count (default 1). |
| `gold` | An expression for gold to add. A negative value takes gold away. |
| `heal` | Fully restores a party member, an ID of actors, or everyone with `all`. |
| `hurt` | Takes HP from party members outside battle, for example on a poison floor. It never reduces anyone below 1. |
| `if` | An expression. The `then` commands run when it holds, otherwise the `else` commands. |
| `image` | Shows a full-screen picture. It can be a file path or a map with options. |
| `join` | Adds an actor to the party, an ID of actors. |
| `learn` | Teaches a party member a skill: the actor and the skill. |
| `leave` | Removes an actor from the party, an ID of actors. |
| `message` | Queues a chat message ({who, text, event}) as a top-right notification on full-screen surfaces; opening shows a bubble and runs the event, which asks for the decision. Queuing does not stop the event. |
| `mood` | Switches the music to a named mood. |
| `move` | Moves the party to another location, an ID of locations. |
| `music` | Plays a song, optionally in a given mood. Written as `song`, `song/mood` or a map. |
| `notice` | Shows a short notice at the top of the screen without stopping the event. |
| `open` | Opens a game screen by name. |
| `open_wall` | Turns the wall next to the party's cell in a dungeon into the tile of the given map character, such as a hidden stair. |
| `order` | Gives an order of the campaign: its ID, or a map of the ID, the general and the target region. The general and the target it needs are asked when not given. Needs schema version 5. |
| `phone` | Show a chat, feed or video phone overlay until dismissed, without mutating game state. |
| `pick` | Picks an item of the current location's menu (a person, action or exit) by its label. |
| `plan` | Apply one deterministic planning operation. |
| `release` | Permanently releases one owned monster instance. Needs schema version 6. |
| `reserve` | Swaps one joined reserve actor into an active party slot. Needs schema version 6. |
| `reset_actor` | Reset progression and restore health while retaining equipment, name, row and party membership. |
| `reset_tree` | Gives back the SP spent in the skill tree of an actor, an ID of actors, or of everyone with `all`. |
| `respond` | Submit text to a speaker reaction book and display its reply. |
| `restore_checkpoint` | If true, discards the current execution and restores the latest checkpoint, then runs its entry event; schema version 9. |
| `save` | If true, opens the save screen. |
| `say` | Shows a line of dialog. It can be plain text or a map with speaker, face, side and text. |
| `sell` | Sells items to a shop, the same as the Sell screen: the shop, the item and the count. |
| `set` | Assigns values to variables or state, mapping each name to an expression. |
| `sfx` | Plays a sound effect by name. A map (version 4) with `wait: true` holds the event until the sound ends. |
| `shop` | Opens a shop, an ID of shops. |
| `sp_apply` | Spends the SP according to the plan made by `sp_suggest`, for all members at once. |
| `sp_decline` | Remembers that the player declined, so `sp_suggest` does not offer again until a member's SP changes. |
| `sp_suggest` | Plan how to spend spare skill points and store the proposal under a variable prefix. |
| `stand` | Place, move or remove a portrait on the novel stage. |
| `step` | Walks the given number of steps forward in a dungeon. Floor traps and battles happen on every step. |
| `stone_apply` | Re-equips everyone's stones at once with the best arrangement found by `stone_suggest`. |
| `stone_choose` | Lets the player choose one of the possible stone combinations. |
| `stone_decline` | Remembers that the player declined, so `stone_suggest` does not offer again until the owned stones change. |
| `stone_suggest` | Suggest a stronger arrangement of owned stones and store the proposed changes under a variable prefix. |
| `stone_wearer` | Lets the player choose who wears the stones. |
| `take` | Removes an item from the bag: the item ID and a count (default 1). |
| `turn` | Turns in a dungeon: `left`, `right` or `back`. |
| `use` | Uses an item from the menu, an ID of items. |
| `wait` | Pauses for the given number of seconds. A map (version 4) with `skip: false` holds even in skip mode. |
| `walk` | Walks the shortest path in a dungeon to the tile of the given map character. |
