import { useEffect, useState } from "react";

function getRemaining(endsAt: string, now: number) {
  const distance = Math.max(0, new Date(endsAt).getTime() - now);
  return {
    days: Math.floor(distance / 86_400_000),
    hours: Math.floor((distance / 3_600_000) % 24),
    minutes: Math.floor((distance / 60_000) % 60),
    seconds: Math.floor((distance / 1_000) % 60),
  };
}

export function CountdownTimer({ endsAt }: { endsAt: string }) {
  const [now, setNow] = useState(Date.now);
  const remaining = getRemaining(endsAt, now);

  useEffect(() => {
    const interval = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(interval);
  }, []);

  const segments = [
    { value: remaining.days, label: "Days" },
    { value: remaining.hours, label: "Hours" },
    { value: remaining.minutes, label: "Minutes" },
    { value: remaining.seconds, label: "Seconds" },
  ];

  return (
    <div
      aria-label="Time remaining in sprint"
      className="flex flex-wrap gap-2"
    >
      {segments.map((segment) => (
        <div
          key={segment.label}
          className="min-w-16 rounded-lg border border-zinc-800 bg-zinc-950/50 px-3 py-2 text-center"
        >
          <div className="font-mono text-lg tabular-nums text-zinc-50">
            {String(segment.value).padStart(2, "0")}
          </div>
          <div className="text-[10px] text-zinc-500">
            {segment.label}
          </div>
        </div>
      ))}
    </div>
  );
}
