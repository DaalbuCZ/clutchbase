using System.Security.Cryptography;
using System.Collections;
using System.Reflection;

namespace ClutchbaseAccess;

public sealed record LiveMapScore(string SetupId, string Map, int Ct, int T, long ObservedAt);

public static class RuntimeEvidence
{
    // This adapter is pinned to original MatchZy 0.9.2's public runtime fields.
    // Unknown layouts/versions must not produce successful evidence.
    public static bool Verify(object? plugin, string state, string version, byte[] roster, string expectedHash)
    {
        if (plugin == null || state != "Loaded" || version != "0.9.2" ||
            plugin.GetType().FullName != "MatchZy.MatchZy" ||
            plugin.GetType().GetField("isWhitelistRequired")?.GetValue(plugin) is not true)
            return false;

        object? everyone = plugin.GetType().GetField("everyoneIsAdmin")?.GetValue(plugin);
        if (everyone?.GetType().GetProperty("Value")?.GetValue(everyone) is not false)
            return false;

        return Convert.ToHexStringLower(SHA256.HashData(roster)) == expectedHash;
    }
    public static string Mode(object? plugin, string state, string version)
    {
        if (plugin == null || state != "Loaded" || version != "0.9.2" || plugin.GetType().FullName != "MatchZy.MatchZy") return "";
        bool? Read(string name) => plugin.GetType().GetField(name)?.GetValue(plugin) as bool?;
        if (Read("isSleep") != false) return "";
        if (Read("isPractice") == true) return "practice";
        if (Read("isPractice") != false) return "";
        return Read("isWarmup") == true || Read("isMatchLive") == true || Read("isKnifeRound") == true || Read("isSideSelectionPhase") == true || Read("matchStarted") == true ? "match" : "";
    }

    public static bool CanSwitchMode(object? plugin,string state,string version)
    {
        if(plugin==null||state!="Loaded"||version!="0.9.2"||plugin.GetType().FullName!="MatchZy.MatchZy")return false;
        bool? Read(string name)=>plugin.GetType().GetField(name)?.GetValue(plugin) as bool?;
        return Mode(plugin,state,version)!=""&&Read("matchStarted")==false;
    }

    public static bool CanLoadMatch(object? plugin,string state,string version)
    {
        if(plugin==null||state!="Loaded"||version!="0.9.2"||plugin.GetType().FullName!="MatchZy.MatchZy")return false;
        bool? Read(string name)=>plugin.GetType().GetField(name)?.GetValue(plugin) as bool?;
        return Mode(plugin,state,version)=="match"&&Read("matchStarted")==false&&Read("isMatchSetup")==false;
    }

    public static string MatchPhase(object? plugin,string state,string version)
    {
        if(plugin==null||state!="Loaded"||version!="0.9.2"||plugin.GetType().FullName!="MatchZy.MatchZy")return "unknown";
        bool? Read(string name)=>plugin.GetType().GetField(name)?.GetValue(plugin) as bool?;
        bool? setup=Read("isMatchSetup"), started=Read("matchStarted"), live=Read("isMatchLive"), paused=Read("isPaused");
        if(setup==null||started==null||live==null||paused==null)return "unknown";
        if(setup==false)return "idle";
        if(started==false||live==false)return "setup";
        return paused==true?"paused":"live";
    }

    public static string ActiveMatchSetupId(object? plugin, string state, string version)
    {
        if (plugin == null || state != "Loaded" || version != "0.9.2" || plugin.GetType().FullName != "MatchZy.MatchZy" ||
            plugin.GetType().GetField("isMatchSetup")?.GetValue(plugin) is not true) return "";
        string path = plugin.GetType().GetField("loadedConfigFile")?.GetValue(plugin) as string ?? "";
        const string prefix = "ClutchbaseAccess/match-", suffix = ".json";
        if (!path.StartsWith(prefix, StringComparison.Ordinal) || !path.EndsWith(suffix, StringComparison.Ordinal)) return "";
        string id = path[prefix.Length..^suffix.Length];
        return ValidId(id) ? id : "";
    }
    public static bool ValidId(string id) => id.Length == 26 && id.All(c => (c >= 'A' && c <= 'Z') || (c >= '2' && c <= '7'));
    public static bool MatchUnloaded(object? plugin, string state, string version) =>
        plugin != null && state == "Loaded" && version == "0.9.2" && plugin.GetType().FullName == "MatchZy.MatchZy" &&
        plugin.GetType().GetField("isMatchSetup")?.GetValue(plugin) is false;

    public static FinishedResult? Finished(object? plugin, string state, string version, string liveSetup, int ctScore, int tScore, string map, bool series = false)
    {
        string setup = ActiveMatchSetupId(plugin, state, version);
        if (!MapPolicy.IsOfficial(map) || setup == "" || setup != liveSetup || plugin?.GetType().GetField("seriesEnded")?.GetValue(plugin) is not true ||
            plugin.GetType().GetField("currentMapFinished", BindingFlags.Instance | BindingFlags.NonPublic)?.GetValue(plugin) is not true ||
            ctScore is < 0 or > 99 || tScore is < 0 or > 99) return null;
        object? team1 = plugin.GetType().GetField("matchzyTeam1")?.GetValue(plugin);
        object? team2 = plugin.GetType().GetField("matchzyTeam2")?.GetValue(plugin);
        if (team1 == null || team2 == null || plugin.GetType().GetField("teamSides")?.GetValue(plugin) is not IDictionary sides) return null;
        if (series)
        {
            if (team1.GetType().GetField("seriesScore")?.GetValue(team1) is not int score1 || team2.GetType().GetField("seriesScore")?.GetValue(team2) is not int score2 || score1 is < 0 or > 5 || score2 is < 0 or > 5) return null;
            return new FinishedResult(setup, ctScore, tScore, Team1Score: score1, Team2Score: score2);
        }
        string? side1 = sides[team1] as string, side2 = sides[team2] as string;
        if (side1 == "CT" && side2 == "TERRORIST") return new FinishedResult(setup, ctScore, tScore);
        if (side1 == "TERRORIST" && side2 == "CT") return new FinishedResult(setup, tScore, ctScore);
        return null;
    }

    public static LiveMapScore? LiveScore(object? plugin, string state, string version, string map, int ct, int t, long observedAt)
    {
        string phase = MatchPhase(plugin, state, version);
        string setup = ActiveMatchSetupId(plugin, state, version);
        if ((phase != "live" && phase != "paused") || setup == "" || observedAt <= 0 || ct is < 0 or > 99 || t is < 0 or > 99 ||
            plugin?.GetType().GetField("seriesEnded")?.GetValue(plugin) is not false ||
            plugin.GetType().GetField("currentMapFinished", BindingFlags.Instance | BindingFlags.NonPublic)?.GetValue(plugin) is not false ||
            !MapPolicy.IsOfficial(map)) return null;
        return new LiveMapScore(setup, map, ct, t, observedAt);
    }

    public static bool CanPauseMatch(object? plugin,string state,string version)=>MatchPhase(plugin,state,version)=="live";
    public static bool CanResumeMatch(object? plugin,string state,string version)=>MatchPhase(plugin,state,version)=="paused";

}
