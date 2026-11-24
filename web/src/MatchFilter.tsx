export type MatchFilterValue = "open" | "closed" | "all";

export function MatchFilter({
  value,
  onChange,
  openCount,
  closedCount,
  label,
}: {
  value: MatchFilterValue;
  onChange: (value: MatchFilterValue) => void;
  openCount: number;
  closedCount: number;
  label: string;
}) {
  const options: { value: MatchFilterValue; label: string; count: number }[] = [
    { value: "open", label: "Open", count: openCount },
    { value: "closed", label: "Closed", count: closedCount },
    { value: "all", label: "All", count: openCount + closedCount },
  ];
  return <div className="match-filter" role="group" aria-label={label}>
    {options.map((option) => <button key={option.value} type="button" aria-pressed={value === option.value} onClick={() => onChange(option.value)}>
      {option.label}<span>{option.count}</span>
    </button>)}
  </div>;
}
