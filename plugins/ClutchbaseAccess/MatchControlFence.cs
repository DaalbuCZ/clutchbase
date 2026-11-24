namespace ClutchbaseAccess;

internal static class MatchControlFence
{
    public static bool Matches(bool verified, string? requested, string active) =>
        verified && RuntimeEvidence.ValidId(requested ?? "") && requested == active;
}
