/* Token-driven so the mark always harmonizes with the sidebar surface. */
export function GatewayLogo({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 220 220"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      aria-hidden="true"
    >
      <g transform="translate(20,20)">
        {/* Outer governance ring */}
        <circle
          cx="90"
          cy="90"
          r="56"
          stroke="var(--sidebar-primary)"
          strokeWidth="14"
          strokeLinecap="round"
        />

        {/* Gate slit (control aperture) — cut by matching the surface */}
        <path
          d="M90 34
             C118 34 140 56 140 84
             C140 112 118 134 90 134"
          stroke="var(--sidebar)"
          strokeWidth="18"
          strokeLinecap="round"
        />

        {/* Inner core */}
        <circle cx="90" cy="90" r="20" fill="var(--sidebar)" />
        <circle
          cx="90"
          cy="90"
          r="20"
          stroke="var(--sidebar-foreground)"
          strokeWidth="2"
          opacity="0.9"
        />

        {/* Agent nodes */}
        <circle cx="40" cy="62" r="6" fill="var(--sidebar-foreground)" />
        <circle cx="152" cy="120" r="6" fill="var(--sidebar-primary)" />
        <circle
          cx="122"
          cy="40"
          r="4.5"
          fill="var(--sidebar-foreground)"
          opacity="0.7"
        />

        {/* Routing lines (straight, angled) */}
        <polyline
          points="40,62 66,62 90,90"
          fill="none"
          stroke="var(--sidebar-foreground)"
          strokeWidth="2.8"
          strokeLinecap="square"
          strokeLinejoin="miter"
          opacity="0.7"
        />
        <polyline
          points="152,120 124,120 90,90"
          fill="none"
          stroke="var(--sidebar-primary)"
          strokeWidth="2.8"
          strokeLinecap="square"
          strokeLinejoin="miter"
          opacity="0.9"
        />
      </g>
    </svg>
  );
}
