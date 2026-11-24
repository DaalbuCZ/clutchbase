package protocol

var Presets = []string{"", "match", "practice"}

func ValidPreset(value string) bool { return value == "" || value == "match" || value == "practice" }
