"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

function Progress({
  value = 0,
  className,
  indicatorClassName,
  ...props
}: React.ComponentProps<"div"> & { value?: number; indicatorClassName?: string }) {
  const clamped = Math.min(Math.max(value, 0), 100);

  return (
    <div
      data-slot="progress"
      role="progressbar"
      aria-valuenow={clamped}
      aria-valuemin={0}
      aria-valuemax={100}
      className={cn("bg-primary/20 relative h-2 w-full overflow-hidden rounded-full", className)}
      {...props}
    >
      <div
        data-slot="progress-indicator"
        className={cn("bg-primary h-full transition-all duration-300", indicatorClassName)}
        style={{ width: `${clamped}%` }}
      />
    </div>
  );
}

export { Progress };
