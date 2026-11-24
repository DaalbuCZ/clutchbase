using System.Runtime.InteropServices;
using System.Text;

namespace ClutchbaseAccess;

// These methods run on the plugin's game thread, which serializes claims and cancellations.
public static class MatchFence
{
    public enum ClaimResult { Claimed, Cancelled, AlreadyIssued }

    public static bool IsInProcessGroup(int expected) =>
        OperatingSystem.IsLinux() && expected > 0 && GetProcessGroup() == expected;

    public static ClaimResult Claim(string directory, string id, string launchNonce)
    {
        Validate(id);
        Validate(launchNonce);
        if (File.Exists(Path.Join(directory, "match-cancelled-" + id))) return ClaimResult.Cancelled;
        string path = Path.Join(directory, "match-issued-" + id);
        if (File.Exists(path)) return ClaimResult.AlreadyIssued;
        CreateMarker(path, launchNonce);
        SyncDirectory(directory);
        return ClaimResult.Claimed;
    }

    public static bool Cancel(string directory, string id, string launchNonce)
    {
        Validate(id);
        Validate(launchNonce);
        string path = Path.Join(directory, "match-cancelled-" + id);
        if (!File.Exists(path)) {
            string issuedPath = Path.Join(directory, "match-issued-" + id);
            if (File.Exists(issuedPath)) {
                // Empty 0.3.4 markers have no launch identity and stay blocked.
                if (new FileInfo(issuedPath).Length != 26) return false;
                string issuedNonce = File.ReadAllText(issuedPath);
                if (!RuntimeEvidence.ValidId(issuedNonce) || issuedNonce == launchNonce) return false;
            }
            CreateMarker(path, "");
        }
        // An existing marker may have survived an earlier failed directory sync.
        SyncDirectory(directory);
        return true;
    }

    private static void Validate(string id)
    {
        if (!RuntimeEvidence.ValidId(id)) throw new ArgumentException("Invalid match setup ID", nameof(id));
    }

    private static void CreateMarker(string path, string value)
    {
        if (!OperatingSystem.IsLinux()) throw new PlatformNotSupportedException("Match fences require Linux");
        using var file = new FileStream(path, new FileStreamOptions { Mode = FileMode.CreateNew, Access = FileAccess.Write, UnixCreateMode = UnixFileMode.UserRead | UnixFileMode.UserWrite });
        file.Write(Encoding.ASCII.GetBytes(value));
        file.Flush(true);
    }

    public static void SyncDirectory(string directory)
    {
        int fd = Open(directory, 0);
        if (fd < 0) throw new IOException("Could not open the match fence directory");
        try { if (Fsync(fd) != 0) throw new IOException("Could not persist the match fence"); }
        finally { Close(fd); }
    }

    [DllImport("libc", EntryPoint = "open", SetLastError = true)] private static extern int Open(string path, int flags);
    [DllImport("libc", EntryPoint = "getpgrp")] private static extern int GetProcessGroup();
    [DllImport("libc", EntryPoint = "fsync", SetLastError = true)] private static extern int Fsync(int fd);
    [DllImport("libc", EntryPoint = "close", SetLastError = true)] private static extern int Close(int fd);
}
