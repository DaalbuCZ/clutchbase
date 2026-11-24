using System.Text.Json;
using System.Runtime.Versioning;
using System.Reflection;
using CounterStrikeSharp.API;
using CounterStrikeSharp.API.Core;
using CounterStrikeSharp.API.Core.Attributes;
using CounterStrikeSharp.API.Core.Plugin.Host;
using CounterStrikeSharp.API.Modules.Timers;

namespace ClutchbaseAccess;

[MinimumApiVersion(378)]
[SupportedOSPlatform("linux")]
public sealed class ClutchbaseAccess : BasePlugin
{
    public override string ModuleName => "Clutchbase Access";
    public override string ModuleVersion => "0.3.15";
    public override string ModuleAuthor => "Clutchbase";
    public override string ModuleDescription => "Local runtime whitelist evidence; keeps password fallback.";

    private static readonly JsonSerializerOptions Json = new() { PropertyNamingPolicy = JsonNamingPolicy.CamelCase };
    private string directory = "";
    private string handledModeID = "";
    private string pendingModeID = "";
    private string pendingMode = "";
    private DateTimeOffset pendingModeAt;
    private string pendingMatchID = "";
    private string pendingMatchFile = "";
    private DateTimeOffset pendingMatchAt;
    private string pendingControlID = "";
    private DateTimeOffset pendingControlAt;
    private string liveSetupID = "";

    public override void Load(bool hotReload)
    {
        directory = Path.Join(Server.GameDirectory, "csgo", "cfg", "ClutchbaseAccess");
        AddTimer(1.0f, Publish, TimerFlags.REPEAT);
    }

    private void Publish()
    {
        string? temporary = null;
        try
        {
            string requestPath = Path.Join(directory, "request.json");
            if (!File.Exists(requestPath) || new FileInfo(requestPath).Length > 8192) return;
            var request = JsonSerializer.Deserialize<AccessRequest>(File.ReadAllText(requestPath), Json);
            if (request == null || !RuntimeEvidence.ValidId(request.Nonce) || !MatchFence.IsInProcessGroup(request.ProcessGroup) || request.RosterHash.Length != 64) return;

            string rosterPath = Path.Join(Server.GameDirectory, "csgo", "cfg", "MatchZy", "whitelist.cfg");
            byte[] roster = File.Exists(rosterPath) && new FileInfo(rosterPath).Length <= 8192
                ? File.ReadAllBytes(rosterPath) : [];
            // CSS 378's plugin DI scope does not expose IPluginManager. Keep
            // this adapter pinned; an unknown host layout must not verify.
            var plugins = Api.GetVersion() == 378
                ? typeof(Application).GetField("_pluginManager", BindingFlags.Instance | BindingFlags.NonPublic)?.GetValue(Application.Instance) as IPluginManager
                : null;
            var matchzy = plugins?.GetLoadedPlugins().SingleOrDefault(p => p.Plugin?.ModuleName == "MatchZy");
            bool verified = RuntimeEvidence.Verify(matchzy?.Plugin, matchzy?.State.ToString() ?? "",
                matchzy?.Plugin?.ModuleVersion ?? "", roster, request.RosterHash);
            ProcessModeCommand(matchzy?.Plugin,matchzy?.State.ToString()??"",matchzy?.Plugin?.ModuleVersion??"",verified);
            ProcessMatchCancellation(matchzy?.Plugin,matchzy?.State.ToString()??"",matchzy?.Plugin?.ModuleVersion??"",verified,request.Nonce);
            ProcessMatchCommand(matchzy?.Plugin,matchzy?.State.ToString()??"",matchzy?.Plugin?.ModuleVersion??"",verified,request.Nonce);
            ProcessMatchControl(matchzy?.Plugin,matchzy?.State.ToString()??"",matchzy?.Plugin?.ModuleVersion??"",verified);
            ModeResult? modeResult = null;
            string resultPath = Path.Join(directory, "mode-result.json");
            if (File.Exists(resultPath) && new FileInfo(resultPath).Length <= 2048)
                modeResult = JsonSerializer.Deserialize<ModeResult>(File.ReadAllText(resultPath), Json);
            MatchSetupResult? matchResult = null;
            string matchResultPath = Path.Join(directory, "match-result.json");
            if (File.Exists(matchResultPath) && new FileInfo(matchResultPath).Length <= 2048)
                matchResult = JsonSerializer.Deserialize<MatchSetupResult>(File.ReadAllText(matchResultPath), Json);
            MatchControlResult? controlResult = null;
            string controlResultPath = Path.Join(directory, "match-control-result.json");
            if (File.Exists(controlResultPath) && new FileInfo(controlResultPath).Length <= 2048)
                controlResult = JsonSerializer.Deserialize<MatchControlResult>(File.ReadAllText(controlResultPath), Json);
            MatchCancelResult? cancelResult = null;
            string cancelResultPath = Path.Join(directory, "match-cancel-result.json");
            if (File.Exists(cancelResultPath) && new FileInfo(cancelResultPath).Length <= 2048)
                cancelResult = JsonSerializer.Deserialize<MatchCancelResult>(File.ReadAllText(cancelResultPath), Json);
            string phase = verified && RuntimeEvidence.Mode(matchzy?.Plugin,matchzy?.State.ToString()??"",matchzy?.Plugin?.ModuleVersion??"")=="match"
                ? RuntimeEvidence.MatchPhase(matchzy?.Plugin,matchzy?.State.ToString()??"",matchzy?.Plugin?.ModuleVersion??"") : "unknown";
            if (verified) CaptureFinishedMatch(matchzy?.Plugin, matchzy?.State.ToString() ?? "", matchzy?.Plugin?.ModuleVersion ?? "", phase);
            long publishedAt = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
            LiveMapScore? liveScore = null;
            if (verified && (phase == "live" || phase == "paused")) {
                try {
                    int? ct = null, t = null;
                    foreach (var team in Utilities.FindAllEntitiesByDesignerName<CCSTeam>("cs_team_manager")) {
                        if (team.Teamname == "CT") { if (ct != null) throw new InvalidOperationException(); ct = team.Score; }
                        if (team.Teamname == "TERRORIST") { if (t != null) throw new InvalidOperationException(); t = team.Score; }
                    }
                    if (ct != null && t != null) liveScore = RuntimeEvidence.LiveScore(matchzy?.Plugin, matchzy?.State.ToString() ?? "", matchzy?.Plugin?.ModuleVersion ?? "", Server.MapName, ct.Value, t.Value, publishedAt);
                } catch (Exception) { /* Unavailable scores must not prevent whitelist evidence. */ }
            }
            var evidence = new AccessEvidence(request.Nonce, Convert.ToHexStringLower(System.Security.Cryptography.SHA256.HashData(roster)),
                request.ProcessGroup, Environment.ProcessId, publishedAt, verified ? "verified" : "blocked",
                verified && RuntimeEvidence.CanSwitchMode(matchzy?.Plugin,matchzy?.State.ToString()??"",matchzy?.Plugin?.ModuleVersion??""),
                verified ? RuntimeEvidence.Mode(matchzy?.Plugin, matchzy?.State.ToString() ?? "", matchzy?.Plugin?.ModuleVersion ?? "") : "",
                modeResult?.Id ?? "", modeResult?.Status ?? "", modeResult?.Error ?? "",
                verified && RuntimeEvidence.CanLoadMatch(matchzy?.Plugin,matchzy?.State.ToString()??"",matchzy?.Plugin?.ModuleVersion??""),
                matchResult?.Id ?? "", matchResult?.Status ?? "", matchResult?.Error ?? "",
                phase, (phase=="setup"||phase=="live"||phase=="paused") ? RuntimeEvidence.ActiveMatchSetupId(matchzy?.Plugin,matchzy?.State.ToString()??"",matchzy?.Plugin?.ModuleVersion??"") : "", phase=="live", phase=="paused", controlResult?.Id ?? "", controlResult?.Action ?? "", controlResult?.Status ?? "", controlResult?.Error ?? "",
                verified, cancelResult?.Id ?? "", cancelResult?.SetupId ?? "", cancelResult?.Status ?? "", cancelResult?.Error ?? "", verified && RuntimeEvidence.CanLoadMatch(matchzy?.Plugin,matchzy?.State.ToString()??"",matchzy?.Plugin?.ModuleVersion??""), verified, liveScore,
                verified && RuntimeEvidence.CanLoadMatch(matchzy?.Plugin,matchzy?.State.ToString()??"",matchzy?.Plugin?.ModuleVersion??""));

            temporary = Path.Join(directory, ".evidence-" + Guid.NewGuid().ToString("N"));
            using (var stream = new FileStream(temporary, new FileStreamOptions
            {
                Mode = FileMode.CreateNew, Access = FileAccess.Write,
                UnixCreateMode = UnixFileMode.UserRead | UnixFileMode.UserWrite
            }))
                JsonSerializer.Serialize(stream, evidence, Json);
            File.Move(temporary, Path.Join(directory, "evidence.json"), true);
        }
        catch (Exception)
        {
            // Missing/broken plugins or unreadable files yield no trusted evidence.
            // The independent host watchdog expires the previous report.
        }
        finally
        {
            if (temporary != null)
            {
                try { File.Delete(temporary); } catch (IOException) { }
            }
        }
    }

    private void CaptureFinishedMatch(object? plugin, string state, string version, string phase)
    {
        string active = RuntimeEvidence.ActiveMatchSetupId(plugin, state, version);
        if ((phase == "live" || phase == "paused") && active != "") liveSetupID = active;
        if (active == "" || active != liveSetupID || plugin?.GetType().GetField("seriesEnded")?.GetValue(plugin) is not true) return;
        int? ct = null, t = null;
        foreach (var team in Utilities.FindAllEntitiesByDesignerName<CCSTeam>("cs_team_manager")) {
            if (team.Teamname == "CT") { if (ct != null) return; ct = team.Score; }
            if (team.Teamname == "TERRORIST") { if (t != null) return; t = team.Score; }
        }
        if (ct == null || t == null) return;
        MatchSetupRequest? request;
        try {
            string path = Path.Join(directory, "match-request.json");
            if (!File.Exists(path) || new FileInfo(path).Length > 16384) return;
            request = JsonSerializer.Deserialize<MatchSetupRequest>(File.ReadAllText(path), Json);
        } catch (Exception) { return; }
        if (request?.Id != active || request.CT == null || request.T == null ||
            !MapPolicy.IsOfficial(request.Map) || request.Maps?.Any(map => !MapPolicy.IsOfficial(map)) == true) return;
        bool series = request.Maps?.Length > 1;
        var result = RuntimeEvidence.Finished(plugin, state, version, liveSetupID, ct.Value, t.Value, Server.MapName, series);
        if (result != null) FinishedResultJournal.Write(directory, result with { Players = CapturePlayerStats(active) });
    }

    private ReportedPlayer[]? CapturePlayerStats(string setupId)
    {
        try {
            string path = Path.Join(directory, "match-request.json");
            if (!File.Exists(path) || new FileInfo(path).Length > 16384) return null;
            var request = JsonSerializer.Deserialize<MatchSetupRequest>(File.ReadAllText(path), Json);
            if (request == null || request.Id != setupId || request.CT == null || request.T == null) return null;
            var roster = request.CT.Concat(request.T).Select(p => p.SteamId).ToHashSet(StringComparer.Ordinal);
            if (roster.Count is < 2 or > 10) return null;
            var players = new Dictionary<string, ReportedPlayer>(StringComparer.Ordinal);
            foreach (var player in Utilities.GetPlayers()) {
                try {
                    if (!player.IsValid || player.IsBot || player.IsHLTV || player.Connected != PlayerConnectedState.Connected || player.ActionTrackingServices == null) continue;
                    string steam = player.SteamID.ToString(System.Globalization.CultureInfo.InvariantCulture);
                    if (!roster.Contains(steam) || players.ContainsKey(steam)) continue;
                    var stats = player.ActionTrackingServices.MatchStats;
                    if (stats.Kills is < 0 or > 999 || stats.Deaths is < 0 or > 999 || stats.Assists is < 0 or > 999) continue;
                    players.Add(steam, new ReportedPlayer(steam, stats.Kills, stats.Deaths, stats.Assists));
                } catch (Exception) { /* A missing player does not invalidate the team score. */ }
            }
            return players.Count == 0 ? null : players.Values.OrderBy(p => p.SteamId, StringComparer.Ordinal).ToArray();
        } catch (Exception) { return null; }
    }

    private void ProcessMatchControl(object? plugin,string state,string version,bool accessVerified)
    {
        string path=Path.Join(directory,"match-control-request.json");
        if(!File.Exists(path)||new FileInfo(path).Length>2048)return;
        MatchControlRequest? request;
        try{request=JsonSerializer.Deserialize<MatchControlRequest>(File.ReadAllText(path),Json);}catch(Exception){return;}
        if(request==null||request.Id.Length!=26||request.Id.Any(c=>!(c>='A'&&c<='Z')&&!(c>='2'&&c<='7'))||
           (request.Action!="pause"&&request.Action!="resume"&&request.Action!="end")||
           !RuntimeEvidence.ValidId(request.SetupId ?? "")||Math.Abs(DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()-request.RequestedAt)>20000)return;
        try{string resultPath=Path.Join(directory,"match-control-result.json");if(File.Exists(resultPath)&&new FileInfo(resultPath).Length<=2048){var old=JsonSerializer.Deserialize<MatchControlResult>(File.ReadAllText(resultPath),Json);if(old?.Id==request.Id)return;}}catch(Exception){}
        if(request.Action=="end") { ProcessEndMatch(request, plugin, state, version, accessVerified); return; }
        if (!MatchControlFence.Matches(accessVerified, request.SetupId, RuntimeEvidence.ActiveMatchSetupId(plugin,state,version))) {
            WriteControlResult(request.Id,request.Action,"failed","The loaded MatchZy setup no longer matches this request");pendingControlID="";return;
        }
        string phase=accessVerified&&RuntimeEvidence.Mode(plugin,state,version)=="match"?RuntimeEvidence.MatchPhase(plugin,state,version):"unknown";
        string target=request.Action=="pause"?"paused":"live";
        if(phase==target){WriteControlResult(request.Id,request.Action,"complete","");pendingControlID="";return;}
        if(pendingControlID!=request.Id){
            if((request.Action=="pause"&&phase!="live")||(request.Action=="resume"&&phase!="paused")){
                WriteControlResult(request.Id,request.Action,"failed","MatchZy is not in a live phase that accepts this action");return;
            }
            Server.ExecuteCommand(request.Action=="pause"?"css_forcepause":"css_forceunpause");
            pendingControlID=request.Id;pendingControlAt=DateTimeOffset.UtcNow;return;
        }
        if(DateTimeOffset.UtcNow-pendingControlAt>TimeSpan.FromSeconds(10)){
            WriteControlResult(request.Id,request.Action,"failed","MatchZy did not report the requested pause state");pendingControlID="";
        }
    }

    private void ProcessEndMatch(MatchControlRequest request, object? plugin, string state, string version, bool verified)
    {
        string markerPath = Path.Join(directory, "match-end-issued.json");
        MatchControlRequest? marker = null;
        try { if (File.Exists(markerPath) && new FileInfo(markerPath).Length <= 2048) marker = JsonSerializer.Deserialize<MatchControlRequest>(File.ReadAllText(markerPath), Json); } catch (Exception) { return; }
        bool issued = marker?.Id == request.Id && marker.Action == "end" && marker.SetupId == request.SetupId;
        if (issued && verified && RuntimeEvidence.MatchUnloaded(plugin, state, version)) {
            WriteControlResult(request.Id, request.Action, "complete", ""); pendingControlID=""; return;
        }
        if (!issued) {
            if (!verified || RuntimeEvidence.ActiveMatchSetupId(plugin, state, version) != request.SetupId) {
                WriteControlResult(request.Id, request.Action, "failed", "The loaded MatchZy setup no longer matches this room"); return;
            }
            string temp = Path.Join(directory, ".match-end-" + Guid.NewGuid().ToString("N"));
            try {
                using (var stream = new FileStream(temp, new FileStreamOptions { Mode=FileMode.CreateNew, Access=FileAccess.Write, UnixCreateMode=UnixFileMode.UserRead|UnixFileMode.UserWrite })) JsonSerializer.Serialize(stream, request, Json);
                File.Move(temp, markerPath, true);
            } catch (Exception) { try { File.Delete(temp); } catch (IOException) {} WriteControlResult(request.Id, request.Action, "failed", "Could not record the end request"); return; }
            Server.ExecuteCommand("get5_endmatch");
        }
        if (pendingControlID != request.Id) { pendingControlID=request.Id; pendingControlAt=DateTimeOffset.UtcNow; }
        if (DateTimeOffset.UtcNow-pendingControlAt>TimeSpan.FromSeconds(10)) {
            WriteControlResult(request.Id, request.Action, "failed", "MatchZy did not confirm that the match ended"); pendingControlID="";
        }
    }

    private void WriteControlResult(string id,string action,string status,string error)
    {
        string? temporary=null;
        try{temporary=Path.Join(directory,".match-control-result-"+Guid.NewGuid().ToString("N"));
            using(var stream=new FileStream(temporary,new FileStreamOptions{Mode=FileMode.CreateNew,Access=FileAccess.Write,UnixCreateMode=UnixFileMode.UserRead|UnixFileMode.UserWrite}))
                JsonSerializer.Serialize(stream,new MatchControlResult(id,action,status,error),Json);
            File.Move(temporary,Path.Join(directory,"match-control-result.json"),true);
        }catch(Exception){if(temporary!=null){try{File.Delete(temporary);}catch(IOException){}}}
    }

    private void ProcessMatchCommand(object? plugin,string state,string version,bool accessVerified,string launchNonce)
    {
        string path=Path.Join(directory,"match-request.json");
        if(!File.Exists(path)||new FileInfo(path).Length>16384)return;
        MatchSetupRequest? request;
        try{request=JsonSerializer.Deserialize<MatchSetupRequest>(File.ReadAllText(path),Json);}catch(Exception){return;}
        if(request==null||request.Id.Length!=26||request.Id.Any(c=>!(c>='A'&&c<='Z')&&!(c>='2'&&c<='7'))||
           !MapPolicy.IsIdentifier(request.Map)||request.PlayersPerSide<1||request.PlayersPerSide>5||request.CT.Length<1||request.T.Length<1||
           request.CT.Length>request.PlayersPerSide||request.T.Length>request.PlayersPerSide||
           request.CT.Concat(request.T).Any(p=>p.SteamId.Length!=17||!p.SteamId.All(char.IsAsciiDigit)||p.GameName.Length is <1 or >32||p.GameName.Any(char.IsControl))||
           Math.Abs(DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()-request.RequestedAt)>60000)return;
        string filename="ClutchbaseAccess/match-"+request.Id+".json";
        string resultPath=Path.Join(directory,"match-result.json");
        try{if(File.Exists(resultPath)&&new FileInfo(resultPath).Length<=2048){var old=JsonSerializer.Deserialize<MatchSetupResult>(File.ReadAllText(resultPath),Json);if(old?.Id==request.Id)return;}}catch(Exception){}
        if(pendingMatchID!=request.Id){
            var maps=request.Maps is null || request.Maps.Length == 0 ? new[] { request.Map } : request.Maps;
            var sides=request.MapSides is null || request.MapSides.Length == 0 ? new[] { "team1_ct" } : request.MapSides;
            object config;
            try {
                config=MatchConfigBuilder.Build(request.Map,maps,sides,request.PlayersPerSide,request.CT.ToDictionary(p=>p.SteamId,p=>p.GameName),request.T.ToDictionary(p=>p.SteamId,p=>p.GameName),request.Overtime,request.ManualReady,Server.GameDirectory);
            } catch (NotSupportedException) { WriteMatchResult(request.Id,"failed",MapPolicy.WorkshopBlocker); return; }
            catch (ArgumentException) { WriteMatchResult(request.Id,"failed","Invalid match maps, installed files or roster"); return; }
            bool alreadyLoaded=plugin?.GetType().GetField("isMatchSetup")?.GetValue(plugin) is true && (plugin.GetType().GetField("loadedConfigFile")?.GetValue(plugin) as string)==filename;
            if(accessVerified&&alreadyLoaded){WriteMatchResult(request.Id,"complete","");return;}
            if(!accessVerified||!RuntimeEvidence.CanLoadMatch(plugin,state,version)){WriteMatchResult(request.Id,"failed","MatchZy already has a match loaded or is outside Match mode");return;}
            try{Directory.CreateDirectory(Path.GetDirectoryName(Path.Join(Server.GameDirectory,"csgo",filename))!);File.WriteAllText(Path.Join(Server.GameDirectory,"csgo",filename),JsonSerializer.Serialize(config));}
            catch(Exception){WriteMatchResult(request.Id,"failed","Could not write the local MatchZy setup file");return;}
            try {
                var claim = MatchFence.Claim(directory, request.Id, launchNonce);
                if (claim == MatchFence.ClaimResult.Cancelled) { WriteMatchResult(request.Id,"failed","Match setup request was cancelled"); return; }
                if (claim == MatchFence.ClaimResult.AlreadyIssued) { WriteMatchResult(request.Id,"failed","The previous match load may already have been issued"); return; }
            } catch (Exception) { WriteMatchResult(request.Id,"failed","Could not persist the match setup fence"); return; }
            Server.ExecuteCommand("matchzy_loadmatch "+filename);
            pendingMatchID=request.Id;pendingMatchFile=filename;pendingMatchAt=DateTimeOffset.UtcNow;return;
        }
        bool setup=plugin?.GetType().GetField("isMatchSetup")?.GetValue(plugin) is true;
        string loaded=plugin?.GetType().GetField("loadedConfigFile")?.GetValue(plugin) as string??"";
        if(accessVerified&&setup&&loaded==pendingMatchFile){WriteMatchResult(request.Id,"complete","");pendingMatchID="";pendingMatchFile="";return;}
        if(DateTimeOffset.UtcNow-pendingMatchAt>TimeSpan.FromSeconds(12)){WriteMatchResult(request.Id,"failed","MatchZy did not confirm the loaded match setup");pendingMatchID="";pendingMatchFile="";}
    }

    private void ProcessMatchCancellation(object? plugin, string state, string version, bool accessVerified, string launchNonce)
    {
        string path = Path.Join(directory,"match-cancel-request.json");
        if (!accessVerified || !RuntimeEvidence.MatchUnloaded(plugin,state,version) || !File.Exists(path) || new FileInfo(path).Length > 2048) return;
        MatchCancelRequest? request;
        try { request = JsonSerializer.Deserialize<MatchCancelRequest>(File.ReadAllText(path),Json); } catch (Exception) { return; }
        if (request?.Id is not string id || request.SetupId is not string setupId || !RuntimeEvidence.ValidId(id) || !RuntimeEvidence.ValidId(setupId)) return;
        try {
            string resultPath = Path.Join(directory,"match-cancel-result.json");
            if (File.Exists(resultPath) && new FileInfo(resultPath).Length <= 2048) {
                var previous = JsonSerializer.Deserialize<MatchCancelResult>(File.ReadAllText(resultPath),Json);
                if (previous != null && previous.Id == id) return;
            }
            bool cancelled = MatchFence.Cancel(directory, setupId, launchNonce);
            string temp = Path.Join(directory,".match-cancel-result-" + Guid.NewGuid().ToString("N"));
            try {
                using (var stream = new FileStream(temp, new FileStreamOptions { Mode=FileMode.CreateNew, Access=FileAccess.Write, UnixCreateMode=UnixFileMode.UserRead|UnixFileMode.UserWrite }))
                    JsonSerializer.Serialize(stream,new MatchCancelResult(id,setupId,cancelled ? "complete" : "failed",cancelled ? "" : "MatchZy may have received this setup. A later server restart may be needed; older requests still need manual intervention."),Json);
                File.Move(temp,resultPath,true);
            } finally { try { File.Delete(temp); } catch (IOException) {} }
        } catch (Exception) { /* Do not acknowledge a fence that could not be persisted. */ }
    }

    private void WriteMatchResult(string id,string status,string error){try{var path=Path.Join(directory,"match-result.json");var temp=Path.Join(directory,".match-result-"+Guid.NewGuid().ToString("N"));using(var stream=new FileStream(temp,new FileStreamOptions{Mode=FileMode.CreateNew,Access=FileAccess.Write,UnixCreateMode=UnixFileMode.UserRead|UnixFileMode.UserWrite}))JsonSerializer.Serialize(stream,new MatchSetupResult(id,status,error),Json);File.Move(temp,path,true);}catch(Exception){}}

    private void ProcessModeCommand(object? plugin,string state,string version,bool accessVerified)
    {
        string path=Path.Join(directory,"mode-request.json");
        if(!File.Exists(path)||new FileInfo(path).Length>2048)return;
        ModeRequest? request;
        try{request=JsonSerializer.Deserialize<ModeRequest>(File.ReadAllText(path),Json);}catch(Exception){return;}
        if(request==null||request.Id.Length!=26||(request.Mode!="match"&&request.Mode!="practice")||
           Math.Abs(DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()-request.RequestedAt)>20000)return;
        string resultPath=Path.Join(directory,"mode-result.json");
        try{
            if(File.Exists(resultPath)&&new FileInfo(resultPath).Length<=2048){var old=JsonSerializer.Deserialize<ModeResult>(File.ReadAllText(resultPath),Json);if(old?.Id==request.Id)return;}
        }catch(Exception){}
        string actual=accessVerified?RuntimeEvidence.Mode(plugin,state,version):"";
        if(actual==request.Mode){WriteModeResult(request.Id,"complete","");handledModeID=request.Id;pendingModeID="";return;}
        if(pendingModeID!=request.Id){
            if(handledModeID==request.Id)return;
            if(!accessVerified||!RuntimeEvidence.CanSwitchMode(plugin,state,version)){
                WriteModeResult(request.Id,"failed","MatchZy is in a phase that cannot switch modes");handledModeID=request.Id;return;
            }
            Server.ExecuteCommand(request.Mode=="practice"?"css_prac":"css_exitprac");
            pendingModeID=request.Id;pendingMode=request.Mode;pendingModeAt=DateTimeOffset.UtcNow;handledModeID=request.Id;
            return;
        }
        if(DateTimeOffset.UtcNow-pendingModeAt>TimeSpan.FromSeconds(10)){
            WriteModeResult(request.Id,"failed","MatchZy did not report the requested mode");pendingModeID="";return;
        }
        if(accessVerified&&RuntimeEvidence.Mode(plugin,state,version)==pendingMode){WriteModeResult(request.Id,"complete","");pendingModeID="";}
    }

    private void WriteModeResult(string id,string status,string error)
    {
        string? temporary=null;
        try{temporary=Path.Join(directory,".mode-result-"+Guid.NewGuid().ToString("N"));
            using(var stream=new FileStream(temporary,new FileStreamOptions{Mode=FileMode.CreateNew,Access=FileAccess.Write,UnixCreateMode=UnixFileMode.UserRead|UnixFileMode.UserWrite}))
                JsonSerializer.Serialize(stream,new ModeResult(id,status,error),Json);
            File.Move(temporary,Path.Join(directory,"mode-result.json"),true);
        }catch(Exception){if(temporary!=null){try{File.Delete(temporary);}catch(IOException){}}}
    }

    private sealed record AccessRequest(string Nonce, string RosterHash, int ProcessGroup);
    private sealed record AccessEvidence(string Nonce, string RosterHash, int ProcessGroup, int ProcessId, long ReportedAt, string Status, bool CanSwitchMode, string Mode, string ModeCommandId, string ModeCommandStatus, string ModeCommandError, bool CanLoadMatch, string MatchSetupId, string MatchSetupStatus, string MatchSetupError, string MatchPhase, string ActiveMatchSetupId, bool CanPauseMatch, bool CanResumeMatch, string MatchControlId, string MatchControlAction, string MatchControlStatus, string MatchControlError, bool SupportsMatchFence, string MatchCancelId, string MatchCancelSetupId, string MatchCancelStatus, string MatchCancelError, bool SupportsMatchSeries, bool SupportsFencedMatchControl, LiveMapScore? LiveScore, bool SupportsManualReady = false);
    private sealed record ModeRequest(string Id,string Mode,long RequestedAt);
    private sealed record ModeResult(string Id,string Status,string Error);
    private sealed record MatchSetupRequest(string Id,string Map,bool Overtime,int PlayersPerSide,MatchSetupPlayer[] CT,MatchSetupPlayer[] T,long RequestedAt,string[]? Maps=null,string[]? MapSides=null,bool ManualReady=false);
    private sealed record MatchSetupPlayer(string SteamId,string GameName);
    private sealed record MatchSetupResult(string Id,string Status,string Error);
    private sealed record MatchCancelRequest(string Id,string SetupId);
    private sealed record MatchCancelResult(string Id,string SetupId,string Status,string Error);
    private sealed record MatchControlRequest(string Id,string Action,string? SetupId,long RequestedAt);
    private sealed record MatchControlResult(string Id,string Action,string Status,string Error);
}
