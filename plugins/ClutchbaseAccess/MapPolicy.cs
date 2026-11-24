using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;

namespace ClutchbaseAccess;

public static class MapPolicy
{
    public const string WorkshopBlocker = "Workshop match loading is unavailable: installed-file discovery does not prove CS2 loading or canonical runtime map identity";
    private static readonly HashSet<string> OfficialMaps = new(StringComparer.Ordinal)
        { "de_mirage", "de_inferno", "de_dust2", "de_nuke", "de_ancient", "de_anubis", "de_train", "de_vertigo" };

    public static bool IsOfficial(string? map) => map != null && OfficialMaps.Contains(map);
    public static bool IsWorkshop(string? map)
    {
        var parts = map?.Split('/');
        return parts is { Length: 3 } && parts[0] == "workshop" && parts[1].Length is >= 1 and <= 20 && parts[1][0] != '0' &&
            parts[1].All(char.IsAsciiDigit) && parts[2].Length is >= 1 and <= 128 &&
            parts[2].All(c => c is >= 'a' and <= 'z' or >= '0' and <= '9' or '_' or '-');
    }
    public static bool IsIdentifier(string? map) => IsOfficial(map) || IsWorkshop(map);

    // A discovery file is not proof of an engine-loadable map. Keep this fence
    // until loading and runtime identity are established for the pinned stack.
    public static void RequireLoadable(string[] maps, string? gameDirectory)
    {
        foreach (string map in maps) {
            if (!IsIdentifier(map)) throw new ArgumentException("Invalid map identifier");
            if (IsWorkshop(map) && !HasInstalledWorkshopMap(gameDirectory, map))
                throw new ArgumentException("Workshop map is not a regular installed file in this instance");
        }
        if (maps.Any(IsWorkshop)) throw new NotSupportedException(WorkshopBlocker);
    }

    public static bool HasInstalledWorkshopMap(string? gameDirectory, string? map)
    {
        if (!OperatingSystem.IsLinux() || !IsWorkshop(map) || string.IsNullOrEmpty(gameDirectory) || !Path.IsPathFullyQualified(gameDirectory)) return false;
        // Walk open directory descriptors so renames cannot redirect a later
        // component through a symlink. O_PATH avoids opening devices or FIFOs.
        SafeFileHandle? current = null;
        try {
            string path = Path.Join(gameDirectory, "csgo", "maps", map! + ".bsp");
            var parts = path.Split('/', StringSplitOptions.RemoveEmptyEntries);
            if (parts.Any(p => p is "." or "..")) return false;
            current = OpenAt(-100, "/", PathOnly | DirectoryOnly | NoFollow | CloseOnExec);
            if (current.IsInvalid) return false;
            for (int i = 0; i < parts.Length; i++) {
                var next = OpenAt(current.DangerousGetHandle().ToInt32(), parts[i], PathOnly | NoFollow | CloseOnExec | (i == parts.Length - 1 ? 0 : DirectoryOnly));
                current.Dispose(); current = next;
                if (current.IsInvalid) return false;
            }
            return Statx(current.DangerousGetHandle().ToInt32(), "", 0x1000, 0x1, out var stat) == 0 && (stat.Mode & 0xf000) == 0x8000;
        } catch (Exception e) when (e is IOException or UnauthorizedAccessException or ArgumentException or EntryPointNotFoundException) {
            return false;
        } finally { current?.Dispose(); }
    }

    private const int PathOnly = 0x200000, DirectoryOnly = 0x10000, NoFollow = 0x20000, CloseOnExec = 0x80000;
    private static SafeFileHandle OpenAt(int directory, string path, int flags) => new((IntPtr)OpenAtNative(directory, path, flags), ownsHandle: true);
    [DllImport("libc", EntryPoint = "openat", SetLastError = true)]
    private static extern int OpenAtNative(int directory, string path, int flags);
    [StructLayout(LayoutKind.Explicit, Size = 256)]
    private struct StatxInfo { [FieldOffset(28)] public ushort Mode; }
    [DllImport("libc", EntryPoint = "statx", SetLastError = true)]
    private static extern int Statx(int directory, string path, int flags, uint mask, out StatxInfo info);
}
