package rpg

import (
	"maps"
	"slices"
)

// Streams is the progress of the live streams (ADR 0048), kept by internal/layer3/broadcast.
type Streams struct {
	// Open is when the game was last open, in Unix seconds (0: never).
	Open int64 `json:"open,omitempty"`
	// Viewer is the name the streamers call the player.
	Viewer string `json:"viewer,omitempty"`
	// Channels are the progress of each channel by its id.
	Channels map[string]*ChannelProgress `json:"channels,omitempty"`
	// Duos are the next episode of each duo by its key (host+with).
	Duos map[string]int `json:"duos,omitempty"`
}

// MaxNextEpisode is the highest next episode a save keeps: the episodes are taken round
// from it, and a stream adds its episodes to it, which must not overflow.
// internal/layer3/broadcast goes on no further when an episode is seen.
const MaxNextEpisode = 1 << 30

// check reports the first next episode of a channel or a duo out of 0 to MaxNextEpisode, as
// the path of the save's field.
func (p *Streams) check() string {
	for _, id := range slices.Sorted(maps.Keys(p.Channels)) {
		if c := p.Channels[id]; c != nil && (c.Next < 0 || c.Next > MaxNextEpisode) {
			return "streams/channels/" + id + "/next"
		}
	}
	for _, key := range slices.Sorted(maps.Keys(p.Duos)) {
		if n := p.Duos[key]; n < 0 || n > MaxNextEpisode {
			return "streams/duos/" + key
		}
	}
	return ""
}

// ChannelProgress is what a channel remembers of the player.
type ChannelProgress struct {
	// Next is the next episode the player has not seen.
	Next int `json:"next,omitempty"`
	// Points are the affection points.
	Points float64 `json:"points,omitempty"`
	// Day is the last day visited (days since 1970-01-01 in local time, 0: never) and
	// Run the number of days in a row up to it.
	Day int `json:"day,omitempty"`
	Run int `json:"run,omitempty"`
	// Visits are how many times the player opened the channel's shows.
	Visits int `json:"visits,omitempty"`
}
