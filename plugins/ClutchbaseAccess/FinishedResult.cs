using System.Text.Json;

namespace ClutchbaseAccess;

public sealed record ReportedPlayer(string SteamId, int Kills, int Deaths, int Assists);
public sealed record FinishedResult(string SetupId, int Ct, int T, ReportedPlayer[]? Players = null, int? Team1Score = null, int? Team2Score = null);

public static class FinishedResultJournal
{
    private static readonly JsonSerializerOptions Json = new() { PropertyNamingPolicy = JsonNamingPolicy.CamelCase };

    public static void Write(string directory, FinishedResult result)
    {
        if (!OperatingSystem.IsLinux()) throw new PlatformNotSupportedException("Match results require Linux");
        if (!RuntimeEvidence.ValidId(result.SetupId) || result.Ct is < 0 or > 99 || result.T is < 0 or > 99 || (result.Team1Score is null) != (result.Team2Score is null) || result.Team1Score is < 0 or > 5 || result.Team2Score is < 0 or > 5 ||
            (result.Players?.Length ?? 0) > 10 || (result.Players?.Any(p => p.SteamId.Length != 17 || !p.SteamId.All(char.IsAsciiDigit) || p.Kills is < 0 or > 999 || p.Deaths is < 0 or > 999 || p.Assists is < 0 or > 999) ?? false)) throw new ArgumentException("Invalid match result");
        string path = Path.Join(directory, "result-finished-" + result.SetupId + ".json");
        if (File.Exists(path)) { MatchFence.SyncDirectory(directory); return; }
        string temp = Path.Join(directory, ".result-" + Guid.NewGuid().ToString("N"));
        try {
            using (var file = new FileStream(temp, new FileStreamOptions { Mode = FileMode.CreateNew, Access = FileAccess.Write, UnixCreateMode = UnixFileMode.UserRead | UnixFileMode.UserWrite })) {
                // Older agents reject negative scores instead of acknowledging a partial record.
                if (result.Players is { Length: > 0 })
                    JsonSerializer.Serialize(file, new { result.SetupId, Ct = -1, T = -1, ScoreCt = result.Ct, ScoreT = result.T, result.Team1Score, result.Team2Score, result.Players }, Json);
                else
                    JsonSerializer.Serialize(file, result, Json);
                file.Flush(true);
            }
            File.Move(temp, path, false);
            MatchFence.SyncDirectory(directory);
        } finally { if (File.Exists(temp)) File.Delete(temp); }
    }
}
