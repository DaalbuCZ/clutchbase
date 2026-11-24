using System.Security.Cryptography;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;
using ClutchbaseAccess;

var roster = Encoding.UTF8.GetBytes("76561198154367261\n");
var hash = Convert.ToHexStringLower(SHA256.HashData(roster));
var plugin = new MatchZy.MatchZy();
void Check(bool condition) { if (!condition) throw new Exception("Runtime evidence assertion failed"); }
var onePlayer = new Dictionary<string, string> { ["76561198154367261"] = "One" };
var twoPlayers = new Dictionary<string, string> { ["76561198154367262"] = "Two", ["76561198154367263"] = "Three" };
using (var config = JsonDocument.Parse(JsonSerializer.Serialize(MatchConfigBuilder.Build("de_mirage", ["de_mirage"], ["team1_ct"], 5, onePlayer, twoPlayers, false)))) {
    Check(config.RootElement.GetProperty("min_players_to_ready").GetInt32() == 1);
    Check(config.RootElement.GetProperty("players_per_team").GetInt32() == 1);
    Check(config.RootElement.GetProperty("cvars").GetProperty("matchzy_ready_mode").GetString() == "0");
    Check(config.RootElement.GetProperty("cvars").GetProperty("mp_overtime_enable").GetString() == "0");
    Check(config.RootElement.GetProperty("team1").GetProperty("players").GetProperty("76561198154367261").GetString() == "One");
}
using (var config = JsonDocument.Parse(JsonSerializer.Serialize(MatchConfigBuilder.Build("de_mirage", ["de_mirage", "de_nuke", "de_inferno"], ["team1_ct", "team1_t", "team1_ct"], 2, twoPlayers, twoPlayers, true)))) {
    Check(config.RootElement.GetProperty("min_players_to_ready").GetInt32() == 2);
    Check(config.RootElement.GetProperty("cvars").GetProperty("matchzy_ready_mode").GetString() == "1");
    Check(config.RootElement.GetProperty("cvars").GetProperty("mp_overtime_enable").GetString() == "1");
    Check(config.RootElement.GetProperty("num_maps").GetInt32() == 3);
    Check(config.RootElement.GetProperty("maplist").GetArrayLength() == 3);
    Check(config.RootElement.GetProperty("map_sides")[1].GetString() == "team1_t");
    Check(config.RootElement.GetProperty("clinch_series").GetBoolean());
}
using (var config = JsonDocument.Parse(JsonSerializer.Serialize(MatchConfigBuilder.Build("de_mirage", ["de_mirage"], ["team1_ct"], 2, twoPlayers, twoPlayers, false, true)))) {
    Check(config.RootElement.GetProperty("cvars").GetProperty("matchzy_ready_mode").GetString() == "0");
}
try { MatchConfigBuilder.Build("de_mirage", ["de_mirage", "de_nuke"], ["team1_ct"], 1, onePlayer, new(), false); throw new Exception("Invalid series accepted"); } catch (ArgumentException) { }
try { MatchConfigBuilder.Build("de_mirage", ["de_mirage"], ["team1_ct"], 1, onePlayer, new(), false); throw new Exception("Empty side accepted"); } catch (ArgumentException) { }
try { MatchConfigBuilder.Build("de_mirage", ["de_mirage"], ["team1_ct"], 1, onePlayer, twoPlayers, false); throw new Exception("Oversized side accepted"); } catch (ArgumentException) { }
const string workshop = "workshop/1234567890/aim_test";
Check(MapPolicy.IsWorkshop(workshop));
Check(MapPolicy.IsWorkshop("workshop/99999999999999999999/" + new string('a', 128)));
foreach (string invalid in new[] { "workshop/0/aim", "workshop/01/aim", "workshop/123/../aim", "workshop/123/aim.bsp", "workshop/123/Aim", "workshop/123/aim;quit", "workshop/123/aim\n", "workshop/１２３/aim", "workshop/123/aim name", "workshop/123/aim\\name", "/workshop/123/aim", "workshop/123/aim/extra", "workshop/123/.", "workshop/123/..", "workshop/123/" + new string('a',129), "workshop/123456789012345678901/aim", "1234567890", "aim_test" })
    Check(!MapPolicy.IsIdentifier(invalid));
Check(!MapPolicy.IsIdentifier(null));
string mapDirectory = Path.Join(Path.GetTempPath(), "clutchbase-maps-" + Guid.NewGuid().ToString("N"));
string game = Path.Join(mapDirectory, "game");
string installed = Path.Join(game, "csgo/maps", workshop + ".bsp");
void RejectMap(Action action, bool capability = false) {
    try { action(); throw new Exception("Unsupported map accepted"); }
    catch (NotSupportedException e) { Check(capability && e.Message == MapPolicy.WorkshopBlocker); }
    catch (ArgumentException) { Check(!capability); }
}
try {
    Check(!MapPolicy.HasInstalledWorkshopMap(game, workshop));
    Directory.CreateDirectory(Path.GetDirectoryName(installed)!);
    File.WriteAllText(installed, "fixture only; not a CS2 map");
    Check(MapPolicy.HasInstalledWorkshopMap(game, workshop));
    Check(!MapPolicy.HasInstalledWorkshopMap(null, workshop));
    Check(!MapPolicy.HasInstalledWorkshopMap("relative", workshop));
    Check(!MapPolicy.HasInstalledWorkshopMap(game, "workshop/1234567890/other"));
    RejectMap(() => MatchConfigBuilder.Build(workshop, [workshop], ["team1_ct"], 1, onePlayer, onePlayer, false));
    RejectMap(() => MatchConfigBuilder.Build(workshop, [workshop], ["team1_ct"], 1, onePlayer, onePlayer, false, gameDirectory: game), true);
    RejectMap(() => MatchConfigBuilder.Build("de_mirage", ["de_mirage", workshop, "de_nuke"], ["team1_ct", "team1_t", "team1_ct"], 1, onePlayer, onePlayer, false, gameDirectory: game), true);
    RejectMap(() => MatchConfigBuilder.Build("de_mirage", ["de_mirage", "de_nuke", "de_train", "de_inferno", workshop], ["team1_ct", "team1_t", "team1_ct", "team1_t", "team1_ct"], 1, onePlayer, onePlayer, false, gameDirectory: game), true);
    RejectMap(() => MatchConfigBuilder.Build(workshop, [workshop, workshop, "de_mirage"], ["team1_ct", "team1_t", "team1_ct"], 1, onePlayer, onePlayer, false, gameDirectory: game));
    File.Delete(installed);
    RejectMap(() => MatchConfigBuilder.Build(workshop, [workshop], ["team1_ct"], 1, onePlayer, onePlayer, false, gameDirectory: game));
    File.WriteAllText(Path.Join(mapDirectory,"outside.bsp"), "fixture");
    File.CreateSymbolicLink(installed, Path.Join(mapDirectory,"outside.bsp"));
    Check(!MapPolicy.HasInstalledWorkshopMap(game, workshop));
    File.Delete(installed);
    Directory.CreateDirectory(installed);
    Check(!MapPolicy.HasInstalledWorkshopMap(game, workshop));
    Directory.Delete(installed);
    Check(NativeProcess.MakeFifo(installed, 0x180) == 0);
    Check(!MapPolicy.HasInstalledWorkshopMap(game, workshop));
    File.Delete(installed);
    File.WriteAllText(installed, "fixture");
    foreach (string component in new[] { "game", "game/csgo", "game/csgo/maps", "game/csgo/maps/workshop", "game/csgo/maps/workshop/1234567890" }) {
        string original = Path.Join(mapDirectory, component), moved = original + "-real";
        Directory.Move(original, moved);
        Directory.CreateSymbolicLink(original, moved);
        Check(!MapPolicy.HasInstalledWorkshopMap(game, workshop));
        Directory.Delete(original);
        Directory.Move(moved, original);
    }
    Check(MapPolicy.HasInstalledWorkshopMap(game, workshop));
} finally { Directory.Delete(mapDirectory, true); }
Console.WriteLine("Workshop identifier, installed-file and capability checks passed.");
Check(RuntimeEvidence.Verify(plugin, "Loaded", "0.9.2", roster, hash));
Check(!RuntimeEvidence.Verify(plugin, "Unloaded", "0.9.2", roster, hash));
Check(!RuntimeEvidence.Verify(plugin, "Loaded", "0.9.3", roster, hash));
Check(!RuntimeEvidence.Verify(null, "Loaded", "0.9.2", roster, hash));
Check(!RuntimeEvidence.Verify(new object(), "Loaded", "0.9.2", roster, hash));
Check(!RuntimeEvidence.Verify(plugin, "Loaded", "0.9.2", [], hash));
plugin.isWhitelistRequired = false;
Check(!RuntimeEvidence.Verify(plugin, "Loaded", "0.9.2", roster, hash));
plugin.isWhitelistRequired = true;
plugin.everyoneIsAdmin.Value = true;
Check(!RuntimeEvidence.Verify(plugin, "Loaded", "0.9.2", roster, hash));
Check(RuntimeEvidence.Mode(plugin, "Loaded", "0.9.2") == "match");
Check(RuntimeEvidence.CanSwitchMode(plugin,"Loaded","0.9.2"));
Check(RuntimeEvidence.CanLoadMatch(plugin,"Loaded","0.9.2"));
Check(RuntimeEvidence.MatchPhase(plugin,"Loaded","0.9.2")=="idle");
plugin.isMatchSetup=true;
plugin.loadedConfigFile="ClutchbaseAccess/match-ABCDEFGHIJKLMNOPQRSTUVWX23.json";
Check(RuntimeEvidence.ActiveMatchSetupId(plugin,"Loaded","0.9.2")=="ABCDEFGHIJKLMNOPQRSTUVWX23");
Check(MatchControlFence.Matches(true, "ABCDEFGHIJKLMNOPQRSTUVWX23", "ABCDEFGHIJKLMNOPQRSTUVWX23"));
Check(!MatchControlFence.Matches(true, "ABCDEFGHIJKLMNOPQRSTUVWX23", "ABCDEFGHIJKLMNOPQRSTUVWX24"));
Check(!MatchControlFence.Matches(false, "ABCDEFGHIJKLMNOPQRSTUVWX23", "ABCDEFGHIJKLMNOPQRSTUVWX23"));
Check(!MatchControlFence.Matches(true, "", ""));
plugin.seriesEnded = true;
plugin.SetMapFinished(true);
plugin.teamSides[plugin.matchzyTeam1] = "CT"; plugin.teamSides[plugin.matchzyTeam2] = "TERRORIST";
Check(RuntimeEvidence.Finished(plugin,"Loaded","0.9.2","",13,9,"de_mirage")==null);
var observed = RuntimeEvidence.Finished(plugin,"Loaded","0.9.2","ABCDEFGHIJKLMNOPQRSTUVWX23",13,9,"de_mirage");
Check(observed == new FinishedResult("ABCDEFGHIJKLMNOPQRSTUVWX23",13,9));
((MatchZy.FakeTeam)plugin.matchzyTeam1).seriesScore = 2; ((MatchZy.FakeTeam)plugin.matchzyTeam2).seriesScore = 1;
Check(RuntimeEvidence.Finished(plugin,"Loaded","0.9.2","ABCDEFGHIJKLMNOPQRSTUVWX23",13,9,"de_mirage",true) == new FinishedResult("ABCDEFGHIJKLMNOPQRSTUVWX23",13,9,Team1Score:2,Team2Score:1));
plugin.teamSides[plugin.matchzyTeam1] = "TERRORIST"; plugin.teamSides[plugin.matchzyTeam2] = "CT";
Check(RuntimeEvidence.Finished(plugin,"Loaded","0.9.2","ABCDEFGHIJKLMNOPQRSTUVWX23",13,9,"de_mirage") == new FinishedResult("ABCDEFGHIJKLMNOPQRSTUVWX23",9,13));
Check(RuntimeEvidence.Finished(plugin,"Loaded","0.9.3","ABCDEFGHIJKLMNOPQRSTUVWX23",13,9,"de_mirage")==null);
Check(RuntimeEvidence.Finished(plugin,"Loaded","0.9.2","ABCDEFGHIJKLMNOPQRSTUVWX23",100,9,"de_mirage")==null);
plugin.SetMapFinished(false);
Check(RuntimeEvidence.Finished(plugin,"Loaded","0.9.2","ABCDEFGHIJKLMNOPQRSTUVWX23",13,9,"de_mirage")==null);
plugin.SetMapFinished(true);
plugin.seriesEnded = false;
Check(RuntimeEvidence.Finished(plugin,"Loaded","0.9.2","ABCDEFGHIJKLMNOPQRSTUVWX23",13,9,"de_mirage")==null);
Check(RuntimeEvidence.ActiveMatchSetupId(plugin,"Loaded","0.9.3")=="");
plugin.loadedConfigFile="../match-ABCDEFGHIJKLMNOPQRSTUVWX23.json";
Check(RuntimeEvidence.ActiveMatchSetupId(plugin,"Loaded","0.9.2")=="");
Check(!RuntimeEvidence.MatchUnloaded(plugin,"Loaded","0.9.2"));
plugin.isMatchSetup=false; Check(RuntimeEvidence.MatchUnloaded(plugin,"Loaded","0.9.2"));
plugin.isMatchSetup=true;plugin.matchStarted=true;plugin.isMatchLive=true;
Check(RuntimeEvidence.MatchPhase(plugin,"Loaded","0.9.2")=="live"&&RuntimeEvidence.CanPauseMatch(plugin,"Loaded","0.9.2"));
plugin.loadedConfigFile="ClutchbaseAccess/match-ABCDEFGHIJKLMNOPQRSTUVWX23.json";
plugin.seriesEnded=false; plugin.SetMapFinished(false);
Check(RuntimeEvidence.LiveScore(plugin,"Loaded","0.9.2","de_mirage",8,5,123) == new LiveMapScore("ABCDEFGHIJKLMNOPQRSTUVWX23","de_mirage",8,5,123));
Check(RuntimeEvidence.LiveScore(plugin,"Loaded","0.9.3","de_mirage",8,5,123) == null);
Check(RuntimeEvidence.LiveScore(plugin,"Loaded","0.9.2","de_mirage",100,5,123) == null);
Check(RuntimeEvidence.LiveScore(plugin,"Loaded","0.9.2","workshop/unknown",8,5,123) == null);
foreach (string unsupported in new[] { workshop, "1234567890", "aim_test", "workshop/1234567890/de_mirage" }) {
    Check(RuntimeEvidence.LiveScore(plugin,"Loaded","0.9.2",unsupported,8,5,123) == null);
    plugin.seriesEnded=true; plugin.SetMapFinished(true);
    Check(RuntimeEvidence.Finished(plugin,"Loaded","0.9.2","ABCDEFGHIJKLMNOPQRSTUVWX23",13,9,unsupported) == null);
    plugin.seriesEnded=false; plugin.SetMapFinished(false);
}
plugin.SetMapFinished(true);
Check(RuntimeEvidence.LiveScore(plugin,"Loaded","0.9.2","de_mirage",8,5,123) == null);
plugin.SetMapFinished(false);
plugin.isPaused=true;
Check(RuntimeEvidence.LiveScore(plugin,"Loaded","0.9.2","de_mirage",8,5,123) != null);
plugin.isPaused=false;
plugin.isPaused=true;
Check(RuntimeEvidence.MatchPhase(plugin,"Loaded","0.9.2")=="paused"&&RuntimeEvidence.CanResumeMatch(plugin,"Loaded","0.9.2"));
plugin.isMatchLive=false;
Check(RuntimeEvidence.MatchPhase(plugin,"Loaded","0.9.2")=="setup"&&!RuntimeEvidence.CanResumeMatch(plugin,"Loaded","0.9.2"));
Check(RuntimeEvidence.MatchPhase(plugin,"Loaded","0.9.3")=="unknown"&&!RuntimeEvidence.CanPauseMatch(plugin,"Loaded","0.9.3"));
plugin.isPaused=false;plugin.isMatchLive=false;plugin.matchStarted=false;plugin.isMatchSetup=false;
plugin.isMatchSetup=true;
Check(!RuntimeEvidence.CanLoadMatch(plugin,"Loaded","0.9.2"));
plugin.isMatchSetup=false;
plugin.matchStarted=true;
Check(!RuntimeEvidence.CanSwitchMode(plugin,"Loaded","0.9.2"));
plugin.matchStarted=false;
plugin.isWarmup = false;
Check(RuntimeEvidence.Mode(plugin, "Loaded", "0.9.2") == "");
plugin.isPractice = true;
Check(RuntimeEvidence.Mode(plugin, "Loaded", "0.9.2") == "practice");
plugin.isSleep = true;
Check(RuntimeEvidence.Mode(plugin, "Loaded", "0.9.2") == "");
Check(RuntimeEvidence.Mode(plugin, "Loaded", "0.9.3") == "");
Check(RuntimeEvidence.Mode(plugin, "Unloaded", "0.9.2") == "");
Check(!RuntimeEvidence.CanSwitchMode(plugin,"Unloaded","0.9.2"));
Console.WriteLine("Runtime evidence checks passed.");

string fenceDirectory = Path.Join(Path.GetTempPath(), "clutchbase-fence-" + Guid.NewGuid().ToString("N"));
Directory.CreateDirectory(fenceDirectory);
try {
    const string cancelled = "ABCDEFGHIJKLMNOPQRSTUVWX23", issued = "BCDEFGHIJKLMNOPQRSTUVWXY23", legacy = "CDEFGHIJKLMNOPQRSTUVWXYZ23";
    const string launchOne = "DEFGHIJKLMNOPQRSTUVWXYZA23", launchTwo = "EFGHIJKLMNOPQRSTUVWXYZAB23";
    FinishedResultJournal.Write(fenceDirectory, observed!);
    FinishedResultJournal.Write(fenceDirectory, new FinishedResult(cancelled, 3, 4));
    using (var saved = JsonDocument.Parse(File.ReadAllText(Path.Join(fenceDirectory, "result-finished-" + cancelled + ".json")))) {
        Check(saved.RootElement.GetProperty("setupId").GetString() == cancelled);
        Check(saved.RootElement.GetProperty("ct").GetInt32() == 13 && saved.RootElement.GetProperty("t").GetInt32() == 9);
    }
    var playerResult = new FinishedResult(issued, 13, 9, [new ReportedPlayer("76561198154367261", 12, 8, 4)]);
    FinishedResultJournal.Write(fenceDirectory, playerResult);
    using (var saved = JsonDocument.Parse(File.ReadAllText(Path.Join(fenceDirectory, "result-finished-" + issued + ".json")))) {
        var player = saved.RootElement.GetProperty("players")[0];
        Check(player.GetProperty("steamId").GetString() == "76561198154367261" && player.GetProperty("kills").GetInt32() == 12);
        Check(saved.RootElement.GetProperty("ct").GetInt32() == -1 && saved.RootElement.GetProperty("t").GetInt32() == -1);
        Check(saved.RootElement.GetProperty("scoreCt").GetInt32() == 13 && saved.RootElement.GetProperty("scoreT").GetInt32() == 9);
    }
    try { FinishedResultJournal.Write(fenceDirectory, new FinishedResult(legacy, 13, 9, [new ReportedPlayer("invalid", 1, 1, 1)])); throw new Exception("Invalid player accepted"); }
    catch (ArgumentException) { }
    Check(MatchFence.IsInProcessGroup(NativeProcess.GetProcessGroup()));
    Check(!MatchFence.IsInProcessGroup(0));
    Check(MatchFence.Cancel(fenceDirectory, cancelled, launchOne));
    Check(MatchFence.Cancel(fenceDirectory, cancelled, launchOne));
    Check(MatchFence.Claim(fenceDirectory, cancelled, launchTwo) == MatchFence.ClaimResult.Cancelled);
    Check(MatchFence.Claim(fenceDirectory, issued, launchOne) == MatchFence.ClaimResult.Claimed);
    Check(File.ReadAllText(Path.Join(fenceDirectory, "match-issued-" + issued)) == launchOne);
    Check(MatchFence.Claim(fenceDirectory, issued, launchOne) == MatchFence.ClaimResult.AlreadyIssued);
    Check(!MatchFence.Cancel(fenceDirectory, issued, launchOne));
    Check(MatchFence.Cancel(fenceDirectory, issued, launchTwo));
    Check(MatchFence.Cancel(fenceDirectory, issued, launchTwo));
    Check(MatchFence.Claim(fenceDirectory, issued, launchTwo) == MatchFence.ClaimResult.Cancelled);
    Check(MatchFence.Claim(fenceDirectory, issued, launchOne) == MatchFence.ClaimResult.Cancelled);
    File.WriteAllText(Path.Join(fenceDirectory, "match-issued-" + legacy), "");
    Check(!MatchFence.Cancel(fenceDirectory, legacy, launchTwo));
    try { MatchFence.Cancel(fenceDirectory, "../unsafe", launchOne); throw new Exception("Unsafe ID accepted"); }
    catch (ArgumentException) { }
    try { MatchFence.Claim(fenceDirectory, issued, "invalid"); throw new Exception("Unsafe nonce accepted"); }
    catch (ArgumentException) { }
} finally { Directory.Delete(fenceDirectory, true); }
Console.WriteLine("Match fence checks passed.");

static class NativeProcess
{
    [DllImport("libc", EntryPoint="mkfifo")]
    public static extern int MakeFifo(string path, uint mode);
    [DllImport("libc", EntryPoint="getpgrp")]
    public static extern int GetProcessGroup();
}

namespace MatchZy
{
    public sealed class MatchZy
    {
        public bool isWhitelistRequired = true;
        public bool isPractice = false, isSleep = false, isWarmup = true, isMatchLive = false, isKnifeRound = false, isSideSelectionPhase = false, matchStarted = false, isMatchSetup = false, isPaused = false;
        public string loadedConfigFile = "";
        public bool seriesEnded = false;
        private bool currentMapFinished = false;
        public void SetMapFinished(bool value) => currentMapFinished = value;
        public object matchzyTeam1 = new FakeTeam(), matchzyTeam2 = new FakeTeam();
        public Dictionary<object, string> teamSides = new();
        public FakeBool everyoneIsAdmin = new();
    }
    public sealed class FakeTeam { public int seriesScore; }
    public sealed class FakeBool { public bool Value { get; set; } }
}
