import { Badge } from "@/components/ui/badge";

export type StatusTone = "success" | "warning" | "danger" | "neutral";

const TONE_BY_STATUS: Record<string, StatusTone> = {
  approved: "success",
  executed: "success",
  resolved: "success",
  active: "success",
  passed: "success",
  ready: "success",
  ok: "success",
  valid: "success",
  enabled: "success",
  completed: "success",
  pending: "warning",
  executing: "warning",
  in_progress: "warning",
  awaiting_approval: "warning",
  failed: "danger",
  denied: "danger",
  expired: "danger",
  revoked: "danger",
  rejected: "danger",
  error: "danger",
  not_ready: "danger",
};

const VARIANT_BY_TONE = {
  success: "success",
  warning: "warning",
  danger: "danger",
  neutral: "secondary",
} as const;

export function formatStatusLabel(status: string): string {
  if (!status) return "Unknown";
  const words = status.toLowerCase().replaceAll("_", " ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}

interface StatusBadgeProps {
  status: string;
  /** Overrides the tone inferred from the status string. */
  tone?: StatusTone;
  className?: string;
}

export function StatusBadge({ status, tone, className }: StatusBadgeProps) {
  const resolvedTone = tone ?? TONE_BY_STATUS[status.toLowerCase()] ?? "neutral";

  return (
    <Badge variant={VARIANT_BY_TONE[resolvedTone]} className={className}>
      {formatStatusLabel(status)}
    </Badge>
  );
}
