# Measuring balance

`gozen sim` fights battles on AUTO with the same rules as the game and prints win rates. Use numbers, not guesses.

<!-- verify: run fixture=cave -->
```shell
gozen sim battle game cave_boss --level 8
gozen sim dive game cave --floors 5 --boss 5:cave_boss
```

- `sim battle GAME TROOP` fights a troop 200 times with the starting party. `--party IDS`, `--level N` and `--hp` change the party and print the HP left after a win.
- `sim dive GAME LOCATION` walks a generated dungeon floor by floor and prints levels, HP and boss win rates.
- `--format json` prints the same numbers for a script.
- A boss near 70 to 85 percent at the level a player reaches feels fair. A normal fight should leave most of the HP.
