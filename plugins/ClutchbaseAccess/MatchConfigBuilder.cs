namespace ClutchbaseAccess;

public static class MatchConfigBuilder
{
    public static object Build(string map, string[] maps, string[] mapSides, int playersPerSide, Dictionary<string, string> ct, Dictionary<string, string> t, bool overtime, bool manualReady = false, string? gameDirectory = null)
    {
        if (playersPerSide < 1 || playersPerSide > 5 || ct.Count < 1 || t.Count < 1 || ct.Count > playersPerSide || t.Count > playersPerSide)
            throw new ArgumentException("Invalid match roster");
        if ((maps.Length != 1 && maps.Length != 3 && maps.Length != 5) || maps[0] != map || mapSides.Length != maps.Length || maps.Distinct().Count() != maps.Length || maps.Any(x => !MapPolicy.IsIdentifier(x)) || mapSides.Any(x => x != "team1_ct" && x != "team1_t"))
            throw new ArgumentException("Invalid match series");
        MapPolicy.RequireLoadable(maps, gameDirectory);
        // For balanced teams, joining both assigned sides is the in-game ready action.
        // Keep manual readiness for uneven teams so the smaller side cannot start
        // the game while rostered players are still joining the larger side.
        return new {
            team1 = new { name = "CT", players = ct },
            team2 = new { name = "T", players = t },
            num_maps = maps.Length, maplist = maps, map_sides = mapSides, clinch_series = true,
            players_per_team = Math.Min(ct.Count, t.Count),
            min_players_to_ready = Math.Min(ct.Count, t.Count),
            cvars = new Dictionary<string, string> {
                ["matchzy_ready_mode"] = !manualReady && ct.Count == t.Count ? "1" : "0",
                ["mp_overtime_enable"] = overtime ? "1" : "0"
            }
        };
    }
}
