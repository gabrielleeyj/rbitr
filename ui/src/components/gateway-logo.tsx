export function GatewayLogo({ className }: { className?: string }) {
  return (
    <svg
      width="220"
      height="220"
      viewBox="0 0 220 220"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
    >
      {/* <!-- rbitr mark (Option A shape) — flat colors, angled routing lines, no text --> */}
      <g transform="translate(20,20)">
        {/* <!-- Outer ring --> */}
        <circle
          cx="90"
          cy="90"
          r="56"
          stroke="#2563EB"
          stroke-width="14"
          stroke-linecap="round"
        />

        {/* <!-- Gate slit (control aperture) --> */}
        <path
          d="M90 34
             C118 34 140 56 140 84
             C140 112 118 134 90 134"
          stroke="#0B1220"
          stroke-width="18"
          stroke-linecap="round"
        />

        {/* <!-- Inner core --> */}
        <circle cx="90" cy="90" r="20" fill="#0B1220" />
        <circle
          cx="90"
          cy="90"
          r="20"
          stroke="#60A5FA"
          stroke-width="2"
          opacity="0.95"
        />

        {/* <!-- Agent nodes --> */}
        <circle cx="40" cy="62" r="6" fill="#06B6D4" />
        <circle cx="152" cy="120" r="6" fill="#7C3AED" />
        <circle cx="122" cy="40" r="4.5" fill="#60A5FA" opacity="0.95" />

        {/* <!-- Routing lines (straight, angled) --> */}
        <polyline
          points="40,62 66,62 90,90"
          fill="none"
          stroke="#06B6D4"
          stroke-width="2.8"
          stroke-linecap="square"
          stroke-linejoin="miter"
          opacity="0.95"
        />
        <polyline
          points="152,120 124,120 90,90"
          fill="none"
          stroke="#7C3AED"
          stroke-width="2.8"
          stroke-linecap="square"
          stroke-linejoin="miter"
          opacity="0.95"
        />
      </g>
    </svg>
  );
}
