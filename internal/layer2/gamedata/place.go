package gamedata

import (
	"maps"
	"reflect"
	"slices"
)

var commandType = reflect.TypeFor[Command]()

// eachCall passes to fn the names of events called by commands inside v (including nested then, else, choice, on_win, etc.).
func eachCall(v reflect.Value, fn func(string)) {
	switch v.Kind() { //nolint:exhaustive // Only schema container/scalar kinds need handling; other reflection kinds are intentionally ignored.
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			eachCall(v.Elem(), fn)
		}
	case reflect.Struct:
		if v.Type() == commandType {
			if c, ok := reflect.TypeAssert[Command](v); ok && c.Call != "" {
				fn(c.Call)
			}
		}
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				eachCall(v.Field(i), fn)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			eachCall(v.Index(i), fn)
		}
	case reflect.Map:
		it := v.MapRange()
		for it.Next() {
			eachCall(it.Value(), fn)
		}
	}
}

// EventPlaces is a table from event names to the ids of the places where the event occurs (undetermined ones are not included).
func (db *DB) EventPlaces() map[string]string {
	homes := map[string]map[string]bool{} // places found at that distance
	out := map[string]string{}
	var frontier []string
	add := func(ev, loc string) {
		if homes[ev] == nil {
			homes[ev] = map[string]bool{}
		}
		homes[ev][loc] = true
	}
	for _, id := range slices.Sorted(maps.Keys(db.Events)) {
		if l := db.Events[id].Location; l != "" && db.Locations[l] != nil {
			add(id, l)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(db.Locations)) {
		eachCall(reflect.ValueOf(db.Locations[id]), func(ev string) {
			if db.Events[ev] != nil && db.Events[ev].Location == "" {
				add(ev, id)
			}
		})
	}
	done := map[string]bool{}
	settle := func() {
		frontier = frontier[:0]
		for _, ev := range slices.Sorted(maps.Keys(homes)) {
			if done[ev] {
				continue
			}
			done[ev] = true
			frontier = append(frontier, ev)
			if len(homes[ev]) == 1 {
				for l := range homes[ev] {
					out[ev] = l
				}
			}
		}
		clear(homes)
	}
	settle()
	for len(frontier) > 0 {
		for _, from := range frontier {
			l, ok := out[from]
			if !ok {
				continue // do not follow from undetermined ones
			}
			eachCall(reflect.ValueOf(db.Events[from].Do), func(ev string) {
				if db.Events[ev] != nil && !done[ev] {
					add(ev, l)
				}
			})
		}
		settle()
	}
	// For an event not called from anywhere (a wrapper only for gallery recollections), if the place it calls from is determined to be one, that place
	for _, id := range slices.Sorted(maps.Keys(db.Events)) {
		if done[id] {
			continue
		}
		seen := map[string]bool{}
		eachCall(reflect.ValueOf(db.Events[id].Do), func(ev string) {
			if l, ok := out[ev]; ok {
				seen[l] = true
			}
		})
		if len(seen) == 1 {
			for l := range seen {
				out[id] = l
			}
		}
	}
	return out
}
