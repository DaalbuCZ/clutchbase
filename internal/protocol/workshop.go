package protocol

import "strings"

// ValidWorkshopMap accepts the canonical identifier workshop/<published-file-id>/<map>.
func ValidWorkshopMap(name string) bool {
	parts := strings.Split(name, "/")
	if len(parts) != 3 || parts[0] != "workshop" || len(parts[1]) == 0 || len(parts[1]) > 20 || parts[1][0] == '0' || len(parts[2]) == 0 || len(parts[2]) > 128 {
		return false
	}
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return false
		}
	}
	for _, c := range parts[2] {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// ValidInstalledMap validates syntax only; Workshop authorization is host-specific.
func ValidInstalledMap(name string) bool { return ValidMap(name) || ValidWorkshopMap(name) }

const MaxWorkshopMaps = 128
const MaxWorkshopMapBytes = 8192

func ValidWorkshopMaps(maps []string) bool {
	if len(maps) > MaxWorkshopMaps {
		return false
	}
	seen := make(map[string]bool, len(maps))
	size := 0
	for _, name := range maps {
		size += len(name)
		if size > MaxWorkshopMapBytes || !ValidWorkshopMap(name) || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}
