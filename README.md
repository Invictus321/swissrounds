# swissrounds

Run Magic: The Gathering Swiss tournaments with a single-elimination top cut,
from your own computer, with a big-screen display for pairings, standings,
the bracket and the round clock. No logins, no external dependencies.

## Run it

```sh
go run .
```

Then open:

- **Organiser view:** <http://localhost:8080/> — set up the event, pair rounds,
  enter results, run the timer, and choose what the big screen shows.
- **Big screen:** <http://localhost:8080/display> — put this on a projector/TV
  (press F11 / ⌃⌘F for full screen). It updates automatically every second.

Flags:

| Flag | Default | Description |
| --- | --- | --- |
| `-addr` | `localhost:8080` | Listen address. Use `:8080` if the display runs on another device on your network (note: anyone on the network can then change results). |
| `-data` | `tournament.json` | Where the tournament is saved. It's written after every change, so you can stop and restart at any time. |

You can pin a display to one view with `?view=pairings`, `?view=standings`,
`?view=bracket` or `?view=timer` (handy with two screens); otherwise it shows
whatever you select in the organiser header.

## Features

- **Setup:** enter the number of players and their names (or paste a list),
  Swiss round count (defaults to Wizards' recommendation), top cut
  (none / 2 / 4 / 8 / 16) and round length.
- **Swiss pairings:** round 1 random; later rounds pair within point groups,
  never repeat an opponent (where possible), and pair down only when needed.
  Odd player counts give a bye (2-0 win) to the lowest-ranked player who
  hasn't had one.
- **Results:** one-click 2-0 / 2-1 / 1-0 / 1-1 / ID (0-0-3) / 0-1 / 1-2 / 0-2,
  custom scores, edit or clear any earlier round.
- **Standings & tiebreakers:** match points, OMW%, GW%, OGW% (33.33% floor,
  byes ignored for opponent averages, dropped players still count), with
  a CSV export. The cut line is shown in the organiser view.
- **Player management:** drop / re-enter, rename, late entries, seat swaps,
  delete and re-pair the latest round.
- **Top cut:** seeded bracket (1v8, 4v5, 2v7, 3v6), higher seed listed first,
  winners advance automatically, final standings by playoff finish.
- **Round timer:** start / pause / ±1 minute / reset; turns amber under
  5 minutes and shows TIME plus overtime at zero.
- **Big screen:** alphabetical pairings (player → table → opponent),
  standings, bracket or a full-screen clock, auto-sized to fit, plus an
  announcement banner.

## Development

```sh
go test ./...
go build ./...
```
